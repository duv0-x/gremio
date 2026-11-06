package plugins

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/duv0-x/gremio/internal/store"
)

const tokenBueno = "ollama-token-bueno-1234"

// fakeOllama imita Ollama: con me=false no tiene /api/me (una instalación propia).
func fakeOllama(t *testing.T, me bool) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/tags", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"models":[{"name":"gemma4:31b"},{"name":"deepseek-v4-flash"}]}`)
	})
	if me {
		mux.HandleFunc("POST /api/me", func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer "+tokenBueno {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			fmt.Fprint(w, `{"Name":"Ana","Plan":"pro","Email":"ana@ejemplo.org"}`)
		})
	}
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

func TestOllama(t *testing.T) {
	ctx := context.Background()
	cloud := fakeOllama(t, true)
	local := fakeOllama(t, false)
	for _, c := range []struct {
		name    string
		cfg     Config
		want    string
		wantErr string
	}{
		{"token válido", Config{"endpoint": cloud.URL + "/", "token": tokenBueno}, "Conectado como Ana · plan pro", ""},
		{"token inválido", Config{"endpoint": cloud.URL, "token": "malo"}, "", "rechazó el token"},
		{"instalación propia sin token", Config{"endpoint": local.URL}, "2 modelos en " + strings.TrimPrefix(local.URL, "http://"), ""},
		{"instalación propia con token y sin /api/me", Config{"endpoint": local.URL, "token": "x"}, "2 modelos en", ""},
		{"Ollama Cloud sin token", Config{"endpoint": OllamaCloud}, "", "necesita un token"},
		{"no es una URL http", Config{"endpoint": "ftp://ejemplo.org"}, "", "http o https"},
		{"no es un Ollama", Config{"endpoint": httptest.NewServer(http.NotFoundHandler()).URL}, "", "no respondió como un Ollama"},
	} {
		got, err := Ollama{}.Check(ctx, c.cfg)
		if c.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("%s: error %v, quería %q", c.name, err, c.wantErr)
			}
			continue
		}
		if err != nil || !strings.HasPrefix(got, c.want) {
			t.Errorf("%s: %q, %v; quería %q", c.name, got, err, c.want)
		}
	}
}

func fakeAnthropic(t *testing.T) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		// El SDK v2 manda la key como Authorization: Bearer; antes iba en x-api-key.
		if r.Header.Get("X-Api-Key") != "sk-ant-buena" && r.Header.Get("Authorization") != "Bearer sk-ant-buena" {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`)
			return
		}
		fmt.Fprint(w, `{"data":[{"type":"model","id":"claude-opus-5-5","display_name":"Claude Opus 5.5","created_at":"2026-09-01T00:00:00Z"},`+
			`{"type":"model","id":"claude-sonnet-5-5","display_name":"Claude Sonnet 5.5","created_at":"2026-09-01T00:00:00Z"}],`+
			`"has_more":false,"first_id":"claude-opus-5-5","last_id":"claude-sonnet-5-5"}`)
	}))
	t.Cleanup(ts.Close)
	return ts
}

func TestClaude(t *testing.T) {
	c := Claude{baseURL: fakeAnthropic(t).URL}
	if got, err := c.Check(context.Background(), Config{"api_key": "sk-ant-buena"}); err != nil || got != "API key válida · 2 modelos disponibles" {
		t.Errorf("key válida: %q, %v", got, err)
	}
	if _, err := c.Check(context.Background(), Config{"api_key": "sk-ant-mala"}); err == nil || err.Error() != "Anthropic rechazó la API key" {
		t.Errorf("key inválida: %v", err)
	}
}

func TestManager(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	cloud := fakeOllama(t, true)
	m := NewManager(st)

	// Un token inválido no habilita nada.
	if _, err := m.Enable(ctx, "ollama", Config{"endpoint": cloud.URL, "token": "malo"}); err == nil {
		t.Fatal("un token inválido no debe habilitar el plugin")
	}
	if _, err := m.Config(ctx, "ollama"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("no debe quedar configuración: %v", err)
	}
	if _, err := m.Enable(ctx, "nope", Config{}); !errors.Is(err, ErrNotFound) {
		t.Errorf("plugin desconocido: %v", err)
	}
	if _, err := m.Enable(ctx, "claude", Config{}); err == nil || err.Error() != "falta API key" {
		t.Errorf("campo obligatorio: %v", err)
	}

	status, err := m.Enable(ctx, "ollama", Config{"endpoint": cloud.URL, "token": tokenBueno})
	if err != nil || status != "Conectado como Ana · plan pro" {
		t.Fatalf("habilitar: %q, %v", status, err)
	}
	states, err := m.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var ollama, claude State
	for _, s := range states {
		switch s.Info.ID {
		case "ollama":
			ollama = s
		case "claude":
			claude = s
		}
	}
	if !ollama.Enabled || ollama.Status != status || ollama.Values["token"] != "•••• 1234" || ollama.Values["endpoint"] != cloud.URL || claude.Enabled {
		t.Errorf("estado: %+v / %+v", ollama, claude)
	}

	// Editar con el secreto vacío conserva el guardado; el endpoint vacío toma el valor por defecto.
	if _, err := m.Enable(ctx, "ollama", Config{"endpoint": cloud.URL, "token": ""}); err != nil {
		t.Fatalf("editar sin cambiar el token: %v", err)
	}
	if cfg, _ := m.Config(ctx, "ollama"); cfg["token"] != tokenBueno {
		t.Errorf("el token guardado se perdió: %+v", cfg)
	}

	// Recheck guarda el error si el servicio deja de aceptar la configuración.
	cloud.Close()
	if _, err := m.Recheck(ctx, "ollama"); err == nil {
		t.Error("recheck con el servicio caído debe fallar")
	}
	if states, _ := m.List(ctx); !strings.HasPrefix(states[0].Status, "Error: ") {
		t.Errorf("el error del chequeo se guarda: %q", states[0].Status)
	}

	if err := m.Disable(ctx, "ollama"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Config(ctx, "ollama"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("deshabilitar borra la configuración: %v", err)
	}
}

func TestMaskSecret(t *testing.T) {
	for in, want := range map[string]string{"": "••••", "corto": "••••", "sk-ant-api03-abcdWXYZ": "•••• WXYZ"} {
		if got := maskSecret(in); got != want {
			t.Errorf("maskSecret(%q) = %q, quería %q", in, got, want)
		}
	}
}
