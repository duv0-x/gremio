package web

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/duv0-x/gremio/internal/instance"
	"github.com/duv0-x/gremio/internal/profile"
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

// fichaDoc es doc con atributos, vistas ponderadas y rasgos (ADR-010).
var fichaDoc = strings.Replace(doc, `"contact": {"email": "private", "phone": "private"}}`, `"contact": {"email": "private", "phone": "private"}, "timezone": "America/Bogota",
  "attributes": [
    {"code": "CNT", "name": "Contenedores", "subs": [{"name": "Kubernetes", "score": 92}, {"name": "EKS", "score": 90}], "skills": ["k8s"], "highlights": ["h1"]},
    {"code": "OBS", "name": "Observabilidad", "subs": [{"name": "Grafana", "score": 70}]},
    {"code": "COD", "name": "Código", "subs": [{"name": "Go", "score": 40, "learning": true}]}],
  "views": [
    {"slug": "sre", "code": "SRE", "level": "sr", "name": "SRE", "weights": {"OBS": 2, "CNT": 1}},
    {"slug": "platform-engineer", "code": "PLE", "name": "Platform Engineer", "weights": {"CNT": 2, "COD": 1}}],
  "traits": [{"name": "Automatizadora", "summary": "Todo pasa por un pipeline."}]}`, 1)

func TestFichaDeJugador(t *testing.T) {
	ts := newTestServer(t, fichaDoc)
	resp, body := get(t, ts.URL+"/@alice")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("HTTP %d", resp.StatusCode)
	}
	for _, want := range []string{
		`aria-label="Índice general 77.`, // SRE: (2·70 + 91) / 3
		`id="atributo-cnt"`, `href="#atributo-cnt"`, ">91<", "Kubernetes", "aprendiendo",
		"Bajó el MTTR 23 %", "respaldado por 1 logro", "SRE<span class=\"sr-only\">", "PLE<span class=\"sr-only\">",
		`href="/@alice/platform-engineer"`, `aria-current="page"`, "Automatizadora",
		"<polygon", `data-tip="Zona horaria: America/Bogota (UTC−5 hoy)"`, "data-cinta-track", `href="/@alice/cv"`,
		// Toda sigla explica su significado al pasar el mouse.
		`data-tip="SRE · Site Reliability Engineer: Mantiene`, `data-tip="Senior"`, "· Senior",
		`data-tip="Contenedores"`, `<dt class="font-mono text-accent">CNT</dt>`, `data-tip="OVR (overall)`,
		`<span class="sr-only"> (Contenedores)</span>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("falta %q", want)
		}
	}
	if strings.Contains(body, "style=") {
		t.Error("la CSP no admite style inline")
	}

	// La ficha no trae el CV ni el contacto: cada pestaña muestra solo lo suyo.
	for _, leak := range []string{"Presentación larga", `id="contacto"`, `id="trabajo-acme"`} {
		if strings.Contains(body, leak) {
			t.Errorf("la pestaña Ficha muestra %q", leak)
		}
	}
	_, body = get(t, ts.URL+"/@alice/cv")
	for _, want := range []string{"Presentación larga", `href="#trabajo-acme"`, `id="trabajo-acme"`, "Respalda estos atributos", `href="/@alice#atributo-cnt"`, "Bajó el MTTR", `href="/@alice/cv" aria-current="page"`} {
		if !strings.Contains(body, want) {
			t.Errorf("CV: falta %q", want)
		}
	}
	if strings.Contains(body, "<polygon") || strings.Contains(body, `id="contacto"`) {
		t.Error("la pestaña CV no debe traer la ficha ni el contacto")
	}
	_, body = get(t, ts.URL+"/@alice/contacto")
	if !strings.Contains(body, `id="contacto"`) || strings.Contains(body, "Bajó el MTTR") || strings.Contains(body, "<polygon") {
		t.Error("la pestaña Contacto debe traer solo el contacto")
	}

	_, body = get(t, ts.URL+"/@alice/platform-engineer")
	if !strings.Contains(body, `aria-label="Índice general 74.`) { // PLE: (2·91 + 40) / 3
		t.Error("la vista PLE debe mostrar su propio OVR")
	}
	for _, path := range []string{"/@alice/sre", "/@alice/nope", "/@bob/sre"} {
		if resp, _ := get(t, ts.URL+path); resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s: HTTP %d", path, resp.StatusCode)
		}
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

func TestRadarMuestraLosMejores(t *testing.T) {
	var attrs []profile.Attribute
	for i, score := range []int{50, 90, 10, 80, 70, 60, 95} {
		attrs = append(attrs, profile.Attribute{Code: string(rune('A'+i)) + "X", Subs: []profile.Sub{{Score: score}}})
	}
	var got []string
	for _, a := range topAttributes(attrs, 6) {
		got = append(got, a.Code)
	}
	// Sale el de 10 (CX) y los demás conservan el orden del perfil.
	if strings.Join(got, " ") != "AX BX DX EX FX GX" {
		t.Errorf("topAttributes = %v", got)
	}
}

func TestTimeZoneConHorarioDeVerano(t *testing.T) {
	p := Page{Resume: &profile.Resume{Gremio: profile.Extension{Timezone: "Asia/Kolkata"}}}
	if got, _ := p.TimeZone(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)); got != "UTC+5:30" {
		t.Errorf("Kolkata = %q", got)
	}
	p.Resume.Gremio.Timezone = "Europe/Madrid"
	for date, want := range map[time.Time]string{
		time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC): "UTC+1",
		time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC): "UTC+2",
	} {
		if got, _ := p.TimeZone(date); got != want {
			t.Errorf("Madrid %s = %q, quiero %q", date.Format("01-02"), got, want)
		}
	}
	p.Resume.Gremio.Timezone = ""
	if got, _ := p.TimeZone(time.Now()); got != "" {
		t.Errorf("sin zona = %q", got)
	}
}
