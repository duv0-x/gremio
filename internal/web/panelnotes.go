package web

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/duv0-x/gremio/internal/instance"
)

// Publicar notas desde el panel (gremio#115): lo mismo que `gremio admin note`, con un formulario.

const maxNoteLen = 1000 // caracteres

// panelNotes es lo que muestra /panel/notas.
type panelNotes struct {
	Version, Handle string
	Notes           []note
	Text            string // lo escrito, si no se publicó
	Notice, Error   string
}

func (s *Server) panelNotesPage(w http.ResponseWriter, r *http.Request, status int, p panelNotes) {
	inst, err := instance.Load(r.Context(), s.st.Read)
	if err != nil {
		slog.Error("notas del panel", "err", err)
		http.Error(w, "instancia no disponible", http.StatusInternalServerError)
		return
	}
	rows, err := s.st.Read.ListNotes(r.Context(), 10)
	if err != nil {
		slog.Error("notas del panel", "err", err)
		http.Error(w, "no se pudieron leer las notas", http.StatusInternalServerError)
		return
	}
	for _, n := range rows {
		published, _ := time.Parse(dbTimeLayout, n.PublishedAt)
		p.Notes = append(p.Notes, note{ID: n.ID, Text: noteText(n.Content), Published: published})
	}
	p.Version, p.Handle = s.version, "@"+inst.Username+"@"+inst.AccountDomain.String
	s.renderPanel(w, r, status, PanelNotesPage(p))
}

func (s *Server) getPanelNotes(w http.ResponseWriter, r *http.Request) {
	if !s.owner(w, r) {
		return
	}
	p := panelNotes{}
	if n := r.URL.Query().Get("publicada"); n != "" {
		p.Notice = "Nota publicada."
		if q, err := strconv.Atoi(n); err == nil && q > 0 {
			p.Notice += " Se entrega a " + strconv.Itoa(q) + " " + plural(q, "servidor", "servidores") + " en unos minutos."
		}
	}
	s.panelNotesPage(w, r, http.StatusOK, p)
}

func (s *Server) postPanelNote(w http.ResponseWriter, r *http.Request) {
	if !s.owner(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "formulario inválido", http.StatusBadRequest)
		return
	}
	text := strings.TrimSpace(strings.ReplaceAll(r.PostFormValue("texto"), "\r\n", "\n"))
	switch {
	case text == "":
		s.panelNotesPage(w, r, http.StatusBadRequest, panelNotes{Error: "La nota está vacía."})
		return
	case utf8.RuneCountInString(text) > maxNoteLen:
		s.panelNotesPage(w, r, http.StatusBadRequest, panelNotes{Text: text, Error: "La nota pasa de " + strconv.Itoa(maxNoteLen) + " caracteres."})
		return
	}
	if s.profiles == nil {
		http.Error(w, "la instancia no puede publicar", http.StatusServiceUnavailable)
		return
	}
	id, queued, err := s.profiles.PublishNote(r.Context(), text)
	if err != nil {
		slog.Error("publicar nota", "err", err)
		s.panelNotesPage(w, r, http.StatusInternalServerError, panelNotes{Text: text, Error: "No se pudo publicar la nota."})
		return
	}
	slog.Info("nota publicada desde el panel", "id", id, "entregas_encoladas", queued)
	http.Redirect(w, r, "/panel/notas?publicada="+strconv.Itoa(queued), http.StatusSeeOther)
}
