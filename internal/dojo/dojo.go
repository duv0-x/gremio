// Package dojo es el motor del puntaje de Entrenamiento (ADR-013): el catálogo de habilidades, las reglas dojo-1 y
// el libro de eventos encadenado del que se calcula todo. El Entrenamiento nunca se guarda: se recalcula desde el
// libro con una fecha explícita, así que cualquiera puede verificarlo con "gremio dojo verify".
package dojo

import (
	"errors"
	"fmt"
	"time"
)

// Rules es la versión de las reglas con la que se calcula el Entrenamiento (ADR-013 §6).
const Rules = "dojo-1"

// Level es un nivel de reto o de banda: N1 Fundamentos a N4 Experto (ADR-013 §3).
type Level int

const (
	N1 Level = iota + 1
	N2
	N3
	N4
)

// Entry es donde empieza la banda: un reto de este nivel deja la barra al menos ahí.
func (l Level) Entry() float64 { return [...]float64{0, 0, 40, 60, 80}[l] }

// Ceiling es el techo de la banda. El 99 de N4 es asintótico: se acerca y no se toca.
func (l Level) Ceiling() float64 { return [...]float64{0, 40, 60, 80, 99}[l] }

func (l Level) Valid() bool { return l >= N1 && l <= N4 }

func (l Level) String() string { return fmt.Sprintf("N%d", int(l)) }

// Trust es la forma en que se verificó una ejecución. No multiplica la ganancia: tapa el nivel alcanzable (§4).
type Trust string

const (
	SelfPractice Trust = "self"         // fuera del catálogo revisado (incluidos los retos de IA) o sin checks reproducibles
	Reproducible Trust = "reproducible" // del catálogo revisado, entrega fijada a un commit, checks públicos
	ThirdParty   Trust = "third-party"  // del catálogo revisado, checks corridos y firmados por alguien distinto del dueño
)

// Cap es el nivel máximo que deja alcanzar la confianza.
func (t Trust) Cap() Level {
	switch t {
	case SelfPractice:
		return N2
	case Reproducible:
		return N3
	case ThirdParty:
		return N4
	}
	return 0
}

func (t Trust) Valid() bool { return t.Cap() != 0 }

// Type es el tipo de un evento del libro.
type Type string

const (
	ChallengeStarted   Type = "challengeStarted"   // empezó un reto (solo Actividad; no puntúa)
	ChallengeCompleted Type = "challengeCompleted" // terminó un reto y publicó la ejecución verificada
	BadgeAwarded       Type = "badgeAwarded"       // ganó una medalla (ADR-015; no puntúa)
	Retired            Type = "retired"            // retiró el evento Retires: queda su tombstone
)

// MaxSkills es cuántas habilidades puede ejercitar un reto como máximo (§3).
const MaxSkills = 3

// SkillLevel es el nivel de un reto en una habilidad; no puede pasar el nivel del reto.
type SkillLevel struct {
	Skill string `json:"skill"` // id del catálogo (skills.go)
	Level Level  `json:"level"`
}

// Proof es lo que respalda la verificación: el commit de la entrega y el enlace a la corrida o a la attestation.
type Proof struct {
	Commit string `json:"commit,omitempty"`
	URL    string `json:"url,omitempty"`
	User   string `json:"user,omitempty"` // usuario de la identidad externa (p. ej. GitHub, ADR-015 §3)
}

// Badge identifica la medalla de un evento badgeAwarded y la regla que la otorgó.
type Badge struct {
	ID   string `json:"id"`
	Rule string `json:"rule,omitempty"`
}

// Event es una entrada del libro. Solo entran ejecuciones publicadas de versiones públicas de un reto (§6).
// Hash es el sha256 (hex) del JSON del evento sin Hash; Prev, el hash del evento anterior en la cadena.
type Event struct {
	Type Type      `json:"type"`
	At   time.Time `json:"at"` // UTC, truncada al segundo

	// Challenge es el id del reto original: la raíz de su linaje basedOn (ADR-002 §3). Un reto suma una sola vez
	// por persona contando todas sus versiones y copias, así que todas comparten este id.
	Challenge string       `json:"challenge,omitempty"`
	Version   int          `json:"version,omitempty"`   // versión del reto que se ejecutó
	Title     string       `json:"title,omitempty"`     // título del reto, para Actividad y el detalle
	URL       string       `json:"url,omitempty"`       // página pública de esa versión del reto
	Execution string       `json:"execution,omitempty"` // página pública de la ejecución
	Level     Level        `json:"level,omitempty"`     // nivel del reto
	Skills    []SkillLevel `json:"skills,omitempty"`
	Trust     Trust        `json:"trust,omitempty"`
	Proof     *Proof       `json:"proof,omitempty"`

	Badge   *Badge `json:"badge,omitempty"`   // solo badgeAwarded
	Retires string `json:"retires,omitempty"` // solo retired: hash del evento retirado

	// Tombstone marca un evento retirado: sus datos se borraron y solo quedan tipo, fecha, reto y prev.
	// Su hash ya no se puede recalcular desde el cuerpo; la cadena sigue enlazada por prev.
	Tombstone bool `json:"tombstone,omitempty"`

	Prev string `json:"prev"`
	Hash string `json:"hash,omitempty"`
}

// Book es el libro exportado (gremio dojo export, /@usuario/dojo.json): los eventos en el orden de la cadena.
type Book struct {
	Rules  string  `json:"rules"`
	Actor  string  `json:"actor,omitempty"`
	Head   string  `json:"head"` // hash del último evento: se publica en cada actividad federada (ADR-014)
	Events []Event `json:"events"`
}

// Live son los eventos vigentes, en el mismo orden: sin tombstones ni eventos retired.
func Live(events []Event) []Event {
	var out []Event
	for _, e := range events {
		if !e.Tombstone && e.Type != Retired {
			out = append(out, e)
		}
	}
	return out
}

// ErrInvalid envuelve los errores de validación de un evento.
var ErrInvalid = errors.New("evento del dojo inválido")

// Validate revisa un evento antes de entrar al libro. Una habilidad fuera del catálogo se acepta (las reglas la
// ignoran y se recupera cuando el catálogo la agrega, §2), pero tiene que tener id.
func (e Event) Validate() error {
	bad := func(format string, a ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
	}
	if e.At.IsZero() {
		return bad("falta la fecha")
	}
	switch e.Type {
	case ChallengeStarted:
		if e.Challenge == "" {
			return bad("falta el reto")
		}
	case ChallengeCompleted:
		if e.Challenge == "" {
			return bad("falta el reto")
		}
		if !e.Level.Valid() {
			return bad("nivel del reto %d fuera de N1–N4", e.Level)
		}
		if !e.Trust.Valid() {
			return bad("sin verificación no hay ganancia: confianza %q desconocida", e.Trust)
		}
		if len(e.Skills) == 0 || len(e.Skills) > MaxSkills {
			return bad("un reto ejercita de 1 a %d habilidades, no %d", MaxSkills, len(e.Skills))
		}
		seen := map[string]bool{}
		for _, s := range e.Skills {
			if s.Skill == "" {
				return bad("habilidad sin id")
			}
			if seen[s.Skill] {
				return bad("la habilidad %s se repite", s.Skill)
			}
			seen[s.Skill] = true
			if !s.Level.Valid() || s.Level > e.Level {
				return bad("el nivel en %s (%d) tiene que estar entre N1 y el nivel del reto (%s)", s.Skill, s.Level, e.Level)
			}
		}
	case BadgeAwarded:
		if e.Badge == nil || e.Badge.ID == "" {
			return bad("falta la medalla")
		}
	case Retired:
		if e.Retires == "" {
			return bad("falta el evento retirado")
		}
	default:
		return bad("tipo %q desconocido", e.Type)
	}
	return nil
}
