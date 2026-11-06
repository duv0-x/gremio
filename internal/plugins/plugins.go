// Package plugins es el catálogo de plugins de la plataforma (ADR-012): integraciones incluidas en el binario que
// el dueño habilita, configura y deshabilita desde el panel. Un plugin está habilitado si tiene fila en la tabla
// plugins; su configuración, secretos incluidos, vive ahí y el panel nunca la devuelve completa.
package plugins

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/duv0-x/gremio/internal/store"
)

// FieldType es el tipo de un campo que pide un plugin.
type FieldType string

const (
	Text   FieldType = "text"
	Secret FieldType = "secret" // se guarda, pero el panel solo muestra los últimos 4 caracteres
)

// Field es un dato que el plugin pide para habilitarse.
type Field struct {
	Key, Label, Help, Default string
	Type                      FieldType
	Required                  bool
}

// Info describe un plugin en el catálogo.
type Info struct {
	ID, Name, Description, Category string
	Fields                          []Field
}

// Config son los valores de los campos, por Key.
type Config map[string]string

// Plugin es una integración del catálogo.
type Plugin interface {
	Info() Info
	// Check valida la configuración contra el servicio y devuelve un resumen para el panel ("Conectado como …").
	// El error tiene que ser legible para el dueño.
	Check(ctx context.Context, cfg Config) (string, error)
}

// Catalog son los plugins disponibles, en el orden en que los muestra el panel.
func Catalog() []Plugin { return []Plugin{Ollama{}, Claude{}} }

// Find busca un plugin del catálogo por id.
func Find(id string) (Plugin, bool) {
	for _, p := range Catalog() {
		if p.Info().ID == id {
			return p, true
		}
	}
	return nil, false
}

// ErrNotFound es un id que no está en el catálogo.
var ErrNotFound = errors.New("plugin desconocido")

const (
	timeLayout   = "2006-01-02T15:04:05.000Z"
	checkTimeout = 10 * time.Second
)

// Manager guarda y consulta los plugins habilitados.
type Manager struct {
	st   *store.Store
	find func(id string) (Plugin, bool)
	now  func() time.Time
}

func NewManager(st *store.Store) *Manager {
	return &Manager{st: st, find: Find, now: func() time.Time { return time.Now().UTC() }}
}

// State es un plugin del catálogo con su estado en esta instancia.
type State struct {
	Info      Info
	Enabled   bool
	Status    string
	CheckedAt time.Time
	// Values son los valores para mostrar: los secretos enmascarados, los demás tal cual.
	Values Config
}

// List devuelve todo el catálogo con el estado de cada plugin.
func (m *Manager) List(ctx context.Context) ([]State, error) {
	rows, err := m.st.Read.ListPlugins(ctx)
	if err != nil {
		return nil, err
	}
	byID := map[string]store.Plugin{}
	for _, r := range rows {
		byID[r.ID] = r
	}
	var out []State
	for _, p := range Catalog() {
		info := p.Info()
		s := State{Info: info}
		if r, ok := byID[info.ID]; ok {
			cfg, err := decode(r.Config)
			if err != nil {
				return nil, err
			}
			checked, _ := time.Parse(timeLayout, r.CheckedAt)
			s.Enabled, s.Status, s.CheckedAt, s.Values = true, r.Status, checked, mask(info, cfg)
		}
		out = append(out, s)
	}
	return out, nil
}

// Enable valida la configuración con el chequeo del plugin y, si pasa, la guarda: el plugin queda habilitado
// (o actualizado, si ya lo estaba). Un campo secret vacío conserva el valor guardado; uno text vacío toma el
// valor por defecto. Si el chequeo falla no se guarda nada.
func (m *Manager) Enable(ctx context.Context, id string, input Config) (string, error) {
	p, ok := m.find(id)
	if !ok {
		return "", ErrNotFound
	}
	saved, err := m.config(ctx, id)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	cfg := Config{}
	for _, f := range p.Info().Fields {
		v := strings.TrimSpace(input[f.Key])
		switch {
		case v == "" && f.Type == Secret:
			v = saved[f.Key]
		case v == "" && f.Type == Text:
			v = f.Default
		}
		if v == "" && f.Required {
			return "", fmt.Errorf("falta %s", f.Label)
		}
		if v != "" {
			cfg[f.Key] = v
		}
	}
	cctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	status, err := p.Check(cctx, cfg)
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}
	now := m.now().Format(timeLayout)
	err = m.st.Write.SavePlugin(ctx, store.SavePluginParams{ID: id, Config: string(raw), Status: status, CheckedAt: now, EnabledAt: now})
	return status, err
}

// Recheck repite el chequeo de un plugin habilitado y guarda el resultado, sea un resumen o un error.
func (m *Manager) Recheck(ctx context.Context, id string) (string, error) {
	p, ok := m.find(id)
	if !ok {
		return "", ErrNotFound
	}
	cfg, err := m.config(ctx, id)
	if err != nil {
		return "", err
	}
	cctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	status, checkErr := p.Check(cctx, cfg)
	if checkErr != nil {
		status = "Error: " + checkErr.Error()
	}
	if err := m.st.Write.SetPluginStatus(ctx, store.SetPluginStatusParams{Status: status, CheckedAt: m.now().Format(timeLayout), ID: id}); err != nil {
		return "", err
	}
	return status, checkErr
}

// Disable borra la configuración del plugin, credenciales incluidas.
func (m *Manager) Disable(ctx context.Context, id string) error {
	if _, ok := m.find(id); !ok {
		return ErrNotFound
	}
	return m.st.Write.DeletePlugin(ctx, id)
}

// Config devuelve la configuración guardada de un plugin habilitado, para que la usen sus capacidades.
// sql.ErrNoRows si no está habilitado.
func (m *Manager) Config(ctx context.Context, id string) (Config, error) { return m.config(ctx, id) }

func (m *Manager) config(ctx context.Context, id string) (Config, error) {
	r, err := m.st.Read.GetPlugin(ctx, id)
	if err != nil {
		return nil, err
	}
	return decode(r.Config)
}

func decode(raw string) (Config, error) {
	var cfg Config
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return nil, fmt.Errorf("configuración de plugin ilegible: %w", err)
	}
	return cfg, nil
}

// mask deja los campos text como están y muestra solo los últimos 4 caracteres de los secret.
func mask(info Info, cfg Config) Config {
	out := Config{}
	for _, f := range info.Fields {
		v, ok := cfg[f.Key]
		if !ok {
			continue
		}
		if f.Type == Secret {
			v = maskSecret(v)
		}
		out[f.Key] = v
	}
	return out
}

func maskSecret(v string) string {
	if r := []rune(v); len(r) >= 12 {
		return "•••• " + string(r[len(r)-4:])
	}
	return "••••"
}
