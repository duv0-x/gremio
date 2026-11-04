package profile

import "testing"

func TestPruneRefs(t *testing.T) {
	r := &Resume{
		Basics: Basics{Name: "Alice"},
		Work:   []Work{{ID: "acme", Name: "Acme", Highlights: []Highlight{{ID: "h1", Text: "x"}}}},
		Skills: []Skill{{ID: "go", Name: "Go"}},
		Gremio: Extension{
			Attributes: []Attribute{
				{Code: "COD", Name: "Código", Subs: []Sub{{Name: "Go", Score: 50}}, Skills: []string{"go", "k8s"}, Highlights: []string{"h1", "h2"}},
				{Code: "OBS", Name: "Obs", Subs: []Sub{{Name: "x", Score: 1}}},
				{Code: "CNT", Name: "Cnt", Subs: []Sub{{Name: "x", Score: 1}}},
			},
			Views: []View{{Slug: "dev", Name: "Dev", Weights: map[string]float64{"COD": 1, "IAC": 2},
				Work: []string{"acme", "viejo"}, Highlights: []string{"h2"}, Skills: []string{"k8s"}}},
		},
	}
	if err := r.Normalize(); err == nil {
		t.Fatal("con referencias rotas, Normalize debe fallar")
	}
	r.PruneRefs()
	if err := r.Normalize(); err != nil {
		t.Fatalf("tras PruneRefs el perfil es válido: %v", err)
	}
	a, v := r.Gremio.Attributes[0], r.Gremio.Views[0]
	if len(a.Skills) != 1 || len(a.Highlights) != 1 || len(v.Work) != 1 || len(v.Highlights) != 0 || len(v.Skills) != 0 || len(v.Weights) != 1 {
		t.Errorf("quedó: atributo %+v vista %+v", a, v)
	}
}
