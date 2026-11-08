package web

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// Reto propio con borrador de IA: generar con un Ollama falso, revisar, publicar, tomar y ver su página pública.
func TestRetosPropios(t *testing.T) {
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			fmt.Fprint(w, `{"models":[{"name":"qwen"}]}`)
		case "/api/chat":
			fmt.Fprint(w, `{"message":{"content":"{\"title\":\"Operador de certificados\",\"author\":\"qwen\",\"level\":3,\"skills\":[{\"skill\":\"kubernetes\",\"level\":3}],\"requirements\":[\"Un CRD\"],\"definitionOfDone\":[\"Rota certificados\"],\"checks\":[{\"id\":\"crd\",\"description\":\"Tiene un CRD\",\"kind\":\"contains\",\"path\":\"**/*.yaml\",\"pattern\":\"kind: CustomResourceDefinition\"}]}"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ollama.Close()
	p := newPanelServer(t, fichaDoc)

	_, body := p.do("/panel/dojo/propios/nuevo", nil, true)
	if !strings.Contains(body, "Habilita Ollama o Claude") {
		t.Error("sin plugins de IA, el panel dice cómo habilitarlos")
	}
	p.do("/panel/plugins/ollama", url.Values{"endpoint": {ollama.URL}}, true)
	_, body = p.do("/panel/dojo/propios/nuevo", nil, true)
	if !strings.Contains(body, `action="/panel/dojo/propios/generar"`) {
		t.Fatal("con Ollama habilitado aparece el formulario de borrador con IA")
	}
	if resp, body := p.do("/panel/dojo/propios/generar", url.Values{"plugin": {"ollama"}, "model": {"qwen"}}, true); resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, "sobre qué") {
		t.Errorf("sin tema: %d", resp.StatusCode)
	}
	resp, _ := p.do("/panel/dojo/propios/generar", url.Values{"plugin": {"ollama"}, "model": {"qwen"}, "topic": {"certificados en Kubernetes"}}, true)
	loc := resp.Header.Get("Location")
	if resp.StatusCode != http.StatusSeeOther || !strings.HasSuffix(loc, "?ok=generado") {
		t.Fatalf("generar: %d %s", resp.StatusCode, loc)
	}
	edit := strings.TrimSuffix(loc, "?ok=generado")
	id := strings.TrimPrefix(edit, "/panel/dojo/propios/")
	_, body = p.do(loc, nil, true)
	for _, want := range []string{"Operador de certificados", "Borrador generado con ollama:qwen", "Publicar versión 1", "kind: CustomResourceDefinition"} {
		if !strings.Contains(body, want) {
			t.Errorf("el borrador no muestra %q", want)
		}
	}

	if resp, _ := p.do(edit+"/tomar", url.Values{}, true); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("tomar sin publicar: %d", resp.StatusCode)
	}
	// Editarlo con checks inválidos no guarda.
	form := url.Values{"title": {"Operador de certificados"}, "level": {"3"}, "skill1": {"kubernetes"}, "skill1level": {"3"},
		"requirements": {"Un CRD, con validación\nUn controller"}, "dod": {"Rota certificados"}, "checks": {"{"}}
	if resp, body := p.do(edit, form, true); resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, "no son un JSON válido") {
		t.Errorf("checks inválidos: %d", resp.StatusCode)
	}
	form.Set("checks", `[{"id":"crd","description":"Tiene un CRD","kind":"file","path":"**/crd.yaml"}]`)
	if resp, _ := p.do(edit, form, true); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("guardar: %d", resp.StatusCode)
	}
	if resp, _ := p.do(edit+"/publicar", url.Values{}, true); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("publicar: %d", resp.StatusCode)
	}
	resp, body = p.do("/dojo/retos/"+id+"/v1", nil, false)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "Un CRD, con validación") || !strings.Contains(body, "Propuesto por alice") || !strings.Contains(body, "borrador generado con ollama:qwen") {
		t.Errorf("página pública del reto propio: %d", resp.StatusCode)
	}
	resp, _ = p.do(edit+"/tomar", url.Values{}, true)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("tomar: %d", resp.StatusCode)
	}
	_, body = p.do(resp.Header.Get("Location"), nil, true)
	if !strings.Contains(body, "<strong>práctica propia</strong>") || !strings.Contains(body, "De 0 a 44") {
		t.Error("un reto propio N3 suma como práctica propia hasta N2")
	}
	_, body = p.do("/panel/dojo", nil, true)
	if !strings.Contains(body, "Borrador de IA") || !strings.Contains(body, "Publicado v1") {
		t.Error("el dojo lista los retos propios")
	}
	for _, path := range []string{"/dojo/retos/" + id + "/v2", "/dojo/retos/nope"} {
		if resp, _ := p.do(path, nil, false); resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s: %d", path, resp.StatusCode)
		}
	}
}
