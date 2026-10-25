package federation

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"html"
	"net/http"
	"strings"
	"time"

	ap "github.com/duv0-x/gremio/internal/goap/activitypub"
	"github.com/duv0-x/gremio/internal/store"
)

const outboxPageSize = 20

// PublishNote guarda un Note público y encola su Create para cada servidor seguidor, en una transacción.
// No toca la red: entrega el worker de `gremio serve` (RunDeliveries).
func (s *Server) PublishNote(ctx context.Context, text string) (string, int, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", 0, errors.New("el texto está vacío")
	}
	var rnd [8]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		return "", 0, err
	}
	n := store.Note{
		ID:          s.inst.Host.String + "/notes/" + hex.EncodeToString(rnd[:]),
		Content:     "<p>" + strings.ReplaceAll(html.EscapeString(text), "\n", "<br>") + "</p>",
		PublishedAt: dbTime(now()),
	}
	var queued int
	err := s.st.Tx(ctx, func(q *store.Queries) error {
		if err := q.CreateNote(ctx, store.CreateNoteParams(n)); err != nil {
			return err
		}
		rows, err := q.ListFollowerInboxes(ctx)
		if err != nil {
			return err
		}
		inboxes := inboxesFor(rows)
		queued = len(inboxes)
		return s.enqueue(ctx, q, inboxes, s.createActivity(n))
	})
	if err != nil {
		return "", 0, err
	}
	s.wake()
	return n.ID, queued, nil
}

func (s *Server) noteObject(n store.Note) *ap.Object {
	published, _ := time.Parse(dbTimeLayout, n.PublishedAt)
	return &ap.Object{
		ID:           ap.IRI(n.ID),
		Type:         ap.NoteType,
		AttributedTo: s.actorID,
		Content:      ap.DefaultLangValue(n.Content),
		URL:          ap.IRI(n.ID),
		Published:    published,
		To:           ap.ItemCollection{ap.PublicNS},
		CC:           ap.ItemCollection{s.actorID + "/followers"},
	}
}

func (s *Server) createActivity(n store.Note) *ap.Activity {
	note := s.noteObject(n)
	return &ap.Activity{
		ID: note.ID + "/activity", Type: ap.CreateType, Actor: s.actorID, Object: note,
		Published: note.Published, To: note.To, CC: note.CC,
	}
}

// shortcut: el outbox muestra los últimos outboxPageSize Notes sin paginación; paginar cuando haya más actividad.
func (s *Server) getOutbox(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	total, err := s.st.Read.CountNotes(ctx)
	if err != nil {
		http.Error(w, "outbox", http.StatusInternalServerError)
		return
	}
	notes, err := s.st.Read.ListNotes(ctx, outboxPageSize)
	if err != nil {
		http.Error(w, "outbox", http.StatusInternalServerError)
		return
	}
	items := make(ap.ItemCollection, 0, len(notes))
	for _, n := range notes {
		items = append(items, s.createActivity(n))
	}
	writeAP(w, &ap.OrderedCollection{ID: s.actorID + "/outbox", Type: ap.OrderedCollectionType, TotalItems: uint(total), OrderedItems: items})
}

func (s *Server) getNote(w http.ResponseWriter, r *http.Request) {
	n, err := s.st.Read.GetNote(r.Context(), s.inst.Host.String+"/notes/"+r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	writeAP(w, s.noteObject(n))
}
