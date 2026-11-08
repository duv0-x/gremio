package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/duv0-x/gremio/internal/dojo"
	"github.com/duv0-x/gremio/internal/instance"
	"github.com/duv0-x/gremio/internal/store"
)

// gremio dojo verify recalcula el libro exportado y da el mismo resultado que la instancia (DoD de gremio#123).
func TestDojoExportVerify(t *testing.T) {
	ctx := context.Background()
	data := t.TempDir()
	st, err := store.Open(ctx, data)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := instance.Init(ctx, st, "ana"); err != nil {
		t.Fatal(err)
	}
	l := dojo.NewLedger(st)
	day := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	for i, e := range []dojo.Event{
		{Type: dojo.ChallengeCompleted, At: day, Challenge: "catalog:a", Level: dojo.N3, Trust: dojo.Reproducible,
			Skills: []dojo.SkillLevel{{Skill: "kubernetes", Level: dojo.N3}, {Skill: "terraform", Level: dojo.N2}}},
		{Type: dojo.ChallengeCompleted, At: day.AddDate(0, 2, 0), Challenge: "catalog:b", Level: dojo.N4, Trust: dojo.ThirdParty,
			Skills: []dojo.SkillLevel{{Skill: "agentic-coding", Level: dojo.N4}}},
		{Type: dojo.BadgeAwarded, At: day, Badge: &dojo.Badge{ID: "primer-reto"}},
	} {
		if _, err := l.Append(ctx, e); err != nil {
			t.Fatal(i, err)
		}
	}

	var exported bytes.Buffer
	if err := dojoExport(ctx, []string{"--data-dir", data}, &exported); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "libro.json")
	if err := os.WriteFile(path, exported.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := dojoVerify([]string{path, "--fecha", "2027-01-15"}, &out); err != nil {
		t.Fatal(err)
	}
	events, err := l.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	shown := map[string]string{}
	for _, line := range strings.Split(out.String(), "\n") {
		if f := strings.Split(line, "\t"); len(f) > 1 {
			shown[f[0]] = f[1]
		}
	}
	now := time.Date(2027, 1, 15, 0, 0, 0, 0, time.UTC)
	for _, skill := range []string{"kubernetes", "terraform", "agentic-coding"} {
		if want := strconv.Itoa(dojo.Training(events, skill, now).Shown()); shown[skill] != want {
			t.Errorf("%s: verify dio %q, la instancia %s\n%s", skill, shown[skill], want, out.String())
		}
	}
	if !strings.Contains(out.String(), "@ana") || !strings.Contains(out.String(), "3 eventos, cadena íntegra") {
		t.Fatalf("encabezado:\n%s", out.String())
	}

	// Un libro reescrito no pasa.
	tampered := strings.Replace(exported.String(), `"catalog:b"`, `"catalog:c"`, 1)
	if err := os.WriteFile(path, []byte(tampered), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := dojoVerify([]string{path}, &out); err == nil || !strings.Contains(err.Error(), "cadena") {
		t.Fatalf("un libro reescrito debe fallar: %v", err)
	}
}
