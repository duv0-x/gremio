package medals

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/duv0-x/gremio/internal/dojo"
	"github.com/duv0-x/gremio/internal/icons"
	"github.com/duv0-x/gremio/internal/profile"
)

var t0 = time.Date(2026, 11, 2, 15, 0, 0, 0, time.UTC) // lunes, 10:00 en Bogotá

// done es un reto terminado; n distingue el reto y el hash.
func done(n int, at time.Time, lvl dojo.Level, skills ...string) dojo.Event {
	if len(skills) == 0 {
		skills = []string{"linux-admin"}
	}
	var sl []dojo.SkillLevel
	for _, s := range skills {
		sl = append(sl, dojo.SkillLevel{Skill: s, Level: lvl})
	}
	return dojo.Event{Type: dojo.ChallengeCompleted, At: at, Challenge: fmt.Sprintf("reto-%d", n), Level: lvl,
		Skills: sl, Trust: dojo.SelfPractice, Execution: fmt.Sprintf("https://g.ejemplo.org/ejecucion/%d", n), Hash: fmt.Sprintf("h%03d", n)}
}

func ids(events []dojo.Event) []string {
	var out []string
	for _, e := range events {
		out = append(out, e.Badge.ID)
	}
	return out
}

func TestCatalogo(t *testing.T) {
	seen := map[string]bool{}
	for _, m := range Catalog {
		if seen[m.ID] || !icons.Exists(m.Icon) || !slices.Contains(profile.TraitColors, m.Color) || m.Name == "" || m.Description == "" {
			t.Errorf("medalla inválida: %+v", m)
		}
		seen[m.ID] = true
	}
}

func TestEvaluate(t *testing.T) {
	bogota, _ := time.LoadLocation("America/Bogota")
	day := 24 * time.Hour
	for _, c := range []struct {
		name   string
		events []dojo.Event
		ctx    Context
		want   []string
	}{
		{"libro vacío", nil, Context{}, nil},
		{"un N2", []dojo.Event{done(1, t0, dojo.N2)}, Context{}, []string{"cinturon-blanco"}},
		{"N4", []dojo.Event{done(1, t0, dojo.N4)}, Context{}, []string{"cinturon-blanco", "primer-n4"}},
		{"tres N1 el mismo día", []dojo.Event{done(1, t0, dojo.N1), done(2, t0.Add(time.Hour), dojo.N1), done(3, t0.Add(2*time.Hour), dojo.N1)},
			Context{}, []string{"cinturon-blanco", "triple-kata"}},
		{"tres N1 pero uno es N2", []dojo.Event{done(1, t0, dojo.N1), done(2, t0, dojo.N2), done(3, t0, dojo.N1)},
			Context{}, []string{"cinturon-blanco"}},
		// 23:30 y 00:30 UTC son el mismo día en Bogotá (UTC-5), pero no en UTC.
		{"mismo día en la zona del perfil", []dojo.Event{done(1, t0.Add(8*time.Hour+30*time.Minute), dojo.N1), done(2, t0.Add(9*time.Hour+30*time.Minute), dojo.N1), done(3, t0.Add(10*time.Hour), dojo.N1)},
			Context{Location: bogota}, []string{"cinturon-blanco", "triple-kata"}},
		{"mismo día en UTC no", []dojo.Event{done(1, t0.Add(8*time.Hour+30*time.Minute), dojo.N1), done(2, t0.Add(9*time.Hour+30*time.Minute), dojo.N1), done(3, t0.Add(10*time.Hour), dojo.N1)},
			Context{}, []string{"cinturon-blanco"}},
	} {
		got := ids(Evaluate(c.events, c.ctx))
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: %v, quería %v", c.name, got, c.want)
		}
	}

	// Racha: 7 días seguidos (con dos retos el mismo día) sí; un hueco reinicia la cuenta.
	var racha []dojo.Event
	for i := range 7 {
		racha = append(racha, done(i, t0.Add(time.Duration(i)*day), dojo.N2))
	}
	racha = append(racha, done(99, t0.Add(3*day+time.Hour), dojo.N2))
	got := Evaluate(racha, Context{})
	if !slices.Contains(ids(got), "racha-7") || !got[1].At.Equal(t0.Add(6*day)) || got[1].Proof.URL != "https://g.ejemplo.org/ejecucion/6" {
		t.Errorf("racha de 7: %+v", got)
	}
	hueco := slices.Clone(racha)
	hueco[3].At = hueco[3].At.Add(10 * day)
	hueco = hueco[:len(hueco)-1]
	if slices.Contains(ids(Evaluate(hueco, Context{})), "racha-7") {
		t.Error("un día sin retos corta la racha")
	}

	// Polímata: 5 atributos distintos; dos habilidades del mismo atributo cuentan una vez y una fuera del
	// catálogo o sin enlazar no cuenta.
	attr := map[string]string{"linux-admin": "OPS", "bash": "OPS", "sql": "DAT", "git": "DEV", "networking": "NET", "kubernetes": "K8S", "no-existe": "X"}
	poli := []dojo.Event{done(1, t0, dojo.N1, "linux-admin", "bash"), done(2, t0.Add(day), dojo.N1, "sql", "no-existe"),
		done(3, t0.Add(2*day), dojo.N1, "git"), done(4, t0.Add(3*day), dojo.N1, "networking", "terraform")}
	if slices.Contains(ids(Evaluate(poli, Context{AttrOf: attr})), "polimata") {
		t.Error("4 atributos no dan polímata")
	}
	poli = append(poli, done(5, t0.Add(4*day), dojo.N1, "kubernetes"))
	if got := Evaluate(poli, Context{AttrOf: attr}); !slices.Contains(ids(got), "polimata") || !got[len(got)-1].At.Equal(t0.Add(4*day)) {
		t.Errorf("5 atributos dan polímata el día del quinto: %+v", got)
	}
}

func TestOtorgadasNoSeRepitenNiSeRevocan(t *testing.T) {
	book := []dojo.Event{done(1, t0, dojo.N4)}
	for _, e := range Evaluate(book, Context{}) {
		book = append(book, e)
	}
	if again := Evaluate(book, Context{}); len(again) != 0 {
		t.Errorf("las medallas otorgadas no se repiten: %v", ids(again))
	}
	// Retirar el reto después deja la tombstone: la medalla sigue ganada.
	book[0] = dojo.Event{Type: dojo.ChallengeCompleted, At: t0, Challenge: "reto-1", Tombstone: true, Hash: "h001"}
	if got := Awarded(book); len(got) != 2 || got[0].ID != "cinturon-blanco" || got[1].ID != "primer-n4" {
		t.Errorf("las medallas ganadas no se pierden: %+v", got)
	}
	if again := Evaluate(book[:1], Context{}); len(again) != 0 {
		t.Errorf("un reto retirado no otorga: %v", ids(again))
	}
}
