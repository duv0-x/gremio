package web

import (
	"cmp"
	"errors"
	"fmt"
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
	{"/panel/skills", "Skills"},
	{"/panel/proyectos", "Proyectos"},
	{"/panel/formacion", "Educación, idiomas y certificados"},
	{"/panel/vistas", "Vistas por rol"},
	{"/panel/atributos", "Atributos de la ficha"},
	{"/panel/rasgos", "Rasgos"},
}

func (s *Server) sections() []editSection {
	return []editSection{
		{path: "/panel/basico", form: BasicsForm, apply: applyBasics},
		{path: "/panel/experiencia", form: WorkListForm, apply: applyWorkList},
		{path: "/panel/skills", form: SkillsForm, apply: applySkills},
		{path: "/panel/proyectos", form: ProjectsForm, apply: applyProjects},
		{path: "/panel/formacion", form: FormationForm, apply: applyFormation},
		{path: "/panel/vistas", form: ViewListForm, apply: applyViewList},
		{path: "/panel/vistas/{id}", form: ViewForm, apply: applyView, exists: func(r *profile.Resume, id string) bool {
			return viewIndex(r, id) >= 0
		}},
		{path: "/panel/atributos", form: AttributeListForm, apply: applyAttributeList},
		{path: "/panel/atributos/{id}", form: AttributeForm, apply: applyAttribute, exists: func(r *profile.Resume, id string) bool {
			return attributeIndex(r, id) >= 0
		}},
		{path: "/panel/rasgos", form: TraitsForm, apply: applyTraits},
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

// at devuelve xs[i], o el valor vacío para las filas nuevas del final.
func at[T any](xs []T, i int) T {
	if i < len(xs) {
		return xs[i]
	}
	var zero T
	return zero
}

var skillLevels = []option{{"", "Sin nivel"}, {"expert", "Experto"}, {"solid", "Sólido"}, {"learning", "Aprendiendo"}}

func applySkills(r *http.Request, res *profile.Resume, _ *string) error {
	res.Skills = nil
	for _, x := range rows(r, "skill") {
		if x.get("name") == "" {
			return errors.New("cada skill necesita un nombre; para quitarla, marca Borrar")
		}
		res.Skills = append(res.Skills, profile.Skill{ID: x.get("id"), Name: x.get("name"), Level: x.get("level"),
			Keywords: splitList(x.get("keywords")), Evidence: lines(x.get("evidence"))})
	}
	return nil
}

func applyProjects(r *http.Request, res *profile.Resume, _ *string) error {
	res.Projects = nil
	for _, x := range rows(r, "proyecto") {
		if x.get("name") == "" {
			return errors.New("cada proyecto necesita un nombre; para quitarlo, marca Borrar")
		}
		res.Projects = append(res.Projects, profile.Project{ID: x.get("id"), Name: x.get("name"), Description: x.get("description"),
			URL: x.get("url"), Image: x.get("image"), Keywords: splitList(x.get("keywords"))})
	}
	return nil
}

func applyFormation(r *http.Request, res *profile.Resume, _ *string) error {
	res.Education, res.Languages, res.Certificates = nil, nil, nil
	for _, x := range rows(r, "estudio") {
		if x.get("institution") == "" {
			return errors.New("cada estudio necesita la institución; para quitarlo, marca Borrar")
		}
		res.Education = append(res.Education, profile.Education{ID: x.get("id"), Institution: x.get("institution"), Area: x.get("area"),
			StudyType: x.get("studyType"), StartDate: x.get("start"), EndDate: x.get("end")})
	}
	for _, x := range rows(r, "idioma") {
		if x.get("language") == "" {
			return errors.New("cada idioma necesita un nombre; para quitarlo, marca Borrar")
		}
		res.Languages = append(res.Languages, profile.Language{Language: x.get("language"), Fluency: x.get("fluency")})
	}
	for _, x := range rows(r, "certificado") {
		if x.get("name") == "" {
			return errors.New("cada certificado necesita un nombre; para quitarlo, marca Borrar")
		}
		res.Certificates = append(res.Certificates, profile.Certificate{Name: x.get("name"), Date: x.get("date"), Issuer: x.get("issuer"), URL: x.get("url")})
	}
	return nil
}

func joinComma(xs []string) string { return strings.Join(xs, ", ") }

// viewIndex y attributeIndex buscan desde el final: un elemento /nuevo se agrega al final y, si su slug o su
// código repite uno existente, el formulario debe volver con el nuevo.
func viewIndex(r *profile.Resume, slug string) int {
	for i := len(r.Gremio.Views) - 1; i >= 0; i-- {
		if r.Gremio.Views[i].Slug == slug {
			return i
		}
	}
	return -1
}

func attributeIndex(r *profile.Resume, code string) int {
	for i := len(r.Gremio.Attributes) - 1; i >= 0; i-- {
		if r.Gremio.Attributes[i].Code == code {
			return i
		}
	}
	return -1
}

func view(e editPage) profile.View {
	if i := viewIndex(e.R, e.ID); i >= 0 {
		return e.R.Gremio.Views[i]
	}
	return profile.View{}
}

func attribute(e editPage) profile.Attribute {
	if i := attributeIndex(e.R, e.ID); i >= 0 {
		return e.R.Gremio.Attributes[i]
	}
	return profile.Attribute{}
}

func titleOr(name, fallback string) string {
	if name != "" {
		return name
	}
	return fallback
}

func applyViewList(r *http.Request, res *profile.Resume, _ *string) error {
	bySlug := map[string]profile.View{}
	for _, v := range res.Gremio.Views {
		bySlug[v.Slug] = v
	}
	res.Gremio.Views = nil
	for _, x := range rows(r, "vista") {
		if v, ok := bySlug[x.get("id")]; ok {
			res.Gremio.Views = append(res.Gremio.Views, v)
		}
	}
	return nil
}

// applyView guarda una vista: sus datos, los pesos de cada atributo y qué trabajos, logros y skills incluye.
func applyView(r *http.Request, res *profile.Resume, id *string) error {
	f := func(k string) string { return strings.TrimSpace(r.PostFormValue(k)) }
	old := profile.View{}
	if i := viewIndex(res, *id); i >= 0 {
		old = res.Gremio.Views[i]
	}
	v := profile.View{Slug: f("slug"), Code: strings.ToUpper(f("code")), Level: f("level"), Name: f("name"), Summary: f("summary"),
		Work: keepOrder(old.Work, r.PostForm["work"]), Highlights: keepOrder(old.Highlights, r.PostForm["highlight"]),
		Skills: keepOrder(old.Skills, r.PostForm["skill"])}
	var bad error
	for _, a := range res.Gremio.Attributes {
		raw := f("peso." + a.Code)
		if raw == "" {
			continue
		}
		w, err := strconv.ParseFloat(strings.ReplaceAll(raw, ",", "."), 64)
		if err != nil || w < 0 {
			bad = fmt.Errorf("el peso de %s debe ser un número positivo (vacío o 0 lo excluye)", a.Code)
			continue
		}
		if w > 0 {
			if v.Weights == nil {
				v.Weights = map[string]float64{}
			}
			v.Weights[a.Code] = w
		}
	}
	if i := viewIndex(res, *id); i >= 0 && *id != "nuevo" {
		res.Gremio.Views[i] = v
	} else {
		res.Gremio.Views = append(res.Gremio.Views, v)
	}
	*id = v.Slug
	return bad
}

func applyAttributeList(r *http.Request, res *profile.Resume, _ *string) error {
	byCode := map[string]profile.Attribute{}
	for _, a := range res.Gremio.Attributes {
		byCode[a.Code] = a
	}
	res.Gremio.Attributes = nil
	for _, x := range rows(r, "atributo") {
		if a, ok := byCode[x.get("id")]; ok {
			res.Gremio.Attributes = append(res.Gremio.Attributes, a)
		}
	}
	return nil
}

// applyAttribute guarda un atributo con sus sub-atributos y lo que lo respalda. Si cambia el código, los pesos
// de las vistas lo siguen.
func applyAttribute(r *http.Request, res *profile.Resume, id *string) error {
	f := func(k string) string { return strings.TrimSpace(r.PostFormValue(k)) }
	old := profile.Attribute{}
	if i := attributeIndex(res, *id); i >= 0 {
		old = res.Gremio.Attributes[i]
	}
	a := profile.Attribute{Code: strings.ToUpper(f("code")), Name: f("name"),
		Skills: keepOrder(old.Skills, r.PostForm["skill"]), Highlights: keepOrder(old.Highlights, r.PostForm["highlight"])}
	var bad error
	for _, x := range rows(r, "sub") {
		score, err := strconv.Atoi(x.get("score"))
		if err != nil {
			bad = fmt.Errorf("el puntaje de %q debe ser un número de 0 a 99", x.get("name"))
		}
		a.Subs = append(a.Subs, profile.Sub{Name: x.get("name"), Score: score, Learning: x.get("learning") != ""})
	}
	if i := attributeIndex(res, *id); i >= 0 && *id != "nuevo" {
		res.Gremio.Attributes[i] = a
		if a.Code != *id {
			for _, v := range res.Gremio.Views {
				if w, ok := v.Weights[*id]; ok {
					delete(v.Weights, *id)
					v.Weights[a.Code] = w
				}
			}
		}
	} else {
		res.Gremio.Attributes = append(res.Gremio.Attributes, a)
	}
	*id = a.Code
	return bad
}

var traitColors = func() []option {
	out := []option{{"", "Sin color"}}
	for _, c := range profile.TraitColors {
		out = append(out, option{c, c})
	}
	return out
}()

func applyTraits(r *http.Request, res *profile.Resume, _ *string) error {
	res.Gremio.Traits = nil
	for _, x := range rows(r, "rasgo") {
		if x.get("name") == "" {
			return errors.New("cada rasgo necesita un nombre; para quitarlo, marca Borrar")
		}
		res.Gremio.Traits = append(res.Gremio.Traits, profile.Trait{Name: x.get("name"), Summary: x.get("summary"), Icon: x.get("icon"), Color: x.get("color")})
	}
	return nil
}

var viewLevels = []option{{"", "Sin nivel"}, {"jr", "Junior"}, {"semi-sr", "Semi Senior"}, {"sr", "Senior"}}

func weightValue(v profile.View, code string) string {
	if w, ok := v.Weights[code]; ok {
		return strconv.FormatFloat(w, 'f', -1, 64)
	}
	return ""
}

func scoreValue(s profile.Sub, isNew bool) string {
	if isNew {
		return ""
	}
	return strconv.Itoa(s.Score)
}

func workOptions(r *profile.Resume) []option {
	var out []option
	for _, w := range r.Work {
		out = append(out, option{w.ID, w.Name})
	}
	return out
}

// highlightOptions son los logros con su trabajo, para reconocerlos fuera de contexto.
func highlightOptions(r *profile.Resume) []option {
	var out []option
	for _, w := range r.Work {
		for _, h := range w.Highlights {
			out = append(out, option{h.ID, w.Name + ": " + h.Text})
		}
	}
	return out
}

func skillOptions(r *profile.Resume) []option {
	var out []option
	for _, s := range r.Skills {
		out = append(out, option{s.ID, s.Name})
	}
	return out
}

// keepOrder devuelve los ids marcados: los que ya estaban, en su orden (que pudo elegirse a mano en el JSON), y
// después los nuevos, en el orden de las casillas.
func keepOrder(old, selected []string) []string {
	var out []string
	for _, id := range old {
		if slices.Contains(selected, id) {
			out = append(out, id)
		}
	}
	for _, id := range selected {
		if !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}
