package dojo

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/duv0-x/gremio/internal/store"
)

const timeLayout = "2006-01-02T15:04:05.000Z"

// Subscriber recibe cada evento que entra al libro (Append y el evento retired de Retire), después del commit y en
// el mismo goroutine. Puede llamar a Append (p. ej. medallas otorga una medalla): recibirá también ese evento.
// Sus errores son suyos: el evento ya está en el libro.
type Subscriber func(ctx context.Context, e Event)

// Ledger es el libro de eventos de la instancia, sobre la tabla dojo_events.
type Ledger struct {
	st   *store.Store
	subs []Subscriber
}

func NewLedger(st *store.Store) *Ledger { return &Ledger{st: st} }

// Subscribe registra un suscriptor. Se llama al cablear el servidor, antes de usar el libro (no es concurrente).
func (l *Ledger) Subscribe(fn Subscriber) { l.subs = append(l.subs, fn) }

// ErrNotFound es un hash que no está en el libro.
var ErrNotFound = errors.New("evento del dojo no encontrado")

// ErrAlreadyRetired es un evento que ya es tombstone, o un evento retired (que no se retira).
var ErrAlreadyRetired = errors.New("el evento ya está retirado")

// Append valida el evento, lo encadena al último y lo guarda. Devuelve el evento con Prev y Hash.
// Retired solo entra por Retire.
func (l *Ledger) Append(ctx context.Context, e Event) (Event, error) {
	if e.Type == Retired {
		return Event{}, fmt.Errorf("%w: un retiro se hace con Retire", ErrInvalid)
	}
	e.At = e.At.UTC().Truncate(time.Second)
	e.Tombstone, e.Hash = false, ""
	if err := e.Validate(); err != nil {
		return Event{}, err
	}
	err := l.st.Tx(ctx, func(q *store.Queries) error {
		var err error
		e, err = appendTx(ctx, q, e)
		return err
	})
	if err != nil {
		return Event{}, err
	}
	l.notify(ctx, e)
	return e, nil
}

// Retire retira un evento (derecho al olvido, §6): su cuerpo pasa a ser una tombstone con el tipo, la fecha, el
// reto y prev, conserva su hash, y se agrega un evento retired que lo referencia. La tombstone sigue contando para
// "un reto suma una sola vez por persona". Devuelve el evento retired.
func (l *Ledger) Retire(ctx context.Context, hash string, at time.Time) (Event, error) {
	r := Event{Type: Retired, At: at.UTC().Truncate(time.Second), Retires: hash}
	if err := r.Validate(); err != nil {
		return Event{}, err
	}
	err := l.st.Tx(ctx, func(q *store.Queries) error {
		row, err := q.GetDojoEvent(ctx, hash)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		old, err := decode(row)
		if err != nil {
			return err
		}
		if old.Tombstone || old.Type == Retired {
			return ErrAlreadyRetired
		}
		tomb, err := json.Marshal(Event{Type: old.Type, At: old.At, Challenge: old.Challenge, Tombstone: true, Prev: old.Prev})
		if err != nil {
			return err
		}
		if _, err := q.ScrubDojoEvent(ctx, store.ScrubDojoEventParams{Body: string(tomb), Hash: hash}); err != nil {
			return err
		}
		r.Challenge = old.Challenge
		r, err = appendTx(ctx, q, r)
		return err
	})
	if err != nil {
		return Event{}, err
	}
	l.notify(ctx, r)
	return r, nil
}

// List devuelve el libro completo en el orden de la cadena.
func (l *Ledger) List(ctx context.Context) ([]Event, error) {
	rows, err := l.st.Read.ListDojoEvents(ctx)
	if err != nil {
		return nil, err
	}
	events := make([]Event, 0, len(rows))
	for _, row := range rows {
		e, err := decode(row)
		if err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, nil
}

// Book devuelve el libro listo para exportar o servir.
func (l *Ledger) Book(ctx context.Context, actor string) (Book, error) {
	events, err := l.List(ctx)
	if err != nil {
		return Book{}, err
	}
	b := Book{Rules: Rules, Actor: actor, Events: events}
	if len(events) > 0 {
		b.Head = events[len(events)-1].Hash
	}
	return b, nil
}

func (l *Ledger) notify(ctx context.Context, e Event) {
	for _, fn := range l.subs {
		fn(ctx, e)
	}
}

func appendTx(ctx context.Context, q *store.Queries, e Event) (Event, error) {
	prev, err := q.LastDojoEventHash(ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Event{}, err
	}
	e.Prev = prev
	body, hash, err := seal(e)
	if err != nil {
		return Event{}, err
	}
	e.Hash = hash
	return e, q.AppendDojoEvent(ctx, store.AppendDojoEventParams{
		Hash: hash, Prev: prev, Type: string(e.Type), At: e.At.Format(timeLayout), Challenge: e.Challenge, Body: string(body),
	})
}

// seal serializa el evento sin Hash y devuelve el cuerpo y su sha256 en hex.
func seal(e Event) ([]byte, string, error) {
	e.Hash = ""
	body, err := json.Marshal(e)
	if err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(body)
	return body, hex.EncodeToString(sum[:]), nil
}

func decode(row store.DojoEvent) (Event, error) {
	var e Event
	if err := json.Unmarshal([]byte(row.Body), &e); err != nil {
		return Event{}, fmt.Errorf("evento %s: %w", row.Hash, err)
	}
	e.Hash = row.Hash
	return e, nil
}

// VerifyChain comprueba que cada evento enlaza con el anterior y que su hash corresponde a su cuerpo
// (salvo las tombstones, cuyo cuerpo se borró). Detecta un libro reescrito.
func VerifyChain(events []Event) error {
	prev := ""
	for i, e := range events {
		if e.Prev != prev {
			return fmt.Errorf("evento %d (%s): prev %q no enlaza con %q", i, e.Hash, e.Prev, prev)
		}
		if !e.Tombstone {
			if _, h, err := seal(e); err != nil {
				return err
			} else if h != e.Hash {
				return fmt.Errorf("evento %d: el hash %s no corresponde a su contenido (%s)", i, e.Hash, h)
			}
		}
		prev = e.Hash
	}
	return nil
}
