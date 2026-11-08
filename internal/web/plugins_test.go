package web

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// Sección Plugins del panel (ADR-012) con un Ollama propio falso: habilitar, probar, editar y deshabilitar.
func TestPanelPlugins(t *testing.T) {
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, `{"models":[{"name":"gemma4"},{"name":"qwen"}]}`)
	}))
	defer ollama.Close()
	host := strings.TrimPrefix(ollama.URL, "http://")
	p := newPanelServer(t, fichaDoc)
	const token = "token-local-secreto-WXYZ"

	if resp, _ := p.do("/panel/plugins", nil, false); resp.Header.Get("Location") != "/login" {
		t.Errorf("sin sesión: %d", resp.StatusCode)
	}
	if resp, _ := p.do("/panel/plugins/ollama", url.Values{"endpoint": {ollama.URL}}, true, "Sec-Fetch-Site", "cross-site"); resp.StatusCode != http.StatusForbidden {
		t.Errorf("POST desde otro origen: %d", resp.StatusCode)
	}
	_, body := p.do("/panel", nil, true)
	if !strings.Contains(body, `href="/panel/plugins"`) {
		t.Error("el panel enlaza a Plugins")
	}
	_, body = p.do("/panel/plugins", nil, true)
	if strings.Count(body, ">Habilitar<") != 3 || !strings.Contains(body, ">Ollama<") || !strings.Contains(body, ">Claude<") || !strings.Contains(body, ">GitHub<") {
		t.Fatal("el catálogo muestra Ollama, Claude y GitHub sin habilitar")
	}
	_, body = p.do("/panel/plugins/ollama", nil, true)
	if !strings.Contains(body, `value="https://ollama.com"`) || !strings.Contains(body, `type="password"`) {
		t.Error("el formulario trae el endpoint por defecto y el token como password")
	}
	if resp, _ := p.do("/panel/plugins/nope", nil, true); resp.StatusCode != http.StatusNotFound {
		t.Errorf("plugin desconocido: %d", resp.StatusCode)
	}

	// Un endpoint que no es un Ollama no se guarda: vuelve con lo escrito y el error.
	resp, body := p.do("/panel/plugins/ollama", url.Values{"endpoint": {"http://127.0.0.1:1"}, "token": {token}}, true)
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, "No se guardó") || !strings.Contains(body, `value="http://127.0.0.1:1"`) || strings.Contains(body, token) {
		t.Errorf("endpoint inválido: %d", resp.StatusCode)
	}

	resp, _ = p.do("/panel/plugins/ollama", url.Values{"endpoint": {ollama.URL}, "token": {token}}, true)
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/panel/plugins?ok=habilitado&p=ollama" {
		t.Fatalf("habilitar: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	_, body = p.do("/panel/plugins?ok=habilitado&p=ollama", nil, true)
	for _, want := range []string{"Ollama quedó habilitado.", ">Habilitado<", "2 modelos en " + host, "•••• WXYZ", "Probar conexión"} {
		if !strings.Contains(body, want) {
			t.Errorf("tarjeta habilitada: falta %q", want)
		}
	}
	_, form := p.do("/panel/plugins/ollama", nil, true)
	if strings.Contains(body+form, token) || !strings.Contains(form, "Déjalo vacío para conservarlo") || !strings.Contains(form, "Editar Ollama") {
		t.Error("el token nunca vuelve completo al navegador")
	}

	// Editar sin escribir el token lo conserva; probar guarda el resultado.
	if resp, _ := p.do("/panel/plugins/ollama", url.Values{"endpoint": {ollama.URL}, "token": {""}}, true); resp.StatusCode != http.StatusSeeOther {
		t.Errorf("editar: %d", resp.StatusCode)
	}
	if cfg, err := p.srv.plugins.Config(t.Context(), "ollama"); err != nil || cfg["token"] != token {
		t.Errorf("el token se conserva al editar: %v %v", cfg, err)
	}
	if resp, _ := p.do("/panel/plugins/ollama/probar", url.Values{}, true); resp.Header.Get("Location") != "/panel/plugins?ok=probado&p=ollama" {
		t.Errorf("probar: %d", resp.StatusCode)
	}
	if _, body := p.do("/panel/plugins?ok=probado&p=ollama", nil, true); !strings.Contains(body, "Ollama respondió bien.") {
		t.Error("probar con el servicio arriba avisa que respondió")
	}
	ollama.Close()
	p.do("/panel/plugins/ollama/probar", url.Values{}, true)
	if _, body := p.do("/panel/plugins?ok=probado&p=ollama", nil, true); !strings.Contains(body, `role="alert"`) || !strings.Contains(body, "Error: no se pudo conectar") {
		t.Error("probar con el servicio caído muestra el error, no un aviso de éxito")
	}

	// Deshabilitar pide confirmación y borra la configuración.
	if _, body := p.do("/panel/plugins/ollama/deshabilitar", nil, true); !strings.Contains(body, "Deshabilitar y borrar") {
		t.Error("deshabilitar pide confirmación")
	}
	if resp, _ := p.do("/panel/plugins/ollama/deshabilitar", url.Values{}, true); resp.Header.Get("Location") != "/panel/plugins?ok=deshabilitado&p=ollama" {
		t.Errorf("deshabilitar: %d", resp.StatusCode)
	}
	if _, err := p.srv.plugins.Config(t.Context(), "ollama"); err == nil {
		t.Error("deshabilitar borra la configuración")
	}
}

// Enlaces rel="me" (gremio#117): se configuran en Básico y contacto y van en el <head> de las páginas públicas.
func TestRelMe(t *testing.T) {
	p := newPanelServer(t, fichaDoc)
	if _, body := p.do("/@alice", nil, false); strings.Contains(body, `rel="me"`) {
		t.Error("sin enlaces no hay rel=me")
	}
	r := p.profile()
	base := url.Values{"name": {r.Basics.Name}, "label": {r.Basics.Label}}
	bad := url.Values{"rel_me": {"javascript:alert(1)"}}
	for k, v := range base {
		bad[k] = v
	}
	if resp, body := p.do("/panel/basico", bad, true); resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, "no es una URL https") {
		t.Errorf("una URL que no es https no se guarda: %d", resp.StatusCode)
	}
	good := url.Values{"rel_me": {"https://mastodon.social/@alice\r\n\r\nhttps://github.com/alice\n"}}
	for k, v := range base {
		good[k] = v
	}
	if resp, _ := p.do("/panel/basico", good, true); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("guardar: %d", resp.StatusCode)
	}
	if got := p.profile().Gremio.RelMe; len(got) != 2 || got[0] != "https://mastodon.social/@alice" {
		t.Errorf("relMe: %v", got)
	}
	for _, path := range []string{"/@alice", "/@alice/cv"} {
		_, body := p.do(path, nil, false)
		head := body[:strings.Index(body, "</head>")]
		if !strings.Contains(head, `<link rel="me" href="https://mastodon.social/@alice">`) || !strings.Contains(head, `<link rel="me" href="https://github.com/alice">`) {
			t.Errorf("%s: el <head> lleva los rel=me", path)
		}
	}
	if _, form := p.do("/panel/basico", nil, true); !strings.Contains(form, "https://mastodon.social/@alice\nhttps://github.com/alice</textarea>") {
		t.Error("el panel muestra los enlaces guardados, uno por línea")
	}
}

// Las notas libres se quitaron (gremio#121): ni pestaña ni formulario, aunque un perfil viejo traiga showNotes.
func TestSinNotas(t *testing.T) {
	p := newPanelServer(t, strings.Replace(fichaDoc, `"timezone": "America/Bogota",`, `"timezone": "America/Bogota", "showNotes": true,`, 1))
	if _, body := p.do("/@alice", nil, false); strings.Contains(body, "/@alice/notas") || strings.Contains(body, "Notas</a>") {
		t.Error("no hay pestaña Notas")
	}
	for _, path := range []string{"/@alice/notas", "/panel/notas"} {
		if resp, _ := p.do(path, nil, true); resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s: %d, quería 404", path, resp.StatusCode)
		}
	}
	if _, body := p.do("/panel", nil, true); strings.Contains(body, "Publicar una nota") {
		t.Error("el panel no ofrece publicar notas")
	}
}
