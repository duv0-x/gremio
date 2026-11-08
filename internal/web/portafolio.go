package web

import (
	"cmp"
	"slices"

	"github.com/duv0-x/gremio/internal/profile"
)

// execution es un reto del dojo terminado y publicado, tal como lo muestra el Portafolio.
type execution struct {
	Date    string // AAAA-MM-DD (UTC)
	Title   string
	URL     string // página pública de la ejecución
	Level   string // N1–N4
	Trust   string // práctica propia, reproducible o verificado por terceros
	Version int
}

// timelineItem es una entrada de la línea de tiempo del Portafolio: un proyecto o una ejecución.
type timelineItem struct {
	Date      string
	Project   *profile.Project
	Execution *execution
}

func (p Page) hasPortfolio() bool { return len(p.Resume.Projects) > 0 || len(p.Executions) > 0 }

// timeline mezcla los proyectos del perfil y las ejecuciones del dojo, de lo más reciente a lo más antiguo. Un
// proyecto en curso se ordena por su inicio; las fechas son AAAA, AAAA-MM o AAAA-MM-DD, que se comparan como texto.
// Los proyectos sin fecha van al final, en el orden del perfil.
func timeline(p Page) []timelineItem {
	var out []timelineItem
	for i := range p.Resume.Projects {
		pr := &p.Resume.Projects[i]
		out = append(out, timelineItem{Date: cmp.Or(pr.EndDate, pr.StartDate), Project: pr})
	}
	for i := range p.Executions {
		out = append(out, timelineItem{Date: p.Executions[i].Date, Execution: &p.Executions[i]})
	}
	slices.SortStableFunc(out, func(a, b timelineItem) int {
		switch {
		case a.Date == b.Date:
			return 0
		case a.Date == "":
			return 1
		case b.Date == "":
			return -1
		}
		return cmp.Compare(b.Date, a.Date)
	})
	return out
}

// timelineDate es la fecha visible de una entrada: el rango del proyecto o el día de la ejecución.
func timelineDate(it timelineItem) string {
	if it.Project != nil {
		return dateRange(it.Project.StartDate, it.Project.EndDate)
	}
	return it.Date
}
