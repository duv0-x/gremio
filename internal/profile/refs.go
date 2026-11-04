package profile

import "slices"

// PruneRefs quita de vistas y atributos las referencias a trabajos, logros, skills y atributos que ya no
// existen: borrar un elemento desde el panel no debe dejar el perfil inválido.
func (r *Resume) PruneRefs() {
	works, highlights, skills, codes := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, w := range r.Work {
		works[w.ID] = true
		for _, h := range w.Highlights {
			highlights[h.ID] = true
		}
	}
	for _, s := range r.Skills {
		skills[s.ID] = true
	}
	for _, a := range r.Gremio.Attributes {
		codes[a.Code] = true
	}
	keep := func(ids []string, known map[string]bool) []string {
		return slices.DeleteFunc(ids, func(id string) bool { return !known[id] })
	}
	for i := range r.Gremio.Attributes {
		a := &r.Gremio.Attributes[i]
		a.Skills, a.Highlights = keep(a.Skills, skills), keep(a.Highlights, highlights)
	}
	for i := range r.Gremio.Views {
		v := &r.Gremio.Views[i]
		v.Work, v.Highlights, v.Skills = keep(v.Work, works), keep(v.Highlights, highlights), keep(v.Skills, skills)
		for c := range v.Weights {
			if !codes[c] {
				delete(v.Weights, c)
			}
		}
	}
}
