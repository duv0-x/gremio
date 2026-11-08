package dojo

import (
	"math"
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

// Training calcula el Entrenamiento de una habilidad a la fecha now desde el libro.
func Training(events []Event, skill string, now time.Time) Score {
	// TODO(gremio#123): reglas dojo-1 (§3–§5). Mientras tanto ninguna habilidad tiene entrenamiento.
	return Score{Skill: skill, Counts: map[Trust]int{}, Rules: Rules, At: now.UTC()}
}

// Explain dice, por cada habilidad del reto candidato, cuánto sumaría si se completara en now y, si no suma, por qué.
func Explain(events []Event, candidate Event, now time.Time) []Outcome {
	// TODO(gremio#123): reglas dojo-1.
	out := make([]Outcome, 0, len(candidate.Skills))
	for _, s := range candidate.Skills {
		out = append(out, Outcome{Skill: s.Skill, Reason: "las reglas dojo-1 todavía no están implementadas"})
	}
	return out
}
