package retos

import (
	"os"
	"testing"

	"github.com/duv0-x/gremio/internal/dojo"
)

func TestCatalog(t *testing.T) {
	if len(Catalog) == 0 {
		t.Fatal("catálogo vacío")
	}
	for slug, e := range Catalog {
		c := e.Latest()
		// Ningún reto del catálogo real está revisado todavía: la IA no revisa y no inventamos revisores.
		if c.Reviewed() {
			t.Errorf("%s figura como revisado (%s): solo con un PR revisado por otra persona", slug, c.Reviewer)
		}
		if c.Origin == "ai" && c.Author == "" {
			t.Errorf("%s: un reto de IA necesita una persona como autora", slug)
		}
	}
}

func TestAddDirFixture(t *testing.T) {
	saved := Catalog
	t.Cleanup(func() { Catalog = saved })
	Catalog = map[string]CatalogEntry{}
	for k, v := range saved {
		Catalog[k] = v
	}
	if err := AddDir("testdata/catalog"); err != nil {
		t.Fatal(err)
	}
	e, ok := Find("qa-revisado-ficticio")
	if !ok || !e.Latest().Reviewed() {
		t.Fatal("el fixture revisado no cargó")
	}
	if err := AddDir("testdata/catalog"); err == nil {
		t.Error("un slug repetido debía fallar")
	}
}

func TestTrust(t *testing.T) {
	ok := Verification{Passed: true}
	reviewed := Challenge{Author: "ana", Reviewer: "bea", Checks: make([]Check, MinCatalogChecks)}
	pending := Challenge{Author: "ana", Checks: make([]Check, MinCatalogChecks)}
	self := Challenge{Author: "ana", Reviewer: "ANA", Checks: make([]Check, MinCatalogChecks)}
	cases := []struct {
		name    string
		c       Challenge
		catalog bool
		want    dojo.Trust
	}{
		{"catálogo revisado", reviewed, true, dojo.Reproducible},
		{"catálogo en revisión", pending, true, dojo.SelfPractice},
		{"revisor = autor", self, true, dojo.SelfPractice},
		{"reto propio aunque diga revisor", reviewed, false, dojo.SelfPractice},
	}
	for _, c := range cases {
		got, err := Trust(c.c, c.catalog, ok)
		if err != nil || got != c.want {
			t.Errorf("%s: %q, %v; quería %q", c.name, got, err, c.want)
		}
	}
	if _, err := Trust(reviewed, true, Verification{}); err == nil {
		t.Error("sin verificación aprobada no hay ganancia")
	}
}

func TestLoadRejects(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(dir+"/x.v1.json", []byte(`{"id":"y","version":1}`), 0o600)
	if _, err := load(os.DirFS(dir)); err == nil {
		t.Error("id distinto del nombre del archivo debía fallar")
	}
}
