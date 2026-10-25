package federation

import (
	"bytes"
	"context"
	"crypto"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"syscall"
	"time"

	"code.superseriousbusiness.org/httpsig"

	ap "github.com/duv0-x/gremio/internal/goap/activitypub"
	"github.com/duv0-x/gremio/internal/goap/jsonld"
)

const maxBody = 1 << 20

// errGone indica que el Actor remoto ya no existe (404/410).
var errGone = errors.New("el actor remoto ya no existe")

type remoteKey struct {
	pub   crypto.PublicKey
	owner string
}

// cgnat es 100.64.0.0/10, que net.IP.IsPrivate no cubre.
var cgnat = netip.MustParsePrefix("100.64.0.0/10")

// newClient arma un cliente que, en producción, no se conecta a direcciones privadas, de loopback
// ni link-local: el inbox hace GETs a URLs que decide quien envía la actividad (SSRF).
func newClient(allowPrivate bool) *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	if !allowPrivate {
		dialer.Control = func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			addr, err := netip.ParseAddr(host)
			if err != nil {
				return fmt.Errorf("destino no permitido: %s", address)
			}
			addr = addr.Unmap()
			if addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() ||
				addr.IsMulticast() || addr.IsUnspecified() || cgnat.Contains(addr) {
				return fmt.Errorf("destino no permitido: %s", address)
			}
			return nil
		}
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.DialContext = dialer.DialContext
	tr.Proxy = nil
	return &http.Client{Timeout: 15 * time.Second, Transport: tr}
}

func (s *Server) sign(req *http.Request, body []byte, headers []string) error {
	req.Header.Set("Date", now().Format(http.TimeFormat))
	req.Header.Set("User-Agent", s.userAgent())
	req.Header.Set("Host", req.URL.Host) // httpsig firma el header Host, no req.Host; net/http lo ignora al enviar
	signer, _, err := httpsig.NewSigner([]httpsig.Algorithm{httpsig.RSA_SHA256}, httpsig.DigestSha256, headers, httpsig.Signature, 120)
	if err != nil {
		return err
	}
	return signer.SignRequest(s.key, s.keyID(), req, body)
}

// deliver hace un POST firmado (request-target, host, date, digest), como esperan Mastodon y GoToSocial.
func (s *Server) deliver(ctx context.Context, inbox string, act *ap.Activity) error {
	if _, err := s.checkURL(inbox); err != nil {
		return err
	}
	body, err := jsonld.WithContext(jsonld.IRI(ap.ActivityBaseURI)).Marshal(act)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, inbox, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", apContentType)
	if err := s.sign(req, body, []string{httpsig.RequestTarget, "host", "date", "digest"}); err != nil {
		return err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, msg)
	}
	return nil
}

// fetchActor hace un GET firmado: GoToSocial y Mastodon firman sus GETs y pueden exigirlo.
func (s *Server) fetchActor(ctx context.Context, iri string) (*ap.Actor, error) {
	if _, err := s.checkURL(iri); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, iri, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", apContentType+`, application/ld+json; profile="https://www.w3.org/ns/activitystreams"`)
	if err := s.sign(req, nil, []string{httpsig.RequestTarget, "host", "date"}); err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		return nil, fmt.Errorf("GET %s: HTTP %d: %w", iri, resp.StatusCode, errGone)
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("GET %s: HTTP %d", iri, resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, err
	}
	it, err := ap.UnmarshalJSON(b)
	if err != nil {
		return nil, err
	}
	return ap.ToActor(it)
}

func (s *Server) fetchKey(ctx context.Context, keyID string) (remoteKey, error) {
	// GoToSocial usa keyId con ruta (/users/x/main-key) y Mastodon con fragmento (#main-key): ambos responden al GET.
	remote, err := s.fetchActor(ctx, trimFragment(keyID))
	if err != nil {
		return remoteKey{}, fmt.Errorf("llave %s: %w", keyID, err)
	}
	block, _ := pem.Decode([]byte(remote.PublicKey.PublicKeyPem))
	if block == nil {
		return remoteKey{}, fmt.Errorf("llave %s: PEM inválido", keyID)
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return remoteKey{}, err
	}
	k := remoteKey{pub: pub, owner: remote.PublicKey.Owner.String()}
	if k.owner == "" {
		k.owner = remote.ID.String()
	}
	s.mu.Lock()
	s.keys[keyID] = k
	s.mu.Unlock()
	return k, nil
}

// verify comprueba Date, Digest y la HTTP Signature, y devuelve el Actor dueño de la llave.
// Si la firma no valida con la llave en caché, la vuelve a pedir una vez (rotación de llaves).
func (s *Server) verify(ctx context.Context, r *http.Request, body []byte) (string, error) {
	date, err := http.ParseTime(r.Header.Get("Date"))
	if err != nil {
		return "", errors.New("Date ausente o inválido")
	}
	if d := now().Sub(date); d > 12*time.Hour || d < -12*time.Hour { // la misma ventana que Mastodon
		return "", fmt.Errorf("Date fuera de la ventana permitida: %s", date)
	}
	sum := sha256.Sum256(body)
	if want := "SHA-256=" + base64.StdEncoding.EncodeToString(sum[:]); r.Header.Get("Digest") != want {
		return "", errors.New("Digest ausente o distinto")
	}
	v, err := httpsig.NewVerifier(r)
	if err != nil {
		return "", err
	}
	keyID := v.KeyId()
	s.mu.Lock()
	k, cached := s.keys[keyID]
	s.mu.Unlock()
	if !cached {
		if k, err = s.fetchKey(ctx, keyID); err != nil {
			return "", err
		}
	}
	if err := v.Verify(k.pub, httpsig.RSA_SHA256); err != nil {
		if !cached {
			return "", err
		}
		if k, err = s.fetchKey(ctx, keyID); err != nil {
			return "", err
		}
		if err := v.Verify(k.pub, httpsig.RSA_SHA256); err != nil {
			return "", err
		}
	}
	return k.owner, nil
}
