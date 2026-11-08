package federation

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/duv0-x/gremio/internal/store"
)

// pending devuelve la actividad de la única entrega pendiente.
func (ti *testInstance) pending(t *testing.T) map[string]any {
	t.Helper()
	due, err := ti.st.Read.DueDeliveries(context.Background(), store.DueDeliveriesParams{NextAttemptAt: "9999", Limit: 2})
	if err != nil || len(due) != 1 {
		t.Fatalf("entregas pendientes = %d, %v", len(due), err)
	}
	var act map[string]any
	if err := json.Unmarshal([]byte(due[0].Activity), &act); err != nil {
		t.Fatal(err)
	}
	return act
}

func TestEventoSePublicaConPropiedadesDeGremioYSeRetira(t *testing.T) {
	a, b := newTestInstance(t, "alice"), newTestInstance(t, "bob")
	b.follow(t, a, 1)
	waitFor(t, "Accept en el inbox de bob", func() bool { return b.inboxes.Load() == 1 })
	b.status.Store(503) // la entrega queda en la cola para revisar su cuerpo
	ctx := context.Background()

	at := time.Date(2026, 11, 1, 15, 4, 5, 0, time.UTC)
	e := Event{
		Hash: "abc123", Type: "challengeCompleted", Ref: "https://alice.example/retos/k8s/1", Rules: "dojo-1",
		Head: "abc123", Text: "Terminó el reto <Operador> (N3, reproducible): +7 en Kubernetes.", At: at,
	}
	if queued, err := a.PublishEvent(ctx, e); err != nil || queued != 1 {
		t.Fatalf("PublishEvent = %d, %v", queued, err)
	}
	if queued, err := a.PublishEvent(ctx, e); err != nil || queued != 0 {
		t.Fatalf("publicar dos veces el mismo evento = %d, %v; no debe encolar de nuevo", queued, err)
	}

	create := a.pending(t)
	ctxJSON, _ := json.Marshal(create["@context"])
	if !strings.Contains(string(ctxJSON), `"gremio":"https://w3id.org/gremio/ns/v1#"`) {
		t.Errorf("@context = %s", ctxJSON)
	}
	note, _ := create["object"].(map[string]any)
	id := a.url + "/notes/abc123"
	want := map[string]any{
		"id": id, "type": "Note", "published": "2026-11-01T15:04:05.000Z", "gremio:event": "challengeCompleted",
		"gremio:ref": e.Ref, "gremio:rules": "dojo-1", "gremio:head": "abc123",
		"content": `<p>Terminó el reto &lt;Operador&gt; (N3, reproducible): +7 en Kubernetes.</p><p><a href="` + e.Ref + `">` + e.Ref + `</a></p>`,
	}
	for k, v := range want {
		if note[k] != v {
			t.Errorf("Note[%s] = %v, quería %v", k, note[k], v)
		}
	}
	// /notes/{id} y el outbox sirven el mismo Note, con las mismas propiedades.
	if got := getJSON(t, id); got["gremio:head"] != "abc123" || got["gremio:event"] != "challengeCompleted" {
		t.Errorf("GET %s = %v", id, got)
	}
	outbox := getJSON(t, string(a.actorID)+"/outbox")
	items, _ := outbox["orderedItems"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["object"].(map[string]any)["gremio:ref"] != e.Ref {
		t.Errorf("outbox = %v", outbox)
	}

	due, _ := a.st.Read.DueDeliveries(ctx, store.DueDeliveriesParams{NextAttemptAt: "9999", Limit: 1})
	if err := a.st.Write.DeleteDelivery(ctx, due[0].ID); err != nil {
		t.Fatal(err)
	}
	if queued, err := a.RetractEvent(ctx, "abc123", "dojo-1", "def456"); err != nil || queued != 1 {
		t.Fatalf("RetractEvent = %d, %v", queued, err)
	}
	del := a.pending(t)
	if del["type"] != "Delete" || del["object"] != id || del["gremio:event"] != "retired" || del["gremio:head"] != "def456" {
		t.Errorf("Delete = %v", del)
	}
	if has, _ := a.st.Read.HasEventNotes(ctx); has {
		t.Error("el Note retirado sigue guardado")
	}
	if queued, err := a.RetractEvent(ctx, "abc123", "dojo-1", "def456"); err != nil || queued != 0 {
		t.Errorf("retirar dos veces = %d, %v; no debe encolar otro Delete", queued, err)
	}
}

func TestEventoIncompletoOConRefInvalidaFalla(t *testing.T) {
	a := newTestInstance(t, "alice")
	ok := Event{Hash: "h", Type: "badgeAwarded", Ref: "https://alice.example/@alice", Rules: "dojo-1", Head: "h", Text: "Ganó una medalla.", At: now()}
	for name, mod := range map[string]func(*Event){
		"sin texto":      func(e *Event) { e.Text = "  " },
		"sin cabeza":     func(e *Event) { e.Head = "" },
		"ref javascript": func(e *Event) { e.Ref = "javascript:alert(1)" },
	} {
		e := ok
		mod(&e)
		if _, err := a.PublishEvent(context.Background(), e); err == nil {
			t.Errorf("%s: debería fallar", name)
		}
	}
}
