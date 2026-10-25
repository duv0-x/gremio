// Package profile es el modelo del perfil (ADR-003): un documento JSON Resume v1.0.0 con ids estables
// y una extensión propia bajo la clave "gremio".
package profile

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Resume es el perfil completo. Las secciones de JSON Resume que no usa el v1 no se guardan.
type Resume struct {
	Basics       Basics        `json:"basics"`
	Work         []Work        `json:"work,omitempty"`
	Education    []Education   `json:"education,omitempty"`
	Skills       []Skill       `json:"skills,omitempty"`
	Languages    []Language    `json:"languages,omitempty"`
	Certificates []Certificate `json:"certificates,omitempty"`
	Projects     []Project     `json:"projects,omitempty"`
	Gremio       Extension     `json:"gremio"`
}

// Basics no tiene fecha de nacimiento, género ni nacionalidad: el modelo no los admite (ADR-003).
type Basics struct {
	Name     string    `json:"name"`
	Label    string    `json:"label,omitempty"`
	Image    string    `json:"image,omitempty"`
	Email    string    `json:"email,omitempty"`
	Phone    string    `json:"phone,omitempty"`
	URL      string    `json:"url,omitempty"`
	Summary  string    `json:"summary,omitempty"`
	Location *Location `json:"location,omitempty"`
	Profiles []Social  `json:"profiles,omitempty"`
}

type Location struct {
	City        string `json:"city,omitempty"`
	Region      string `json:"region,omitempty"`
	CountryCode string `json:"countryCode,omitempty"`
}

type Social struct {
	Network  string `json:"network"`
	Username string `json:"username,omitempty"`
	URL      string `json:"url,omitempty"`
}

type Work struct {
	ID         string      `json:"id"`
	Name       string      `json:"name"`
	Position   string      `json:"position,omitempty"`
	Location   string      `json:"location,omitempty"`
	URL        string      `json:"url,omitempty"`
	StartDate  string      `json:"startDate,omitempty"`
	EndDate    string      `json:"endDate,omitempty"`
	Summary    string      `json:"summary,omitempty"`
	Highlights []Highlight `json:"highlights,omitempty"`
}

// Highlight es un logro. En JSON Resume es un string; aquí lleva id y evidencia, y se acepta en las dos formas.
type Highlight struct {
	ID       string   `json:"id"`
	Text     string   `json:"text"`
	Evidence []string `json:"evidence,omitempty"`
}

func (h *Highlight) UnmarshalJSON(b []byte) error {
	if s := ""; json.Unmarshal(b, &s) == nil {
		*h = Highlight{Text: s}
		return nil
	}
	type plain Highlight
	return json.Unmarshal(b, (*plain)(h))
}

type Education struct {
	ID          string `json:"id"`
	Institution string `json:"institution"`
	Area        string `json:"area,omitempty"`
	StudyType   string `json:"studyType,omitempty"`
	StartDate   string `json:"startDate,omitempty"`
	EndDate     string `json:"endDate,omitempty"`
}

// Skill.Level es "expert", "solid", "learning" o vacío.
type Skill struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Level    string   `json:"level,omitempty"`
	Keywords []string `json:"keywords,omitempty"`
	Evidence []string `json:"evidence,omitempty"`
}

type Language struct {
	Language string `json:"language"`
	Fluency  string `json:"fluency,omitempty"`
}

type Certificate struct {
	Name   string `json:"name"`
	Date   string `json:"date,omitempty"`
	Issuer string `json:"issuer,omitempty"`
	URL    string `json:"url,omitempty"`
}

type Project struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	URL         string   `json:"url,omitempty"`
	Keywords    []string `json:"keywords,omitempty"`
}

// Extension son los datos propios de Gremio, que el JSON Resume estándar no tiene.
type Extension struct {
	// About es la presentación larga de la página web; basics.summary queda como bio corta, que es lo que
	// muestran Mastodon y las demás instancias.
	About      string      `json:"about,omitempty"`
	Contact    Contact     `json:"contact"`
	Views      []View      `json:"views,omitempty"`
	Attributes []Attribute `json:"attributes,omitempty"`
	Traits     []Trait     `json:"traits,omitempty"`
}

// Attribute es un eje de la ficha (ADR-010): su puntaje es el promedio de sus sub-atributos.
type Attribute struct {
	Code       string   `json:"code"`
	Name       string   `json:"name"`
	Subs       []Sub    `json:"subs"`
	Skills     []string `json:"skills,omitempty"`
	Highlights []string `json:"highlights,omitempty"`
}

// Sub es un sub-atributo con puntaje autodeclarado de 0 a 99.
type Sub struct {
	Name     string `json:"name"`
	Score    int    `json:"score"`
	Learning bool   `json:"learning,omitempty"`
}

// Score es el promedio redondeado de los sub-atributos.
func (a Attribute) Score() int {
	if len(a.Subs) == 0 {
		return 0
	}
	sum := 0
	for _, s := range a.Subs {
		sum += s.Score
	}
	return (2*sum + len(a.Subs)) / (2 * len(a.Subs))
}

type Trait struct {
	Name    string `json:"name"`
	Summary string `json:"summary,omitempty"`
}

// Contact dice qué datos de contacto son públicos; por defecto, ninguno (ADR-003 §4).
type Contact struct {
	Email string `json:"email"` // "public" o "private"
	Phone string `json:"phone"`
}

// View es una vista por rol: un filtro sobre el mismo perfil, referenciado por ids (ADR-003 §3).
type View struct {
	Slug       string             `json:"slug"`
	Code       string             `json:"code,omitempty"` // posición: PLE, SRE…
	Name       string             `json:"name"`
	Weights    map[string]float64 `json:"weights,omitempty"`
	Summary    string   `json:"summary,omitempty"`
	Work       []string `json:"work,omitempty"`
	Highlights []string `json:"highlights,omitempty"`
	Skills     []string `json:"skills,omitempty"`
}

// Overall es el OVR de la vista: el promedio de los atributos ponderado por sus pesos (ADR-010 §3).
// Sin pesos, todos los atributos pesan igual.
func (r *Resume) Overall(v View) int {
	var sum, total float64
	for _, a := range r.Gremio.Attributes {
		w := 1.0
		if len(v.Weights) > 0 {
			w = v.Weights[a.Code]
		}
		sum += w * float64(a.Score())
		total += w
	}
	if total == 0 {
		return 0
	}
	return int(sum/total + 0.5)
}

var (
	codeRe = regexp.MustCompile(`^[A-Z]{2,4}$`)
	dateRe = regexp.MustCompile(`^\d{4}(-\d{2}(-\d{2})?)?$`)
	slugRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	// Claves que el modelo no admite; si vienen en un archivo importado se descartan y se avisa.
	forbidden = []string{"birthDate", "birthdate", "dateOfBirth", "age", "gender", "sex", "nationality", "maritalStatus", "salary"}
)

// Parse lee un perfil importado (JSON Resume, con o sin la extensión), lo normaliza y lo valida.
// Devuelve avisos de lo que se descartó.
func Parse(data []byte) (*Resume, []string, error) {
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, nil, fmt.Errorf("JSON inválido: %w", err)
	}
	warnings := forbiddenKeys(raw, "")
	var r Resume
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, nil, fmt.Errorf("el archivo no sigue el esquema de JSON Resume: %w", err)
	}
	if err := r.Normalize(); err != nil {
		return nil, nil, err
	}
	return &r, warnings, nil
}

func forbiddenKeys(v any, path string) []string {
	var out []string
	switch v := v.(type) {
	case map[string]any:
		for k, child := range v {
			p := strings.TrimPrefix(path+"."+k, ".")
			if slices.Contains(forbidden, k) {
				out = append(out, fmt.Sprintf("se descartó %s: el perfil no guarda ese dato", p))
				continue
			}
			out = append(out, forbiddenKeys(child, p)...)
		}
	case []any:
		for i, child := range v {
			out = append(out, forbiddenKeys(child, fmt.Sprintf("%s[%d]", path, i))...)
		}
	}
	slices.Sort(out)
	return out
}

// Normalize recorta espacios, asigna ids a los elementos que no tienen y valida el documento.
func (r *Resume) Normalize() error {
	r.Basics.Name = strings.TrimSpace(r.Basics.Name)
	if r.Basics.Name == "" {
		return errors.New("basics.name es obligatorio")
	}
	seen := map[string]bool{}
	id := func(cur *string, what string) error {
		*cur = strings.TrimSpace(*cur)
		if *cur == "" {
			*cur = newID()
		}
		if seen[*cur] {
			return fmt.Errorf("id repetido %q (%s)", *cur, what)
		}
		seen[*cur] = true
		return nil
	}
	date := func(d, what string) error {
		if d != "" && !dateRe.MatchString(d) {
			return fmt.Errorf("%s: fecha %q inválida (usa AAAA, AAAA-MM o AAAA-MM-DD)", what, d)
		}
		return nil
	}
	works, highlights, skills := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for i := range r.Work {
		w := &r.Work[i]
		what := fmt.Sprintf("work[%d]", i)
		if err := id(&w.ID, what); err != nil {
			return err
		}
		works[w.ID] = true
		if strings.TrimSpace(w.Name) == "" {
			return fmt.Errorf("%s: falta name", what)
		}
		if err := date(w.StartDate, what); err != nil {
			return err
		}
		if err := date(w.EndDate, what); err != nil {
			return err
		}
		for j := range w.Highlights {
			h := &w.Highlights[j]
			if err := id(&h.ID, fmt.Sprintf("%s.highlights[%d]", what, j)); err != nil {
				return err
			}
			h.Text = strings.TrimSpace(h.Text)
			highlights[h.ID] = true
		}
	}
	for i := range r.Education {
		e := &r.Education[i]
		if err := id(&e.ID, fmt.Sprintf("education[%d]", i)); err != nil {
			return err
		}
		if err := date(e.StartDate, "education"); err != nil {
			return err
		}
		if err := date(e.EndDate, "education"); err != nil {
			return err
		}
	}
	for i := range r.Skills {
		s := &r.Skills[i]
		if err := id(&s.ID, fmt.Sprintf("skills[%d]", i)); err != nil {
			return err
		}
		skills[s.ID] = true
		switch s.Level {
		case "", "expert", "solid", "learning":
		default:
			return fmt.Errorf("skills[%d]: nivel %q inválido (expert, solid o learning)", i, s.Level)
		}
	}
	for i := range r.Projects {
		if err := id(&r.Projects[i].ID, fmt.Sprintf("projects[%d]", i)); err != nil {
			return err
		}
	}

	c := &r.Gremio.Contact
	for _, v := range []*string{&c.Email, &c.Phone} {
		switch *v {
		case "public", "private":
		case "":
			*v = "private"
		default:
			return fmt.Errorf("gremio.contact: visibilidad %q inválida (public o private)", *v)
		}
	}
	codes := map[string]bool{}
	if n := len(r.Gremio.Attributes); n > 0 && (n < 3 || n > 8) {
		return fmt.Errorf("gremio.attributes: el radar necesita de 3 a 8 atributos, hay %d", n)
	}
	for i := range r.Gremio.Attributes {
		a := &r.Gremio.Attributes[i]
		what := fmt.Sprintf("gremio.attributes[%d]", i)
		if !codeRe.MatchString(a.Code) || codes[a.Code] {
			return fmt.Errorf("%s: código %q inválido o repetido (2 a 4 letras mayúsculas)", what, a.Code)
		}
		codes[a.Code] = true
		if strings.TrimSpace(a.Name) == "" || len(a.Subs) == 0 {
			return fmt.Errorf("%s: falta name o subs", what)
		}
		for j, sub := range a.Subs {
			if strings.TrimSpace(sub.Name) == "" || sub.Score < 0 || sub.Score > 99 {
				return fmt.Errorf("%s.subs[%d]: falta name o el puntaje no está entre 0 y 99", what, j)
			}
		}
		if err := refs(what, a.Skills, skills, "skills"); err != nil {
			return err
		}
		if err := refs(what, a.Highlights, highlights, "highlights"); err != nil {
			return err
		}
	}
	slugs := map[string]bool{}
	for i, v := range r.Gremio.Views {
		what := fmt.Sprintf("gremio.views[%d]", i)
		if !slugRe.MatchString(v.Slug) || slugs[v.Slug] {
			return fmt.Errorf("%s: slug %q inválido o repetido", what, v.Slug)
		}
		slugs[v.Slug] = true
		if strings.TrimSpace(v.Name) == "" {
			return fmt.Errorf("%s: falta name", what)
		}
		if v.Code != "" && !codeRe.MatchString(v.Code) {
			return fmt.Errorf("%s: código %q inválido (2 a 4 letras mayúsculas)", what, v.Code)
		}
		for c, w := range v.Weights {
			if !codes[c] || w <= 0 {
				return fmt.Errorf("%s.weights: el atributo %q no existe o su peso no es positivo", what, c)
			}
		}
		if err := refs(what, v.Work, works, "work"); err != nil {
			return err
		}
		if err := refs(what, v.Highlights, highlights, "highlights"); err != nil {
			return err
		}
		if err := refs(what, v.Skills, skills, "skills"); err != nil {
			return err
		}
	}
	for i, t := range r.Gremio.Traits {
		if strings.TrimSpace(t.Name) == "" {
			return fmt.Errorf("gremio.traits[%d]: falta name", i)
		}
	}
	return nil
}

func refs(what string, ids []string, known map[string]bool, kind string) error {
	for _, x := range ids {
		if !known[x] {
			return fmt.Errorf("%s.%s: no existe el id %q", what, kind, x)
		}
	}
	return nil
}

// Export devuelve el perfil como JSON Resume estándar: logros como strings, sin ids ni extensión.
// Con extensions, devuelve el documento tal como se guarda.
func (r *Resume) Export(extensions bool) ([]byte, error) {
	if extensions {
		return marshal(r)
	}
	type work struct {
		Work
		ID         string   `json:"id,omitempty"`
		Highlights []string `json:"highlights,omitempty"`
	}
	out := map[string]any{"basics": r.Basics}
	if len(r.Work) > 0 {
		ws := make([]work, len(r.Work))
		for i, w := range r.Work {
			ws[i] = work{Work: w}
			for _, h := range w.Highlights {
				ws[i].Highlights = append(ws[i].Highlights, h.Text)
			}
		}
		out["work"] = ws
	}
	for k, v := range map[string]any{"education": r.Education, "skills": r.Skills, "languages": r.Languages, "certificates": r.Certificates, "projects": r.Projects} {
		if b, _ := json.Marshal(v); string(b) != "null" {
			var items []map[string]any
			_ = json.Unmarshal(b, &items)
			for _, it := range items {
				delete(it, "id")
				delete(it, "evidence")
			}
			out[k] = items
		}
	}
	return marshal(out)
}

func marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	err := enc.Encode(v)
	return buf.Bytes(), err
}

// Load lee el documento guardado. Los perfiles anteriores a este modelo (solo basics) también se leen.
func Load(doc string) (*Resume, error) {
	var r Resume
	if err := json.Unmarshal([]byte(doc), &r); err != nil {
		return nil, err
	}
	return &r, r.Normalize()
}

// Diff resume, por sección, cuántos elementos tiene el perfil actual y cuántos tendrá el importado.
func Diff(cur, next *Resume) []string {
	row := func(name string, a, b int) string {
		return fmt.Sprintf("%-13s %3d → %3d", name, a, b)
	}
	return []string{
		fmt.Sprintf("%-13s %q → %q", "nombre", cur.Basics.Name, next.Basics.Name),
		row("experiencia", len(cur.Work), len(next.Work)),
		row("logros", countHighlights(cur), countHighlights(next)),
		row("educación", len(cur.Education), len(next.Education)),
		row("skills", len(cur.Skills), len(next.Skills)),
		row("idiomas", len(cur.Languages), len(next.Languages)),
		row("certificados", len(cur.Certificates), len(next.Certificates)),
		row("proyectos", len(cur.Projects), len(next.Projects)),
		row("vistas", len(cur.Gremio.Views), len(next.Gremio.Views)),
		row("atributos", len(cur.Gremio.Attributes), len(next.Gremio.Attributes)),
	}
}

func countHighlights(r *Resume) int {
	n := 0
	for _, w := range r.Work {
		n += len(w.Highlights)
	}
	return n
}

func newID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
