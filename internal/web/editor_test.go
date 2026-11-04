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
