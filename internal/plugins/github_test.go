package plugins

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/duv0-x/gremio/internal/store"
)

func fakeGitHub(t *testing.T, blog string, contributors int) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /users/ana", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"login":"Ana","blog":%q}`, blog)
	})
	mux.HandleFunc("GET /repos/duv0-x/gremio/contributors", func(w http.ResponseWriter, r *http.Request) {
		// Página 1: contributors-1 ajenos; página 2: ana, para probar la paginación.
		var logins []string
		switch r.URL.Query().Get("page") {
		case "1":
			for i := range min(contributors, 100) {
				logins = append(logins, fmt.Sprintf(`{"login":"otro%d"}`, i))
			}
		case "2":
			if contributors > 100 {
				logins = append(logins, `{"login":"ana"}`)
			}
		}
		fmt.Fprintf(w, "[%s]", strings.Join(logins, ","))
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

func instanceAt(host string) func(context.Context) (store.Instance, error) {
	return func(context.Context) (store.Instance, error) {
		return store.Instance{Username: "ana", Host: sql.NullString{String: host, Valid: true}}, nil
	}
}

func TestGitHubCheck(t *testing.T) {
	ctx := context.Background()
	for blog, ok := range map[string]bool{
		"https://gremio.ejemplo.org":           true,
		"https://gremio.ejemplo.org/":          true,
		"gremio.ejemplo.org":                   true,
		"https://Gremio.ejemplo.org/@ana/":     true,
		"https://gremio.ejemplo.org/@otra":     false,
		"http://gremio.ejemplo.org":            false,
		"https://gremio.ejemplo.org.evil":      false,
		"https://evil.org/?gremio.ejemplo.org": false,
		"":                                     false,
	} {
		g := GitHub{API: fakeGitHub(t, blog, 1).URL, Instance: instanceAt("https://gremio.ejemplo.org")}
		got, err := g.Check(ctx, Config{"username": "ana"})
		if ok && (err != nil || got != "Identidad comprobada como Ana") {
			t.Errorf("blog %q: %q, %v", blog, got, err)
		}
		if !ok && err == nil {
			t.Errorf("blog %q no debería comprobar la identidad", blog)
		}
	}
	g := GitHub{API: fakeGitHub(t, "", 1).URL, Instance: instanceAt("https://gremio.ejemplo.org")}
	if _, err := g.Check(ctx, Config{"username": "nadie"}); err == nil || !strings.Contains(err.Error(), "no encontró") {
		t.Errorf("usuario inexistente: %v", err)
	}
}

func TestGitHubIsContributor(t *testing.T) {
	ctx := context.Background()
	for n, want := range map[int]bool{1: false, 101: true} {
		g := GitHub{API: fakeGitHub(t, "", n).URL}
		ok, proof, err := g.IsContributor(ctx, Config{"username": "ana"}, "duv0-x/gremio")
		if err != nil || ok != want || !strings.HasSuffix(proof, "/repos/duv0-x/gremio/contributors?per_page=100") {
			t.Errorf("%d contribuidores: %v, %q, %v; quería %v", n, ok, proof, err, want)
		}
	}
}
