package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// OllamaCloud es el endpoint por defecto del plugin Ollama.
const OllamaCloud = "https://ollama.com"

// Ollama sirve para Ollama Cloud y para instalaciones propias (ADR-012 §4).
type Ollama struct{}

func (Ollama) Info() Info {
	return Info{
		ID:          "ollama",
		Name:        "Ollama",
		Category:    "IA",
		Description: "Modelos de IA de Ollama Cloud o de tu propia instalación de Ollama.",
		Fields: []Field{
			{Key: "endpoint", Label: "Endpoint", Type: Text, Default: OllamaCloud, Required: true,
				Help: "Ollama Cloud es " + OllamaCloud + ". Para tu propia instalación, su URL, p. ej. http://192.168.1.50:11434."},
			{Key: "token", Label: "Token", Type: Secret,
				Help: "Obligatorio con Ollama Cloud (créalo en ollama.com, en tu cuenta). Opcional en una instalación propia."},
		},
	}
}

// Check confirma que el endpoint es un Ollama (lista sus modelos) y, si hay token, que lo acepta. En Ollama Cloud
// /api/tags responde aunque no haya token, así que el token se valida aparte con /api/me.
func (Ollama) Check(ctx context.Context, cfg Config) (string, error) {
	endpoint := strings.TrimRight(cfg["endpoint"], "/")
	u, err := url.Parse(endpoint)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", errors.New("el endpoint debe ser una URL http o https, p. ej. " + OllamaCloud)
	}
	token := cfg["token"]
	if u.Hostname() == "ollama.com" && token == "" {
		return "", errors.New("Ollama Cloud necesita un token")
	}

	var tags struct {
		Models []json.RawMessage `json:"models"`
	}
	resp, err := ollamaDo(ctx, http.MethodGet, endpoint+"/api/tags", token)
	if err != nil {
		return "", fmt.Errorf("no se pudo conectar con Ollama en %s", u.Host)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&tags) != nil || tags.Models == nil {
		return "", fmt.Errorf("%s no respondió como un Ollama (HTTP %d en /api/tags)", u.Host, resp.StatusCode)
	}

	if token != "" {
		me, err := ollamaDo(ctx, http.MethodPost, endpoint+"/api/me", token)
		if err != nil {
			return "", fmt.Errorf("no se pudo validar el token en %s", u.Host)
		}
		defer me.Body.Close()
		switch me.StatusCode {
		case http.StatusOK:
			var who struct{ Name, Plan string }
			if json.NewDecoder(io.LimitReader(me.Body, 1<<20)).Decode(&who) == nil && who.Name != "" {
				if who.Plan != "" {
					return fmt.Sprintf("Conectado como %s · plan %s", who.Name, who.Plan), nil
				}
				return "Conectado como " + who.Name, nil
			}
		case http.StatusUnauthorized, http.StatusForbidden:
			return "", errors.New("Ollama rechazó el token")
		case http.StatusNotFound:
			// Una instalación propia puede no tener /api/me: basta con que liste sus modelos.
		default:
			return "", fmt.Errorf("Ollama respondió HTTP %d al validar el token", me.StatusCode)
		}
	}
	return fmt.Sprintf("%d %s en %s", len(tags.Models), plural(len(tags.Models), "modelo", "modelos"), u.Host), nil
}

func ollamaDo(ctx context.Context, method, url, token string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return http.DefaultClient.Do(req)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
