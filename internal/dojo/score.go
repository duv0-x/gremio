package dojo

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
)

// Contribution es una línea del detalle de la barra (ADR-013 §7): un reto completado que ejercitó la habilidad y
// cuánto aportó (Before → After, sin redondear; After == Before si no sumó).
type Contribution struct {
	Hash      string
	At        time.Time
	Challenge string
	Title     string
	URL       string
	Execution string
	Version   int
	Level     Level // nivel del reto en esta habilidad
	Trust     Trust
	Proof     *Proof
	Before    float64
	After     float64
}

// Gain es lo que aportó, sin redondear.
func (c Contribution) Gain() float64 { return c.After - c.Before }

// Score es el Entrenamiento de una habilidad a una fecha. Con Contributions vacío no hay entrenamiento todavía:
// la ficha muestra "Sin entrenamiento todavía", no un 0.
type Score struct {
	Skill         string
	Value         float64 // estado exacto; solo se redondea al mostrar (Shown)
	Band          Level   // banda en la que cae Value
	Trust         Trust   // la confianza más alta entre los retos que sumaron
	Max           float64 // máximo histórico alcanzado
	MaxAt         time.Time
	LastPractice  time.Time // el último reto en la habilidad, sumara o no
	Counts        map[Trust]int
	Contributions []Contribution // en el orden en que se aplicaron
	Rules         string
	At            time.Time // fecha de cálculo
}

// Empty dice si la habilidad no tiene ningún reto.
func (s Score) Empty() bool { return len(s.Contributions) == 0 }

// Shown es el número que se muestra: redondeo al entero más cercano, half away from zero (§6).
func (s Score) Shown() int { return int(math.Round(s.Value)) }

// Outcome es lo que haría un reto en una habilidad si se completara ahora (el "cuánto sumaría" del panel).
// Reason explica por qué no suma; vacía si suma.
type Outcome struct {
	Skill  string
	Before float64
	After  float64
	Reason string
}

// Training calcula el Entrenamiento de una habilidad a la fecha now desde el libro (reglas dojo-1, ADR-013 §3–§5).
// Los eventos posteriores a now no cuentan. Una habilidad fuera del catálogo no tiene entrenamiento.
func Training(events []Event, skill string, now time.Time) Score {
	return replay(events, skill, now)
}

// Explain dice, por cada habilidad del reto candidato, cuánto sumaría si se completara en now y, si no suma, por qué.
func Explain(events []Event, candidate Event, now time.Time) []Outcome {
	used := firstByChallenge(events, now)
	out := make([]Outcome, 0, len(candidate.Skills))
	for _, s := range candidate.Skills {
		sc := replay(events, s.Skill, now)
		o := Outcome{Skill: s.Skill, Before: sc.Value, After: sc.Value}
		L := min(s.Level, candidate.Trust.Cap())
		first, done := used[candidate.Challenge]
		switch {
		case !knownSkill(s.Skill):
			o.Reason = fmt.Sprintf("la habilidad %q no está en el catálogo: no suma", s.Skill)
		case done && first.Tombstone:
			o.Reason = "retiraste este reto: no vuelve a sumar"
		case done:
			o.Reason = "ya hiciste este reto (o una versión o copia suya): un reto suma una sola vez"
		case !candidate.Trust.Valid():
			o.Reason = "sin verificación no hay ganancia"
		case !s.Level.Valid():
			o.Reason = "el reto no declara su nivel en esta habilidad"
		default:
			after, ok := gain(sc.Value, L)
			if ok {
				o.After = after
				break
			}
			o.Reason = fmt.Sprintf("ya superaste el techo de %s (%.0f): necesitas un %s", L, L.Ceiling(), L+1)
			if L < s.Level {
				o.Reason += fmt.Sprintf("; con esta verificación el reto cuenta como %s como máximo", L)
				if candidate.Trust == SelfPractice {
					o.Reason += ": propónlo al catálogo revisado para que cuente más"
				}
			}
		}
		out = append(out, o)
	}
	return out
}

// gain es la regla de ganancia (§3): "cada reto te lleva al menos a la entrada de su banda y cierra el 20 % de lo
// que falta hasta el techo". ok es false si actual ya llegó al techo de L.
func gain(actual float64, L Level) (float64, bool) {
	if actual >= L.Ceiling() {
		return actual, false
	}
	b := max(actual, L.Entry())
	return b + 0.2*(L.Ceiling()-b), true
}

// curve es el estado de una habilidad entre retos: v0 tras el último reto que sumó, en el día gainDay.
type curve struct {
	v0, max  float64
	gainDay  int64
	halfLife float64
}

// at aplica el decaimiento hacia el piso (§5): piso + (v₀ − piso) × 0.5^(Δt/H), con Δt en días UTC enteros.
func (c curve) at(day int64) float64 {
	floor := 0.4 * c.max
	return floor + (c.v0-floor)*math.Pow(0.5, float64(day-c.gainDay)/c.halfLife)
}

// utcDay es el número de día UTC desde el epoch.
func utcDay(t time.Time) int64 { return int64(math.Floor(float64(t.Unix()) / 86400)) }

// scoring devuelve los completados (incluidas las tombstones) hasta now, en el orden de cálculo: (día UTC, hash).
func scoring(events []Event, now time.Time) []Event {
	today := utcDay(now)
	var out []Event
	for _, e := range events {
		if e.Type == ChallengeCompleted && utcDay(e.At) <= today {
			out = append(out, e)
		}
	}
	slices.SortStableFunc(out, func(a, b Event) int {
		return cmp.Or(cmp.Compare(utcDay(a.At), utcDay(b.At)), strings.Compare(a.Hash, b.Hash))
	})
	return out
}

// firstByChallenge es la primera ejecución de cada reto en el orden de cálculo: la única que puede sumar.
// Si es una tombstone, el reto ya no suma nunca (§3, §6).
func firstByChallenge(events []Event, now time.Time) map[string]Event {
	first := map[string]Event{}
	for _, e := range scoring(events, now) {
		if _, ok := first[e.Challenge]; !ok {
			first[e.Challenge] = e
		}
	}
	return first
}

func knownSkill(id string) bool {
	_, ok := FindSkill(id)
	return ok
}

func replay(events []Event, skill string, now time.Time) Score {
	sc := Score{Skill: skill, Counts: map[Trust]int{}, Rules: Rules, At: now.UTC()}
	sk, ok := FindSkill(skill)
	if !ok {
		return sc
	}
	c := curve{halfLife: sk.Volatility.HalfLife()}
	seen := map[string]bool{}
	for _, e := range scoring(events, now) {
		if seen[e.Challenge] {
			continue
		}
		seen[e.Challenge] = true
		if e.Tombstone {
			continue
		}
		i := slices.IndexFunc(e.Skills, func(s SkillLevel) bool { return s.Skill == skill })
		if i < 0 {
			continue
		}
		L := min(e.Skills[i].Level, e.Trust.Cap())
		day := utcDay(e.At)
		before := 0.0
		if len(sc.Contributions) > 0 {
			before = c.at(day)
		}
		after, summed := gain(before, L)
		if summed {
			c.v0, c.gainDay = after, day
			if after > c.max {
				c.max, sc.MaxAt = after, e.At
			}
			if e.Trust.Cap() > sc.Trust.Cap() {
				sc.Trust = e.Trust
			}
		}
		sc.Counts[e.Trust]++
		sc.LastPractice = e.At
		sc.Contributions = append(sc.Contributions, Contribution{
			Hash: e.Hash, At: e.At, Challenge: e.Challenge, Title: e.Title, URL: e.URL, Execution: e.Execution,
			Version: e.Version, Level: e.Skills[i].Level, Trust: e.Trust, Proof: e.Proof, Before: before, After: after,
		})
	}
	if len(sc.Contributions) > 0 {
		sc.Value = c.at(utcDay(now))
	}
	sc.Max = c.max
	sc.Band = band(sc.Value)
	return sc
}

// band es la banda en la que cae un valor.
func band(v float64) Level {
	for _, l := range []Level{N1, N2, N3} {
		if v < l.Ceiling() {
			return l
		}
	}
	return N4
}
