package web

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/duv0-x/gremio/internal/auth"
	"github.com/duv0-x/gremio/internal/federation"
	"github.com/duv0-x/gremio/internal/instance"
	"github.com/duv0-x/gremio/internal/profile"
	"github.com/duv0-x/gremio/internal/store"
)

// panelServer es una instancia con sesión del dueño que guarda con la federación real.
type panelServer struct {
	t       *testing.T
	ts      *httptest.Server
	st      *store.Store
	session *http.Cookie
	client  *http.Client
}

func newPanelServer(t *testing.T, document string) *panelServer {
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
	fed, err := federation.New(ctx, st, federation.Options{Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	a := auth.New(st)
	tok, _ := a.RequestLink(ctx)
	session, err := a.UseLink(ctx, tok)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	New(st, "1.2.3", nil, fed).Register(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return &panelServer{t: t, ts: ts, st: st, session: &http.Cookie{Name: sessionCookie, Value: session},
		client: &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

// do hace la petición; con form != nil es un POST de formulario. Sin sesión si withSession es false.
func (p *panelServer) do(path string, form url.Values, withSession bool, header ...string) (*http.Response, string) {
	p.t.Helper()
	method, body := "GET", io.Reader(nil)
	if form != nil {
		method, body = "POST", strings.NewReader(form.Encode())
	}
	req, _ := http.NewRequest(method, p.ts.URL+path, body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for i := 0; i < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	if withSession {
		req.AddCookie(p.session)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		p.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

func (p *panelServer) profile() *profile.Resume {
	p.t.Helper()
	doc, err := p.st.Read.GetProfile(context.Background())
	if err != nil {
		p.t.Fatal(err)
	}
	r, err := profile.Load(doc)
	if err != nil {
		p.t.Fatal(err)
	}
	return r
}

// requireOwner comprueba que la sección no se ve ni se guarda sin sesión ni desde otro origen.
func (p *panelServer) requireOwner(path string, form url.Values) {
	p.t.Helper()
	before := p.profile()
	if resp, _ := p.do(path, nil, false); resp.Header.Get("Location") != "/login" {
		p.t.Errorf("GET %s sin sesión: %d", path, resp.StatusCode)
	}
	if resp, _ := p.do(path, form, false); resp.Header.Get("Location") != "/login" {
		p.t.Errorf("POST %s sin sesión: %d", path, resp.StatusCode)
	}
	if resp, _ := p.do(path, form, true, "Sec-Fetch-Site", "cross-site"); resp.StatusCode != http.StatusForbidden {
		p.t.Errorf("POST %s desde otro origen: %d", path, resp.StatusCode)
	}
	if after := p.profile(); after.Basics.Name != before.Basics.Name || len(after.Work) != len(before.Work) {
		p.t.Errorf("%s cambió el perfil sin sesión", path)
	}
}

func TestEditorBasico(t *testing.T) {
	p := newPanelServer(t, fichaDoc)
	form := url.Values{
		"name": {"Alice Editada"}, "label": {"Platform Engineer"}, "summary": {"Bio"}, "about": {"Presentación nueva"},
		"city": {"Bogotá"}, "country": {"co"}, "timezone": {"America/Bogota"},
		"email": {"alice@ejemplo.org"}, "email_publico": {"1"}, "phone": {"+57 300 000 0000"}, "whatsapp_publico": {"1"},
		"red.0.network": {"GitHub"}, "red.0.url": {"https://github.com/alice"}, "red.0.orden": {"2"},
		"red.1.network": {"Mastodon"}, "red.1.url": {"https://mastodon.social/@alice"}, "red.1.orden": {"1"},
		"red.2.network": {""},
	}
	p.requireOwner("/panel/basico", form)

	resp, body := p.do("/panel/basico", nil, true)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `value="Alice Ejemplo"`) || !strings.Contains(body, `name="red.0.network"`) {
		t.Fatalf("formulario: %d", resp.StatusCode)
	}
	if resp, _ := p.do("/panel/basico", form, true); resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/panel/basico?ok=guardado" {
		t.Fatalf("guardar: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	r := p.profile()
	if r.Basics.Name != "Alice Editada" || r.Gremio.About != "Presentación nueva" || r.Basics.Location.CountryCode != "CO" ||
		r.Gremio.Contact.Email != "public" || r.Gremio.Contact.Phone != "private" || r.Gremio.Contact.Whatsapp != "public" {
		t.Errorf("guardado: %+v %+v", r.Basics, r.Gremio.Contact)
	}
	if len(r.Basics.Profiles) != 2 || r.Basics.Profiles[0].Network != "Mastodon" {
		t.Errorf("redes en el orden pedido y sin la fila vacía: %+v", r.Basics.Profiles)
	}
	if len(r.Gremio.Attributes) != 3 || len(r.Work) != 1 {
		t.Error("guardar una sección no toca las demás")
	}
	if n, _ := p.st.Read.CountDeliveries(context.Background()); n != 0 {
		t.Errorf("sin seguidores no se encola nada: %d", n)
	}

	// Inválido: vuelve el formulario con lo escrito y el error, y el perfil no cambia.
	form.Set("name", "Otro nombre")
	form.Set("timezone", "Marte/Olympus")
	resp, body = p.do("/panel/basico", form, true)
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, "Marte/Olympus") || !strings.Contains(body, `value="Otro nombre"`) || !strings.Contains(body, `role="alert"`) {
		t.Errorf("inválido: %d", resp.StatusCode)
	}
	if p.profile().Basics.Name != "Alice Editada" {
		t.Error("un formulario inválido no guarda")
	}
	// Borrar una red.
	form.Set("timezone", "America/Bogota")
	form.Set("red.0.borrar", "1")
	p.do("/panel/basico", form, true)
	if rs := p.profile().Basics.Profiles; len(rs) != 1 || rs[0].Network != "Mastodon" {
		t.Errorf("borrar: %+v", rs)
	}
}

func TestEditorExperiencia(t *testing.T) {
	p := newPanelServer(t, fichaDoc)
	p.requireOwner("/panel/experiencia", url.Values{"trabajo.0.id": {"acme"}, "trabajo.0.borrar": {"1"}})
	p.requireOwner("/panel/experiencia/acme", url.Values{"name": {"Otra"}})
	if resp, _ := p.do("/panel/experiencia/no-existe", nil, true); resp.StatusCode != http.StatusNotFound {
		t.Errorf("trabajo inexistente: %d", resp.StatusCode)
	}

	// Uno nuevo va primero, con sus logros y su evidencia.
	nuevo := url.Values{"name": {"Globex"}, "position": {"SRE"}, "start": {"2024-01"},
		"logro.0.text": {"Migró 40 servicios a EKS"}, "logro.0.evidence": {"https://ejemplo.org/a\n\nhttps://ejemplo.org/b"}}
	resp, _ := p.do("/panel/experiencia/nuevo", nuevo, true)
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/panel/experiencia?ok=guardado" {
		t.Fatalf("nuevo: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	r := p.profile()
	if len(r.Work) != 2 || r.Work[0].Name != "Globex" || len(r.Work[0].Highlights) != 1 || len(r.Work[0].Highlights[0].Evidence) != 2 {
		t.Fatalf("nuevo guardado: %+v", r.Work)
	}
	globex := r.Work[0].ID

	// Inválido: vuelve con lo escrito, sin guardar.
	resp, body := p.do("/panel/experiencia/nuevo", url.Values{"name": {""}, "logro.0.text": {"Texto que no se pierde"}}, true)
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, "Texto que no se pierde") || !strings.Contains(body, `action="/panel/experiencia/nuevo"`) {
		t.Errorf("nuevo inválido: %d", resp.StatusCode)
	}
	if len(p.profile().Work) != 2 {
		t.Error("un trabajo inválido no se guarda")
	}

	// Borrar un logro que respalda un atributo quita también la referencia.
	_, body = p.do("/panel/experiencia/acme", nil, true)
	if !strings.Contains(body, `name="logro.0.id" value="h1"`) || !strings.Contains(body, "Bajó el MTTR") {
		t.Fatal("el formulario trae los logros con su id")
	}
	edit := url.Values{"name": {"Acme Corp"}, "position": {"SRE"}, "start": {"2021-05"},
		"logro.0.id": {"h1"}, "logro.0.text": {"Bajó el MTTR 23 %"}, "logro.0.borrar": {"1"}, "logro.1.text": {"Otro logro"}}
	if resp, _ := p.do("/panel/experiencia/acme", edit, true); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("editar: %d", resp.StatusCode)
	}
	r = p.profile()
	acme := r.Work[1]
	if acme.ID != "acme" || acme.Name != "Acme Corp" || len(acme.Highlights) != 1 || acme.Highlights[0].Text != "Otro logro" {
		t.Errorf("editado: %+v", acme)
	}
	if hs := r.Gremio.Attributes[0].Highlights; len(hs) != 0 {
		t.Errorf("la referencia al logro borrado sigue en el atributo: %v", hs)
	}

	// La lista ordena y borra.
	if resp, _ := p.do("/panel/experiencia", url.Values{"trabajo.0.id": {globex}, "trabajo.0.orden": {"2"}, "trabajo.1.id": {"acme"}, "trabajo.1.orden": {"1"}}, true); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("ordenar: %d", resp.StatusCode)
	}
	if r := p.profile(); r.Work[0].ID != "acme" || r.Work[1].ID != globex {
		t.Errorf("orden: %s, %s", r.Work[0].ID, r.Work[1].ID)
	}
	p.do("/panel/experiencia", url.Values{"trabajo.0.id": {"acme"}, "trabajo.1.id": {globex}, "trabajo.1.borrar": {"1"}}, true)
	if r := p.profile(); len(r.Work) != 1 || r.Work[0].ID != "acme" {
		t.Errorf("borrar: %+v", r.Work)
	}
}
