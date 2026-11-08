package web

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// serveDojoBook sirve el libro público del dojo (ADR-013 §6): solo trae ejecuciones publicadas, porque lo privado
// nunca entra al libro. Se sirve el libro y no el número: quien lo lee recalcula con la fecha del día.
func (s *Server) serveDojoBook(w http.ResponseWriter, r *http.Request, actor string) {
	book, err := s.Dojo.Book(r.Context(), actor)
	if err != nil {
		slog.Error("libro del dojo", "err", err)
		http.Error(w, "libro no disponible", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Access-Control-Allow-Origin", "*") // es público: otras instancias y herramientas lo leen
	json.NewEncoder(w).Encode(book)
}
