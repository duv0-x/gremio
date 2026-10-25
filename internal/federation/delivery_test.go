package federation

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/duv0-x/gremio/internal/store"
)

func (ti *testInstance) queued(t *testing.T) int64 {
	t.Helper()
	n, err := ti.st.Read.CountDeliveries(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestBackoff(t *testing.T) {
	s := &Server{opts: Options{RetryBase: 30 * time.Second}}
	for attempts, want := range map[int64]time.Duration{1: 30 * time.Second, 2: time.Minute, 3: 2 * time.Minute, 10: 4*time.Hour + 16*time.Minute, 11: 6 * time.Hour, 12: 6 * time.Hour} {
		if got := s.backoff(attempts); got != want {
			t.Errorf("backoff(%d) = %v, quería %v", attempts, got, want)
		}
	}
}

func TestPermanent(t *testing.T) {
	for status, want := range map[int]bool{400: true, 401: true, 404: true, 410: true, 408: false, 429: false, 500: false, 503: false} {
		if got := permanent(&deliveryError{status: status}); got != want {
			t.Errorf("permanent(%d) = %v", status, got)
		}
	}
	if permanent(context.DeadlineExceeded) {
		t.Error("un error de red no es definitivo")
	}
}

func TestInboxesForAgrupaPorSharedInbox(t *testing.T) {
	shared := sql.NullString{String: "https://masto.example/inbox", Valid: true}
	got := inboxesFor([]store.ListFollowerInboxesRow{
		{Inbox: "https://masto.example/users/a/inbox", SharedInbox: shared},
		{Inbox: "https://masto.example/users/b/inbox", SharedInbox: shared},
		{Inbox: "https://gts.example/users/c/inbox"},
	})
	if strings.Join(got, " ") != "https://masto.example/inbox https://gts.example/users/c/inbox" {
		t.Errorf("inboxes = %v", got)
	}
}

func TestNoteLlegaAlSeguidorYApareceEnElOutbox(t *testing.T) {
	a, b := newTestInstance(t, "alice"), newTestInstance(t, "bob")
	b.follow(t, a, 1)
	waitFor(t, "Accept en el inbox de bob", func() bool { return b.inboxes.Load() == 1 })

	id, queued, err := a.PublishNote(context.Background(), "hola <b>fediverso</b>\nsegunda línea")
	if err != nil || queued != 1 {
		t.Fatalf("PublishNote = %d, %v", queued, err)
	}
	waitFor(t, "Create en el sharedInbox de bob", func() bool { return b.shared.Load() == 1 })
	waitFor(t, "cola vacía", func() bool { return a.queued(t) == 0 })

	note := getJSON(t, id)
	if note["content"] != "<p>hola &lt;b&gt;fediverso&lt;/b&gt;<br>segunda línea</p>" || note["attributedTo"] != string(a.actorID) {
		t.Errorf("note = %v", note)
	}
	outbox := getJSON(t, string(a.actorID)+"/outbox")
	items, _ := outbox["orderedItems"].([]any)
	if outbox["totalItems"] != 1.0 || len(items) != 1 || items[0].(map[string]any)["object"].(map[string]any)["id"] != id {
		t.Errorf("outbox = %v", outbox)
	}
	if _, _, err := a.PublishNote(context.Background(), "   "); err == nil {
		t.Error("un Note vacío debería fallar")
	}
}

// La instancia remota está caída cuando se publica, Gremio se reinicia y, al volver el remoto, entrega.
func TestEntregaSobreviveCaidaRemotaYReinicio(t *testing.T) {
	a, b := newTestInstance(t, "alice"), newTestInstance(t, "bob")
	b.follow(t, a, 1)
	waitFor(t, "Accept en el inbox de bob", func() bool { return b.inboxes.Load() == 1 })

	b.status.Store(503)
	if _, _, err := a.PublishNote(context.Background(), "¿me recibes?"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "al menos un reintento", func() bool {
		due, _ := a.st.Read.DueDeliveries(context.Background(), store.DueDeliveriesParams{NextAttemptAt: "9999", Limit: 1})
		return len(due) == 1 && due[0].Attempts >= 1 && strings.Contains(due[0].LastError.String, "503")
	})

	a.stop() // "apagado" de alice: la entrega sigue en la base
	if n := a.queued(t); n != 1 {
		t.Fatalf("entregas pendientes = %d", n)
	}
	a2, err := New(context.Background(), a.st, fastOpts)
	if err != nil {
		t.Fatal(err)
	}
	b.status.Store(0)
	runWorker(t, a2)
	waitFor(t, "Create entregado tras reiniciar", func() bool { return b.shared.Load() == 1 })
	waitFor(t, "cola vacía", func() bool { return a.queued(t) == 0 })
}

func TestEntregaDescartadaPor4xxOIntentosAgotados(t *testing.T) {
	a, b := newTestInstance(t, "alice"), newTestInstance(t, "bob")
	ctx := context.Background()

	b.status.Store(404)
	if err := a.enqueue(ctx, a.st.Write, []string{b.url + "/inbox"}, a.createActivity(store.Note{ID: a.url + "/notes/1", Content: "x", PublishedAt: dbTime(now())})); err != nil {
		t.Fatal(err)
	}
	a.wake()
	waitFor(t, "descarte por 404", func() bool { return a.queued(t) == 0 })

	// Una entrega en su último intento que vuelve a fallar con 503 se descarta.
	b.status.Store(503)
	if err := a.st.Write.EnqueueDelivery(ctx, store.EnqueueDeliveryParams{Inbox: b.url + "/inbox", Activity: `{}`, NextAttemptAt: dbTime(now())}); err != nil {
		t.Fatal(err)
	}
	due, _ := a.st.Read.DueDeliveries(ctx, store.DueDeliveriesParams{NextAttemptAt: "9999", Limit: 1})
	if err := a.st.Write.RescheduleDelivery(ctx, store.RescheduleDeliveryParams{ID: due[0].ID, Attempts: maxAttempts - 1, NextAttemptAt: dbTime(now())}); err != nil {
		t.Fatal(err)
	}
	a.wake()
	waitFor(t, "descarte por intentos agotados", func() bool { return a.queued(t) == 0 })
}
