// Package retos son los retos del dojo y sus ejecuciones (ADR-002 enmendado por ADR-013): el enunciado, el
// catálogo revisado que viaja en el binario, los checks reproducibles y la confianza que da cada ejecución.
package retos

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/duv0-x/gremio/internal/dojo"
)

// Challenge es el enunciado de un reto, con la estructura del mockup Retos y ADR-002 §1. Es el mismo formato para
// el catálogo revisado y para los retos propios.
type Challenge struct {
	ID      string `json:"id"` // slug en el catálogo; id local en un reto propio
	Version int    `json:"version"`

	Title             string            `json:"title"`
	Pitch             string            `json:"pitch"`
	Context           string            `json:"context"` // contexto de negocio
	Level             dojo.Level        `json:"level"`
	Skills            []dojo.SkillLevel `json:"skills"` // nivel por habilidad: máx. 3, ninguno pasa Level (ADR-013 §3)
	Disciplines       []string          `json:"disciplines,omitempty"`
	Stack             []string          `json:"stack,omitempty"`
	Requirements      []string          `json:"requirements"`
	Constraints       []string          `json:"constraints,omitempty"`
	DefinitionOfDone  []string          `json:"definitionOfDone"`
	Deliverables      []string          `json:"deliverables,omitempty"`
	EstimatedDuration string            `json:"estimatedDuration,omitempty"` // xsd:duration, p. ej. P2W
	License           string            `json:"license"`                     // CC-BY-4.0 por defecto (ADR-002 §6)

	Origin string `json:"origin"`          // human | ai (ADR-002 §4)
	Model  string `json:"model,omitempty"` // modelo de IA si origin es ai
	// Author es la persona que lo propuso o adoptó el borrador de IA; Reviewer, quien lo revisó en el PR del
	// catálogo (handles de GitHub). La IA nunca es autor ni revisor. Sin revisor, o con revisor == autor, el reto
	// está "en revisión" y cuenta como práctica propia aunque esté en el repo.
	Author   string `json:"author"`
	Reviewer string `json:"reviewer,omitempty"`
	BasedOn  string `json:"basedOn,omitempty"` // id raíz del linaje, si es copia o fork de otro reto

	Checks []Check `json:"checks"`
}

// LicenseDefault es la licencia de contenido por defecto (ADR-002 §6).
const LicenseDefault = "CC-BY-4.0"

var slugRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// Validate revisa el enunciado. Una habilidad fuera del catálogo se acepta en un reto propio (las reglas la
// ignoran, ADR-013 §2); el catálogo revisado exige habilidades conocidas (ver Catalog).
func (c Challenge) Validate() error {
	if !slugRe.MatchString(c.ID) {
		return fmt.Errorf("id %q: usa minúsculas, números y guiones", c.ID)
	}
	if c.Version < 1 {
		return fmt.Errorf("%s: versión %d inválida", c.ID, c.Version)
	}
	if strings.TrimSpace(c.Title) == "" || len(c.Requirements) == 0 || len(c.DefinitionOfDone) == 0 {
		return fmt.Errorf("%s: faltan título, requisitos o Definition of Done", c.ID)
	}
	if !c.Level.Valid() {
		return fmt.Errorf("%s: nivel %d fuera de N1–N4", c.ID, c.Level)
	}
	if len(c.Skills) == 0 || len(c.Skills) > dojo.MaxSkills {
		return fmt.Errorf("%s: un reto ejercita de 1 a %d habilidades", c.ID, dojo.MaxSkills)
	}
	seen := map[string]bool{}
	for _, s := range c.Skills {
		if s.Skill == "" || seen[s.Skill] {
			return fmt.Errorf("%s: habilidad vacía o repetida %q", c.ID, s.Skill)
		}
		seen[s.Skill] = true
		if !s.Level.Valid() || s.Level > c.Level {
			return fmt.Errorf("%s: el nivel en %s tiene que estar entre N1 y %s", c.ID, s.Skill, c.Level)
		}
	}
	if c.Origin != "human" && c.Origin != "ai" {
		return fmt.Errorf("%s: origin %q: human o ai", c.ID, c.Origin)
	}
	if c.License == "" {
		return fmt.Errorf("%s: falta la licencia", c.ID)
	}
	ids := map[string]bool{}
	for _, ch := range c.Checks {
		if err := ch.Validate(); err != nil {
			return fmt.Errorf("%s: %w", c.ID, err)
		}
		if ids[ch.ID] {
			return fmt.Errorf("%s: check %s repetido", c.ID, ch.ID)
		}
		ids[ch.ID] = true
	}
	return nil
}

// Reviewed dice si el reto es del catálogo revisado: revisor visible y distinto del autor (ADR-013 §8).
func (c Challenge) Reviewed() bool {
	return c.Reviewer != "" && !strings.EqualFold(c.Reviewer, c.Author)
}

// CatalogPrefix antecede al slug en el id de linaje de un reto del catálogo (Event.Challenge).
const CatalogPrefix = "catalog:"

// MinCatalogChecks es cuántos checks pide como mínimo un reto del catálogo: un solo check trivial no prueba que
// se resolvió.
const MinCatalogChecks = 4

//go:embed catalog/*.json
var catalogFS embed.FS

// Catalog es el catálogo revisado de retos que viaja en el binario: cada slug con sus versiones, de la 1 a la
// última. Se carga al iniciar; un archivo inválido es un error de compilación del catálogo (lo atrapa el test).
var Catalog = mustLoad(catalogFS)

func mustLoad(fsys embed.FS) map[string]CatalogEntry {
	sub, err := fs.Sub(fsys, "catalog")
	if err == nil {
		var c map[string]CatalogEntry
		if c, err = load(sub); err == nil {
			return c
		}
	}
	panic(err)
}

// AddDir suma al catálogo los retos de un directorio (<slug>.v<N>.json), para QA y tests con retos revisados
// ficticios (gremio serve --dojo-catalog). Un slug que ya está en el catálogo es un error. Se llama al iniciar.
func AddDir(dir string) error {
	extra, err := load(os.DirFS(dir))
	if err != nil {
		return err
	}
	for slug := range extra {
		if _, ok := Catalog[slug]; ok {
			return fmt.Errorf("%s: el reto %s ya está en el catálogo", dir, slug)
		}
	}
	maps.Copy(Catalog, extra)
	return nil
}

// CatalogEntry es un reto del catálogo con todas sus versiones; Versions[i] es la versión i+1.
type CatalogEntry struct {
	Versions []Challenge
}

// Latest es la última versión.
func (e CatalogEntry) Latest() Challenge { return e.Versions[len(e.Versions)-1] }

// Version devuelve la versión n, si existe.
func (e CatalogEntry) Version(n int) (Challenge, bool) {
	if n < 1 || n > len(e.Versions) {
		return Challenge{}, false
	}
	return e.Versions[n-1], true
}

// Find busca un reto del catálogo por slug.
func Find(slug string) (CatalogEntry, bool) {
	e, ok := Catalog[slug]
	return e, ok
}

// Slugs devuelve los slugs del catálogo ordenados.
func Slugs() []string {
	out := make([]string, 0, len(Catalog))
	for s := range Catalog {
		out = append(out, s)
	}
	slices.Sort(out)
	return out
}

// load lee catalog/<slug>.v<N>.json. Las versiones son inmutables: una versión nueva es un archivo nuevo.
func load(fsys fs.FS) (map[string]CatalogEntry, error) {
	files, err := fs.Glob(fsys, "*.json")
	if err != nil {
		return nil, err
	}
	byID := map[string][]Challenge{}
	for _, f := range files {
		slug, v, ok := strings.Cut(strings.TrimSuffix(path.Base(f), ".json"), ".v")
		n, err := strconv.Atoi(v)
		if !ok || err != nil {
			return nil, fmt.Errorf("%s: el nombre tiene que ser <slug>.v<N>.json", f)
		}
		raw, err := fs.ReadFile(fsys, f)
		if err != nil {
			return nil, err
		}
		var c Challenge
		dec := json.NewDecoder(strings.NewReader(string(raw)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&c); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		if c.ID != slug || c.Version != n {
			return nil, fmt.Errorf("%s: id %q y versión %d no coinciden con el nombre del archivo", f, c.ID, c.Version)
		}
		if err := validateCatalog(c); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		byID[slug] = append(byID[slug], c)
	}
	out := map[string]CatalogEntry{}
	for slug, vs := range byID {
		slices.SortFunc(vs, func(a, b Challenge) int { return a.Version - b.Version })
		for i, c := range vs {
			if c.Version != i+1 {
				return nil, fmt.Errorf("catálogo %s: falta la versión %d", slug, i+1)
			}
		}
		out[slug] = CatalogEntry{Versions: vs}
	}
	return out, nil
}

func validateCatalog(c Challenge) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if c.BasedOn != "" {
		return errors.New("un reto del catálogo es una raíz de linaje: no lleva basedOn")
	}
	for _, s := range c.Skills {
		if _, ok := dojo.FindSkill(s.Skill); !ok {
			return fmt.Errorf("habilidad %q fuera del catálogo de habilidades", s.Skill)
		}
	}
	if len(c.Checks) < MinCatalogChecks {
		return fmt.Errorf("el catálogo pide al menos %d checks, tiene %d", MinCatalogChecks, len(c.Checks))
	}
	if c.Author == "" {
		return errors.New("falta el autor")
	}
	if c.Origin == "ai" && c.Model == "" {
		return errors.New("un reto de origen ai declara el modelo")
	}
	return nil
}

// Trust es la confianza que da una ejecución con verificación aprobada (ADR-013 §4). Un reto del catálogo
// revisado con checks aprobados en un commit fijado es reproducible; cualquier otro (propio, de IA o del catálogo
// todavía en revisión) es práctica propia. Verificada por terceros llegará con los checks de CI firmados.
func Trust(c Challenge, fromCatalog bool, v Verification) (dojo.Trust, error) {
	if !v.Passed {
		return "", errors.New("sin verificación aprobada no hay ganancia: corre los checks y que pasen todos")
	}
	if fromCatalog && c.Reviewed() && len(c.Checks) >= MinCatalogChecks {
		return dojo.Reproducible, nil
	}
	return dojo.SelfPractice, nil
}
