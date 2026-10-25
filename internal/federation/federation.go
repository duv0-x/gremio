// Package federation expone la instancia al fediverso por ActivityPub (ADR-001): WebFinger, host-meta,
// NodeInfo, el Actor del dueño, sus colecciones y un inbox que solo procesa follows.
package federation

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	ap "github.com/duv0-x/gremio/internal/goap/activitypub"
	"github.com/duv0-x/gremio/internal/goap/jsonld"
	"github.com/duv0-x/gremio/internal/instance"
	"github.com/duv0-x/gremio/internal/store"
)

const apContentType = `application/activity+json`

// Options ajusta el comportamiento de red; los valores por defecto son los de producción.
type Options struct {
	Version string
	// AllowPrivateNetworks permite http y destinos privados o de loopback. Solo para tests.
	AllowPrivateNetworks bool
	// RetryBase es la espera antes del primer reintento de una entrega (30 s por defecto).
	RetryBase time.Duration
	// PollInterval es cada cuánto el worker revisa la cola (5 s por defecto).
	PollInterval time.Duration
}

// Server sirve la federación de una instancia ya inicializada y con host fijado.
type Server struct {
	st      *store.Store
	inst    store.Instance
	key     *rsa.PrivateKey
	opts    Options
	client  *http.Client
	mu      sync.Mutex
	keys    map[string]remoteKey // keyId -> llave remota verificada
	actorID ap.IRI
	wakeCh  chan struct{}
}

// New carga la instancia y su llave. La instancia debe tener host (ver instance.BindHost).
func New(ctx context.Context, st *store.Store, opts Options) (*Server, error) {
	inst, err := instance.Load(ctx, st.Read)
	if err != nil {
		return nil, err
	}
	if !inst.Host.Valid {
		return nil, errors.New("la instancia no tiene host fijado")
	}
	key, err := instance.PrivateKey(inst)
	if err != nil {
		return nil, err
	}
	if opts.Version == "" {
		opts.Version = "dev"
	}
	if opts.RetryBase == 0 {
		opts.RetryBase = 30 * time.Second
	}
	if opts.PollInterval == 0 {
		opts.PollInterval = 5 * time.Second
	}
	return &Server{
		st:      st,
		inst:    inst,
		key:     key,
		opts:    opts,
		client:  newClient(opts.AllowPrivateNetworks),
		keys:    map[string]remoteKey{},
		actorID: ap.IRI(inst.Host.String + "/users/" + inst.Username),
		wakeCh:  make(chan struct{}, 1),
	}, nil
}

// Register agrega las rutas de federación al mux.
func (s *Server) Register(mux *http.ServeMux) {
	actor := "/users/" + s.inst.Username
	mux.HandleFunc("GET /.well-known/webfinger", s.webfinger)
	mux.HandleFunc("GET /.well-known/host-meta", s.hostMeta)
	mux.HandleFunc("GET /.well-known/nodeinfo", s.nodeInfoIndex)
	mux.HandleFunc("GET /nodeinfo/2.1", s.nodeInfo)
	mux.HandleFunc("GET "+actor, s.getActor)
	mux.HandleFunc("GET "+actor+"/outbox", s.getOutbox)
	mux.HandleFunc("GET "+actor+"/followers", s.getFollowers)
	mux.HandleFunc("GET "+actor+"/following", s.getFollowing)
	mux.HandleFunc("POST "+actor+"/inbox", s.postInbox)
	mux.HandleFunc("POST /inbox", s.postInbox)
	mux.HandleFunc("GET /notes/{id}", s.getNote)
}

func (s *Server) keyID() string  { return string(s.actorID) + "#main-key" }
func (s *Server) handle() string { return s.inst.Username + "@" + s.inst.AccountDomain.String }
func (s *Server) userAgent() string {
	return "gremio/" + s.opts.Version + " (+" + s.inst.Host.String + ")"
}

// writeAP serializa con los @context de ActivityStreams y security, que exige la llave pública.
func writeAP(w http.ResponseWriter, it any) {
	b, err := jsonld.WithContext(jsonld.IRI(ap.ActivityBaseURI), jsonld.IRI(ap.SecurityContextURI)).Marshal(it)
	if err != nil {
		http.Error(w, "serialización", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", apContentType)
	_, _ = w.Write(b)
}

func writeJSON(w http.ResponseWriter, contentType string, v any) {
	w.Header().Set("Content-Type", contentType)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) webfinger(w http.ResponseWriter, r *http.Request) {
	res := r.URL.Query().Get("resource")
	if res != "acct:"+s.handle() && res != string(s.actorID) {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, "application/jrd+json", map[string]any{
		"subject": "acct:" + s.handle(),
		"aliases": []string{string(s.actorID)},
		"links": []map[string]string{
			{"rel": "self", "type": apContentType, "href": string(s.actorID)},
		},
	})
}

func (s *Server) hostMeta(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/xrd+xml")
	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<XRD xmlns="http://docs.oasis-open.org/ns/xri/xrd-1.0">
  <Link rel="lrdd" template="%s/.well-known/webfinger?resource={uri}"/>
</XRD>
`, s.inst.Host.String)
}

func (s *Server) nodeInfoIndex(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, "application/json", map[string]any{
		"links": []map[string]string{{
			"rel":  "http://nodeinfo.diaspora.software/ns/schema/2.1",
			"href": s.inst.Host.String + "/nodeinfo/2.1",
		}},
	})
}

func (s *Server) nodeInfo(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, `application/json; profile="http://nodeinfo.diaspora.software/ns/schema/2.1#"`, map[string]any{
		"version": "2.1",
		"software": map[string]string{
			"name": "gremio", "version": s.opts.Version, "repository": "https://github.com/duv0-x/gremio",
		},
		"protocols":         []string{"activitypub"},
		"services":          map[string][]string{"inbound": {}, "outbound": {}},
		"openRegistrations": false,
		"usage":             map[string]any{"users": map[string]int{"total": 1, "activeMonth": 1, "activeHalfyear": 1}},
		"metadata":          map[string]any{},
	})
}

// profileBasics es lo que el Actor toma del perfil JSON Resume (ADR-003).
type profileBasics struct {
	Basics struct {
		Name    string `json:"name"`
		Summary string `json:"summary"`
	} `json:"basics"`
}

func (s *Server) actor(ctx context.Context) (*ap.Actor, error) {
	doc, err := s.st.Read.GetProfile(ctx)
	if err != nil {
		return nil, err
	}
	var p profileBasics
	if err := json.Unmarshal([]byte(doc), &p); err != nil {
		return nil, err
	}
	id := s.actorID
	a := &ap.Actor{
		ID:                id,
		Type:              ap.PersonType,
		PreferredUsername: ap.DefaultLangValue(s.inst.Username),
		Name:              ap.DefaultLangValue(p.Basics.Name),
		URL:               id, // shortcut: sin página HTML del perfil todavía; cambia con el módulo Perfil (ADR-008)
		Inbox:             id + "/inbox",
		Outbox:            id + "/outbox",
		Followers:         id + "/followers",
		Following:         id + "/following",
		Endpoints:         &ap.Endpoints{SharedInbox: ap.IRI(s.inst.Host.String + "/inbox")},
		PublicKey:         ap.PublicKey{ID: ap.ID(s.keyID()), Owner: id, PublicKeyPem: s.inst.PublicKeyPem},
	}
	if p.Basics.Summary != "" {
		a.Summary = ap.DefaultLangValue(p.Basics.Summary)
	}
	return a, nil
}

func (s *Server) getActor(w http.ResponseWriter, r *http.Request) {
	a, err := s.actor(r.Context())
	if err != nil {
		slog.Error("actor", "err", err)
		http.Error(w, "perfil no disponible", http.StatusInternalServerError)
		return
	}
	writeAP(w, a)
}

// getFollowers publica solo el total: la lista de seguidores no se expone.
func (s *Server) getFollowers(w http.ResponseWriter, r *http.Request) {
	n, err := s.st.Read.CountFollowers(r.Context())
	if err != nil {
		http.Error(w, "seguidores", http.StatusInternalServerError)
		return
	}
	writeAP(w, &ap.OrderedCollection{ID: s.actorID + "/followers", Type: ap.OrderedCollectionType, TotalItems: uint(n)})
}

func (s *Server) getFollowing(w http.ResponseWriter, _ *http.Request) {
	writeAP(w, &ap.OrderedCollection{ID: s.actorID + "/following", Type: ap.OrderedCollectionType})
}

// checkURL exige https a menos que se permitan redes privadas (tests).
func (s *Server) checkURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("URL inválida %q", raw)
	}
	if u.Scheme != "https" && !(s.opts.AllowPrivateNetworks && u.Scheme == "http") {
		return nil, fmt.Errorf("URL no permitida %q: solo https", raw)
	}
	return u, nil
}

func trimFragment(iri string) string {
	base, _, _ := strings.Cut(iri, "#")
	return base
}

func now() time.Time { return time.Now().UTC() }
