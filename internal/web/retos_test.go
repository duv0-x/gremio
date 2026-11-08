package web

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"maps"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/duv0-x/gremio/internal/dojo"
	"github.com/duv0-x/gremio/internal/dojo/retos"
)

type tarForge struct{ body []byte }

func (f tarForge) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(bytes.NewReader(f.body)), Header: http.Header{}}, nil
}

// fakeRepo hace que las entregas se bajen de un tarball en memoria con los archivos dados.
func fakeRepo(t *testing.T, files map[string]string) {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		tw.WriteHeader(&tar.Header{Name: "repo-x/" + name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg})
		tw.Write([]byte(body))
	}
	tw.Close()
	gz.Close()
	saved := retos.Client
	retos.Client = &http.Client{Transport: tarForge{buf.Bytes()}}
	t.Cleanup(func() { retos.Client = saved })
}

func withFixtureCatalog(t *testing.T) {
	t.Helper()
	saved := retos.Catalog
	retos.Catalog = maps.Clone(saved)
	t.Cleanup(func() { retos.Catalog = saved })
	if err := retos.AddDir("../dojo/retos/testdata/catalog"); err != nil {
		t.Fatal(err)
	}
}

var execPath = regexp.MustCompile(`^/panel/dojo/ejecuciones/([a-z0-9]+)\?ok=tomado$`)

// Ciclo del DoD de gremio#124: tomar un reto revisado → entregar → verificar → terminar → publicar → evento en el
// libro → la página pública muestra la verificación.
func TestDojoCiclo(t *testing.T) {
	withFixtureCatalog(t)
	fakeRepo(t, map[string]string{"README.md": "x", "LICENSE": "MIT", "go.mod": "module x", "x_test.go": "func TestX(t *testing.T) {}"})
	p := newPanelServer(t, fichaDoc)
	if resp, _ := p.do("/panel/dojo", nil, false); resp.Header.Get("Location") != "/login" {
		t.Errorf("el dojo del panel sin sesión: %d", resp.StatusCode)
	}
	take := "/panel/dojo/catalogo/go-cli-healthcheck/tomar"
	if resp, _ := p.do(take, url.Values{}, false); resp.Header.Get("Location") != "/login" {
		t.Errorf("tomar sin sesión: %d", resp.StatusCode)
	}
	if resp, _ := p.do(take, url.Values{}, true, "Sec-Fetch-Site", "cross-site"); resp.StatusCode != http.StatusForbidden {
		t.Errorf("tomar desde otro origen: %d", resp.StatusCode)
	}

	_, body := p.do("/panel", nil, true)
	if !strings.Contains(body, `href="/panel/dojo"`) {
		t.Error("el panel enlaza al dojo")
	}
	_, body = p.do("/panel/dojo", nil, true)
	for _, want := range []string{"Kubernetes", "Reto revisado ficticio", "Revisado por revisor-ficticio", "En revisión: cuenta como práctica propia"} {
		if !strings.Contains(body, want) {
			t.Errorf("el catálogo no muestra %q", want)
		}
	}
	_, body = p.do("/panel/dojo/catalogo/qa-revisado-ficticio", nil, true)
	if !strings.Contains(body, "Cuánto sumaría") || !strings.Contains(body, "De 0 a 64") || !strings.Contains(body, "<strong>reproducible</strong>") {
		t.Errorf("el reto revisado dice cuánto sumaría:\n%s", body)
	}
	_, body = p.do("/panel/dojo/catalogo/k8s-despliegue-resiliente", nil, true)
	if !strings.Contains(body, "De 0 a 44") || !strings.Contains(body, "práctica propia") {
		t.Error("un reto en revisión tapa en N2")
	}

	resp, _ := p.do("/panel/dojo/catalogo/qa-revisado-ficticio/tomar", url.Values{}, true)
	m := execPath.FindStringSubmatch(resp.Header.Get("Location"))
	if m == nil {
		t.Fatalf("tomar: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	id := m[1]
	act := func(action string, form url.Values) (*http.Response, string) {
		return p.do("/panel/dojo/ejecuciones/"+id+"/"+action, form, true)
	}
	if resp, body := act("terminar", url.Values{}); resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, "sin verificación aprobada") {
		t.Errorf("terminar sin verificar: %d", resp.StatusCode)
	}
	if resp, body := act("entrega", url.Values{"repo": {"https://evil.example/a/b"}, "commit": {strings.Repeat("a", 40)}}); resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, "no soportado") {
		t.Errorf("repo de un forge desconocido: %d", resp.StatusCode)
	}
	sha := strings.Repeat("a", 40)
	for _, step := range []struct{ action, ok string }{
		{"entrega", "entrega"}, {"verificar", "verificado"}, {"terminar", "terminado"}, {"publicar", "publicado"},
	} {
		form := url.Values{}
		if step.action == "entrega" {
			form = url.Values{"repo": {"https://github.com/alice/x"}, "commit": {sha}}
		}
		resp, body := act(step.action, form)
		if resp.StatusCode != http.StatusSeeOther || !strings.HasSuffix(resp.Header.Get("Location"), "?ok="+step.ok) {
			t.Fatalf("%s: %d %s %s", step.action, resp.StatusCode, resp.Header.Get("Location"), body)
		}
	}
	book, err := p.srv.Dojo.List(context.Background())
	if err != nil || len(book) != 2 || book[1].Type != dojo.ChallengeCompleted || book[1].Trust != dojo.Reproducible {
		t.Fatalf("libro: %v %+v", err, book)
	}
	if book[1].Execution != "https://gremio.ejemplo.org/dojo/ejecuciones/"+id {
		t.Errorf("el evento enlaza la ejecución pública: %s", book[1].Execution)
	}
	if got := dojo.Training(book, "kubernetes", book[1].At).Shown(); got != 64 {
		t.Errorf("Training = %d, quería 64", got)
	}

	resp, body = p.do("/dojo/ejecuciones/"+id, nil, false)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, sha) || !strings.Contains(body, "Pasaron todos los checks") || strings.Contains(body, "Notas privadas") {
		t.Errorf("página pública: %d", resp.StatusCode)
	}
	if resp, _ := p.do("/dojo/catalogo/qa-revisado-ficticio/v1", nil, false); resp.StatusCode != http.StatusOK {
		t.Errorf("página pública del reto: %d", resp.StatusCode)
	}
	for _, path := range []string{"/dojo/catalogo/qa-revisado-ficticio/v2", "/dojo/catalogo/qa-revisado-ficticio/v01", "/dojo/catalogo/nope"} {
		if resp, _ := p.do(path, nil, false); resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s: %d", path, resp.StatusCode)
		}
	}

	if resp, _ := act("retirar", url.Values{}); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("retirar: %d", resp.StatusCode)
	}
	if resp, _ := p.do("/dojo/ejecuciones/"+id, nil, false); resp.StatusCode != http.StatusGone {
		t.Errorf("una ejecución retirada responde 410, no %d", resp.StatusCode)
	}
}

func TestDojoEjecucionPrivadaNoEsPublica(t *testing.T) {
	p := newPanelServer(t, fichaDoc)
	resp, _ := p.do("/panel/dojo/catalogo/go-cli-healthcheck/tomar", url.Values{}, true)
	id := strings.TrimSuffix(strings.TrimPrefix(resp.Header.Get("Location"), "/panel/dojo/ejecuciones/"), "?ok=tomado")
	if resp, _ := p.do("/dojo/ejecuciones/"+id, nil, false); resp.StatusCode != http.StatusNotFound {
		t.Errorf("una ejecución privada no tiene página pública: %d", resp.StatusCode)
	}
	_, body := p.do("/panel/dojo", nil, true)
	if !strings.Contains(body, "Hoy el máximo es 60") {
		t.Error("sin retos revisados, el panel dice que el máximo es 60")
	}
}
