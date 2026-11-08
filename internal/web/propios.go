package web

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/duv0-x/gremio/internal/dojo"
	"github.com/duv0-x/gremio/internal/dojo/retos"
	"github.com/duv0-x/gremio/internal/instance"
	"github.com/duv0-x/gremio/internal/plugins"
)

// Retos propios del dojo (ADR-002 §1 y §4, ADR-013 §8): crearlos a mano o pedir un borrador a un plugin de IA,
// publicarlos como versiones inmutables y tomarlos. Cuentan como práctica propia: N2 como máximo.

func (s *Server) registerOwn(mux *http.ServeMux, post func(string, http.HandlerFunc)) {
	mux.HandleFunc("GET /panel/dojo/propios/nuevo", s.getOwnNew)
	post("/panel/dojo/propios", s.postOwnNew)
	post("/panel/dojo/propios/generar", s.postOwnGenerate)
	mux.HandleFunc("GET /panel/dojo/propios/{id}", s.getOwn)
	post("/panel/dojo/propios/{id}", s.postOwn)
	post("/panel/dojo/propios/{id}/publicar", s.postOwnPublish)
	post("/panel/dojo/propios/{id}/tomar", s.postOwnTake)
	mux.HandleFunc("GET /dojo/retos/{id}", s.getPublicOwn)
	mux.HandleFunc("GET /dojo/retos/{id}/{version}", s.getPublicOwn)
}

// generatorOption es un plugin de IA habilitado que puede generar borradores.
type generatorOption struct {
	ID, Name, Model string
}

type ownPage struct {
	Version    string
	ID         string // vacío en un reto nuevo
	Challenge  retos.Challenge
	Checks     string // el JSON de los checks, tal como se escribió
	Published  int    // última versión publicada; 0 si nunca se publicó
	Generators []generatorOption
	Notice     string
	Error      string
}

var ownNotices = map[string]string{
	"creado":    "Borrador guardado.",
	"generado":  "Borrador generado con IA. Revísalo entero: el nivel que propone es solo una sugerencia y los checks tienen que probar el reto de verdad.",
	"guardado":  "Borrador guardado.",
	"publicado": "Versión publicada. Ya la puedes tomar; las versiones publicadas no cambian.",
}

func (s *Server) generators(r *http.Request) []generatorOption {
	states, err := s.plugins.List(r.Context())
	if err != nil {
		slog.Error("plugins", "err", err)
		return nil
	}
	var out []generatorOption
	for _, st := range states {
		p, _ := s.plugins.Find(st.Info.ID)
		if g, ok := p.(plugins.Generator); ok && st.Enabled {
			out = append(out, generatorOption{ID: st.Info.ID, Name: st.Info.Name, Model: g.DefaultModel()})
		}
	}
	return out
}

func (s *Server) getOwnNew(w http.ResponseWriter, r *http.Request) {
	if !s.owner(w, r) {
		return
	}
	c := retos.Challenge{Level: dojo.N1, Origin: "human", License: retos.LicenseDefault}
	s.renderPanel(w, r, http.StatusOK, OwnPage(ownPage{Version: s.version, Challenge: c, Checks: "[]", Generators: s.generators(r)}))
}

func (s *Server) postOwnNew(w http.ResponseWriter, r *http.Request) {
	if !s.owner(w, r) {
		return
	}
	c, checks, err := ownForm(w, r)
	if err != nil {
		s.renderPanel(w, r, http.StatusBadRequest, OwnPage(ownPage{Version: s.version, Challenge: c, Checks: checks, Generators: s.generators(r), Error: err.Error()}))
		return
	}
	c.Origin = "human"
	id, err := s.retos().CreateOwn(r.Context(), c)
	if err != nil {
		s.dojoError(w, "reto propio", err)
		return
	}
	http.Redirect(w, r, "/panel/dojo/propios/"+id+"?ok=creado", http.StatusSeeOther)
}

func (s *Server) postOwnGenerate(w http.ResponseWriter, r *http.Request) {
	if !s.owner(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "formulario inválido", http.StatusBadRequest)
		return
	}
	fail := func(msg string) {
		c := retos.Challenge{Level: dojo.N1, Origin: "human", License: retos.LicenseDefault}
		s.renderPanel(w, r, http.StatusBadRequest, OwnPage(ownPage{Version: s.version, Challenge: c, Checks: "[]", Generators: s.generators(r), Error: msg}))
	}
	id, topic, model := r.PostFormValue("plugin"), strings.TrimSpace(r.PostFormValue("topic")), strings.TrimSpace(r.PostFormValue("model"))
	p, _ := s.plugins.Find(id)
	g, ok := p.(plugins.Generator)
	cfg, err := s.plugins.Config(r.Context(), id)
	if !ok || err != nil {
		fail("Habilita el plugin de IA en Plugins antes de generar.")
		return
	}
	if topic == "" {
		fail("Escribe sobre qué quieres el reto.")
		return
	}
	model = cmp.Or(model, g.DefaultModel())
	system, prompt := retos.DraftPrompt(topic)
	text, err := g.Generate(r.Context(), cfg, model, system, prompt)
	if err != nil {
		slog.Info("dojo: borrador con IA", "plugin", id, "err", err)
		fail("No se generó el borrador: " + err.Error())
		return
	}
	c, err := retos.ParseDraft(text, id+":"+model)
	if err != nil {
		fail("No se generó el borrador: " + err.Error())
		return
	}
	newID, err := s.retos().CreateOwn(r.Context(), c)
	if err != nil {
		s.dojoError(w, "reto propio", err)
		return
	}
	slog.Info("dojo: borrador con IA", "plugin", id, "modelo", model, "reto", newID)
	http.Redirect(w, r, "/panel/dojo/propios/"+newID+"?ok=generado", http.StatusSeeOther)
}

func (s *Server) ownOr404(w http.ResponseWriter, r *http.Request) (retos.Own, bool) {
	o, err := s.retos().GetOwn(r.Context(), r.PathValue("id"))
	if errors.Is(err, retos.ErrNotFound) {
		http.NotFound(w, r)
		return o, false
	}
	if err != nil {
		s.dojoError(w, "reto propio", err)
		return o, false
	}
	return o, true
}

func (s *Server) renderOwn(w http.ResponseWriter, r *http.Request, status int, o retos.Own, c retos.Challenge, checks, notice, msg string) {
	s.renderPanel(w, r, status, OwnPage(ownPage{
		Version: s.version, ID: o.ID, Challenge: c, Checks: checks, Published: o.Versions,
		Notice: notice, Error: msg,
	}))
}

func (s *Server) getOwn(w http.ResponseWriter, r *http.Request) {
	if !s.owner(w, r) {
		return
	}
	o, ok := s.ownOr404(w, r)
	if !ok {
		return
	}
	s.renderOwn(w, r, http.StatusOK, o, o.Draft, checksJSON(o.Draft.Checks), ownNotices[r.URL.Query().Get("ok")], "")
}

func (s *Server) postOwn(w http.ResponseWriter, r *http.Request) {
	if !s.owner(w, r) {
		return
	}
	o, ok := s.ownOr404(w, r)
	if !ok {
		return
	}
	c, checks, err := ownForm(w, r)
	// El origen y el modelo vienen del borrador: editarlo no borra que lo propuso una IA.
	c.Origin, c.Model = o.Draft.Origin, o.Draft.Model
	if err == nil {
		err = s.retos().SaveOwn(r.Context(), o.ID, c)
	}
	if err != nil {
		s.renderOwn(w, r, http.StatusBadRequest, o, c, checks, "", err.Error())
		return
	}
	http.Redirect(w, r, "/panel/dojo/propios/"+o.ID+"?ok=guardado", http.StatusSeeOther)
}

func (s *Server) postOwnPublish(w http.ResponseWriter, r *http.Request) {
	if !s.owner(w, r) {
		return
	}
	o, ok := s.ownOr404(w, r)
	if !ok {
		return
	}
	inst, err := instance.Load(r.Context(), s.st.Read)
	if err != nil {
		s.dojoError(w, "instancia", err)
		return
	}
	if _, err := s.retos().PublishOwn(r.Context(), o.ID, inst.Username); err != nil {
		s.renderOwn(w, r, http.StatusBadRequest, o, o.Draft, checksJSON(o.Draft.Checks), "", "No se publicó: "+err.Error())
		return
	}
	http.Redirect(w, r, "/panel/dojo/propios/"+o.ID+"?ok=publicado", http.StatusSeeOther)
}

func (s *Server) postOwnTake(w http.ResponseWriter, r *http.Request) {
	if !s.owner(w, r) {
		return
	}
	urls, err := s.dojoURLs(r)
	if err != nil {
		s.dojoError(w, "instancia", err)
		return
	}
	e, err := s.retos().TakeOwn(r.Context(), r.PathValue("id"), urls)
	if errors.Is(err, retos.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		o, _ := s.retos().GetOwn(r.Context(), r.PathValue("id"))
		s.renderOwn(w, r, http.StatusBadRequest, o, o.Draft, checksJSON(o.Draft.Checks), "", err.Error())
		return
	}
	http.Redirect(w, r, "/panel/dojo/ejecuciones/"+e.ID+"?ok=tomado", http.StatusSeeOther)
}

// getPublicOwn muestra una versión publicada de un reto propio; sin versión, la última.
func (s *Server) getPublicOwn(w http.ResponseWriter, r *http.Request) {
	svc := s.retos()
	o, err := svc.GetOwn(r.Context(), r.PathValue("id"))
	if errors.Is(err, retos.ErrNotFound) || (err == nil && o.Latest == nil) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		s.dojoError(w, "reto propio", err)
		return
	}
	c := *o.Latest
	if v := r.PathValue("version"); v != "" {
		n, ok := parseVersion(v)
		if !ok {
			http.NotFound(w, r)
			return
		}
		if c, err = svc.OwnVersion(r.Context(), o.ID, n); err != nil {
			http.NotFound(w, r)
			return
		}
	}
	p, ok := s.publicPage(w, r)
	if !ok {
		return
	}
	setPublicHeaders(w)
	if err := PublicChallengePage(p, "/dojo/retos/"+o.ID, c, o.Versions).Render(r.Context(), w); err != nil {
		slog.Error("render reto propio", "err", err)
	}
}

// parseVersion lee "v<N>" sin ceros a la izquierda.
func parseVersion(v string) (int, bool) {
	n, err := strconv.Atoi(strings.TrimPrefix(v, "v"))
	return n, err == nil && n > 0 && "v"+strconv.Itoa(n) == v
}

// ownForm lee el formulario del reto. Las listas van una por línea y los checks en JSON.
func ownForm(w http.ResponseWriter, r *http.Request) (retos.Challenge, string, error) {
	r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
	if err := r.ParseForm(); err != nil {
		return retos.Challenge{}, "", errors.New("formulario inválido")
	}
	f := r.PostFormValue
	level, _ := strconv.Atoi(f("level"))
	c := retos.Challenge{
		Title: strings.TrimSpace(f("title")), Pitch: strings.TrimSpace(f("pitch")), Context: strings.TrimSpace(f("context")),
		Level: dojo.Level(level), Stack: splitList(f("stack")),
		Requirements: lines(f("requirements")), Constraints: lines(f("constraints")),
		DefinitionOfDone: lines(f("dod")), Deliverables: lines(f("deliverables")),
		EstimatedDuration: strings.TrimSpace(f("duration")), License: cmp.Or(strings.TrimSpace(f("license")), retos.LicenseDefault),
	}
	for i := 1; i <= dojo.MaxSkills; i++ {
		if id := f(fmt.Sprintf("skill%d", i)); id != "" {
			n, _ := strconv.Atoi(f(fmt.Sprintf("skill%dlevel", i)))
			c.Skills = append(c.Skills, dojo.SkillLevel{Skill: id, Level: dojo.Level(n)})
		}
	}
	checks := strings.TrimSpace(f("checks"))
	if checks == "" {
		checks = "[]"
	}
	if err := json.Unmarshal([]byte(checks), &c.Checks); err != nil {
		return c, checks, fmt.Errorf("los checks no son un JSON válido: %w", err)
	}
	return c, checks, nil
}

func checksJSON(c []retos.Check) string {
	if len(c) == 0 {
		return "[]"
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	return string(b)
}
