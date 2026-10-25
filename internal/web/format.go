package web

import (
	"strconv"
	"strings"

	"github.com/duv0-x/gremio/internal/profile"
)

// Page son los datos de la página del perfil.
type Page struct {
	Resume  *profile.Resume
	Handle  string // @usuario@dominio
	Actor   string // URL del Actor de ActivityPub
	Version string
}

// SourceURL apunta al código de la versión desplegada (ADR-004: el enlace "Source" es obligatorio).
func (p Page) SourceURL() string {
	const repo = "https://github.com/duv0-x/gremio"
	// GoReleaser inyecta la versión sin la "v" del tag.
	if p.Version == "" || p.Version == "dev" {
		return repo
	}
	return repo + "/tree/v" + strings.TrimPrefix(p.Version, "v")
}

// Email devuelve el email solo si el dueño lo hizo público (ADR-003 §4).
func (p Page) Email() string {
	if p.Resume.Gremio.Contact.Email == "public" {
		return p.Resume.Basics.Email
	}
	return ""
}

func (p Page) About() string {
	if p.Resume.Gremio.About != "" {
		return p.Resume.Gremio.About
	}
	return p.Resume.Basics.Summary
}

func (p Page) Location() string {
	l := p.Resume.Basics.Location
	if l == nil {
		return ""
	}
	parts := []string{}
	for _, s := range []string{l.City, l.Region, countryName(l.CountryCode)} {
		if s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, ", ")
}

func countryName(code string) string {
	// shortcut: solo los países que hoy usa alguna instancia; agregar al necesitarlo o usar x/text/language/display.
	switch strings.ToUpper(code) {
	case "CO":
		return "Colombia"
	default:
		return code
	}
}

// dateRange da "2021-05 → hoy" o "2015 → 2018-12".
func dateRange(start, end string) string {
	if start == "" && end == "" {
		return ""
	}
	if end == "" {
		end = "hoy"
	}
	return start + " → " + end
}

func levelLabel(level string) string {
	switch level {
	case "expert":
		return "Experto"
	case "solid":
		return "Sólido"
	case "learning":
		return "Aprendiendo"
	}
	return ""
}

// levelBar es la clase de la barra de nivel: llena, al 70 % o al 35 % punteada (DESIGN.md).
func levelBar(level string) string {
	switch level {
	case "expert":
		return "w-full bg-ink"
	case "solid":
		return "w-[70%] bg-control"
	case "learning":
		return "w-[35%] border-[1.5px] border-dashed border-control"
	}
	return "hidden"
}

func initials(name string) string {
	var out []rune
	for _, w := range strings.Fields(name) {
		if len(out) == 2 {
			break
		}
		out = append(out, []rune(w)[0])
	}
	return strings.ToUpper(string(out))
}

// evidenceSuffix numera los enlaces cuando hay más de uno: "evidencia 1", "evidencia 2".
func evidenceSuffix(i, n int) string {
	if n < 2 {
		return ""
	}
	return " " + strconv.Itoa(i+1)
}
