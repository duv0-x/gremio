package dojo

import (
	"encoding/json"
	"flag"
	"math"
	"os"
	"strings"
	"testing"
	"time"
)

var update = flag.Bool("update", false, "sella prev y hash de los eventos de testdata/vectors.json")

type vectorFile struct {
	Rules   string   `json:"rules"`
	Vectors []vector `json:"vectors"`
}

type vector struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	At          time.Time `json:"at"`
	Events      []Event   `json:"events"`
	Expect      map[string]*struct {
		Value float64 `json:"value"`
		Shown int     `json:"shown"`
	} `json:"expect"`
}

// chain encadena los eventos en orden, como lo haría Append (las tombstones también se sellan: en un libro real su
// hash sería el del evento original).
func chain(events []Event) {
	prev := ""
	for i := range events {
		events[i].Prev = prev
		_, h, _ := seal(events[i])
		events[i].Hash, prev = h, h
	}
}

// TestVectors corre los test vectors publicados en testdata/vectors.json. Los valores esperados se calcularon con una
// implementación independiente de las reglas (Python), no con este paquete.
func TestVectors(t *testing.T) {
	const path = "testdata/vectors.json"
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var f vectorFile
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	if f.Rules != Rules {
		t.Fatalf("los vectors son de %s, las reglas de %s", f.Rules, Rules)
	}
	if *update {
		for i := range f.Vectors {
			chain(f.Vectors[i].Events)
		}
		out, _ := json.MarshalIndent(f, "", "  ")
		if err := os.WriteFile(path, append(out, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, v := range f.Vectors {
		t.Run(v.Name, func(t *testing.T) {
			if err := VerifyChain(v.Events); err != nil {
				t.Fatalf("%v (corre go test -run TestVectors -update)", err)
			}
			for skill, want := range v.Expect {
				got := Training(v.Events, skill, v.At)
				switch {
				case want == nil && !got.Empty():
					t.Errorf("%s: esperaba sin entrenamiento, dio %v", skill, got.Value)
				case want == nil:
				case got.Empty():
					t.Errorf("%s: esperaba %v, dio sin entrenamiento", skill, want.Value)
				case math.Abs(got.Value-want.Value) > 1e-9 || got.Shown() != want.Shown:
					t.Errorf("%s: dio %v (%d), esperaba %v (%d)", skill, got.Value, got.Shown(), want.Value, want.Shown)
				}
			}
		})
	}
}

var day0 = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func ev(day int, challenge string, level Level, trust Trust) Event {
	return Event{Type: ChallengeCompleted, At: day0.AddDate(0, 0, day), Challenge: challenge, Level: level,
		Skills: []SkillLevel{{Skill: "kubernetes", Level: level}}, Trust: trust}
}

// mixed es un libro con niveles y confianzas variadas en días distintos.
func mixed() []Event {
	return []Event{
		ev(0, "a", N2, SelfPractice), ev(40, "b", N3, Reproducible), ev(90, "c", N1, Reproducible),
		ev(200, "d", N4, ThirdParty), ev(500, "e", N3, Reproducible), ev(520, "f", N2, Reproducible),
		ev(900, "g", N4, ThirdParty), ev(1300, "h", N3, SelfPractice),
	}
}

// El orden de los mismos retos casi no cambia el resultado (S5): hacer los mismos retos de menor a mayor nivel o de
// mayor a menor, en las mismas fechas, deja la barra a menos de 3 puntos.
func TestOrder(t *testing.T) {
	asc := []Event{ev(0, "a", N1, ThirdParty), ev(10, "b", N2, ThirdParty), ev(20, "c", N3, ThirdParty), ev(30, "d", N4, ThirdParty)}
	desc := []Event{ev(0, "d", N4, ThirdParty), ev(10, "c", N3, ThirdParty), ev(20, "b", N2, ThirdParty), ev(30, "a", N1, ThirdParty)}
	now := day0.AddDate(0, 0, 30)
	a, d := Training(asc, "kubernetes", now), Training(desc, "kubernetes", now)
	if math.Abs(a.Value-d.Value) >= 3 {
		t.Fatalf("el orden cambia demasiado: %v vs %v", a.Value, d.Value)
	}
}

// Retirar solo puede bajar el puntaje: quitar cualquier evento, en cualquier fecha de cálculo, nunca lo sube.
func TestRetireNeverRaises(t *testing.T) {
	book := mixed()
	chain(book)
	for days := 0; days <= 2000; days += 50 {
		now := day0.AddDate(0, 0, days)
		full := Training(book, "kubernetes", now).Value
		for i := range book {
			retired := append([]Event{}, book...)
			retired[i] = Event{Type: ChallengeCompleted, At: book[i].At, Challenge: book[i].Challenge, Tombstone: true, Hash: book[i].Hash}
			if got := Training(retired, "kubernetes", now).Value; got > full+1e-9 {
				t.Fatalf("día %d: retirar %s sube de %v a %v", days, book[i].Challenge, full, got)
			}
		}
	}
}

// Esperar a que decaiga nunca conviene: el valor después de un reto crece con el valor de antes.
func TestWaitingNeverPays(t *testing.T) {
	base := []Event{ev(0, "a", N3, Reproducible)}
	prev := math.Inf(1)
	for wait := 0; wait <= 1500; wait += 30 {
		after := Training(append(base, ev(wait, "b", N3, Reproducible)), "kubernetes", day0.AddDate(0, 0, wait)).Value
		if after > prev+1e-9 {
			t.Fatalf("esperar %d días deja %v, más que esperar menos (%v)", wait, after, prev)
		}
		prev = after
	}
}

func TestScoreDetails(t *testing.T) {
	book := mixed()
	chain(book)
	now := day0.AddDate(0, 0, 1400)
	s := Training(book, "kubernetes", now)
	if len(s.Contributions) != len(book) || s.Counts[ThirdParty] != 2 || s.Counts[SelfPractice] != 2 || s.Counts[Reproducible] != 4 {
		t.Fatalf("detalle: %d contribuciones, counts %v", len(s.Contributions), s.Counts)
	}
	if s.Trust != ThirdParty || !s.LastPractice.Equal(book[7].At) || s.Max < s.Value || s.Band != band(s.Value) {
		t.Fatalf("score: %+v", s)
	}
	// El N1 del día 90 llega cuando la barra ya pasó 40: no suma y no reinicia la curva.
	if c := s.Contributions[2]; c.Gain() != 0 || c.Challenge != "c" {
		t.Fatalf("el N1 no debía sumar: %+v", c)
	}
	if !Training(book, "terraform", now).Empty() || !Training(book, "no-existe", now).Empty() {
		t.Fatal("una habilidad sin retos o fuera del catálogo no tiene entrenamiento")
	}
	if !Training(book, "kubernetes", day0.AddDate(0, 0, -1)).Empty() {
		t.Fatal("los eventos posteriores a la fecha de cálculo no cuentan")
	}
}

func TestExplain(t *testing.T) {
	// Un N3 reproducible deja la barra en 64, por encima del techo de N2.
	book := []Event{ev(0, "a", N3, Reproducible), {Type: ChallengeCompleted, At: day0, Challenge: "r", Tombstone: true}}
	chain(book)
	now := day0

	cases := []struct {
		name   string
		e      Event
		reason string
		after  float64
	}{
		{"suma", ev(0, "nuevo", N3, Reproducible), "", 64 + 0.2*16},
		{"techo de N2", ev(0, "nuevo", N2, Reproducible), "ya superaste el techo de N2 (60): necesitas un N3", 64},
		{"propio tapado", ev(0, "nuevo", N3, SelfPractice), "propónlo al catálogo", 64},
		{"ya hecho", ev(0, "a", N3, Reproducible), "una sola vez", 64},
		{"retirado", ev(0, "r", N3, Reproducible), "retiraste", 64},
		{"sin verificación", ev(0, "nuevo", N3, ""), "sin verificación", 64},
	}
	for _, c := range cases {
		o := Explain(book, c.e, now)
		if len(o) != 1 || !strings.Contains(o[0].Reason, c.reason) || (c.reason == "") != (o[0].Reason == "") ||
			o[0].Before != 64 || math.Abs(o[0].After-c.after) > 1e-9 {
			t.Errorf("%s: %+v", c.name, o)
		}
	}
}
