// Package activity convierte los eventos públicos del libro del dojo en los Notes de la pestaña Actividad
// (ADR-014): arma el texto, corto y factual, y los publica o los retira por federation.
package activity

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"

	"github.com/duv0-x/gremio/internal/dojo"
	"github.com/duv0-x/gremio/internal/federation"
)

// Publisher es lo que usa Actividad de federation.Server.
type Publisher interface {
	PublishEvent(ctx context.Context, e federation.Event) (int, error)
	RetractEvent(ctx context.Context, hash, rules, head string) (int, error)
}

// Subscriber publica cada evento que entra al libro y retira el Note de cada evento retirado. Va registrado en el
// único dojo.Ledger del servidor. profileURL es https://<host>/@usuario: el respaldo de una medalla es su
// fila en la ficha.
func Subscriber(pub Publisher, l *dojo.Ledger, profileURL string) dojo.Subscriber {
	return func(ctx context.Context, e dojo.Event) {
		var err error
		if e.Type == dojo.Retired {
			// La cabeza nueva es el propio evento retired: acaba de entrar al libro.
			_, err = pub.RetractEvent(ctx, e.Retires, dojo.Rules, e.Hash)
		} else {
			var events []dojo.Event
			if events, err = l.List(ctx); err == nil {
				var n federation.Event
				if n, err = Note(events, e, profileURL); err == nil {
					_, err = pub.PublishEvent(ctx, n)
				}
			}
		}
		if err != nil {
			// El evento ya está en el libro; solo falta su Note.
			slog.Error("actividad: no se pudo federar el evento", "hash", e.Hash, "type", e.Type, "err", err)
		}
	}
}

// Note arma el Note de un evento del libro. events es el libro (o un prefijo que incluye e): lo que sumó un reto
// se calcula con lo que había hasta ese evento, en su fecha. La cabeza que se publica es el propio evento, que es
// la cabeza del libro cuando entra.
func Note(events []dojo.Event, e dojo.Event, profileURL string) (federation.Event, error) {
	if e.Hash == "" {
		return federation.Event{}, errors.New("evento sin hash: todavía no está en el libro")
	}
	n := federation.Event{Hash: e.Hash, Type: string(e.Type), Rules: dojo.Rules, Head: e.Hash, At: e.At}
	switch e.Type {
	case dojo.ChallengeStarted:
		n.Ref, n.Text = challengeRef(e), "Empezó el reto «"+title(e)+"»"
		if e.Level.Valid() {
			n.Text += " (" + e.Level.String() + ")"
		}
		n.Text += "."
	case dojo.ChallengeCompleted:
		n.Ref = challengeRef(e)
		n.Text = fmt.Sprintf("Terminó el reto «%s» (%s, %s)", title(e), e.Level, trustLabel(e.Trust))
		if gains := gains(events, e); len(gains) > 0 {
			n.Text += ": " + strings.Join(gains, ", ") + "."
		} else {
			n.Text += ", sin sumar Entrenamiento."
		}
	case dojo.BadgeAwarded:
		n.Ref = profileURL + "#medalla-" + e.Badge.ID
		if e.Proof != nil && e.Proof.URL != "" {
			n.Ref = e.Proof.URL
		}
		// shortcut: el texto usa el id de la medalla; usar medals.Find (nombre y descripción) cuando internal/medals esté en main.
		n.Text = "Ganó la medalla «" + e.Badge.ID + "»."
	default:
		return federation.Event{}, fmt.Errorf("el evento %s no va a Actividad", e.Type)
	}
	return n, nil
}

func title(e dojo.Event) string {
	if e.Title != "" {
		return e.Title
	}
	return e.Challenge
}

// challengeRef es lo que respalda un evento de reto: la ejecución con su evidencia o, si no hay, la página del reto.
func challengeRef(e dojo.Event) string {
	if e.Execution != "" {
		return e.Execution
	}
	return e.URL
}

// trustLabel es la confianza con texto, como en la barra de la ficha (ADR-013 §7).
func trustLabel(t dojo.Trust) string {
	switch t {
	case dojo.ThirdParty:
		return "verificado por terceros"
	case dojo.Reproducible:
		return "reproducible"
	}
	return "práctica propia"
}

// gains es lo que sumó el reto en cada habilidad, como "+7 en Kubernetes": la diferencia entre los números que
// se muestran antes y después, con las reglas en la fecha del evento. Las habilidades en las que no sumó no salen.
func gains(events []dojo.Event, e dojo.Event) []string {
	for i, ev := range events {
		if ev.Hash == e.Hash {
			events = events[:i+1]
			break
		}
	}
	var out []string
	for _, s := range e.Skills {
		for _, c := range dojo.Training(events, s.Skill, e.At).Contributions {
			if c.Hash != e.Hash {
				continue
			}
			if d := int(math.Round(c.After) - math.Round(c.Before)); d > 0 {
				name := s.Skill
				if sk, ok := dojo.FindSkill(s.Skill); ok {
					name = sk.Name
				}
				out = append(out, fmt.Sprintf("+%d en %s", d, name))
			}
		}
	}
	return out
}
