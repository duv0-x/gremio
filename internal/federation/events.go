package federation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/url"
	"strings"
	"time"

	ap "github.com/duv0-x/gremio/internal/goap/activitypub"
	"github.com/duv0-x/gremio/internal/goap/jsonld"
	"github.com/duv0-x/gremio/internal/store"
)

// GremioNS es el contexto JSON-LD de Gremio (ADR-005). No cambia nunca.
const GremioNS = "https://w3id.org/gremio/ns/v1"

// apContext es el @context de todo lo que publica la instancia: ActivityStreams y el prefijo gremio: para las
// propiedades propias de los Note de Actividad.
var apContext = jsonld.Context{{IRI: jsonld.IRI(ap.ActivityBaseURI)}, {Term: "gremio", IRI: GremioNS + "#"}}

// Event es un evento público del libro del dojo (ADR-013 §6) listo para federarse como Note (ADR-014 §2).
type Event struct {
	Hash  string    // hash del evento en el libro: el Note vive en /notes/<hash>
	Type  string    // gremio:event (challengeStarted, challengeCompleted, badgeAwarded)
	Ref   string    // gremio:ref: URL del objeto que lo respalda (ejecución, reto, medalla)
	Rules string    // gremio:rules: versión de las reglas del dojo
	Head  string    // gremio:head: cabeza de la cadena de hashes del libro al publicar
	Text  string    // texto generado por Gremio, sin HTML
	At    time.Time // fecha original del evento
}

// gremioProps son las propiedades de Gremio de un Note o de su Delete, en el orden en que se publican.
type gremioProps struct {
	Event string `json:"gremio:event"`
	Ref   string `json:"gremio:ref"`
	Rules string `json:"gremio:rules"`
	Head  string `json:"gremio:head"`
}

// PublishEvent publica un evento de Actividad: guarda su Note, con las propiedades de Gremio que se publican, y
// encola su Create para los seguidores. Publicar dos veces el mismo hash no hace nada.
func (s *Server) PublishEvent(ctx context.Context, e Event) (int, error) {
	text := strings.TrimSpace(e.Text)
	if e.Hash == "" || e.Type == "" || e.Rules == "" || e.Head == "" || text == "" || e.At.IsZero() {
		return 0, errors.New("evento de Actividad incompleto")
	}
	if u, err := url.Parse(e.Ref); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return 0, fmt.Errorf("gremio:ref %q no es una URL http(s)", e.Ref)
	}
	props, err := json.Marshal(gremioProps{e.Type, e.Ref, e.Rules, e.Head})
	if err != nil {
		return 0, err
	}
	ref := html.EscapeString(e.Ref)
	return s.publish(ctx, store.Note{
		ID:          s.eventNoteID(e.Hash),
		Content:     "<p>" + html.EscapeString(text) + `</p><p><a href="` + ref + `">` + ref + "</a></p>",
		PublishedAt: dbTime(e.At.UTC()),
		Gremio:      string(props),
	})
}

// RetractEvent retira el Note de un evento (ADR-014 §2): lo borra y encola un Delete con el evento de revocación
// (gremio:event "retired") y la cabeza nueva del libro. Si el evento nunca se publicó, no hace nada.
func (s *Server) RetractEvent(ctx context.Context, hash, rules, head string) (int, error) {
	id := s.eventNoteID(hash)
	props, err := json.Marshal(gremioProps{"retired", id, rules, head})
	if err != nil {
		return 0, err
	}
	var queued int
	err = s.st.Tx(ctx, func(q *store.Queries) error {
		deleted, err := q.DeleteNote(ctx, id)
		if err != nil || deleted == 0 {
			return err
		}
		rows, err := q.ListFollowerInboxes(ctx)
		if err != nil {
			return err
		}
		inboxes := inboxesFor(rows)
		queued = len(inboxes)
		del := &ap.Activity{
			ID: ap.IRI(id + "/delete"), Type: ap.DeleteType, Actor: s.actorID, Object: ap.IRI(id),
			Published: now(), To: ap.ItemCollection{ap.PublicNS}, CC: ap.ItemCollection{s.actorID + "/followers"},
		}
		return s.enqueue(ctx, q, inboxes, gremioItem{del, string(props)})
	})
	if err != nil {
		return 0, err
	}
	s.wake()
	return queued, nil
}

func (s *Server) eventNoteID(hash string) string { return s.inst.Host.String + "/notes/" + hash }

// gremioItem es un objeto de ActivityPub con propiedades de Gremio: props es un objeto JSON cuyos campos se pegan
// al final del objeto serializado. go-ap no tiene campos de extensión, y así el JSON guardado se publica tal cual.
type gremioItem struct {
	ap.Item
	props string
}

func (g gremioItem) MarshalJSON() ([]byte, error) {
	m, ok := g.Item.(json.Marshaler)
	if !ok {
		return nil, fmt.Errorf("%T no se puede serializar", g.Item)
	}
	b, err := m.MarshalJSON()
	if err != nil {
		return nil, err
	}
	if len(b) < 2 || b[len(b)-1] != '}' || len(g.props) < 3 || g.props[0] != '{' {
		return nil, errors.New("propiedades de Gremio mal formadas")
	}
	return append(append(b[:len(b)-1], ','), g.props[1:]...), nil
}
