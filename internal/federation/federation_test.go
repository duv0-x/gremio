package federation

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"code.superseriousbusiness.org/httpsig"

	ap "github.com/duv0-x/gremio/internal/goap/activitypub"
	"github.com/duv0-x/gremio/internal/goap/jsonld"
	"github.com/duv0-x/gremio/internal/instance"
	"github.com/duv0-x/gremio/internal/store"
)

type testInstance struct {
	*Server
	st      *store.Store
	dir     string
	url     string
	inboxes atomic.Int32 // POSTs recibidos en el inbox personal
	shared  atomic.Int32 // POSTs recibidos en el sharedInbox
	gone    atomic.Bool  // simula una cuenta borrada: el Actor responde 410
	status  atomic.Int32 // si no es 0, los inboxes responden este código (instancia caída o que rechaza)
	stop    context.CancelFunc
}

var fastOpts = Options{AllowPrivateNetworks: true, RetryBase: 20 * time.Millisecond, PollInterval: 20 * time.Millisecond}

// runWorker arranca el worker de entregas de s y devuelve cómo detenerlo.
func runWorker(t *testing.T, s *Server) context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.RunDeliveries(ctx); close(done) }()
	stop := func() { cancel(); <-done }
	t.Cleanup(stop)
	return stop
}

func newTestInstance(t *testing.T, user string) *testInstance {
	t.Helper()
	ctx := context.Background()
	ti := &testInstance{dir: t.TempDir()}
	var mux http.Handler
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && (r.URL.Path == "/users/"+user+"/inbox" || r.URL.Path == "/inbox") {
			if code := ti.status.Load(); code != 0 {
				http.Error(w, "simulado", int(code))
				return
			}
			if r.URL.Path == "/inbox" {
				ti.shared.Add(1)
			} else {
				ti.inboxes.Add(1)
			}
		}
		if ti.gone.Load() && r.Method == http.MethodGet && r.URL.Path == "/users/"+user {
			http.Error(w, "gone", http.StatusGone)
			return
		}
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(ts.Close)
	ti.url = ts.URL

	st, err := store.Open(ctx, ti.dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := instance.Init(ctx, st, user); err != nil {
		t.Fatal(err)
	}
	if _, err := instance.BindHost(ctx, st, ts.URL, ""); err != nil {
		t.Fatal(err)
	}
	ti.st = st
	if ti.Server, err = New(ctx, st, fastOpts); err != nil {
		t.Fatal(err)
	}
	ti.stop = runWorker(t, ti.Server)
	m := http.NewServeMux()
	ti.Register(m)
	mux = m
	return ti
}

func getJSON(t *testing.T, url string) map[string]any {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: HTTP %d", url, resp.StatusCode)
	}
	var m map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatal(err)
	}
	return m
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); !cond(); {
		if time.Now().After(deadline) {
			t.Fatalf("timeout esperando: %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func (ti *testInstance) follow(t *testing.T, target *testInstance, n int) *ap.Activity {
	t.Helper()
	f := &ap.Activity{ID: ti.actorID + ap.IRI("/follows/"+string(rune('0'+n))), Type: ap.FollowType, Actor: ti.actorID, Object: target.actorID}
	if err := ti.deliver(context.Background(), string(target.actorID)+"/inbox", f); err != nil {
		t.Fatalf("Follow: %v", err)
	}
	return f
}

func (ti *testInstance) followers(t *testing.T) int64 {
	t.Helper()
	n, err := ti.st.Read.CountFollowers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestWebfingerYDescubrimiento(t *testing.T) {
	b := newTestInstance(t, "bob")
	handle := "bob@" + strings.Split(strings.TrimPrefix(b.url, "http://"), ":")[0]
	for _, res := range []string{"acct:" + handle, string(b.actorID)} {
		jrd := getJSON(t, b.url+"/.well-known/webfinger?resource="+res)
		if jrd["subject"] != "acct:"+handle {
			t.Errorf("resource %s: subject = %v", res, jrd["subject"])
		}
	}
	resp, _ := http.Get(b.url + "/.well-known/webfinger?resource=acct:otro@ejemplo.org")
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("recurso ajeno: HTTP %d, quería 404", resp.StatusCode)
	}

	idx := getJSON(t, b.url+"/.well-known/nodeinfo")
	links, _ := idx["links"].([]any)
	if len(links) != 1 || links[0].(map[string]any)["href"] != b.url+"/nodeinfo/2.1" {
		t.Errorf("nodeinfo index = %v", idx)
	}
	ni := getJSON(t, b.url+"/nodeinfo/2.1")
	if ni["software"].(map[string]any)["name"] != "gremio" || ni["openRegistrations"] != false {
		t.Errorf("nodeinfo = %v", ni)
	}
}

func TestActorDesdeLaBase(t *testing.T) {
	b := newTestInstance(t, "bob")
	actor := getJSON(t, string(b.actorID))
	pk, _ := actor["publicKey"].(map[string]any)
	if pk["publicKeyPem"] != b.inst.PublicKeyPem || pk["id"] != b.keyID() {
		t.Errorf("publicKey = %v", pk)
	}
	if actor["type"] != "Person" || actor["preferredUsername"] != "bob" || actor["name"] != "bob" {
		t.Errorf("actor = %v", actor)
	}
	ctx, _ := json.Marshal(actor["@context"])
	if !strings.Contains(string(ctx), "w3id.org/security/v1") {
		t.Errorf("@context sin security: %s", ctx)
	}
}

func TestFollowPersisteYSobreviveAlReinicio(t *testing.T) {
	a, b := newTestInstance(t, "alice"), newTestInstance(t, "bob")
	a.follow(t, b, 1)

	f, err := b.st.Read.GetFollower(context.Background(), string(a.actorID))
	if err != nil {
		t.Fatal(err)
	}
	if f.Inbox != string(a.actorID)+"/inbox" || f.SharedInbox.String != a.url+"/inbox" {
		t.Errorf("seguidor = %+v", f)
	}
	waitFor(t, "Accept en el inbox de alice", func() bool { return a.inboxes.Load() == 1 })
	if got := getJSON(t, string(b.actorID)+"/followers")["totalItems"]; got != 1.0 {
		t.Errorf("followers.totalItems = %v", got)
	}

	// Reinicio: misma base, Server nuevo. El seguidor y la llave siguen ahí.
	pem := b.inst.PublicKeyPem
	b.st.Close()
	st, err := store.Open(context.Background(), b.dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	b2, err := New(context.Background(), st, Options{AllowPrivateNetworks: true})
	if err != nil {
		t.Fatal(err)
	}
	if b2.inst.PublicKeyPem != pem {
		t.Error("la llave cambió al reiniciar")
	}
	if n, _ := st.Read.CountFollowers(context.Background()); n != 1 {
		t.Errorf("seguidores tras reiniciar = %d", n)
	}
}

func TestUndoYDeleteEliminanAlSeguidor(t *testing.T) {
	a, b := newTestInstance(t, "alice"), newTestInstance(t, "bob")
	ctx := context.Background()

	f := a.follow(t, b, 1)
	undo := &ap.Activity{ID: a.actorID + "/undos/1", Type: ap.UndoType, Actor: a.actorID, Object: f}
	if err := a.deliver(ctx, string(b.actorID)+"/inbox", undo); err != nil {
		t.Fatal(err)
	}
	if n := b.followers(t); n != 0 {
		t.Errorf("tras Undo: seguidores = %d", n)
	}

	a.follow(t, b, 2)
	del := &ap.Activity{ID: a.actorID + "/deletes/1", Type: ap.DeleteType, Actor: a.actorID, Object: a.actorID}
	if err := a.deliver(ctx, b.url+"/inbox", del); err != nil {
		t.Fatal(err)
	}
	if n := b.followers(t); n != 0 {
		t.Errorf("tras Delete: seguidores = %d", n)
	}
}

func TestDeleteDeUnaCuentaBorradaSinLlave(t *testing.T) {
	a, b := newTestInstance(t, "alice"), newTestInstance(t, "bob")
	a.follow(t, b, 1)

	// La cuenta de alice se borra: su Actor (y su llave) responde 410, y bob no la tiene en caché.
	a.gone.Store(true)
	b.mu.Lock()
	clear(b.keys)
	b.mu.Unlock()
	del := &ap.Activity{ID: a.actorID + "/deletes/1", Type: ap.DeleteType, Actor: a.actorID, Object: a.actorID}
	if err := a.deliver(context.Background(), b.url+"/inbox", del); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if n := b.followers(t); n != 0 {
		t.Errorf("seguidores = %d, quería 0", n)
	}

	// Pero un Follow de una cuenta borrada no se acepta sin firma verificable.
	f := &ap.Activity{ID: a.actorID + "/follows/9", Type: ap.FollowType, Actor: a.actorID, Object: b.actorID}
	if err := a.deliver(context.Background(), b.url+"/inbox", f); err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("Follow sin llave verificable: err = %v, quería 401", err)
	}
}

func post(t *testing.T, url string, body []byte, sign func(*http.Request)) int {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", apContentType)
	if sign != nil {
		sign(req)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func TestInboxRechaza(t *testing.T) {
	a, b, c := newTestInstance(t, "alice"), newTestInstance(t, "bob"), newTestInstance(t, "carol")
	inbox := b.url + "/inbox"
	follow := func(actor ap.IRI) []byte {
		body, _ := jsonld.WithContext(jsonld.IRI(ap.ActivityBaseURI)).Marshal(&ap.Activity{ID: actor + "/f", Type: ap.FollowType, Actor: actor, Object: b.actorID})
		return body
	}
	headers := []string{httpsig.RequestTarget, "host", "date", "digest"}

	if code := post(t, inbox, follow(a.actorID), nil); code != http.StatusUnauthorized {
		t.Errorf("sin firma: HTTP %d", code)
	}
	body := follow(a.actorID)
	if code := post(t, inbox, append(body, ' '), func(r *http.Request) { _ = a.sign(r, body, headers) }); code != http.StatusUnauthorized {
		t.Errorf("cuerpo alterado: HTTP %d", code)
	}
	// alice firma una actividad que dice venir de carol.
	body = follow(c.actorID)
	if code := post(t, inbox, body, func(r *http.Request) { _ = a.sign(r, body, headers) }); code != http.StatusUnauthorized {
		t.Errorf("actor distinto al firmante: HTTP %d", code)
	}
	if n := b.followers(t); n != 0 {
		t.Errorf("seguidores = %d, quería 0", n)
	}
}

func TestSSRF(t *testing.T) {
	interna := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer interna.Close()
	s := &Server{opts: Options{}, client: newClient(false), keys: map[string]remoteKey{}}
	b := newTestInstance(t, "bob")
	s.key, s.inst, s.actorID = b.key, b.inst, b.actorID

	if _, err := s.fetchActor(context.Background(), "http://ejemplo.org/users/x"); err == nil || !strings.Contains(err.Error(), "solo https") {
		t.Errorf("http: err = %v", err)
	}
	if _, err := s.fetchActor(context.Background(), interna.URL+"/users/x"); err == nil || !strings.Contains(err.Error(), "destino no permitido") {
		t.Errorf("loopback: err = %v", err)
	}
}
