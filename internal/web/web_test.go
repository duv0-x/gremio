package web

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	return newTestServerIn(t, document, t.TempDir())
}

func newTestServerIn(t *testing.T, document, dataDir string) *httptest.Server {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, dataDir)
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
var fichaDoc = strings.Replace(doc, `"contact": {"email": "private", "phone": "private"}}`, `"contact": {"email": "private", "phone": "private", "whatsapp": "public"}, "timezone": "America/Bogota",
  "attributes": [
    {"code": "CNT", "name": "Contenedores", "subs": [{"name": "Kubernetes", "score": 92}, {"name": "EKS", "score": 90}], "skills": ["k8s"], "highlights": ["h1"]},
    {"code": "OBS", "name": "Observabilidad", "subs": [{"name": "Grafana", "score": 70}]},
    {"code": "COD", "name": "Código", "subs": [{"name": "Go", "score": 40, "learning": true}]}],
  "views": [
    {"slug": "sre", "code": "SRE", "level": "sr", "name": "SRE", "weights": {"OBS": 2, "CNT": 1}},
    {"slug": "platform-engineer", "code": "PLE", "name": "Platform Engineer", "weights": {"CNT": 2, "COD": 1}}],
  "traits": [{"name": "Automatizadora", "summary": "Todo pasa por un pipeline.", "icon": "bot", "color": "violeta"}, {"name": "Sin ícono"}]}`, 1)

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
		"badge-violeta", `data-tip="Automatizadora: Todo pasa por un pipeline."`, `<path d="M12 8V4H8"/>`, "badge-verde", ">SÍ</text>",
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

	// Rol y pestaña cambian #vista sin recargar (static/vista.js); sin JS, los enlaces navegan igual.
	for _, want := range []string{`<div id="vista">`, `href="/@alice/platform-engineer" class=`, `data-swap>`, `src="/static/vista.js?v=`} {
		if !strings.Contains(body, want) {
			t.Errorf("vista: falta %q", want)
		}
	}
	if resp, js := get(t, ts.URL+"/static/vista.js"); resp.StatusCode != http.StatusOK || !strings.Contains(js, "startViewTransition") {
		t.Errorf("vista.js: HTTP %d", resp.StatusCode)
	}
	// El recuadro de perfil: nombre, hora local, empresa actual y el panel para seguir desde otra instancia.
	for _, want := range []string{`<h1 id="nombre"`, "UTC−5 · ", "Acme", `href="#seguir"`, `id="seguir"`, `action="/@alice/seguir"`} {
		if !strings.Contains(body, want) {
			t.Errorf("recuadro: falta %q", want)
		}
	}
	// La ficha abre con la presentación (escapada), pero no trae el CV ni el contacto.
	if !strings.Contains(body, "Presentación larga &lt;script&gt;") {
		t.Error("la ficha no muestra la presentación escapada")
	}
	for _, leak := range []string{`id="contacto"`, `id="trabajo-acme"`, "Línea de tiempo", "logros con contexto"} {
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
	if strings.Contains(body, "<polygon") {
		t.Error("la pestaña CV no debe traer la ficha")
	}
	if !strings.Contains(body, `<h1 id="nombre"`) || strings.Count(body, "Presentación larga") != 1 {
		t.Error("el CV comparte el recuadro de perfil y no repite la presentación")
	}
	if strings.Contains(body, `id="contacto"`) {
		t.Error("el CV no lleva sección de contacto: el contacto rápido son los botones fijos")
	}
	noFollow := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	if resp, err := noFollow.Get(ts.URL + "/@alice/contacto"); err != nil || resp.StatusCode != http.StatusMovedPermanently || resp.Header.Get("Location") != "/@alice/cv" {
		t.Errorf("/contacto debe redirigir al CV: %v %v", resp, err)
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

func TestRadarSegunElRol(t *testing.T) {
	var attrs []profile.Attribute
	for i, score := range []int{50, 90, 10, 80, 70, 60, 95} {
		attrs = append(attrs, profile.Attribute{Code: string(rune('A'+i)) + "X", Subs: []profile.Sub{{Score: score}}})
	}
	codes := func(v profile.View) string {
		var got []string
		for _, a := range radarAttributes(attrs, v, 6) {
			got = append(got, a.Code)
		}
		return strings.Join(got, " ")
	}
	for name, c := range map[string]struct {
		weights map[string]float64
		want    string
	}{
		// Sin pesos sale el de menor puntaje (CX) y los demás conservan el orden del perfil.
		"sin pesos": {nil, "AX BX DX EX FX GX"},
		// Un peso alto mete a CX (10 × 9 = 90) por encima de AX (50 × 1).
		"peso alto": {map[string]float64{"AX": 1, "BX": 1, "CX": 9, "DX": 1, "EX": 1, "FX": 1, "GX": 1}, "BX CX DX EX FX GX"},
		// Peso 0 o ausente: no es parte del rol, aunque sobre espacio.
		"solo los del rol": {map[string]float64{"BX": 1, "DX": 2, "GX": 1, "AX": 0}, "BX DX GX"},
		// Con menos de 3 el radar no tendría forma: se usan todos.
		"muy pocos": {map[string]float64{"BX": 1}, "AX BX DX EX FX GX"},
	} {
		if got := codes(profile.View{Weights: c.weights}); got != c.want {
			t.Errorf("%s: %q, quería %q", name, got, c.want)
		}
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

func TestIniciales(t *testing.T) {
	for name, want := range map[string]string{"Líder de equipo": "LE", "Ingeniería agéntica": "IA", "Self-hosted": "SH", "Automatizador": "AU"} {
		if got := traitInitials(name); got != want {
			t.Errorf("traitInitials(%q) = %q, quiero %q", name, got, want)
		}
	}
}

func TestAccionesDescargarCVyWhatsApp(t *testing.T) {
	dir := t.TempDir()
	ts := newTestServerIn(t, fichaDoc, dir)

	resp, body := get(t, ts.URL+"/@alice")
	if !strings.Contains(body, `href="/@alice/whatsapp"`) || strings.Contains(body, "wa.me") || strings.Contains(body, "300") || strings.Contains(body, "/@alice/cv.pdf") {
		t.Errorf("sin PDF: debe estar WhatsApp, sin el número en el HTML, y no la descarga (HTTP %d)", resp.StatusCode)
	}
	noFollow := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	if resp, err := noFollow.Get(ts.URL + "/@alice/whatsapp"); err != nil || resp.StatusCode != http.StatusFound ||
		!strings.HasPrefix(resp.Header.Get("Location"), "https://wa.me/573000000000?text=Hola+Alice%2C+vi+tu+perfil+en+Gremio") {
		t.Errorf("/whatsapp debe redirigir a wa.me: %v %v", resp, err)
	}
	if resp, _ := get(t, ts.URL+"/@alice/cv.pdf"); resp.StatusCode != http.StatusNotFound {
		t.Errorf("cv.pdf sin subir: HTTP %d", resp.StatusCode)
	}

	if err := SaveCV(dir, []byte("no soy un pdf")); err == nil {
		t.Error("SaveCV aceptó un archivo que no es PDF")
	}
	if err := SaveCV(dir, []byte("%PDF-1.7 hola")); err != nil {
		t.Fatal(err)
	}
	_, body = get(t, ts.URL+"/@alice/cv")
	if !strings.Contains(body, `href="/@alice/cv.pdf" download`) {
		t.Error("con PDF, el recuadro debe ofrecer la descarga en todas las pestañas")
	}
	// Con ficha, las acciones van en el recuadro, debajo de Seguir: no hay barra flotante.
	if strings.Contains(body, `aria-label="Acciones"`) || strings.Index(body, `href="#seguir"`) > strings.Index(body, `href="/@alice/cv.pdf"`) {
		t.Error("con ficha, Descargar CV y WhatsApp van debajo de Seguir y sin barra flotante")
	}
	resp, pdf := get(t, ts.URL+"/@alice/cv.pdf")
	if resp.StatusCode != http.StatusOK || pdf != "%PDF-1.7 hola" || !strings.HasPrefix(resp.Header.Get("Content-Disposition"), "attachment;") {
		t.Errorf("cv.pdf: HTTP %d %q %q", resp.StatusCode, resp.Header.Get("Content-Disposition"), pdf)
	}
}

func TestSeguir(t *testing.T) {
	ts := newTestServer(t, fichaDoc)
	noFollow := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for in, host := range map[string]string{
		"mastodon.social":                "mastodon.social",
		" HTTPS://Mastodon.Social/@bob ": "mastodon.social",
		"@bob@hachyderm.io":              "hachyderm.io",
		"gts.ejemplo.org:8443":           "gts.ejemplo.org:8443",
	} {
		resp, err := noFollow.Get(ts.URL + "/@alice/seguir?instancia=" + url.QueryEscape(in))
		if err != nil {
			t.Fatal(err)
		}
		want := "https://" + host + "/authorize_interaction?uri=" + url.QueryEscape("https://gremio.ejemplo.org/users/alice")
		if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != want {
			t.Errorf("%q: HTTP %d → %q, quería %q", in, resp.StatusCode, resp.Header.Get("Location"), want)
		}
	}
	// Solo nombres de host: nada que cambie el destino ni la ruta.
	for _, in := range []string{"", "localhost", "evil.org?x=1", "evil.org#x", "a b.org", "javascript:alert(1)"} {
		if resp, _ := noFollow.Get(ts.URL + "/@alice/seguir?instancia=" + url.QueryEscape(in)); resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%q: HTTP %d, quería 400", in, resp.StatusCode)
		}
	}
}
