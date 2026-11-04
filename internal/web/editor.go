package web

import (
	"cmp"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/a-h/templ"

	"github.com/duv0-x/gremio/internal/profile"
)

// Editor del perfil en el panel (gremio#28): cada sección es un formulario que funciona con submit normal.
// Guardar carga el perfil, aplica el formulario, lo valida con Normalize y lo guarda con ReplaceProfile, que
// avisa a los seguidores. Si no es válido, el formulario vuelve con lo que escribió el dueño y el error.

// editPage es lo que recibe cada formulario.
type editPage struct {
	Version string
	R       *profile.Resume
	Notice  string
	Error   string
	ID      string // elemento que se edita en las páginas de un solo trabajo, vista o atributo
}

// editSection es una página del editor: su formulario y cómo aplicarlo al perfil.
type editSection struct {
	path  string
	form  func(editPage) templ.Component
	apply func(r *http.Request, res *profile.Resume, id string) error
	// exists dice si id existe, en las secciones de un elemento; nil en las demás.
	exists func(res *profile.Resume, id string) bool
}

const blankRows = 2 // filas vacías al final de cada lista, para agregar sin JavaScript

func (s *Server) registerEditor(mux *http.ServeMux, post func(string, http.HandlerFunc)) {
	for _, sec := range s.sections() {
		mux.HandleFunc("GET "+sec.path, func(w http.ResponseWriter, r *http.Request) { s.getSection(w, r, sec) })
		post(sec.path, func(w http.ResponseWriter, r *http.Request) { s.postSection(w, r, sec) })
	}
}

func (s *Server) getSection(w http.ResponseWriter, r *http.Request, sec editSection) {
	if !s.owner(w, r) {
		return
	}
	res, err := s.currentProfile(r.Context())
	if err != nil {
		slog.Error("editor", "err", err)
		http.Error(w, "perfil no disponible", http.StatusInternalServerError)
		return
	}
	id := r.PathValue("id")
	if sec.exists != nil && id != "nuevo" && !sec.exists(res, id) {
		http.NotFound(w, r)
		return
	}
	e := editPage{Version: s.version, R: res, ID: id}
	if r.URL.Query().Get("ok") == "guardado" {
		e.Notice = "Guardado. Los seguidores reciben la actualización en unos minutos."
	}
	s.renderPanel(w, r, http.StatusOK, sec.form(e))
}

func (s *Server) postSection(w http.ResponseWriter, r *http.Request, sec editSection) {
	if !s.owner(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxImport)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "formulario inválido", http.StatusBadRequest)
		return
	}
	res, err := s.currentProfile(r.Context())
	if err != nil {
		slog.Error("editor", "err", err)
		http.Error(w, "perfil no disponible", http.StatusInternalServerError)
		return
	}
	id := r.PathValue("id")
	if sec.exists != nil && id != "nuevo" && !sec.exists(res, id) {
		http.NotFound(w, r)
		return
	}
	e := editPage{Version: s.version, R: res, ID: id}
	if err := sec.apply(r, res, id); err != nil {
		e.Error = err.Error()
		s.renderPanel(w, r, http.StatusBadRequest, sec.form(e))
		return
	}
	res.PruneRefs()
	if err := res.Normalize(); err != nil {
		e.Error = "No se guardó: " + err.Error()
		s.renderPanel(w, r, http.StatusBadRequest, sec.form(e))
		return
	}
	if s.profiles == nil {
		http.Error(w, "la instancia no puede guardar el perfil", http.StatusServiceUnavailable)
		return
	}
	if _, err := s.profiles.ReplaceProfile(r.Context(), res); err != nil {
		slog.Error("editor: guardar", "err", err)
		e.Error = "No se pudo guardar el perfil."
		s.renderPanel(w, r, http.StatusInternalServerError, sec.form(e))
		return
	}
	slog.Info("perfil editado desde el panel", "sección", sec.path)
	back := r.URL.Path
	if r.FormValue("despues") == "lista" {
		back = strings.TrimSuffix(back, "/"+id)
	}
	http.Redirect(w, r, back+"?ok=guardado", http.StatusSeeOther)
}

// row es una fila de una lista del formulario: los campos prefix.N.campo.
type row url.Values

func (r row) get(k string) string { return strings.TrimSpace(url.Values(r).Get(k)) }

func (r row) list(k string) []string { return url.Values(r)[k] }

// rows lee las filas prefix.N.* en el orden del campo "orden" (y, a igualdad, el de la página), sin las
// marcadas para borrar ni las que quedaron vacías.
func rows(r *http.Request, prefix string) []row {
	byIndex := map[int]url.Values{}
	for key, vals := range r.PostForm {
		rest, ok := strings.CutPrefix(key, prefix+".")
		if !ok {
			continue
		}
		n, field, ok := strings.Cut(rest, ".")
		i, err := strconv.Atoi(n)
		if !ok || err != nil {
			continue
		}
		if byIndex[i] == nil {
			byIndex[i] = url.Values{}
		}
		byIndex[i][field] = vals
	}
	type indexed struct {
		i     int
		order float64
		row   row
	}
	var out []indexed
	for i, v := range byIndex {
		if v.Get("borrar") != "" || empty(v) {
			continue
		}
		order, err := strconv.ParseFloat(v.Get("orden"), 64)
		if err != nil {
			order = float64(i + 1)
		}
		out = append(out, indexed{i, order, row(v)})
	}
	slices.SortFunc(out, func(a, b indexed) int { return cmp.Or(cmp.Compare(a.order, b.order), cmp.Compare(a.i, b.i)) })
	rs := make([]row, len(out))
	for i, x := range out {
		rs[i] = x.row
	}
	return rs
}

// empty dice si la fila no tiene datos: id, orden y las casillas no cuentan.
func empty(v url.Values) bool {
	for k, vals := range v {
		if k == "id" || k == "orden" || k == "borrar" {
			continue
		}
		for _, x := range vals {
			if strings.TrimSpace(x) != "" {
				return false
			}
		}
	}
	return true
}

// splitList separa una lista escrita a mano, por comas o por líneas.
func splitList(s string) []string {
	var out []string
	for _, x := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '\n' }) {
		if x = strings.TrimSpace(x); x != "" {
			out = append(out, x)
		}
	}
	return out
}

// lines separa por líneas (para URLs, que pueden llevar comas).
func lines(s string) []string {
	var out []string
	for _, x := range strings.Split(s, "\n") {
		if x = strings.TrimSpace(x); x != "" {
			out = append(out, x)
		}
	}
	return out
}

func checked(r *http.Request, name string) bool { return r.PostFormValue(name) != "" }

func visibility(public bool) string {
	if public {
		return "public"
	}
	return "private"
}

func field(prefix string, i int, name string) string {
	return prefix + "." + strconv.Itoa(i) + "." + name
}

// pad devuelve n + blankRows, el número de filas que muestra una lista con n elementos.
func pad(n int) int { return n + blankRows }

// editorLinks son las secciones del editor que lista /panel.
var editorLinks = []option{
	{"/panel/basico", "Básico y contacto"},
}

func (s *Server) sections() []editSection {
	return []editSection{
		{path: "/panel/basico", form: BasicsForm, apply: applyBasics},
	}
}

func applyBasics(r *http.Request, res *profile.Resume, _ string) error {
	b := &res.Basics
	b.Name, b.Label, b.Summary, b.URL = r.PostFormValue("name"), r.PostFormValue("label"), r.PostFormValue("summary"), r.PostFormValue("url")
	b.Email, b.Phone = strings.TrimSpace(r.PostFormValue("email")), strings.TrimSpace(r.PostFormValue("phone"))
	res.Gremio.About, res.Gremio.Timezone = strings.TrimSpace(r.PostFormValue("about")), strings.TrimSpace(r.PostFormValue("timezone"))
	l := profile.Location{City: strings.TrimSpace(r.PostFormValue("city")), Region: strings.TrimSpace(r.PostFormValue("region")),
		CountryCode: strings.ToUpper(strings.TrimSpace(r.PostFormValue("country")))}
	b.Location = nil
	if l != (profile.Location{}) {
		b.Location = &l
	}
	c := &res.Gremio.Contact
	c.Email, c.Phone, c.Whatsapp = visibility(checked(r, "email_publico")), visibility(checked(r, "telefono_publico")), visibility(checked(r, "whatsapp_publico"))
	b.Profiles = nil
	for _, x := range rows(r, "red") {
		if x.get("network") == "" {
			return errors.New("cada red necesita un nombre, p. ej. GitHub")
		}
		b.Profiles = append(b.Profiles, profile.Social{Network: x.get("network"), Username: x.get("username"), URL: x.get("url")})
	}
	return nil
}
