package web

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/duv0-x/gremio/internal/dojo"
	"github.com/duv0-x/gremio/internal/store"
)

// La fila Medallas (ADR-015 §4): solo las ganadas, con cómo se ganó y la fecha; Forjador sin el plugin GitHub
// queda sin comprobar, con el borde punteado y dicho en el texto.
func TestFichaMedallas(t *testing.T) {
	dir := t.TempDir()
	ts := newTestServerIn(t, fichaDoc, dir)
	ctx := context.Background()
	st, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	l := dojo.NewLedger(st)
	at := time.Date(2026, 11, 2, 15, 0, 0, 0, time.UTC)
	for _, e := range []dojo.Event{
		{Type: dojo.BadgeAwarded, At: at, Badge: &dojo.Badge{ID: "cinturon-blanco", Rule: "medals-1"}},
		{Type: dojo.BadgeAwarded, At: at, Badge: &dojo.Badge{ID: "forjador", Rule: "medals-1"}, Proof: &dojo.Proof{User: "alice", URL: "https://api.github.com/x"}},
	} {
		if _, err := l.Append(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	_, body := get(t, ts.URL+"/@alice")
	for _, want := range []string{
		`id="medallas"`, `id="medalla-cinturon-blanco"`, "badge-gris", "badge-azul", `data-guia-paso="medallas"`,
		`data-tip="Cinturón blanco: Terminó su primer reto verificado. Ganada el 2026-11-02."`,
		"Sin comprobar: la identidad de GitHub dejó de comprobarse.", `stroke-dasharray="14 10"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("falta %q", want)
		}
	}
	if strings.Contains(body, "medalla-racha-7") {
		t.Error("solo se muestran las medallas ganadas")
	}
}
