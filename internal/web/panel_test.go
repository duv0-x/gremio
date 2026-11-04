package web

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/duv0-x/gremio/internal/auth"
	"github.com/duv0-x/gremio/internal/instance"
	"github.com/duv0-x/gremio/internal/profile"
	"github.com/duv0-x/gremio/internal/store"
)

type fakeMailer struct{ to, body string }

func (f *fakeMailer) Send(_ context.Context, to, _, body string) error {
	f.to, f.body = to, body
	return nil
}

func TestLoginDelPanel(t *testing.T) {
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
	m := &fakeMailer{}
	mux := http.NewServeMux()
	New(st, "1.2.3", m, nil).Register(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	do := func(method, path string, cookie *http.Cookie, header ...string) (*http.Response, string) {
		t.Helper()
		req, _ := http.NewRequest(method, ts.URL+path, nil)
		if cookie != nil {
			req.AddCookie(cookie)
		}
		for i := 0; i < len(header); i += 2 {
			req.Header.Set(header[i], header[i+1])
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp, string(b)
	}

	if resp, _ := do("GET", "/panel", nil); resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/login" {
		t.Errorf("sin sesión, /panel lleva a /login: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	if resp, body := do("GET", "/login", nil); resp.StatusCode != http.StatusOK || !strings.Contains(body, `action="/login"`) || strings.Contains(body, "<input") {
		t.Errorf("/login es un solo botón, sin campos: %d", resp.StatusCode)
	}
	if resp, _ := do("POST", "/login", nil); resp.StatusCode != http.StatusServiceUnavailable || m.to != "" {
		t.Errorf("sin correo del dueño no envía nada: %d", resp.StatusCode)
	}

	if err := instance.SetOwnerEmail(ctx, st, "alice@ejemplo.org"); err != nil {
		t.Fatal(err)
	}
	if resp, body := do("POST", "/login", nil); resp.StatusCode != http.StatusOK || !strings.Contains(body, "Revisa tu correo") {
		t.Fatalf("pedir el enlace: %d", resp.StatusCode)
	}
	if m.to != "alice@ejemplo.org" {
		t.Errorf("destinatario %q", m.to)
	}
	link := regexp.MustCompile(`https://gremio\.ejemplo\.org/login/([A-Za-z0-9_-]{43})`).FindStringSubmatch(m.body)
	if link == nil {
		t.Fatalf("el correo no trae el enlace:\n%s", m.body)
	}
	path := "/login/" + link[1]
	if resp, _ := do("POST", "/login", nil); resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("segundo envío en el mismo minuto: %d", resp.StatusCode)
	}

	// Abrir el enlace (GET) solo muestra el botón: ni sesión ni token gastado.
	resp, body := do("GET", path, nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `action="`+path+`"`) || len(resp.Cookies()) != 0 {
		t.Errorf("GET del enlace: %d, cookies %v", resp.StatusCode, resp.Cookies())
	}
	if resp.Header.Get("Referrer-Policy") != "no-referrer" || resp.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("cabeceras del enlace: %v", resp.Header)
	}
	if resp, _ := do("POST", path, nil, "Sec-Fetch-Site", "cross-site"); resp.StatusCode != http.StatusForbidden {
		t.Errorf("POST desde otro origen: %d", resp.StatusCode)
	}

	resp, _ = do("POST", path, nil)
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/panel" {
		t.Fatalf("entrar: %d", resp.StatusCode)
	}
	var session *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == sessionCookie {
			session = c
		}
	}
	if session == nil || !session.HttpOnly || !session.Secure || session.SameSite != http.SameSiteLaxMode || session.Path != "/" || session.Domain != "" {
		t.Fatalf("cookie de sesión: %+v", session)
	}
	if resp, _ := do("POST", path, nil); resp.StatusCode != http.StatusGone {
		t.Errorf("el enlace sirve una sola vez: %d", resp.StatusCode)
	}
	if resp, _ := do("GET", "/login/no-es-un-token", nil); resp.StatusCode != http.StatusGone {
		t.Errorf("token mal formado: %d", resp.StatusCode)
	}

	if resp, body := do("GET", "/panel", session); resp.StatusCode != http.StatusOK || !strings.Contains(body, "Tu perfil") {
		t.Errorf("panel con sesión: %d", resp.StatusCode)
	}
	if resp, _ := do("GET", "/login", session); resp.Header.Get("Location") != "/panel" {
		t.Error("con sesión, /login lleva al panel")
	}
	if resp, _ := do("POST", "/panel/cerrar-sesiones", session, "Sec-Fetch-Site", "cross-site"); resp.StatusCode != http.StatusForbidden {
		t.Errorf("cerrar sesiones desde otro origen: %d", resp.StatusCode)
	}
	if resp, _ := do("POST", "/panel/cerrar-sesiones", session); resp.StatusCode != http.StatusSeeOther {
		t.Errorf("cerrar todas: %d", resp.StatusCode)
	}
	if resp, _ := do("GET", "/panel", session); resp.Header.Get("Location") != "/login" {
		t.Error("tras cerrar todas, la sesión ya no sirve")
	}
}

type fakeProfiles struct{ saved *profile.Resume }

func (f *fakeProfiles) ReplaceProfile(_ context.Context, r *profile.Resume) (int, error) {
	f.saved = r
	return 0, nil
}

func TestImportarYExportarDesdeElPanel(t *testing.T) {
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
	if err := st.Write.UpdateProfile(ctx, doc); err != nil {
		t.Fatal(err)
	}
	a := auth.New(st)
	tok, _ := a.RequestLink(ctx)
	sessionToken, err := a.UseLink(ctx, tok)
	if err != nil {
		t.Fatal(err)
	}
	session := &http.Cookie{Name: sessionCookie, Value: sessionToken}
	profiles := &fakeProfiles{}
	mux := http.NewServeMux()
	New(st, "1.2.3", nil, profiles).Register(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	upload := func(cookie *http.Cookie, content string, header ...string) (*http.Response, string) {
		t.Helper()
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fw, _ := mw.CreateFormFile("archivo", "perfil.json")
		fw.Write([]byte(content))
		mw.Close()
		req, _ := http.NewRequest("POST", ts.URL+"/panel/importar", &buf)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		for i := 0; i < len(header); i += 2 {
			req.Header.Set(header[i], header[i+1])
		}
		if cookie != nil {
			req.AddCookie(cookie)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp, string(b)
	}
	confirm := func(cookie *http.Cookie, document string, header ...string) *http.Response {
		t.Helper()
		req, _ := http.NewRequest("POST", ts.URL+"/panel/importar/confirmar", strings.NewReader(url.Values{"documento": {document}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		for i := 0; i < len(header); i += 2 {
			req.Header.Set(header[i], header[i+1])
		}
		if cookie != nil {
			req.AddCookie(cookie)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp
	}
	export := func(cookie *http.Cookie, query string) (*http.Response, string) {
		t.Helper()
		req, _ := http.NewRequest("GET", ts.URL+"/panel/exportar"+query, nil)
		if cookie != nil {
			req.AddCookie(cookie)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp, string(b)
	}

	next := strings.Replace(doc, "Alice Ejemplo", "Alice Nueva", 1)
	// Sin sesión, nada: ni vista previa, ni guardar, ni exportar.
	if resp, _ := upload(nil, next); resp.Header.Get("Location") != "/login" {
		t.Errorf("importar sin sesión: %d", resp.StatusCode)
	}
	if resp := confirm(nil, next); resp.Header.Get("Location") != "/login" || profiles.saved != nil {
		t.Errorf("confirmar sin sesión: %d", resp.StatusCode)
	}
	if resp, body := export(nil, ""); resp.Header.Get("Location") != "/login" || strings.Contains(body, "Alice") {
		t.Errorf("exportar sin sesión: %d", resp.StatusCode)
	}
	// Desde otro origen tampoco, aunque haya sesión.
	if resp, _ := upload(session, next, "Sec-Fetch-Site", "cross-site"); resp.StatusCode != http.StatusForbidden {
		t.Errorf("importar desde otro origen: %d", resp.StatusCode)
	}
	if resp := confirm(session, next, "Sec-Fetch-Site", "cross-site"); resp.StatusCode != http.StatusForbidden || profiles.saved != nil {
		t.Errorf("confirmar desde otro origen: %d", resp.StatusCode)
	}

	// La vista previa muestra qué cambia y no guarda.
	resp, body := upload(session, next)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "Revisa los cambios") || !strings.Contains(body, "&#34;Alice Nueva&#34;") || profiles.saved != nil {
		t.Fatalf("vista previa: %d %v", resp.StatusCode, profiles.saved)
	}
	if resp, body := upload(session, "{no es json"); resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, "JSON inválido") {
		t.Errorf("archivo inválido: %d", resp.StatusCode)
	}
	if resp := confirm(session, next); resp.StatusCode != http.StatusSeeOther || profiles.saved == nil || profiles.saved.Basics.Name != "Alice Nueva" {
		t.Errorf("confirmar: %d %v", resp.StatusCode, profiles.saved)
	}

	resp, body = export(session, "")
	if resp.StatusCode != http.StatusOK || !strings.Contains(resp.Header.Get("Content-Disposition"), "perfil-gremio.json") || !strings.Contains(body, `"gremio"`) {
		t.Errorf("exportar con extensiones: %d %s", resp.StatusCode, resp.Header.Get("Content-Disposition"))
	}
	if _, body := export(session, "?formato=estandar"); strings.Contains(body, `"gremio"`) || strings.Contains(body, `"id"`) {
		t.Error("el JSON Resume estándar no lleva la extensión ni ids")
	}
}
