package web

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/duv0-x/gremio/internal/profile"
)

// Geometría del radar (ADR-010 §5): SVG renderizado en el servidor, porque la CSP no admite style inline.
const (
	radarSize = 340.0
	radarC    = radarSize / 2
	radarR    = 112.0
)

type radarLabel struct {
	X, Y   string
	Anchor string
	Code   string
	Name   string
	Score  int
}

// radarPoint da el punto del eje i de n a la fracción f del radio. El primer eje apunta hacia arriba.
func radarPoint(i, n int, f float64) (float64, float64) {
	a := -math.Pi/2 + 2*math.Pi*float64(i)/float64(n)
	return radarC + f*radarR*math.Cos(a), radarC + f*radarR*math.Sin(a)
}

func points(n int, frac func(i int) float64) string {
	var b strings.Builder
	for i := range n {
		x, y := radarPoint(i, n, frac(i))
		fmt.Fprintf(&b, "%.1f,%.1f ", x, y)
	}
	return strings.TrimSpace(b.String())
}

func radarRing(n int, f float64) string {
	return points(n, func(int) float64 { return f })
}

func radarShape(attrs []profile.Attribute) string {
	return points(len(attrs), func(i int) float64 { return float64(attrs[i].Score()) / 100 })
}

func radarAxis(i, n int) (string, string) {
	x, y := radarPoint(i, n, 1)
	return fmt.Sprintf("%.1f", x), fmt.Sprintf("%.1f", y)
}

func radarLabels(attrs []profile.Attribute) []radarLabel {
	out := make([]radarLabel, len(attrs))
	for i, a := range attrs {
		x, y := radarPoint(i, len(attrs), 1.22)
		anchor := "middle"
		switch {
		case x < radarC-8:
			anchor = "end"
		case x > radarC+8:
			anchor = "start"
		}
		out[i] = radarLabel{X: fmt.Sprintf("%.1f", x), Y: fmt.Sprintf("%.1f", y+4), Anchor: anchor, Code: a.Code, Name: a.Name, Score: a.Score()}
	}
	return out
}

// hexagon es el contorno de la insignia del OVR, con la punta hacia arriba.
func hexagon(cx, cy, r float64) string {
	var b strings.Builder
	for i := range 6 {
		a := -math.Pi/2 + math.Pi/3*float64(i)
		fmt.Fprintf(&b, "%.1f,%.1f ", cx+r*math.Cos(a), cy+r*math.Sin(a))
	}
	return strings.TrimSpace(b.String())
}

// scoreClass colorea el puntaje: ≥85 acento, 70–84 tinta, <70 apagado. El número siempre está escrito.
func scoreClass(score int) string {
	switch {
	case score >= 85:
		return "text-accent"
	case score >= 70:
		return "text-ink"
	}
	return "text-muted"
}

// fillFor es scoreClass para texto SVG; las clases van literales para que Tailwind las vea.
func fillFor(score int) string {
	switch {
	case score >= 85:
		return "fill-accent"
	case score >= 70:
		return "fill-ink"
	}
	return "fill-muted"
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func barClass(s profile.Sub) string {
	switch {
	case s.Learning:
		return "fill-none stroke-control"
	case s.Score >= 85:
		return "fill-accent"
	}
	return "fill-ink"
}

// ovrTitle explica la sigla OVR (ADR-010 §3).
const ovrTitle = "OVR (overall): índice general del rol, el promedio de los atributos ponderado por su peso en ese rol"

func workAnchor(w profile.Work) string { return "trabajo-" + w.ID }

// workAttributes son los atributos que el trabajo respalda: los que citan alguno de sus logros.
func workAttributes(r *profile.Resume, w profile.Work) []profile.Attribute {
	var out []profile.Attribute
	for _, a := range r.Gremio.Attributes {
		if slices.ContainsFunc(w.Highlights, func(h profile.Highlight) bool { return slices.Contains(a.Highlights, h.ID) }) {
			out = append(out, a)
		}
	}
	return out
}

func attrAnchor(code string) string { return "atributo-" + strings.ToLower(code) }

func itoa(n int) string { return strconv.Itoa(n) }

// viewPath es la URL de una vista; la primera es la principal y vive en /@usuario.
func (p Page) viewPath(i int) string {
	base := "/@" + p.Username
	if i == 0 {
		return base
	}
	return base + "/" + p.Resume.Gremio.Views[i].Slug
}

func viewCode(v profile.View) string {
	if v.Code != "" {
		return v.Code
	}
	return strings.ToUpper(v.Slug[:min(3, len(v.Slug))])
}

// positionName es el nombre de la posición en el catálogo (ADR-011), o el de la vista si el código no es estándar.
func positionName(v profile.View) string {
	if pos, ok := profile.LookupPosition(v.Code); ok {
		return pos.Name
	}
	return v.Name
}

// positionTitle es el tooltip de la sigla de una posición.
func positionTitle(v profile.View) string {
	if pos, ok := profile.LookupPosition(v.Code); ok {
		return pos.Code + " · " + pos.Name + ": " + pos.Summary
	}
	return v.Name + " (posición fuera del catálogo de Gremio)"
}

func levelShort(v profile.View) string { return profile.Levels[v.Level][0] }
func levelLong(v profile.View) string  { return profile.Levels[v.Level][1] }

// radarSlots es cuántos ejes muestra el radar: los atributos son libres, pero en el radar van los mejores.
const radarSlots = 6

// topAttributes son los n atributos de mayor puntaje, en el orden del perfil para que la forma sea estable.
func topAttributes(attrs []profile.Attribute, n int) []profile.Attribute {
	if len(attrs) <= n {
		return attrs
	}
	idx := make([]int, len(attrs))
	for i := range idx {
		idx[i] = i
	}
	slices.SortStableFunc(idx, func(a, b int) int { return attrs[b].Score() - attrs[a].Score() })
	idx = idx[:n]
	slices.Sort(idx)
	out := make([]profile.Attribute, n)
	for i, j := range idx {
		out[i] = attrs[j]
	}
	return out
}

type weightedView struct {
	Code   string
	Name   string
	Weight string
}

// weightsFor dice cuánto pesa un atributo en cada vista que lo pondera.
func weightsFor(r *profile.Resume, code string) []weightedView {
	var out []weightedView
	for _, v := range r.Gremio.Views {
		w := 1.0
		if len(v.Weights) > 0 {
			w = v.Weights[code]
		}
		if w > 0 {
			out = append(out, weightedView{viewCode(v), positionTitle(v), strconv.FormatFloat(w, 'g', 3, 64)})
		}
	}
	return out
}

type evidence struct {
	Work string
	profile.Highlight
}

func attrHighlights(r *profile.Resume, a profile.Attribute) []evidence {
	var out []evidence
	for _, id := range a.Highlights {
		for _, w := range r.Work {
			for _, h := range w.Highlights {
				if h.ID == id {
					out = append(out, evidence{w.Name, h})
				}
			}
		}
	}
	return out
}

// attrWorks son los trabajos donde se usó el atributo: los de sus logros, en el orden del perfil.
func attrWorks(r *profile.Resume, a profile.Attribute) []profile.Work {
	var out []profile.Work
	for _, w := range r.Work {
		for _, h := range w.Highlights {
			if slices.Contains(a.Highlights, h.ID) {
				out = append(out, w)
				break
			}
		}
	}
	return out
}

func attrKeywords(r *profile.Resume, a profile.Attribute) []string {
	var out []string
	for _, s := range r.Skills {
		if slices.Contains(a.Skills, s.ID) {
			for _, k := range s.Keywords {
				if !slices.Contains(out, k) {
					out = append(out, k)
				}
			}
		}
	}
	return out
}

func totalHighlights(r *profile.Resume) int {
	n := 0
	for _, w := range r.Work {
		n += len(w.Highlights)
	}
	return n
}

func languagesLine(r *profile.Resume) string {
	parts := make([]string, len(r.Languages))
	for i, l := range r.Languages {
		parts[i] = strings.TrimSpace(l.Language + " " + l.Fluency)
	}
	return strings.Join(parts, " · ")
}

// Línea de tiempo: un segmento por trabajo sobre un viewBox de 1000 de ancho, en carriles si se solapan.
type segment struct {
	X, W, Y, H string
	Title      string
	Current    bool
}

type timeline struct {
	Segments []segment
	From     string
}

func month(d string) (int, bool) {
	if len(d) < 4 {
		return 0, false
	}
	y, err := strconv.Atoi(d[:4])
	if err != nil {
		return 0, false
	}
	m := 1
	if len(d) >= 7 {
		m, _ = strconv.Atoi(d[5:7])
	}
	return y*12 + m - 1, true
}

func buildTimeline(work []profile.Work, now time.Time) timeline {
	type span struct {
		start, end int
		w          profile.Work
	}
	cur := now.Year()*12 + int(now.Month()) - 1
	var spans []span
	first := cur
	for _, w := range work {
		s, ok := month(w.StartDate)
		if !ok {
			continue
		}
		e := cur + 1
		if w.EndDate != "" {
			if m, ok := month(w.EndDate); ok {
				e = m + 1
				if len(w.EndDate) == 4 {
					e = m + 12
				}
			}
		}
		spans = append(spans, span{s, max(e, s+1), w})
		first = min(first, s)
	}
	if len(spans) == 0 {
		return timeline{}
	}
	// Carriles: cada trabajo va al primero que ya terminó cuando él empieza (los trabajos vienen del más reciente al más viejo).
	lane := make([]int, len(spans))
	var ends []int
	for i := len(spans) - 1; i >= 0; i-- {
		l := 0
		for l < len(ends) && ends[l] > spans[i].start {
			l++
		}
		if l == len(ends) {
			ends = append(ends, 0)
		}
		ends[l] = spans[i].end
		lane[i] = l
	}
	total := float64(cur + 1 - first)
	h := 40.0 / float64(len(ends))
	t := timeline{From: strconv.Itoa(first / 12)}
	for i, s := range spans {
		x := float64(s.start-first) / total * 1000
		w := float64(s.end-s.start)/total*1000 - 4
		t.Segments = append(t.Segments, segment{
			X: fmt.Sprintf("%.1f", x), W: fmt.Sprintf("%.1f", max(w, 2)),
			Y: fmt.Sprintf("%.1f", float64(lane[i])*h), H: fmt.Sprintf("%.1f", h-3),
			Title:   s.w.Name + " · " + dateRange(s.w.StartDate, s.w.EndDate),
			Current: s.w.EndDate == "",
		})
	}
	return t
}
