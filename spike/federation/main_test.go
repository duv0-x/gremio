package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"code.superseriousbusiness.org/httpsig"

	ap "github.com/duv0-x/gremio/internal/goap/activitypub"
	"github.com/duv0-x/gremio/internal/goap/jsonld"
)

func startServer(t *testing.T, user string) (*server, *httptest.Server) {
	t.Helper()
	var s *server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { s.routes().ServeHTTP(w, r) }))
	t.Cleanup(ts.Close)
	var err error
	if s, err = newServer(ts.URL, user, "secreto"); err != nil {
		t.Fatal(err)
	}
	return s, ts
}

func TestActorJSON(t *testing.T) {
	s, ts := startServer(t, "spike")
	resp, err := http.Get(ts.URL + "/users/spike")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var got map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	ctx, _ := json.Marshal(got["@context"])
	for _, want := range []string{string(ap.ActivityBaseURI), string(ap.SecurityContextURI)} {
		if !strings.Contains(string(ctx), want) {
			t.Errorf("@context = %s, falta %s", ctx, want)
		}
	}
	pk, _ := got["publicKey"].(map[string]any)
	if pk["id"] != s.keyID() || pk["owner"] != string(s.actorIRI()) || !strings.Contains(pk["publicKeyPem"].(string), "BEGIN PUBLIC KEY") {
		t.Errorf("publicKey = %v", pk)
	}
	if got["type"] != "Person" || got["preferredUsername"] != "spike" || got["inbox"] != string(s.actorIRI())+"/inbox" {
		t.Errorf("actor = %v", got)
	}
}

func TestWebfinger(t *testing.T) {
	s, ts := startServer(t, "spike")
	resp, err := http.Get(ts.URL + "/.well-known/webfinger?resource=acct:spike@" + s.host)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var jrd struct {
		Links []struct{ Rel, Href string }
	}
	_ = json.NewDecoder(resp.Body).Decode(&jrd)
	if len(jrd.Links) != 1 || jrd.Links[0].Href != string(s.actorIRI()) {
		t.Errorf("links = %v", jrd.Links)
	}
}

// TestFollowEntreInstancias: A envía un Follow firmado a B; B verifica la firma
// obteniendo el Actor de A, responde con un Accept firmado y registra a A como seguidor.
func TestFollowEntreInstancias(t *testing.T) {
	a, _ := startServer(t, "alice")
	b, _ := startServer(t, "bob")
	follow := &ap.Activity{ID: a.actorIRI() + "/follows/1", Type: ap.FollowType, Actor: a.actorIRI(), Object: b.actorIRI()}
	if err := a.deliver(string(b.actorIRI())+"/inbox", follow); err != nil {
		t.Fatalf("Follow: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		b.mu.Lock()
		inbox, ok := b.inboxes[a.actorIRI()]
		b.mu.Unlock()
		if ok {
			if inbox != a.base+"/inbox" {
				t.Errorf("inbox del seguidor = %s, quería el sharedInbox", inbox)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("B nunca aceptó el Follow")
		}
		time.Sleep(20 * time.Millisecond)
	}

	undo := &ap.Activity{ID: a.actorIRI() + "/undos/1", Type: ap.UndoType, Actor: a.actorIRI(), Object: follow}
	if err := a.deliver(string(b.actorIRI())+"/inbox", undo); err != nil {
		t.Fatalf("Undo: %v", err)
	}
	b.mu.Lock()
	_, still := b.inboxes[a.actorIRI()]
	b.mu.Unlock()
	if still {
		t.Error("el Undo no eliminó al seguidor")
	}
}

func TestInboxRechazaSinFirmaODigestAlterado(t *testing.T) {
	a, _ := startServer(t, "alice")
	b, ts := startServer(t, "bob")
	body, _ := jsonld.WithContext(jsonld.IRI(ap.ActivityBaseURI)).Marshal(&ap.Activity{Type: ap.FollowType, Actor: a.actorIRI(), Object: b.actorIRI()})

	resp, err := http.Post(ts.URL+"/inbox", apContentType, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("sin firma: HTTP %d, quería 401", resp.StatusCode)
	}

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/inbox", bytes.NewReader(append(body, ' ')))
	if err := a.sign(req, body, []string{httpsig.RequestTarget, "host", "date", "digest"}); err != nil {
		t.Fatal(err)
	}
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("cuerpo alterado: HTTP %d, quería 401", resp.StatusCode)
	}
}

func TestPostNoteRequiereToken(t *testing.T) {
	_, ts := startServer(t, "spike")
	resp, err := http.Post(ts.URL+"/admin/note", "text/plain", strings.NewReader("hola"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("HTTP %d, quería 401", resp.StatusCode)
	}
}
