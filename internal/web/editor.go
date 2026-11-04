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
	Path    string // la URL de la página, que es también el action del formulario
}

// editSection es una página del editor: su formulario y cómo aplicarlo al perfil.
type editSection struct {
	path string
	form func(editPage) templ.Component
	// apply aplica el formulario. En /nuevo, cambia *id por el del elemento creado, para que si el perfil no
	// valida el formulario vuelva con lo escrito.
	apply func(r *http.Request, res *profile.Resume, id *string) error
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
	e := editPage{Version: s.version, R: res, ID: id, Path: r.URL.Path}
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
	e := editPage{Version: s.version, R: res, ID: id, Path: r.URL.Path}
	if err := sec.apply(r, res, &e.ID); err != nil {
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
	// Un elemento (un trabajo, una vista) vuelve a su lista: su URL puede haber cambiado o ser /nuevo.
	back := r.URL.Path
	if sec.exists != nil {
		back = back[:strings.LastIndex(back, "/")]
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

// empty dice si la fila no tiene datos: el orden y la casilla de borrar no cuentan, el id oculto sí (una fila
// que ya existía nunca se descarta por vacía).
func empty(v url.Values) bool {
	for k, vals := range v {
		if k == "orden" || k == "borrar" {
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
	{"/panel/experiencia", "Experiencia"},
}

func (s *Server) sections() []editSection {
	return []editSection{
		{path: "/panel/basico", form: BasicsForm, apply: applyBasics},
		{path: "/panel/experiencia", form: WorkListForm, apply: applyWorkList},
		{path: "/panel/experiencia/{id}", form: WorkForm, apply: applyWork, exists: func(r *profile.Resume, id string) bool {
			return slices.ContainsFunc(r.Work, func(w profile.Work) bool { return w.ID == id })
		}},
	}
}

func applyBasics(r *http.Request, res *profile.Resume, _ *string) error {
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

// applyWorkList ordena y borra trabajos; los datos de cada uno se editan en su página.
func applyWorkList(r *http.Request, res *profile.Resume, _ *string) error {
	byID := map[string]profile.Work{}
	for _, w := range res.Work {
		byID[w.ID] = w
	}
	res.Work = nil
	for _, x := range rows(r, "trabajo") {
		if w, ok := byID[x.get("id")]; ok {
			res.Work = append(res.Work, w)
		}
	}
	return nil
}

// applyWork guarda un trabajo y sus logros; uno nuevo va primero, como el más reciente.
func applyWork(r *http.Request, res *profile.Resume, id *string) error {
	f := r.PostFormValue
	w := profile.Work{ID: *id, Name: f("name"), Position: f("position"), Location: f("location"), URL: strings.TrimSpace(f("url")),
		StartDate: strings.TrimSpace(f("start")), EndDate: strings.TrimSpace(f("end")), Summary: strings.TrimSpace(f("summary"))}
	for _, x := range rows(r, "logro") {
		w.Highlights = append(w.Highlights, profile.Highlight{ID: x.get("id"), Text: x.get("text"), Evidence: lines(x.get("evidence"))})
	}
	if i := slices.IndexFunc(res.Work, func(x profile.Work) bool { return x.ID == *id }); i >= 0 {
		res.Work[i] = w
	} else {
		w.ID = profile.NewID()
		*id = w.ID
		res.Work = append([]profile.Work{w}, res.Work...)
	}
	if strings.TrimSpace(w.Name) == "" {
		return errors.New("falta el nombre de la empresa u organización")
	}
	for _, h := range w.Highlights {
		if h.Text == "" {
			return errors.New("cada logro necesita un texto; para quitarlo, marca Borrar")
		}
	}
	return nil
}

func work(e editPage) profile.Work {
	if i := slices.IndexFunc(e.R.Work, func(w profile.Work) bool { return w.ID == e.ID }); i >= 0 {
		return e.R.Work[i]
	}
	return profile.Work{}
}

func workTitle(e editPage) string {
	if w := work(e); w.Name != "" {
		return w.Name
	}
	return "Nuevo trabajo"
}

func highlight(e editPage, i int) profile.Highlight {
	if hs := work(e).Highlights; i < len(hs) {
		return hs[i]
	}
	return profile.Highlight{}
}

func joinLines(xs []string) string { return strings.Join(xs, "\n") }
