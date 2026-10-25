package web

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/duv0-x/gremio/internal/instance"
	"github.com/duv0-x/gremio/internal/store"
)

const doc = `{
  "basics": {"name": "Alice Ejemplo", "label": "SRE", "email": "alice@ejemplo.org", "phone": "+57 300 000 0000",
    "summary": "Bio corta", "location": {"city": "Medellín", "countryCode": "CO"},
    "profiles": [{"network": "GitHub", "username": "alice", "url": "https://github.com/alice"}]},
  "work": [{"id": "acme", "name": "Acme", "position": "SRE", "startDate": "2021-05",
    "highlights": [{"id": "h1", "text": "Bajó el MTTR 23 %", "evidence": ["https://ejemplo.org/charla"]}]}],
  "skills": [{"id": "k8s", "name": "Kubernetes", "level": "expert", "keywords": ["EKS"]}],
  "gremio": {"about": "Presentación larga <script>alert(1)</script>", "contact": {"email": "private", "phone": "private"}}
}`

func newTestServer(t *testing.T, document string) *httptest.Server {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := instance.Init(ctx, st, "alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := instance.BindHost(ctx, st, "https://gremio.ejemplo.org", "ejemplo.org"); err != nil {
		t.Fatal(err)
	}
	if err := st.Write.UpdateProfile(ctx, document); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	New(st, "1.2.3").Register(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

func get(t *testing.T, url string) (*http.Response, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

func TestPerfilPublico(t *testing.T) {
	ts := newTestServer(t, doc)
	resp, body := get(t, ts.URL+"/@alice")
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") {
		t.Fatalf("HTTP %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	for _, want := range []string{
		`<html lang="es">`, "Alice Ejemplo", "Presentación larga", "Medellín, Colombia", "@alice@ejemplo.org",
		"2021-05 → hoy", "Bajó el MTTR 23 %", `href="https://ejemplo.org/charla"`, "Experto",
		`href="https://github.com/duv0-x/gremio/tree/v1.2.3"`, "/static/app.css?v=1.2.3",
		`<meta name="description" content="Bio corta">`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("falta %q", want)
		}
	}
	for _, leak := range []string{"alice@ejemplo.org\"", "mailto:", "+57 300", "<script>"} {
		if strings.Contains(body, leak) {
			t.Errorf("la página publica %q", leak)
		}
	}
	if csp := resp.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'self'") {
		t.Errorf("CSP = %q", csp)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-transform" {
		t.Errorf("Cache-Control = %q", cc)
	}
}

func TestEmailPublicoSeMuestraYTelefonoNunca(t *testing.T) {
	ts := newTestServer(t, strings.Replace(doc, `"email": "private"`, `"email": "public"`, 1))
	_, body := get(t, ts.URL+"/@alice")
	if !strings.Contains(body, `href="mailto:alice@ejemplo.org"`) || strings.Contains(body, "+57 300") {
		t.Error("el email público debe verse y el teléfono no")
	}
}

func TestRaizRedirigeAlPerfil(t *testing.T) {
	ts := newTestServer(t, doc)
	resp, body := get(t, ts.URL+"/")
	if resp.StatusCode != http.StatusOK || resp.Request.URL.Path != "/@alice" || !strings.Contains(body, "Alice Ejemplo") {
		t.Fatalf("HTTP %d en %s", resp.StatusCode, resp.Request.URL.Path)
	}
}

func TestRutasDesconocidasYEstaticos(t *testing.T) {
	ts := newTestServer(t, doc)
	for _, path := range []string{"/@bob", "/alice", "/@alice/../etc"} {
		if resp, _ := get(t, ts.URL+path); resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s: HTTP %d", path, resp.StatusCode)
		}
	}
	resp, css := get(t, ts.URL+"/static/app.css?v=1.2.3")
	if resp.StatusCode != http.StatusOK || !strings.Contains(css, "--accent") || !strings.Contains(resp.Header.Get("Cache-Control"), "immutable") {
		t.Errorf("app.css: HTTP %d, %d bytes, %v", resp.StatusCode, len(css), resp.Header)
	}
	if resp, _ := get(t, ts.URL+"/static/fonts/ibm-plex-sans-latin-400-normal.woff2"); resp.StatusCode != http.StatusOK {
		t.Errorf("fuente: HTTP %d", resp.StatusCode)
	}
}
