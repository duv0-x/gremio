package federation

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	ap "github.com/duv0-x/gremio/internal/goap/activitypub"
	"github.com/duv0-x/gremio/internal/goap/jsonld"
	"github.com/duv0-x/gremio/internal/store"
)

// Política de entrega (ADR-001): backoff exponencial desde Options.RetryBase hasta maxRetryDelay;
// tras maxAttempts intentos (unas 14,5 h con la base de 30 s: 30 s, 1 min, 2 min… y 6 h al final) la entrega
// se descarta. Los 4xx, salvo
// 408 y 429, son definitivos: reintentarlos no cambia nada.
const (
	maxAttempts   = 12
	maxRetryDelay = 6 * time.Hour
	batchSize     = 20
	dbTimeLayout  = "2006-01-02T15:04:05.000Z"
)

// deliveryError es una respuesta HTTP no exitosa del inbox remoto.
type deliveryError struct {
	status int
	body   string
}

func (e *deliveryError) Error() string { return fmt.Sprintf("HTTP %d: %s", e.status, e.body) }

func permanent(err error) bool {
	var de *deliveryError
	return errors.As(err, &de) && de.status >= 400 && de.status < 500 && de.status != 408 && de.status != 429
}

func (s *Server) backoff(attempts int64) time.Duration {
	d := s.opts.RetryBase
	for i := int64(1); i < attempts && d < maxRetryDelay; i++ {
		d *= 2
	}
	return min(d, maxRetryDelay)
}

func dbTime(t time.Time) string { return t.UTC().Format(dbTimeLayout) }

// inboxesFor devuelve un inbox por servidor: el sharedInbox si el seguidor lo publica.
func inboxesFor(rows []store.ListFollowerInboxesRow) []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range rows {
		inbox := r.Inbox
		if r.SharedInbox.Valid {
			inbox = r.SharedInbox.String
		}
		if !seen[inbox] {
			seen[inbox] = true
			out = append(out, inbox)
		}
	}
	return out
}

// enqueue serializa la actividad una vez y la encola para cada inbox. Usa q para poder ir dentro de una
// transacción; quien llama debe invocar s.wake() después del commit.
func (s *Server) enqueue(ctx context.Context, q *store.Queries, inboxes []string, act *ap.Activity) error {
	body, err := jsonld.WithContext(jsonld.IRI(ap.ActivityBaseURI)).Marshal(act)
	if err != nil {
		return err
	}
	for _, inbox := range inboxes {
		if _, err := s.checkURL(inbox); err != nil {
			slog.Warn("inbox descartado", "inbox", inbox, "err", err)
			continue
		}
		if err := q.EnqueueDelivery(ctx, store.EnqueueDeliveryParams{Inbox: inbox, Activity: string(body), NextAttemptAt: dbTime(now())}); err != nil {
			return err
		}
	}
	return nil
}

// wake avisa al worker de este proceso que hay entregas nuevas; si ya está avisado, no bloquea.
func (s *Server) wake() {
	select {
	case s.wakeCh <- struct{}{}:
	default:
	}
}

// RunDeliveries entrega la cola hasta que ctx se cancela. Revisa la base cada Options.PollInterval
// (para lo que encolan otros procesos, como `gremio admin note`) o en cuanto este proceso encola.
func (s *Server) RunDeliveries(ctx context.Context) {
	tick := time.NewTicker(s.opts.PollInterval)
	defer tick.Stop()
	for {
		s.deliverDue(ctx)
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		case <-s.wakeCh:
		}
	}
}

// shortcut: entrega secuencial; alcanza para una instancia de una persona. Paralelizar por inbox si crecen los seguidores.
func (s *Server) deliverDue(ctx context.Context) {
	for ctx.Err() == nil {
		due, err := s.st.Write.DueDeliveries(ctx, store.DueDeliveriesParams{NextAttemptAt: dbTime(now()), Limit: batchSize})
		if err != nil {
			if ctx.Err() == nil {
				slog.Error("leer cola de entrega", "err", err)
			}
			return
		}
		for _, d := range due {
			s.attempt(ctx, d)
		}
		if len(due) < batchSize {
			return
		}
	}
}

func (s *Server) attempt(ctx context.Context, d store.DeliveryQueue) {
	err := s.post(ctx, d.Inbox, []byte(d.Activity))
	if ctx.Err() != nil {
		return // apagando: el intento no cuenta y la entrega sigue en la cola
	}
	attempts := d.Attempts + 1
	switch {
	case err == nil:
		err = s.st.Write.DeleteDelivery(ctx, d.ID)
		slog.Info("entregado", "inbox", d.Inbox, "intentos", attempts)
	case permanent(err) || attempts >= maxAttempts:
		slog.Warn("entrega descartada", "inbox", d.Inbox, "intentos", attempts, "err", err)
		err = s.st.Write.DeleteDelivery(ctx, d.ID)
	default:
		next := s.backoff(attempts)
		slog.Info("entrega reintentará", "inbox", d.Inbox, "intentos", attempts, "en", next, "err", err)
		err = s.st.Write.RescheduleDelivery(ctx, store.RescheduleDeliveryParams{
			ID: d.ID, Attempts: attempts, NextAttemptAt: dbTime(now().Add(next)),
			LastError: sql.NullString{String: err.Error(), Valid: true},
		})
	}
	if err != nil {
		slog.Error("actualizar cola de entrega", "id", d.ID, "err", err)
	}
}
