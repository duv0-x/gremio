// Package medals es el motor de medallas (ADR-015): insignias ganadas que registran un hecho puntual y no se
// pierden. Lee el mismo libro de eventos del dojo y otorga cada medalla una sola vez, como un evento badgeAwarded
// en ese libro. Las reglas son deterministas: el mismo libro (y la misma zona horaria) da las mismas medallas.
package medals

import (
	"slices"
	"time"

	"github.com/duv0-x/gremio/internal/dojo"
)

// Rules es la versión de las reglas de medallas; va en Badge.Rule de cada medalla otorgada.
const Rules = "medals-1"

// Medal es una entrada del catálogo. El id es estable: no se renombra ni se borra.
type Medal struct {
	ID, Name, Description string
	Icon                  string // nombre de Lucide
	Color                 string // de profile.TraitColors
	External              bool   // su regla consulta un servicio externo con una identidad demostrada
}

// Catalog es el catálogo de medallas (ADR-015 §2), en el orden en que se muestran.
var Catalog = []Medal{
	{ID: "cinturon-blanco", Name: "Cinturón blanco", Description: "Terminó su primer reto verificado.", Icon: "award", Color: "gris"},
	{ID: "triple-kata", Name: "Triple kata", Description: "Terminó 3 retos N1 verificados el mismo día.", Icon: "swords", Color: "verde"},
	{ID: "primer-n4", Name: "Contra el maestro", Description: "Terminó su primer reto N4 verificado.", Icon: "mountain", Color: "rojo"},
	{ID: "racha-7", Name: "Siete amaneceres", Description: "Terminó retos verificados 7 días seguidos.", Icon: "sunrise", Color: "ambar"},
	{ID: "polimata", Name: "Polímata", Description: "Sumó Entrenamiento en habilidades de 5 atributos distintos.", Icon: "shapes", Color: "violeta"},
	{ID: Forjador, Name: "Forjador del Gremio", Description: "Es contribuidor comprobado de duv0-x/gremio en GitHub.", Icon: "hammer", Color: "azul", External: true},
}

// Forjador es la medalla externa de contribuidor del repo de Gremio.
const Forjador = "forjador"

// Find busca una medalla del catálogo por id.
func Find(id string) (Medal, bool) {
	i := slices.IndexFunc(Catalog, func(m Medal) bool { return m.ID == id })
	if i < 0 {
		return Medal{}, false
	}
	return Catalog[i], true
}

// Context es lo que las reglas necesitan del perfil.
type Context struct {
	Location *time.Location    // zona del perfil para "el mismo día"; nil es UTC
	AttrOf   map[string]string // habilidad del catálogo → código del atributo cuyo sub-atributo la enlaza (dojoSkill)
}

// Award es una medalla ganada, leída del libro.
type Award struct {
	Medal
	At    time.Time
	Proof *dojo.Proof
	Hash  string // del evento badgeAwarded
	// Unverified marca una medalla externa cuya identidad dejó de comprobarse ("sin comprobar"); la llena Medals.
	Unverified bool
}

// Awarded devuelve las medallas ganadas del libro, en orden de catálogo. Una medalla fuera del catálogo se ignora.
func Awarded(events []dojo.Event) []Award {
	byID := map[string]Award{}
	for _, e := range events {
		if e.Type != dojo.BadgeAwarded || e.Tombstone || e.Badge == nil {
			continue
		}
		m, ok := Find(e.Badge.ID)
		if _, dup := byID[m.ID]; !ok || dup {
			continue
		}
		byID[m.ID] = Award{Medal: m, At: e.At, Proof: e.Proof, Hash: e.Hash}
	}
	var out []Award
	for _, m := range Catalog {
		if a, ok := byID[m.ID]; ok {
			out = append(out, a)
		}
	}
	return out
}

// Evaluate devuelve los eventos badgeAwarded de las medallas del dojo que el libro ya cumple y que todavía no
// están otorgadas. Cada uno lleva la fecha y la ejecución del reto que cumplió la condición. Las medallas
// ganadas no se revocan: retirar un reto después no las quita, pero un reto retirado ya no cuenta para otorgar.
func Evaluate(events []dojo.Event, c Context) []dojo.Event {
	have := map[string]bool{}
	for _, a := range Awarded(events) {
		have[a.ID] = true
	}
	done := completed(events)
	var out []dojo.Event
	for _, r := range []struct {
		id   string
		rule func([]dojo.Event, Context) *dojo.Event
	}{
		{"cinturon-blanco", firstWhere(func(dojo.Event) bool { return true })},
		{"triple-kata", tripleKata},
		{"primer-n4", firstWhere(func(e dojo.Event) bool { return e.Level == dojo.N4 })},
		{"racha-7", streak7},
		{"polimata", polymath},
	} {
		if have[r.id] {
			continue
		}
		if e := r.rule(done, c); e != nil {
			out = append(out, award(r.id, e.At, &dojo.Proof{URL: e.Execution}))
		}
	}
	return out
}

func award(id string, at time.Time, proof *dojo.Proof) dojo.Event {
	return dojo.Event{Type: dojo.BadgeAwarded, At: at, Badge: &dojo.Badge{ID: id, Rule: Rules}, Proof: proof}
}

// completed son los retos terminados y no retirados, ordenados por (fecha, hash) como el motor (ADR-013 §6).
// Todo challengeCompleted del libro está verificado: sin verificación no entra.
func completed(events []dojo.Event) []dojo.Event {
	var out []dojo.Event
	for _, e := range events {
		if e.Type == dojo.ChallengeCompleted && !e.Tombstone {
			out = append(out, e)
		}
	}
	slices.SortStableFunc(out, func(a, b dojo.Event) int {
		if c := a.At.Compare(b.At); c != 0 {
			return c
		}
		return compareStrings(a.Hash, b.Hash)
	})
	return out
}

func compareStrings(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func firstWhere(ok func(dojo.Event) bool) func([]dojo.Event, Context) *dojo.Event {
	return func(done []dojo.Event, _ Context) *dojo.Event {
		for i := range done {
			if ok(done[i]) {
				return &done[i]
			}
		}
		return nil
	}
}

// day es el día civil del evento en la zona del perfil.
func day(e dojo.Event, c Context) time.Time {
	loc := c.Location
	if loc == nil {
		loc = time.UTC
	}
	y, m, d := e.At.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func tripleKata(done []dojo.Event, c Context) *dojo.Event {
	perDay := map[time.Time]int{}
	for i, e := range done {
		if e.Level != dojo.N1 {
			continue
		}
		if perDay[day(e, c)]++; perDay[day(e, c)] == 3 {
			return &done[i]
		}
	}
	return nil
}

func streak7(done []dojo.Event, c Context) *dojo.Event {
	var last time.Time
	run := 0
	for i, e := range done {
		d := day(e, c)
		switch {
		case d.Equal(last):
			continue
		case !last.IsZero() && d.Equal(last.AddDate(0, 0, 1)):
			run++
		default:
			run = 1
		}
		last = d
		if run == 7 {
			return &done[i]
		}
	}
	return nil
}

// polymath cuenta los atributos con alguna habilidad del catálogo ejercitada. El primer reto en una habilidad
// siempre suma (la barra empieza en 0 y todo techo es al menos 40), así que "sumó Entrenamiento" es "la ejercitó".
func polymath(done []dojo.Event, c Context) *dojo.Event {
	attrs := map[string]bool{}
	for i, e := range done {
		for _, s := range e.Skills {
			if _, ok := dojo.FindSkill(s.Skill); !ok {
				continue
			}
			if a := c.AttrOf[s.Skill]; a != "" && !attrs[a] {
				if attrs[a] = true; len(attrs) == 5 {
					return &done[i]
				}
			}
		}
	}
	return nil
}
