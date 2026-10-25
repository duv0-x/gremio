// Spike de gremio#1: federación mínima contra GoToSocial.
// Estado en memoria y código desechable: valida ADR-001, internal/goap y httpsig.
package main

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"code.superseriousbusiness.org/httpsig"

	ap "github.com/duv0-x/gremio/internal/goap/activitypub"
	"github.com/duv0-x/gremio/internal/goap/jsonld"
)

const apContentType = `application/activity+json`

type server struct {
	base     string // https://<host>, sin "/" final
	host     string
	user     string
	token    string
	key      *rsa.PrivateKey
	pubPEM   string
	client   *http.Client
	mu       sync.Mutex
	inboxes  map[ap.IRI]string // seguidor -> inbox (o sharedInbox)
	notes    []*ap.Activity    // Create{Note}, del más viejo al más nuevo
	keyCache map[string]remoteKey
}

type remoteKey struct {
	pub   crypto.PublicKey
	owner string
}

func main() {
	base := flag.String("base", "", "URL pública de la instancia, p. ej. https://x.trycloudflare.com")
	user := flag.String("user", "spike", "usuario del Actor")
	addr := flag.String("addr", "127.0.0.1:8080", "dirección donde escucha")
	flag.Parse()
	token := os.Getenv("SPIKE_TOKEN")
	if *base == "" || token == "" {
		fmt.Fprintln(os.Stderr, "uso: SPIKE_TOKEN=<secreto> spike -base https://<host>")
		os.Exit(2)
	}
	s, err := newServer(strings.TrimSuffix(*base, "/"), *user, token)
	if err != nil {
		slog.Error("init", "err", err)
		os.Exit(1)
	}
	slog.Info("escuchando", "addr", *addr, "actor", s.actorIRI(), "handle", "@"+s.user+"@"+s.host)
	if err := http.ListenAndServe(*addr, s.routes()); err != nil {
		slog.Error("server", "err", err)
		os.Exit(1)
	}
}

func newServer(base, user, token string) (*server, error) {
	u, err := url.Parse(base)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("base inválida %q", base)
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return nil, err
	}
	return &server{
		base:     base,
		host:     u.Host,
		user:     user,
		token:    token,
		key:      key,
		pubPEM:   string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})),
		client:   &http.Client{Timeout: 15 * time.Second},
		inboxes:  map[ap.IRI]string{},
		keyCache: map[string]remoteKey{},
	}, nil
}

func (s *server) actorIRI() ap.IRI { return ap.IRI(s.base + "/users/" + s.user) }
func (s *server) keyID() string    { return string(s.actorIRI()) + "#main-key" }

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	actor := "/users/" + s.user
	mux.HandleFunc("GET /.well-known/webfinger", s.webfinger)
	mux.HandleFunc("GET "+actor, s.getActor)
	mux.HandleFunc("POST "+actor+"/inbox", s.postInbox)
	mux.HandleFunc("POST /inbox", s.postInbox)
	mux.HandleFunc("GET "+actor+"/outbox", s.getOutbox)
	mux.HandleFunc("GET "+actor+"/followers", s.getFollowers)
	mux.HandleFunc("GET "+actor+"/following", s.getFollowing)
	mux.HandleFunc("GET /notes/{id}", s.getNote)
	mux.HandleFunc("POST /admin/note", s.postNote)
	return logRequests(mux)
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slog.Info("req", "method", r.Method, "path", r.URL.RequestURI(), "ua", r.UserAgent(), "signed", r.Header.Get("Signature") != "")
		next.ServeHTTP(w, r)
	})
}

// writeAP serializa con los @context de ActivityStreams y security, que exige la llave pública.
func writeAP(w http.ResponseWriter, status int, it any) {
	b, err := jsonld.WithContext(jsonld.IRI(ap.ActivityBaseURI), jsonld.IRI(ap.SecurityContextURI)).Marshal(it)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", apContentType)
	w.WriteHeader(status)
	_, _ = w.Write(b)
}

func (s *server) webfinger(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("resource") != "acct:"+s.user+"@"+s.host {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/jrd+json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"subject": "acct:" + s.user + "@" + s.host,
		"aliases": []string{string(s.actorIRI())},
		"links": []map[string]string{
			{"rel": "self", "type": apContentType, "href": string(s.actorIRI())},
		},
	})
}

func (s *server) actor() *ap.Actor {
	id := s.actorIRI()
	return &ap.Actor{
		ID:                id,
		Type:              ap.PersonType,
		Name:              ap.DefaultLangValue("Spike de Gremio"),
		PreferredUsername: ap.DefaultLangValue(s.user),
		Summary:           ap.DefaultLangValue("Instancia desechable del spike gremio#1."),
		URL:               id,
		Inbox:             id + "/inbox",
		Outbox:            id + "/outbox",
		Followers:         id + "/followers",
		Following:         id + "/following",
		Endpoints:         &ap.Endpoints{SharedInbox: ap.IRI(s.base + "/inbox")},
		PublicKey:         ap.PublicKey{ID: ap.ID(s.keyID()), Owner: id, PublicKeyPem: s.pubPEM},
	}
}

func (s *server) getActor(w http.ResponseWriter, _ *http.Request) { writeAP(w, 200, s.actor()) }

func (s *server) getOutbox(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	items := make(ap.ItemCollection, 0, len(s.notes))
	for i := len(s.notes) - 1; i >= 0; i-- {
		items = append(items, s.notes[i])
	}
	s.mu.Unlock()
	writeAP(w, 200, &ap.OrderedCollection{
		ID: s.actorIRI() + "/outbox", Type: ap.OrderedCollectionType,
		TotalItems: uint(len(items)), OrderedItems: items,
	})
}

func (s *server) getFollowers(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	n := len(s.inboxes)
	s.mu.Unlock()
	writeAP(w, 200, &ap.OrderedCollection{ID: s.actorIRI() + "/followers", Type: ap.OrderedCollectionType, TotalItems: uint(n)})
}

func (s *server) getFollowing(w http.ResponseWriter, _ *http.Request) {
	writeAP(w, 200, &ap.OrderedCollection{ID: s.actorIRI() + "/following", Type: ap.OrderedCollectionType})
}

func (s *server) getNote(w http.ResponseWriter, r *http.Request) {
	want := ap.IRI(s.base + "/notes/" + r.PathValue("id"))
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.notes {
		if c.Object.GetLink() == want {
			writeAP(w, 200, c.Object)
			return
		}
	}
	http.NotFound(w, r)
}

// postInbox acepta solo Follow y Undo{Follow} (ADR-001); el resto responde 202 y se descarta.
func (s *server) postInbox(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "body", http.StatusBadRequest)
		return
	}
	signer, err := s.verify(r, body)
	if err != nil {
		slog.Warn("firma rechazada", "err", err)
		http.Error(w, "firma inválida", http.StatusUnauthorized)
		return
	}
	it, err := ap.UnmarshalJSON(body)
	if err != nil {
		http.Error(w, "json", http.StatusBadRequest)
		return
	}
	act, err := ap.ToActivity(it)
	if err != nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	if act.Actor.GetLink().String() != signer {
		slog.Warn("actor distinto al firmante", "actor", act.Actor.GetLink(), "firmante", signer)
		http.Error(w, "actor no coincide con la firma", http.StatusUnauthorized)
		return
	}
	slog.Info("actividad", "type", act.Type, "actor", act.Actor.GetLink())
	switch {
	case ap.FollowType.Match(act.Type) && act.Object.GetLink() == s.actorIRI():
		go s.acceptFollow(act)
	case ap.UndoType.Match(act.Type):
		if inner, err := ap.ToActivity(act.Object); err == nil && ap.FollowType.Match(inner.Type) {
			s.mu.Lock()
			delete(s.inboxes, act.Actor.GetLink())
			s.mu.Unlock()
			slog.Info("seguidor eliminado", "actor", act.Actor.GetLink())
		}
	}
	w.WriteHeader(http.StatusAccepted)
}

func (s *server) acceptFollow(follow *ap.Activity) {
	follower := follow.Actor.GetLink()
	remote, err := s.fetchActor(string(follower))
	if err != nil {
		slog.Error("no pude obtener al seguidor", "actor", follower, "err", err)
		return
	}
	inbox := remote.Inbox.GetLink().String()
	if remote.Endpoints != nil && remote.Endpoints.SharedInbox != nil {
		inbox = remote.Endpoints.SharedInbox.GetLink().String()
	}
	accept := &ap.Activity{
		ID:     ap.ID(fmt.Sprintf("%s/accepts/%d", s.actorIRI(), time.Now().UnixNano())),
		Type:   ap.AcceptType,
		Actor:  s.actorIRI(),
		Object: follow,
		To:     ap.ItemCollection{follower},
	}
	if err := s.deliver(remote.Inbox.GetLink().String(), accept); err != nil {
		slog.Error("Accept falló", "inbox", remote.Inbox.GetLink(), "err", err)
		return
	}
	s.mu.Lock()
	s.inboxes[follower] = inbox
	s.mu.Unlock()
	slog.Info("seguidor aceptado", "actor", follower, "inbox", inbox)
}

func (s *server) postNote(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer "+s.token {
		http.Error(w, "no autorizado", http.StatusUnauthorized)
		return
	}
	text, _ := io.ReadAll(io.LimitReader(r.Body, 4096))
	now := time.Now().UTC()
	id := ap.IRI(fmt.Sprintf("%s/notes/%d", s.base, now.UnixNano()))
	to := ap.ItemCollection{ap.PublicNS}
	cc := ap.ItemCollection{s.actorIRI() + "/followers"}
	note := &ap.Object{
		ID: id, Type: ap.NoteType, AttributedTo: s.actorIRI(),
		Content: ap.DefaultLangValue("<p>" + htmlEscape(string(text)) + "</p>"),
		URL:     id, Published: now, To: to, CC: cc,
	}
	create := &ap.Activity{ID: id + "/activity", Type: ap.CreateType, Actor: s.actorIRI(), Object: note, Published: now, To: to, CC: cc}

	s.mu.Lock()
	s.notes = append(s.notes, create)
	targets := map[string]bool{}
	for _, inbox := range s.inboxes {
		targets[inbox] = true
	}
	s.mu.Unlock()

	var failed []string
	for inbox := range targets {
		if err := s.deliver(inbox, create); err != nil {
			failed = append(failed, inbox+": "+err.Error())
		}
	}
	slog.Info("note publicada", "id", id, "entregas", len(targets), "fallidas", len(failed))
	_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "delivered": len(targets) - len(failed), "failed": failed})
}

func htmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}

// deliver hace un POST firmado (request-target, host, date, digest), como espera Mastodon/GoToSocial.
func (s *server) deliver(inbox string, act *ap.Activity) error {
	body, err := jsonld.WithContext(jsonld.IRI(ap.ActivityBaseURI)).Marshal(act)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, inbox, bytes.NewReader(body))
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

func (s *server) sign(req *http.Request, body []byte, headers []string) error {
	req.Header.Set("Date", time.Now().UTC().Format(http.TimeFormat))
	req.Header.Set("Host", req.URL.Host)
	signer, _, err := httpsig.NewSigner([]httpsig.Algorithm{httpsig.RSA_SHA256}, httpsig.DigestSha256, headers, httpsig.Signature, 120)
	if err != nil {
		return err
	}
	return signer.SignRequest(s.key, s.keyID(), req, body)
}

// fetchActor hace un GET firmado: GoToSocial exige firma para entregar un Actor.
func (s *server) fetchActor(iri string) (*ap.Actor, error) {
	req, err := http.NewRequest(http.MethodGet, iri, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", apContentType)
	if err := s.sign(req, nil, []string{httpsig.RequestTarget, "host", "date"}); err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: HTTP %d", iri, resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	it, err := ap.UnmarshalJSON(b)
	if err != nil {
		return nil, err
	}
	return ap.ToActor(it)
}

// verify comprueba el Digest del cuerpo y la HTTP Signature, y devuelve el Actor dueño de la llave.
func (s *server) verify(r *http.Request, body []byte) (string, error) {
	sum := sha256.Sum256(body)
	if want := "SHA-256=" + base64.StdEncoding.EncodeToString(sum[:]); r.Header.Get("Digest") != want {
		return "", errors.New("digest ausente o distinto")
	}
	v, err := httpsig.NewVerifier(r)
	if err != nil {
		return "", err
	}
	keyID := v.KeyId()
	s.mu.Lock()
	k, ok := s.keyCache[keyID]
	s.mu.Unlock()
	if !ok {
		// GoToSocial usa keyId con ruta (/users/x/main-key) y Mastodon con fragmento (#main-key): ambos responden al GET.
		url, _, _ := strings.Cut(keyID, "#")
		remote, err := s.fetchActor(url)
		if err != nil {
			return "", fmt.Errorf("llave %s: %w", keyID, err)
		}
		block, _ := pem.Decode([]byte(remote.PublicKey.PublicKeyPem))
		if block == nil {
			return "", fmt.Errorf("llave %s: PEM inválido", keyID)
		}
		if k.pub, err = x509.ParsePKIXPublicKey(block.Bytes); err != nil {
			return "", err
		}
		k.owner = remote.PublicKey.Owner.String()
		if k.owner == "" {
			k.owner = remote.ID.String()
		}
		s.mu.Lock()
		s.keyCache[keyID] = k
		s.mu.Unlock()
	}
	if err := v.Verify(k.pub, httpsig.RSA_SHA256); err != nil {
		return "", err
	}
	return k.owner, nil
}
