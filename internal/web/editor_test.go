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
	"github.com/duv0-x/gremio/internal/dojo"
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
	srv     *Server
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
	srv := New(st, "1.2.3", nil, fed, dojo.NewLedger(st))
	srv.Register(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return &panelServer{t: t, ts: ts, st: st, srv: srv, session: &http.Cookie{Name: sessionCookie, Value: session},
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
		"email": {"alice@ejemplo.org"}, "email_publico": {"1"}, "phone": {"+57 300 000 0000"},
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
		r.Gremio.Contact.Email != "public" || r.Gremio.Contact.Phone != "private" {
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

func TestEditorListas(t *testing.T) {
	p := newPanelServer(t, fichaDoc)
	for _, path := range []string{"/panel/skills", "/panel/proyectos", "/panel/formacion"} {
		p.requireOwner(path, url.Values{"skill.0.name": {"x"}})
		if resp, _ := p.do(path, nil, true); resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s: %d", path, resp.StatusCode)
		}
	}

	// Skills: la que existe conserva su id; borrar k8s quita su referencia del atributo CNT.
	_, body := p.do("/panel/skills", nil, true)
	if !strings.Contains(body, `name="skill.0.id" value="k8s"`) || !strings.Contains(body, ">EKS<") {
		t.Fatal("el formulario trae las skills con su id y keywords")
	}
	if resp, _ := p.do("/panel/skills", url.Values{"skill.0.id": {"k8s"}, "skill.0.name": {"Kubernetes"}, "skill.0.borrar": {"1"},
		"skill.1.name": {"Go"}, "skill.1.level": {"learning"}, "skill.1.keywords": {"templ, sqlc"}}, true); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("skills: %d", resp.StatusCode)
	}
	r := p.profile()
	if len(r.Skills) != 1 || r.Skills[0].Name != "Go" || r.Skills[0].Level != "learning" || len(r.Skills[0].Keywords) != 2 || len(r.Gremio.Attributes[0].Skills) != 0 {
		t.Errorf("skills: %+v, atributo %v", r.Skills, r.Gremio.Attributes[0].Skills)
	}
	if resp, _ := p.do("/panel/skills", url.Values{"skill.0.name": {"Go"}, "skill.0.level": {"guru"}}, true); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("nivel inválido: %d", resp.StatusCode)
	}

	if resp, _ := p.do("/panel/proyectos", url.Values{"proyecto.0.name": {"papiro"}, "proyecto.0.url": {"https://papiro.ejemplo.org"},
		"proyecto.0.keywords": {"Go, HTMX"}, "proyecto.1.description": {"sin nombre"}}, true); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("proyecto sin nombre: %d", resp.StatusCode)
	}
	p.do("/panel/proyectos", url.Values{"proyecto.0.name": {"papiro"}, "proyecto.0.url": {"https://papiro.ejemplo.org"}, "proyecto.0.keywords": {"Go, HTMX"}}, true)
	if r := p.profile(); len(r.Projects) != 1 || r.Projects[0].ID == "" || len(r.Projects[0].Keywords) != 2 {
		t.Errorf("proyectos: %+v", r.Projects)
	}

	p.do("/panel/formacion", url.Values{"estudio.0.institution": {"UdeA"}, "estudio.0.start": {"2005"},
		"idioma.0.language": {"Español"}, "idioma.0.fluency": {"Nativo"}, "certificado.0.name": {"CKA"}, "certificado.0.date": {"2023-04"}}, true)
	if r := p.profile(); len(r.Education) != 1 || len(r.Languages) != 1 || len(r.Certificates) != 1 || r.Certificates[0].Name != "CKA" {
		t.Errorf("formación: %+v %+v %+v", r.Education, r.Languages, r.Certificates)
	}
	if resp, _ := p.do("/panel/formacion", url.Values{"estudio.0.institution": {"UdeA"}, "estudio.0.start": {"ayer"}}, true); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("fecha inválida: %d", resp.StatusCode)
	}
}

func TestEditorVistasYAtributos(t *testing.T) {
	p := newPanelServer(t, fichaDoc)
	p.requireOwner("/panel/vistas", url.Values{"vista.0.id": {"sre"}, "vista.0.borrar": {"1"}})
	p.requireOwner("/panel/vistas/sre", url.Values{"name": {"x"}, "slug": {"sre"}})
	p.requireOwner("/panel/atributos/CNT", url.Values{"code": {"CNT"}, "name": {"x"}})
	if resp, _ := p.do("/panel/vistas/no-existe", nil, true); resp.StatusCode != http.StatusNotFound {
		t.Errorf("vista inexistente: %d", resp.StatusCode)
	}

	// Editar una vista: pesos (vacío excluye), qué incluye y datos.
	_, body := p.do("/panel/vistas/sre", nil, true)
	if !strings.Contains(body, `name="peso.OBS"`) || !strings.Contains(body, `value="2"`) || !strings.Contains(body, `<datalist id="posiciones">`) {
		t.Fatal("formulario de vista")
	}
	if resp, _ := p.do("/panel/vistas/sre", url.Values{"name": {"Site Reliability"}, "slug": {"sre"}, "code": {"sre"}, "level": {"sr"},
		"peso.OBS": {"3"}, "peso.CNT": {""}, "peso.COD": {"0,5"}, "work": {"acme"}, "skill": {"k8s"}, "highlight": {"h1"}}, true); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("editar vista: %d", resp.StatusCode)
	}
	v := p.profile().Gremio.Views[0]
	if v.Name != "Site Reliability" || v.Code != "SRE" || len(v.Weights) != 2 || v.Weights["COD"] != 0.5 || len(v.Work) != 1 || len(v.Highlights) != 1 {
		t.Errorf("vista: %+v", v)
	}

	// Nueva: va al final; inválida, vuelve con lo escrito.
	resp, body := p.do("/panel/vistas/nuevo", url.Values{"name": {"DevOps"}, "slug": {"Mal Slug"}}, true)
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, `value="Mal Slug"`) || !strings.Contains(body, `value="DevOps"`) {
		t.Errorf("vista nueva inválida: %d", resp.StatusCode)
	}
	p.do("/panel/vistas/nuevo", url.Values{"name": {"DevOps"}, "slug": {"devops"}, "code": {"DVO"}, "peso.CNT": {"1"}}, true)
	if vs := p.profile().Gremio.Views; len(vs) != 3 || vs[2].Slug != "devops" {
		t.Fatalf("vista nueva: %+v", vs)
	}
	// La lista ordena: la principal pasa a ser devops.
	p.do("/panel/vistas", url.Values{"vista.0.id": {"sre"}, "vista.0.orden": {"3"}, "vista.1.id": {"platform-engineer"}, "vista.1.orden": {"2"},
		"vista.2.id": {"devops"}, "vista.2.orden": {"1"}}, true)
	if vs := p.profile().Gremio.Views; vs[0].Slug != "devops" || vs[2].Slug != "sre" {
		t.Errorf("orden de vistas: %s %s %s", vs[0].Slug, vs[1].Slug, vs[2].Slug)
	}

	// Renombrar un atributo mueve sus pesos en todas las vistas.
	if resp, _ := p.do("/panel/atributos/CNT", url.Values{"code": {"cnx"}, "name": {"Contenedores"}, "sub.0.name": {"Kubernetes"}, "sub.0.score": {"95"}, "sub.0.dojoSkill": {"kubernetes"},
		"sub.1.name": {"EKS"}, "sub.1.score": {"85"}, "sub.1.learning": {"1"}, "skill": {"k8s"}}, true); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("editar atributo: %d", resp.StatusCode)
	}
	r := p.profile()
	if a := r.Gremio.Attributes[0]; a.Code != "CNX" || a.Score() != 90 || !a.Subs[1].Learning || a.Subs[0].DojoSkill != "kubernetes" || len(a.Highlights) != 0 {
		t.Errorf("atributo: %+v", a)
	}
	for _, v := range r.Gremio.Views {
		if _, ok := v.Weights["CNT"]; ok {
			t.Errorf("la vista %s conserva el código viejo: %v", v.Slug, v.Weights)
		}
	}
	if r.Gremio.Views[0].Weights["CNX"] != 1 || r.Gremio.Views[1].Weights["CNX"] != 2 {
		t.Errorf("pesos renombrados: %v %v", r.Gremio.Views[0].Weights, r.Gremio.Views[1].Weights)
	}
	if resp, _ := p.do("/panel/atributos/nuevo", url.Values{"code": {"IAC"}, "name": {"IaC"}, "sub.0.name": {"Terraform"}, "sub.0.score": {"mucho"}}, true); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("puntaje inválido: %d", resp.StatusCode)
	}
	p.do("/panel/atributos/nuevo", url.Values{"code": {"IAC"}, "name": {"IaC"}, "sub.0.name": {"Terraform"}, "sub.0.score": {"88"}}, true)
	// Borrar OBS lo quita de los pesos; quedarse con 2 no se permite.
	p.do("/panel/atributos", url.Values{"atributo.0.id": {"CNX"}, "atributo.1.id": {"OBS"}, "atributo.1.borrar": {"1"}, "atributo.2.id": {"COD"}, "atributo.3.id": {"IAC"}}, true)
	r = p.profile()
	if len(r.Gremio.Attributes) != 3 || r.Gremio.Views[2].Weights["OBS"] != 0 {
		t.Errorf("borrar atributo: %d atributos, pesos sre %v", len(r.Gremio.Attributes), r.Gremio.Views[2].Weights)
	}
	if resp, _ := p.do("/panel/atributos", url.Values{"atributo.0.id": {"CNX"}, "atributo.1.id": {"COD"}, "atributo.2.id": {"IAC"}, "atributo.2.borrar": {"1"}}, true); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("con 2 atributos el radar no se arma: %d", resp.StatusCode)
	}

	// Los rasgos ya no se editan (ADR-015 §4), pero se conservan en el perfil.
	if resp, _ := p.do("/panel/rasgos", nil, true); resp.StatusCode != http.StatusNotFound {
		t.Errorf("el editor de rasgos ya no existe: %d", resp.StatusCode)
	}
	if ts := p.profile().Gremio.Traits; len(ts) != 2 {
		t.Errorf("los rasgos se conservan: %+v", ts)
	}
}

func TestKeepOrder(t *testing.T) {
	if got := keepOrder([]string{"c", "a", "b"}, []string{"a", "b", "d", "c"}); strings.Join(got, ",") != "c,a,b,d" {
		t.Errorf("keepOrder = %v", got)
	}
	if got := keepOrder([]string{"c", "a"}, []string{"a"}); strings.Join(got, ",") != "a" {
		t.Errorf("desmarcar: %v", got)
	}
}

// Con htmx (gremio#90): las mismas rutas, con respuestas que htmx sabe poner en la página sin recargarla.
func TestEditorConHTMX(t *testing.T) {
	p := newPanelServer(t, fichaDoc)
	hx := []string{"HX-Request", "true"}

	// Sin sesión no hay redirect a seguir: htmx navega al login.
	if resp, _ := p.do("/panel/skills", url.Values{"skill.0.name": {"x"}}, false, hx...); resp.StatusCode != http.StatusNoContent || resp.Header.Get("HX-Redirect") != "/login" {
		t.Errorf("sin sesión: %d %q", resp.StatusCode, resp.Header.Get("HX-Redirect"))
	}

	// Una sección se guarda como sin JavaScript: htmx sigue el redirect y toma el #editor con el aviso.
	_, body := p.do("/panel/skills", nil, true)
	for _, want := range []string{`<div id="editor"`, `hx-post="/panel/skills"`, `hx-select="#editor"`, `id="editor-aviso"`, `/static/htmx.min.js?v=1.2.3`} {
		if !strings.Contains(body, want) {
			t.Errorf("el formulario no trae %s", want)
		}
	}
	if resp, _ := p.do("/panel/skills", url.Values{"skill.0.id": {"k8s"}, "skill.0.name": {"Kubernetes"}}, true, hx...); resp.StatusCode != http.StatusSeeOther {
		t.Errorf("guardar una sección: %d", resp.StatusCode)
	}
	if _, body := p.do("/panel/skills?ok=guardado", nil, true); !strings.Contains(body, `role="status" tabindex="-1" autofocus`) {
		t.Error("el aviso de guardado toma el foco")
	}

	// Un elemento vuelve a su lista con HX-Location, que cambia la URL como el redirect.
	if resp, _ := p.do("/panel/experiencia/acme", url.Values{"name": {"Acme"}, "start": {"2020-01"}}, true, hx...); resp.StatusCode != http.StatusNoContent || resp.Header.Get("HX-Location") != "/panel/experiencia?ok=guardado" {
		t.Errorf("guardar un elemento: %d %q", resp.StatusCode, resp.Header.Get("HX-Location"))
	}

	// Inválido: 400 con el formulario y lo escrito, que htmx también pone en la página.
	if resp, body := p.do("/panel/experiencia/acme", url.Values{"name": {""}, "summary": {"No se pierde"}}, true, hx...); resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, "No se pierde") || !strings.Contains(body, `role="alert" tabindex="-1" autofocus`) {
		t.Errorf("elemento inválido: %d", resp.StatusCode)
	}

	// Un error sin formulario va a #editor-aviso, sin reemplazar lo escrito.
	resp, body := p.do("/panel/experiencia/no-existe", url.Values{"name": {"x"}}, true, hx...)
	if resp.StatusCode != http.StatusNotFound || resp.Header.Get("HX-Retarget") != "#editor-aviso" || !strings.Contains(body, "ya no existe") {
		t.Errorf("error sin formulario: %d %q %q", resp.StatusCode, resp.Header.Get("HX-Retarget"), body)
	}

	// Agregar otra fila: ?mas=skill&n=1 trae una fila vacía extra con foco y el botón que pide la siguiente.
	n := len(p.profile().Skills)
	nueva := "fila-skill-" + itoa(n+blankRows)
	_, body = p.do("/panel/skills", nil, true)
	if !strings.Contains(body, `hx-get="/panel/skills?mas=skill&amp;n=1"`) || !strings.Contains(body, `hx-select="#`+nueva+`, #mas-skill"`) || strings.Contains(body, `id="`+nueva+`"`) {
		t.Fatal("el botón pide la fila siguiente y la página no la trae todavía")
	}
	_, body = p.do("/panel/skills?mas=skill&n=1", nil, true, hx...)
	if !strings.Contains(body, `id="`+nueva+`" class="flex flex-col gap-4 rounded-md border p-4 border-dashed border-control" tabindex="-1" autofocus`) ||
		!strings.Contains(body, `name="skill.`+itoa(n+blankRows)+`.name"`) || !strings.Contains(body, `hx-get="/panel/skills?mas=skill&amp;n=2"`) {
		t.Error("la respuesta trae la fila nueva con foco y el botón para la siguiente")
	}
	// La fila nueva se guarda como cualquier otra.
	if resp, _ := p.do("/panel/skills", url.Values{"skill." + itoa(n+blankRows) + ".name": {"Rust"}}, true, hx...); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("guardar la fila nueva: %d", resp.StatusCode)
	}
	if r := p.profile(); r.Skills[len(r.Skills)-1].Name != "Rust" {
		t.Errorf("fila nueva: %+v", r.Skills)
	}
}
