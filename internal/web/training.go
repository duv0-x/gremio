package web

import (
	"cmp"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/duv0-x/gremio/internal/dojo"
	"github.com/duv0-x/gremio/internal/profile"
)

// trainingRow es la barra Entrenamiento de un sub-atributo enlazado al dojo (ADR-013 §7).
type trainingRow struct {
	Sub   profile.Sub
	Skill dojo.Skill
	dojo.Score
}

// training calcula el Entrenamiento del sub-atributo con la fecha de la página. ok es false si no está enlazado a
// una habilidad del catálogo: entonces solo tiene lo declarado.
func (p Page) training(s profile.Sub) (trainingRow, bool) {
	sk, ok := dojo.FindSkill(s.DojoSkill)
	if !ok {
		return trainingRow{}, false
	}
	return trainingRow{Sub: s, Skill: sk, Score: dojo.Training(p.Book, sk.ID, p.Now)}, true
}

// hasTraining dice si algún sub-atributo del atributo está enlazado: la tarjeta lleva entonces la leyenda.
func hasTraining(a profile.Attribute) bool {
	for _, s := range a.Subs {
		if _, ok := dojo.FindSkill(s.DojoSkill); ok {
			return true
		}
	}
	return false
}

// adr013URL explica las reglas y cómo recalcularlas con gremio dojo verify.
const adr013URL = "https://github.com/duv0-x/gremio/blob/main/docs/adr/ADR-013-dojo-y-entrenamiento.md"

func cmpOr(a, b string) string { return cmp.Or(a, b) }

func trainingAnchor(skill string) string { return "entrenamiento-" + skill }

// otherName es el nombre de la habilidad del catálogo si no coincide con el del sub-atributo (§2).
func (t trainingRow) otherName() string {
	if strings.EqualFold(strings.TrimSpace(t.Sub.Name), t.Skill.Name) {
		return ""
	}
	return t.Skill.Name
}

func trustLabel(t dojo.Trust) string {
	switch t {
	case dojo.SelfPractice:
		return "práctica propia"
	case dojo.Reproducible:
		return "reproducible"
	case dojo.ThirdParty:
		return "verificado por terceros"
	}
	return ""
}

func volatilityLabel(v dojo.Volatility) string {
	switch v {
	case dojo.Stable:
		return "Estable"
	case dojo.Medium:
		return "Media"
	case dojo.Volatile:
		return "Volátil"
	}
	return ""
}

// volatilityWord es volatilityLabel dentro de una frase.
func volatilityWord(v dojo.Volatility) string { return strings.ToLower(volatilityLabel(v)) }

// halfLifeLabel dice la vida media en años o meses, que es como la escribe ADR-013 §2.
func halfLifeLabel(v dojo.Volatility) string {
	d := v.HalfLife()
	if d >= 365 {
		y := int(d/365.25 + 0.5)
		return itoa(y) + " " + plural(y, "año", "años")
	}
	m := int(d/30.44 + 0.5)
	return itoa(m) + " " + plural(m, "mes", "meses")
}

var meses = [...]string{"ene", "feb", "mar", "abr", "may", "jun", "jul", "ago", "sep", "oct", "nov", "dic"}

// monthYear es una fecha aproximada, "nov 2026": la ficha no muestra días para no exponer pausas (ADR-013 §9).
func monthYear(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	t = t.UTC()
	return meses[t.Month()-1] + " " + strconv.Itoa(t.Year())
}

func day(t time.Time) string { return t.UTC().Format("2006-01-02") }

// trainingTip es el tooltip de la barra: última práctica, retos por confianza, máximo y volatilidad (§7).
func (t trainingRow) tip() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Entrenamiento · %s: ", t.Skill.Name)
	if t.Empty() {
		b.WriteString("sin entrenamiento todavía. ")
	} else {
		fmt.Fprintf(&b, "última práctica %s. ", monthYear(t.LastPractice))
		var parts []string
		for _, tr := range []dojo.Trust{dojo.ThirdParty, dojo.Reproducible, dojo.SelfPractice} {
			if n := t.Counts[tr]; n > 0 {
				parts = append(parts, fmt.Sprintf("%d %s", n, trustLabel(tr)))
			}
		}
		if len(parts) > 0 {
			fmt.Fprintf(&b, "Retos: %s. ", strings.Join(parts, ", "))
		}
		fmt.Fprintf(&b, "Máximo %d (%s). ", round(t.Max), monthYear(t.MaxAt))
	}
	fmt.Fprintf(&b, "Volatilidad %s: vida media de %s.", volatilityWord(t.Skill.Volatility), halfLifeLabel(t.Skill.Volatility))
	return b.String()
}

// round redondea como dojo.Score.Shown: al entero más cercano, half away from zero.
func round(v float64) int { return dojo.Score{Value: v}.Shown() }

// gainLabel es cuánto aportó un evento, ya redondeado: "+24" o "no sumó".
func gainLabel(c dojo.Contribution) string {
	if c.Gain() <= 0 {
		return "no sumó"
	}
	if g := round(c.After) - round(c.Before); g > 0 {
		return "+" + itoa(g)
	}
	return "+<1"
}

// dojoExecutions son los retos terminados y vigentes del libro, para el Portafolio.
func dojoExecutions(events []dojo.Event) []execution {
	retired := map[string]bool{}
	for _, e := range events {
		if e.Type == dojo.Retired {
			retired[e.Retires] = true
		}
	}
	var out []execution
	for _, e := range events {
		// shortcut: filtro propio de vigentes; pasar a dojo.Live cuando llegue con las reglas (gremio#123).
		if e.Type != dojo.ChallengeCompleted || e.Tombstone || retired[e.Hash] || e.Execution == "" {
			continue
		}
		out = append(out, execution{Date: day(e.At), Title: e.Title, URL: e.Execution, Level: e.Level.String(), Trust: trustLabel(e.Trust), Version: e.Version})
	}
	return out
}
