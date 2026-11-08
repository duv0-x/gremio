package activity

import (
	"context"
	"testing"
	"time"

	"github.com/duv0-x/gremio/internal/dojo"
	"github.com/duv0-x/gremio/internal/federation"
	"github.com/duv0-x/gremio/internal/medals"
	"github.com/duv0-x/gremio/internal/store"
)

type fakePub struct {
	published []federation.Event
	retracted [][3]string
}

func (f *fakePub) PublishEvent(_ context.Context, e federation.Event) (int, error) {
	f.published = append(f.published, e)
	return 1, nil
}

func (f *fakePub) RetractEvent(_ context.Context, hash, rules, head string) (int, error) {
	f.retracted = append(f.retracted, [3]string{hash, rules, head})
	return 1, nil
}

const profileURL = "https://gremio.ejemplo.org/@alice"

func TestEventosDelLibroLleganAActividad(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	l := dojo.NewLedger(st)
	pub := &fakePub{}
	l.Subscribe(Subscriber(pub, l, profileURL))

	day := time.Date(2026, 11, 7, 10, 30, 0, 0, time.UTC)
	exec := "https://gremio.ejemplo.org/@alice/retos/k8s-operator/1"
	started, err := l.Append(ctx, dojo.Event{Type: dojo.ChallengeStarted, At: day, Challenge: "catalog:k8s-operator",
		Title: "Desplegar un operador", Level: dojo.N3, Execution: exec})
	if err != nil {
		t.Fatal(err)
	}
	done, err := l.Append(ctx, dojo.Event{
		Type: dojo.ChallengeCompleted, At: day.Add(time.Hour), Challenge: "catalog:k8s-operator", Version: 1,
		Title: "Desplegar un operador", Execution: exec, Level: dojo.N3, Trust: dojo.Reproducible,
		Skills: []dojo.SkillLevel{{Skill: "kubernetes", Level: dojo.N3}, {Skill: "go", Level: dojo.N1}},
		Proof:  &dojo.Proof{Commit: "abc123", URL: "https://ci.example/run/1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.Append(ctx, dojo.Event{Type: dojo.BadgeAwarded, At: day.Add(time.Hour), Badge: &dojo.Badge{ID: "cinturon-blanco"}}); err != nil {
		t.Fatal(err)
	}
	if len(pub.published) != 3 {
		t.Fatalf("publicados = %d", len(pub.published))
	}

	s := pub.published[0]
	if s.Hash != started.Hash || s.Head != started.Hash || s.Type != "challengeStarted" || s.Ref != exec || s.Rules != dojo.Rules ||
		s.Text != "Empezó el reto «Desplegar un operador» (N3)." || !s.At.Equal(day) {
		t.Errorf("empezado = %+v", s)
	}
	// Un N3 reproducible desde cero deja Kubernetes en 64 (ADR-013 §3); el N1 en Go cierra el 20 % hasta 40: +8.
	c := pub.published[1]
	if c.Ref != exec || c.Text != "Terminó el reto «Desplegar un operador» (N3, reproducible): +64 en Kubernetes core, +8 en Go." {
		t.Errorf("terminado = %q (ref %s)", c.Text, c.Ref)
	}
	cinturon, _ := medals.Find("cinturon-blanco")
	b := pub.published[2]
	if b.Type != "badgeAwarded" || b.Ref != profileURL+"#medalla-cinturon-blanco" || b.Text != "Ganó la medalla «Cinturón blanco»: "+cinturon.Description {
		t.Errorf("medalla = %+v", b)
	}

	r, err := l.Retire(ctx, done.Hash, day.Add(48*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(pub.retracted) != 1 || pub.retracted[0] != [3]string{done.Hash, dojo.Rules, r.Hash} {
		t.Errorf("retirados = %v", pub.retracted)
	}
	if len(pub.published) != 3 {
		t.Error("el evento retired no es actividad nueva: solo retira el Note")
	}
}

func TestNoteSinRespaldoOSinSumar(t *testing.T) {
	at := time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC)
	medal := dojo.Event{Type: dojo.BadgeAwarded, At: at, Hash: "h", Badge: &dojo.Badge{ID: "forjador"}, Proof: &dojo.Proof{URL: "https://api.github.com/repos/duv0-x/gremio/contributors"}}
	if n, err := Note(nil, medal, profileURL); err != nil || n.Ref != profileURL+"#medalla-forjador" {
		t.Errorf("una medalla se respalda con su fila en la ficha, no con la prueba: %+v, %v", n, err)
	}
	if _, err := Note(nil, dojo.Event{Type: dojo.BadgeAwarded, At: at, Hash: "h", Badge: &dojo.Badge{ID: "no-existe"}}, profileURL); err == nil {
		t.Error("una medalla fuera del catálogo no se publica con un texto inventado")
	}
	if _, err := Note(nil, dojo.Event{Type: dojo.ChallengeStarted, At: at}, profileURL); err == nil {
		t.Error("un evento sin hash no está en el libro")
	}
	// Ya por encima del techo de N2: un N2 de práctica propia no suma.
	first := dojo.Event{Type: dojo.ChallengeCompleted, At: at, Hash: "a", Challenge: "a", Level: dojo.N3, Trust: dojo.ThirdParty,
		Skills: []dojo.SkillLevel{{Skill: "kubernetes", Level: dojo.N3}}}
	second := dojo.Event{Type: dojo.ChallengeCompleted, At: at, Hash: "b", Prev: "a", Challenge: "b", Title: "Propio", Level: dojo.N2,
		Trust: dojo.SelfPractice, Skills: []dojo.SkillLevel{{Skill: "kubernetes", Level: dojo.N2}}, URL: "https://gremio.ejemplo.org/retos/b/1"}
	n, err := Note([]dojo.Event{first, second}, second, profileURL)
	if err != nil || n.Text != "Terminó el reto «Propio» (N2, práctica propia), sin sumar Entrenamiento." || n.Ref != second.URL {
		t.Errorf("sin sumar = %q (ref %s), %v", n.Text, n.Ref, err)
	}
}
