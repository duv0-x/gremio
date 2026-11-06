package web

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/duv0-x/gremio/internal/plugins"
)

// Sección Plugins del panel (ADR-012): el catálogo, habilitar con el formulario de cada plugin y su chequeo,
// probar la conexión, editar y deshabilitar. Los secretos nunca vuelven al navegador.

func (s *Server) registerPlugins(mux *http.ServeMux, post func(string, http.HandlerFunc)) {
	mux.HandleFunc("GET /panel/plugins", s.getPlugins)
	mux.HandleFunc("GET /panel/plugins/{id}", s.getPluginForm)
	post("/panel/plugins/{id}", s.postPluginForm)
	post("/panel/plugins/{id}/probar", s.postPluginCheck)
	mux.HandleFunc("GET /panel/plugins/{id}/deshabilitar", s.getPluginDisable)
	post("/panel/plugins/{id}/deshabilitar", s.postPluginDisable)
}

// pluginsPage es lo que muestra /panel/plugins; Notice viene del redirect tras una acción.
type pluginsPage struct {
	Version string
	States  []plugins.State
	Notice  string
	Error   string
}

// pluginForm es el formulario de un plugin. Values trae lo escrito (sin secretos) cuando el chequeo falla.
type pluginForm struct {
	Version string
	State   plugins.State
	Values  plugins.Config
	Error   string
}

func (s *Server) getPlugins(w http.ResponseWriter, r *http.Request) {
	if !s.owner(w, r) {
		return
	}
	states, err := s.plugins.List(r.Context())
	if err != nil {
		slog.Error("plugins", "err", err)
		http.Error(w, "no se pudieron leer los plugins", http.StatusInternalServerError)
		return
	}
	p := pluginsPage{Version: s.version, States: states}
	if st, ok := findState(states, r.URL.Query().Get("p")); ok {
		switch r.URL.Query().Get("ok") {
		case "habilitado":
			p.Notice = st.Info.Name + " quedó habilitado."
		case "probado":
			if isPluginError(st.Status) {
				p.Error = "La prueba de " + st.Info.Name + " falló; el detalle está en su tarjeta."
			} else {
				p.Notice = st.Info.Name + " respondió bien."
			}
		case "deshabilitado":
			p.Notice = st.Info.Name + " quedó deshabilitado y se borró su configuración."
		}
	}
	s.renderPanel(w, r, http.StatusOK, PluginsPage(p))
}

// state busca el plugin de la URL; responde 404 si no está en el catálogo.
func (s *Server) state(w http.ResponseWriter, r *http.Request) (plugins.State, bool) {
	states, err := s.plugins.List(r.Context())
	if err != nil {
		slog.Error("plugins", "err", err)
		http.Error(w, "no se pudieron leer los plugins", http.StatusInternalServerError)
		return plugins.State{}, false
	}
	st, ok := findState(states, r.PathValue("id"))
	if !ok {
		http.NotFound(w, r)
	}
	return st, ok
}

func findState(states []plugins.State, id string) (plugins.State, bool) {
	for _, st := range states {
		if st.Info.ID == id {
			return st, true
		}
	}
	return plugins.State{}, false
}

func (s *Server) getPluginForm(w http.ResponseWriter, r *http.Request) {
	if !s.owner(w, r) {
		return
	}
	st, ok := s.state(w, r)
	if !ok {
		return
	}
	s.renderPanel(w, r, http.StatusOK, PluginFormPage(pluginForm{Version: s.version, State: st, Values: st.Values}))
}

func (s *Server) postPluginForm(w http.ResponseWriter, r *http.Request) {
	if !s.owner(w, r) {
		return
	}
	st, ok := s.state(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "formulario inválido", http.StatusBadRequest)
		return
	}
	input := plugins.Config{}
	for _, f := range st.Info.Fields {
		input[f.Key] = r.PostFormValue(f.Key)
	}
	if _, err := s.plugins.Enable(r.Context(), st.Info.ID, input); err != nil {
		// Vuelve con lo escrito, salvo los secretos: el dueño los escribe de nuevo o deja el guardado.
		values := plugins.Config{}
		for _, f := range st.Info.Fields {
			if f.Type == plugins.Text {
				values[f.Key] = input[f.Key]
			} else if st.Enabled {
				values[f.Key] = st.Values[f.Key]
			}
		}
		slog.Info("plugin no habilitado", "plugin", st.Info.ID, "err", err)
		s.renderPanel(w, r, http.StatusBadRequest, PluginFormPage(pluginForm{Version: s.version, State: st, Values: values, Error: err.Error()}))
		return
	}
	slog.Info("plugin habilitado", "plugin", st.Info.ID)
	http.Redirect(w, r, "/panel/plugins?ok=habilitado&p="+st.Info.ID, http.StatusSeeOther)
}

func (s *Server) postPluginCheck(w http.ResponseWriter, r *http.Request) {
	if !s.owner(w, r) {
		return
	}
	st, ok := s.state(w, r)
	if !ok {
		return
	}
	if !st.Enabled {
		http.Redirect(w, r, "/panel/plugins", http.StatusSeeOther)
		return
	}
	// El resultado, bueno o malo, queda en el estado del plugin y se ve en su tarjeta.
	if _, err := s.plugins.Recheck(r.Context(), st.Info.ID); err != nil && errors.Is(err, plugins.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "/panel/plugins?ok=probado&p="+st.Info.ID, http.StatusSeeOther)
}

func (s *Server) getPluginDisable(w http.ResponseWriter, r *http.Request) {
	if !s.owner(w, r) {
		return
	}
	st, ok := s.state(w, r)
	if !ok {
		return
	}
	if !st.Enabled {
		http.Redirect(w, r, "/panel/plugins", http.StatusSeeOther)
		return
	}
	s.renderPanel(w, r, http.StatusOK, PluginDisablePage(s.version, st))
}

func (s *Server) postPluginDisable(w http.ResponseWriter, r *http.Request) {
	if !s.owner(w, r) {
		return
	}
	st, ok := s.state(w, r)
	if !ok {
		return
	}
	if err := s.plugins.Disable(r.Context(), st.Info.ID); err != nil {
		slog.Error("plugins: deshabilitar", "plugin", st.Info.ID, "err", err)
		http.Error(w, "no se pudo deshabilitar el plugin", http.StatusInternalServerError)
		return
	}
	slog.Info("plugin deshabilitado", "plugin", st.Info.ID)
	http.Redirect(w, r, "/panel/plugins?ok=deshabilitado&p="+st.Info.ID, http.StatusSeeOther)
}
