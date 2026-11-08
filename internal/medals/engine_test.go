package medals

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/duv0-x/gremio/internal/dojo"
	"github.com/duv0-x/gremio/internal/instance"
	"github.com/duv0-x/gremio/internal/plugins"
	"github.com/duv0-x/gremio/internal/store"
)

func TestEngine(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := instance.Init(ctx, st, "ana"); err != nil {
		t.Fatal(err)
	}
	if _, err := instance.BindHost(ctx, st, "https://g.ejemplo.org", ""); err != nil {
		t.Fatal(err)
	}
	blog := "https://g.ejemplo.org/"
	mux := http.NewServeMux()
	mux.HandleFunc("GET /users/ana", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"login":"ana","blog":%q}`, blog)
	})
	mux.HandleFunc("GET /repos/duv0-x/gremio/contributors", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"login":"duv0-x"},{"login":"ana"}]`)
	})
	gh := httptest.NewServer(mux)
	defer gh.Close()

	// Cinco atributos enlazados al catálogo: un reto que ejercite sus habilidades da Polímata.
	doc := `{"basics":{"name":"Ana"},"gremio":{"timezone":"America/Bogota","contact":{},"attributes":[` +
		`{"code":"OPS","name":"Ops","subs":[{"name":"Linux","score":80,"dojoSkill":"linux-admin"}]},` +
		`{"code":"NET","name":"Redes","subs":[{"name":"Redes","score":70,"dojoSkill":"networking"}]},` +
		`{"code":"DAT","name":"Datos","subs":[{"name":"SQL","score":70,"dojoSkill":"sql"}]}]}}`
	if err := st.Write.UpdateProfile(ctx, doc); err != nil {
		t.Fatal(err)
	}
	if c, err := (&Engine{st: st}).context(ctx); err != nil || c.Location.String() != "America/Bogota" || c.AttrOf["sql"] != "DAT" || len(c.AttrOf) != 3 {
		t.Fatalf("context del perfil: %+v, %v", c, err)
	}

	pm := plugins.NewManager(st, plugins.GitHub{API: gh.URL, Instance: st.Read.GetInstance})
	ledger := dojo.NewLedger(st)
	en := New(st, ledger, pm)
	en.Subscribe()

	medals := func() map[string]Award {
		t.Helper()
		as, err := en.Medals(ctx)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]Award{}
		for _, a := range as {
			out[a.ID] = a
		}
		return out
	}

	// Sin plugin, CheckExternal no hace nada.
	if err := en.CheckExternal(ctx); err != nil || len(medals()) != 0 {
		t.Fatalf("sin plugin: %v, %v", err, medals())
	}

	// Un reto N4 terminado otorga dos medallas por el hook, sin loop.
	e := done(1, t0, dojo.N4)
	e.Trust, e.Hash = dojo.ThirdParty, ""
	if _, err := ledger.Append(ctx, e); err != nil {
		t.Fatal(err)
	}
	if m := medals(); len(m) != 2 || m["cinturon-blanco"].Proof.URL != e.Execution || !m["primer-n4"].At.Equal(t0) {
		t.Fatalf("medallas del dojo: %+v", m)
	}

	if _, err := pm.Enable(ctx, "github", plugins.Config{"username": "ana"}); err != nil {
		t.Fatal(err)
	}
	for range 2 { // la segunda vez no la repite
		if err := en.CheckExternal(ctx); err != nil {
			t.Fatal(err)
		}
	}
	events, _ := ledger.List(ctx)
	if len(events) != 4 {
		t.Errorf("1 reto + 3 medallas, hay %d eventos", len(events))
	}
	f := medals()[Forjador]
	if f.Proof == nil || f.Proof.User != "ana" || f.Proof.URL != gh.URL+"/repos/duv0-x/gremio/contributors?per_page=100" || f.Unverified {
		t.Fatalf("forjador: %+v", f)
	}

	// Si el blog deja de apuntar a la instancia, la medalla queda sin comprobar, pero no se borra.
	blog = "https://otro.ejemplo.org"
	if err := en.CheckExternal(ctx); err != nil {
		t.Fatal(err)
	}
	if f := medals()[Forjador]; f.ID != Forjador || !f.Unverified {
		t.Errorf("forjador tras perder la identidad: %+v", f)
	}
	if medals()["cinturon-blanco"].Unverified {
		t.Error("las medallas del dojo nunca quedan sin comprobar")
	}
}
