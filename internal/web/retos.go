package web

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/duv0-x/gremio/internal/dojo"
	"github.com/duv0-x/gremio/internal/dojo/retos"
	"github.com/duv0-x/gremio/internal/instance"
	"github.com/duv0-x/gremio/internal/profile"
)

// El dojo en el panel (ADR-013, gremio#124): el catálogo, tomar un reto, entregar, verificar, terminar, publicar y
// retirar, con "cuánto sumaría" y "por qué no sumó". Y las páginas públicas de cada reto y de cada ejecución
// publicada, que enlazan la ficha, Actividad y el libro.

func (s *Server) registerDojo(mux *http.ServeMux, post func(string, http.HandlerFunc)) {
	mux.HandleFunc("GET /panel/dojo", s.getDojo)
	mux.HandleFunc("GET /panel/dojo/catalogo/{slug}", s.getDojoChallenge)
	post("/panel/dojo/catalogo/{slug}/tomar", s.postDojoTake)
	mux.HandleFunc("GET /panel/dojo/ejecuciones/{id}", s.getDojoExecution)
	post("/panel/dojo/ejecuciones/{id}/{action}", s.postDojoExecution)
	mux.HandleFunc("GET /panel/dojo/ejecuciones/{id}/retirar", s.getDojoRetire)
	mux.HandleFunc("GET /dojo/catalogo/{slug}", s.getPublicChallenge)
	mux.HandleFunc("GET /dojo/catalogo/{slug}/{version}", s.getPublicChallenge)
	mux.HandleFunc("GET /dojo/ejecuciones/{id}", s.getPublicExecution)
	s.registerOwn(mux, post)
}

func (s *Server) retos() *retos.Service { return retos.NewService(s.st, s.Dojo) }

func (s *Server) dojoURLs(r *http.Request) (retos.URLs, error) {
	inst, err := instance.Load(r.Context(), s.st.Read)
	if err != nil {
		return retos.URLs{}, err
	}
	return retos.URLs{Host: inst.Host.String}, nil
}

type catalogItem struct {
	Slug      string
	Challenge retos.Challenge
}

type dojoPage struct {
	Version    string
	Catalog    []catalogItem
	Executions []retos.Execution
	Own        []retos.Own
	Notice     string
	Error      string
}

type challengePage struct {
	Version   string
	Slug      string
	Challenge retos.Challenge
	Trust     dojo.Trust // la que daría si la verificación pasa
	Outcomes  []dojo.Outcome
}

type executionPage struct {
	Version   string
	Execution retos.Execution
	Trust     dojo.Trust
	Outcomes  []dojo.Outcome
	Public    string // URL de la página pública
	Notice    string
	Error     string
}

func catalogItems() []catalogItem {
	var out []catalogItem
	for _, slug := range retos.Slugs() {
		e, _ := retos.Find(slug)
		out = append(out, catalogItem{Slug: slug, Challenge: e.Latest()})
	}
	return out
}

func (s *Server) getDojo(w http.ResponseWriter, r *http.Request) {
	if !s.owner(w, r) {
		return
	}
	list, err := s.retos().List(r.Context())
	if err != nil {
		s.dojoError(w, "ejecuciones", err)
		return
	}
	own, err := s.retos().ListOwn(r.Context())
	if err != nil {
		s.dojoError(w, "retos propios", err)
		return
	}
	s.renderPanel(w, r, http.StatusOK, DojoPage(dojoPage{Version: s.version, Catalog: catalogItems(), Executions: list, Own: own}))
}

func (s *Server) getDojoChallenge(w http.ResponseWriter, r *http.Request) {
	if !s.owner(w, r) {
		return
	}
	slug := r.PathValue("slug")
	entry, ok := retos.Find(slug)
	if !ok {
		http.NotFound(w, r)
		return
	}
	urls, err := s.dojoURLs(r)
	if err != nil {
		s.dojoError(w, "instancia", err)
		return
	}
	c := entry.Latest()
	svc := s.retos()
	candidate := retos.Execution{Challenge: retos.CatalogPrefix + slug, Version: c.Version, Snapshot: c}
	outcomes, err := svc.Explain(r.Context(), candidate, urls)
	if err != nil {
		s.dojoError(w, "libro", err)
		return
	}
	s.renderPanel(w, r, http.StatusOK, DojoChallengePage(challengePage{
		Version: s.version, Slug: slug, Challenge: c, Trust: svc.ExpectedTrust(candidate), Outcomes: outcomes,
	}))
}

func (s *Server) postDojoTake(w http.ResponseWriter, r *http.Request) {
	if !s.owner(w, r) {
		return
	}
	e, err := s.retos().Take(r.Context(), r.PathValue("slug"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	slog.Info("dojo: reto tomado", "reto", e.Challenge, "ejecución", e.ID)
	http.Redirect(w, r, "/panel/dojo/ejecuciones/"+e.ID+"?ok=tomado", http.StatusSeeOther)
}

func (s *Server) execution(w http.ResponseWriter, r *http.Request) (retos.Execution, bool) {
	e, err := s.retos().Get(r.Context(), r.PathValue("id"))
	if errors.Is(err, retos.ErrNotFound) {
		http.NotFound(w, r)
		return e, false
	}
	if err != nil {
		s.dojoError(w, "ejecución", err)
		return e, false
	}
	return e, true
}

var dojoNotices = map[string]string{
	"tomado":     "Tomaste el reto. Su enunciado quedó copiado en esta ejecución: no cambia aunque salga una versión nueva.",
	"entrega":    "Entrega guardada. Corre los checks contra ese commit.",
	"verificado": "Checks corridos; el resultado está abajo.",
	"terminado":  "Ejecución terminada.",
	"publicado":  "Ejecución publicada: ya está en tu perfil y en el libro del dojo.",
	"abandonado": "Ejecución abandonada.",
	"retirado":   "Ejecución retirada. Su página ya no se muestra y el reto no vuelve a sumar.",
}

func (s *Server) getDojoExecution(w http.ResponseWriter, r *http.Request) {
	if !s.owner(w, r) {
		return
	}
	e, ok := s.execution(w, r)
	if !ok {
		return
	}
	s.renderExecution(w, r, http.StatusOK, e, dojoNotices[r.URL.Query().Get("ok")], "")
}

func (s *Server) renderExecution(w http.ResponseWriter, r *http.Request, status int, e retos.Execution, notice, msg string) {
	urls, err := s.dojoURLs(r)
	if err != nil {
		s.dojoError(w, "instancia", err)
		return
	}
	svc := s.retos()
	p := executionPage{Version: s.version, Execution: e, Trust: svc.ExpectedTrust(e), Public: urls.Execution(e.ID), Notice: notice, Error: msg}
	if !e.Retired() && e.Status != retos.StatusAbandoned {
		if p.Outcomes, err = svc.Explain(r.Context(), e, urls); err != nil {
			s.dojoError(w, "libro", err)
			return
		}
	}
	s.renderPanel(w, r, status, DojoExecutionPage(p))
}

func (s *Server) postDojoExecution(w http.ResponseWriter, r *http.Request) {
	if !s.owner(w, r) {
		return
	}
	e, ok := s.execution(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "formulario inválido", http.StatusBadRequest)
		return
	}
	urls, err := s.dojoURLs(r)
	if err != nil {
		s.dojoError(w, "instancia", err)
		return
	}
	svc, ctx, action := s.retos(), r.Context(), r.PathValue("action")
	var done string
	switch action {
	case "entrega":
		done, err = "entrega", svc.Deliver(ctx, e.ID, retos.Delivery{
			Repo: r.PostFormValue("repo"), Commit: r.PostFormValue("commit"), Demo: r.PostFormValue("demo"),
			Report: r.PostFormValue("report"), Notes: r.PostFormValue("notes"),
		})
	case "verificar":
		done = "verificado"
		_, err = svc.Verify(ctx, e.ID)
	case "terminar":
		done = "terminado"
		_, err = svc.Complete(ctx, e.ID, urls)
	case "publicar":
		done = "publicado"
		_, err = svc.Publish(ctx, e.ID, urls)
	case "abandonar":
		done, err = "abandonado", svc.Abandon(ctx, e.ID)
	case "retirar":
		done, err = "retirado", svc.Retire(ctx, e.ID)
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		slog.Info("dojo: acción rechazada", "acción", action, "ejecución", e.ID, "err", err)
		if e, err2 := svc.Get(ctx, e.ID); err2 == nil {
			s.renderExecution(w, r, http.StatusBadRequest, e, "", err.Error())
			return
		}
		s.dojoError(w, action, err)
		return
	}
	slog.Info("dojo", "acción", action, "ejecución", e.ID)
	http.Redirect(w, r, "/panel/dojo/ejecuciones/"+e.ID+"?ok="+done, http.StatusSeeOther)
}

func (s *Server) getDojoRetire(w http.ResponseWriter, r *http.Request) {
	if !s.owner(w, r) {
		return
	}
	e, ok := s.execution(w, r)
	if !ok {
		return
	}
	if !e.Published() {
		http.Redirect(w, r, "/panel/dojo/ejecuciones/"+e.ID, http.StatusSeeOther)
		return
	}
	s.renderPanel(w, r, http.StatusOK, DojoRetirePage(s.version, e))
}

func (s *Server) dojoError(w http.ResponseWriter, what string, err error) {
	slog.Error("dojo: "+what, "err", err)
	http.Error(w, "el dojo no está disponible", http.StatusInternalServerError)
}

// publicPage arma el Page del layout público para las páginas del dojo.
func (s *Server) publicPage(w http.ResponseWriter, r *http.Request) (Page, bool) {
	inst, err := instance.Load(r.Context(), s.st.Read)
	if err != nil {
		s.dojoError(w, "instancia", err)
		return Page{}, false
	}
	doc, err := s.st.Read.GetProfile(r.Context())
	if err != nil {
		s.dojoError(w, "perfil", err)
		return Page{}, false
	}
	res, err := profile.Load(doc)
	if err != nil {
		s.dojoError(w, "perfil", err)
		return Page{}, false
	}
	return Page{
		Resume: res, Username: inst.Username, Version: s.version,
		Handle: "@" + inst.Username + "@" + inst.AccountDomain.String,
		Actor:  inst.Host.String + "/users/" + inst.Username,
	}, true
}

func setPublicHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' https:; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
	h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
	h.Set("Cache-Control", "no-transform")
}

// getPublicChallenge muestra un reto del catálogo: la última versión en /dojo/catalogo/<slug>, o una versión fija
// (inmutable) en /dojo/catalogo/<slug>/v<N>, que es la que enlaza el libro.
func (s *Server) getPublicChallenge(w http.ResponseWriter, r *http.Request) {
	entry, ok := retos.Find(r.PathValue("slug"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	c := entry.Latest()
	if v := r.PathValue("version"); v != "" {
		n, ok := parseVersion(v)
		if !ok {
			http.NotFound(w, r)
			return
		}
		if c, ok = entry.Version(n); !ok {
			http.NotFound(w, r)
			return
		}
	}
	p, ok := s.publicPage(w, r)
	if !ok {
		return
	}
	setPublicHeaders(w)
	if err := PublicChallengePage(p, "/dojo/catalogo/"+r.PathValue("slug"), c, len(entry.Versions)).Render(r.Context(), w); err != nil {
		slog.Error("render reto", "err", err)
	}
}

// getPublicExecution muestra una ejecución publicada con su entrega y su verificación. Una retirada responde 410.
func (s *Server) getPublicExecution(w http.ResponseWriter, r *http.Request) {
	e, err := s.retos().Get(r.Context(), r.PathValue("id"))
	if errors.Is(err, retos.ErrNotFound) || (err == nil && e.PublishedAt.IsZero()) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		s.dojoError(w, "ejecución", err)
		return
	}
	if e.Retired() {
		http.Error(w, "Esta ejecución se retiró.", http.StatusGone)
		return
	}
	p, ok := s.publicPage(w, r)
	if !ok {
		return
	}
	trust, _ := s.retos().Trust(e)
	setPublicHeaders(w)
	if err := PublicExecutionPage(p, e, trust).Render(r.Context(), w); err != nil {
		slog.Error("render ejecución", "err", err)
	}
}
