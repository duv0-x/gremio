package web

import (
	"testing"

	"github.com/duv0-x/gremio/internal/profile"
)

func TestTimeline(t *testing.T) {
	p := Page{
		Resume: &profile.Resume{Projects: []profile.Project{
			{Name: "sin fecha"},
			{Name: "viejo", StartDate: "2019", EndDate: "2020-03"},
			{Name: "en curso", StartDate: "2025-06"},
		}},
		Executions: []execution{{Date: "2026-11-01", Title: "reto"}},
	}
	var got []string
	for _, it := range timeline(p) {
		if it.Project != nil {
			got = append(got, it.Project.Name)
		} else {
			got = append(got, it.Execution.Title)
		}
	}
	want := []string{"reto", "en curso", "viejo", "sin fecha"}
	if len(got) != len(want) {
		t.Fatalf("timeline = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("timeline = %v, quiero %v", got, want)
		}
	}
}
