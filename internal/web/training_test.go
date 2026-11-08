package web

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/duv0-x/gremio/internal/dojo"
	"github.com/duv0-x/gremio/internal/profile"
)

func TestFichaSinEntrenamiento(t *testing.T) {
	doc := strings.Replace(fichaDoc, `{"name": "Kubernetes", "score": 92}`, `{"name": "Kubernetes", "score": 92, "dojoSkill": "kubernetes"}`, 1)
	ts := newTestServer(t, doc)
	_, body := get(t, ts.URL+"/@alice")
	for _, want := range []string{"Declarado", "Entrenamiento", "Sin entrenamiento todavía", "Volatilidad media: vida media de 2 años."} {
		if !strings.Contains(body, want) {
			t.Errorf("ficha: falta %q", want)
		}
	}
	// Sin retos no hay número ni detalle con eventos.
	if strings.Contains(body, `id="entrenamiento-kubernetes"`) {
		t.Error("sin retos no debe haber detalle de entrenamiento")
	}
}

func TestBarraYDetalleDeEntrenamiento(t *testing.T) {
	at := time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)
	sk, _ := dojo.FindSkill("aws-core")
	row := trainingRow{Sub: profile.Sub{Name: "AWS", DojoSkill: "aws-core"}, Skill: sk, Score: dojo.Score{
		Skill: "aws-core", Value: 52.5, Band: dojo.N2, Trust: dojo.SelfPractice, Max: 52.5, MaxAt: at, LastPractice: at,
		Counts: map[dojo.Trust]int{dojo.SelfPractice: 2}, Rules: dojo.Rules, At: time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC),
		Contributions: []dojo.Contribution{
			{At: at, Title: "VPC con IaC", URL: "https://ej.org/retos/vpc", Execution: "https://ej.org/ejecuciones/1", Version: 2, Level: dojo.N2, Trust: dojo.SelfPractice, Before: 40, After: 44},
			{At: at, Title: "Repetido", Level: dojo.N1, Trust: dojo.SelfPractice, Before: 44, After: 44},
		},
	}}
	var b bytes.Buffer
	p := Page{Username: "alice"}
	_ = trainingBar(row, true).Render(context.Background(), &b)
	_ = trainingDetail(p, profile.Attribute{Code: "NUB"}, row).Render(context.Background(), &b)
	html := b.String()
	for _, want := range []string{
		"53 · N2", // 52.5 redondea hacia arriba (half away from zero)
		"· AWS core", "práctica propia · última práctica mar 2026", "Retos: 2 práctica propia.", "Máximo 53 (mar 2026).",
		`href="#entrenamiento-aws-core"`, `href="#atributo-nub"`, "+4", "no sumó", "v2", "ejecución ↗",
		"Reglas dojo-1 · calculado el 2026-11-07", `href="/@alice/dojo.json"`, "cómo recalcular",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("falta %q", want)
		}
	}
	for _, bad := range []string{"perdiendo", "ranking", "percentil"} {
		if strings.Contains(html, bad) {
			t.Errorf("texto no neutral: %q", bad)
		}
	}
}

func TestDojoExecutions(t *testing.T) {
	at := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	got := dojoExecutions([]dojo.Event{
		{Type: dojo.ChallengeCompleted, At: at, Title: "vigente", Execution: "https://x/1", Level: dojo.N3, Trust: dojo.Reproducible, Hash: "a"},
		{Type: dojo.ChallengeCompleted, At: at, Title: "retirado", Execution: "https://x/2", Hash: "b"},
		{Type: dojo.Retired, At: at, Retires: "b"},
		{Type: dojo.ChallengeCompleted, Tombstone: true, Hash: "c"},
		{Type: dojo.ChallengeStarted, At: at, Title: "empezado", Hash: "d"},
	})
	if len(got) != 1 || got[0].Title != "vigente" || got[0].Date != "2026-05-01" || got[0].Level != "N3" || got[0].Trust != "reproducible" {
		t.Fatalf("dojoExecutions = %+v", got)
	}
}
