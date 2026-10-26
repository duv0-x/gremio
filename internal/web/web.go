// Package web sirve las páginas HTML de la instancia (ADR-008): por ahora, el perfil público.
package web

//go:generate go tool templ generate
//go:generate ../../scripts/tailwind.sh styles/input.css static/app.css

import (
	"embed"
	"io/fs"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/duv0-x/gremio/internal/instance"
	"github.com/duv0-x/gremio/internal/profile"
	"github.com/duv0-x/gremio/internal/store"
)

//go:embed static
var staticFS embed.FS

// Server sirve la web pública de la instancia.
type Server struct {
	st      *store.Store
	version string
}

func New(st *store.Store, version string) *Server {
	if version == "" {
		version = "dev"
	}
	return &Server{st: st, version: version}
}

// Register agrega las rutas web al mux.
func (s *Server) Register(mux *http.ServeMux) {
	static, _ := fs.Sub(staticFS, "static")
	files := http.FileServerFS(static)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Las URLs llevan ?v=<versión>: cambian con cada release, así que se pueden cachear sin límite.
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		files.ServeHTTP(w, r)
	})))
	mux.HandleFunc("GET /{handle}", s.getProfile)
	// {view...} y no {view}: así /static/ es más específico y no choca.
	mux.HandleFunc("GET /{handle}/{view...}", s.getProfile)
	mux.HandleFunc("GET /{$}", s.getRoot)
}

// getRoot lleva al perfil: la instancia es de un solo usuario (ADR-001).
func (s *Server) getRoot(w http.ResponseWriter, r *http.Request) {
	inst, err := instance.Load(r.Context(), s.st.Read)
	if err != nil {
		slog.Error("instancia", "err", err)
		http.Error(w, "instancia no disponible", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/@"+inst.Username, http.StatusFound)
}

func (s *Server) getProfile(w http.ResponseWriter, r *http.Request) {
	handle, ok := strings.CutPrefix(r.PathValue("handle"), "@")
	if !ok {
		http.NotFound(w, r)
		return
	}
	inst, err := instance.Load(r.Context(), s.st.Read)
	if err != nil || handle != inst.Username {
		http.NotFound(w, r)
		return
	}
	doc, err := s.st.Read.GetProfile(r.Context())
	if err != nil {
		slog.Error("perfil", "err", err)
		http.Error(w, "perfil no disponible", http.StatusInternalServerError)
		return
	}
	res, err := profile.Load(doc)
	if err != nil {
		slog.Error("perfil", "err", err)
		http.Error(w, "perfil no disponible", http.StatusInternalServerError)
		return
	}
	page := Page{
		Resume:   res,
		Username: inst.Username,
		Handle:   "@" + inst.Username + "@" + inst.AccountDomain.String,
		Actor:    inst.Host.String + "/users/" + inst.Username,
		Version:  s.version,
	}
	page.Tab = "ficha"
	page.HasCV = s.hasCV()
	switch slug := r.PathValue("view"); slug {
	case "":
	case "cv.pdf":
		s.serveCV(w, r, res.Basics.Name)
		return
	case "contacto":
		// La pestaña Contacto se quitó: el contacto rápido son los botones fijos.
		http.Redirect(w, r, "/@"+inst.Username+"/cv", http.StatusMovedPermanently)
		return
	case "whatsapp":
		// Redirige a wa.me en el clic: así el número no está en el HTML que leen los scrapers.
		wa := page.WhatsApp()
		if wa == "" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Robots-Tag", "noindex")
		http.Redirect(w, r, wa, http.StatusFound)
		return
	case "cv":
		page.Tab = slug
	default:
		i := slices.IndexFunc(res.Gremio.Views, func(v profile.View) bool { return v.Slug == slug })
		// La primera vista vive en /@usuario: su slug no es una segunda URL.
		if i <= 0 {
			http.NotFound(w, r)
			return
		}
		page.View = i
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// Sin scripts de terceros ni inline: solo static/cinta.js, que es opcional.
	w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' https:; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	// El handle @usuario@dominio parece un email: no-transform evita que un proxy como Cloudflare lo ofusque.
	w.Header().Set("Cache-Control", "no-transform")
	if err := ProfilePage(page).Render(r.Context(), w); err != nil {
		slog.Error("render perfil", "err", err)
	}
}
