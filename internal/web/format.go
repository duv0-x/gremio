package web

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/duv0-x/gremio/internal/profile"
)

// Page son los datos de la página del perfil.
type Page struct {
	Resume   *profile.Resume
	Username string
	Handle   string // @usuario@dominio
	Actor    string // URL del Actor de ActivityPub
	Version  string
	View     int    // vista seleccionada; la primera es la principal
	Tab      string // pestaña: ficha o cv
	HasCV    bool   // hay un PDF del CV para descargar
}

// tabPath es la URL de una pestaña; la ficha conserva la vista elegida.
func (p Page) tabPath(tab string) string {
	if tab == "ficha" {
		return p.viewPath(p.View)
	}
	return "/@" + p.Username + "/" + tab
}

func (p Page) Title() string {
	t := p.Resume.Basics.Name + " · " + p.Resume.Basics.Label
	if p.Tab == "cv" {
		return p.Resume.Basics.Name + " · CV"
	}
	return t
}

// HasFicha dice si el perfil tiene atributos; sin ellos la página es el CV de v0.2 (ADR-010).
func (p Page) HasFicha() bool { return len(p.Resume.Gremio.Attributes) > 0 }

// Selected es la vista elegida, si el perfil tiene vistas.
func (p Page) Selected() (profile.View, bool) {
	if p.View < len(p.Resume.Gremio.Views) {
		return p.Resume.Gremio.Views[p.View], true
	}
	return profile.View{}, false
}

// Overall es el OVR de la vista elegida, o de todos los atributos por igual si no hay vistas.
func (p Page) Overall() int {
	v, _ := p.Selected()
	return p.Resume.Overall(v)
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

// WhatsApp es el enlace wa.me con un mensaje inicial, solo si el dueño lo hizo público (el número no se muestra).
func (p Page) WhatsApp() string {
	if p.Resume.Gremio.Contact.Whatsapp != "public" {
		return ""
	}
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, p.Resume.Basics.Phone)
	if digits == "" {
		return ""
	}
	first := strings.Fields(p.Resume.Basics.Name)[0]
	return "https://wa.me/" + digits + "?text=" + url.QueryEscape("Hola "+first+", vi tu perfil en Gremio ("+p.Handle+") y me gustaría conversar.")
}

func (p Page) cvPDFPath() string { return "/@" + p.Username + "/cv.pdf" }

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

// TimeZone da el desfase actual de la zona del dueño ("UTC−5", "UTC+5:30") y el nombre IANA para el tooltip.
func (p Page) TimeZone(now time.Time) (offset, tip string) {
	loc, err := time.LoadLocation(p.Resume.Gremio.Timezone)
	if p.Resume.Gremio.Timezone == "" || err != nil {
		return "", ""
	}
	_, secs := now.In(loc).Zone()
	sign := "+"
	if secs < 0 {
		sign, secs = "−", -secs
	}
	offset = "UTC" + sign + strconv.Itoa(secs/3600)
	if m := secs % 3600 / 60; m != 0 {
		offset += fmt.Sprintf(":%02d", m)
	}
	return offset, "Zona horaria: " + p.Resume.Gremio.Timezone + " (" + offset + " hoy)"
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
