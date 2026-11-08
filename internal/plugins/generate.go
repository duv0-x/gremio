package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go/v2"
)

// Generator es la capacidad de los plugins de IA de generar texto (gremio#124: borradores de retos del dojo).
// La llama el servidor con la configuración guardada; el resultado lo revisa una persona antes de publicarse.
type Generator interface {
	// DefaultModel es el modelo que propone el panel; vacío si el dueño tiene que escribirlo.
	DefaultModel() string
	Generate(ctx context.Context, cfg Config, model, system, prompt string) (string, error)
}

const generateTimeout = 2 * time.Minute

func (Claude) DefaultModel() string { return "claude-opus-5-5" }

func (c Claude) Generate(ctx context.Context, cfg Config, model, system, prompt string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, generateTimeout)
	defer cancel()
	client := c.Client(cfg)
	msg, err := client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: 8192,
		System:    []anthropic.TextBlockParam{{Text: system}},
		Messages:  []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(prompt))},
	})
	if err != nil {
		var apierr *anthropic.Error
		if errors.As(err, &apierr) {
			return "", fmt.Errorf("Anthropic respondió HTTP %d", apierr.StatusCode)
		}
		return "", errors.New("no se pudo conectar con la API de Anthropic")
	}
	var out strings.Builder
	for _, b := range msg.Content {
		if b.Type == "text" {
			out.WriteString(b.Text)
		}
	}
	if out.Len() == 0 {
		return "", errors.New("el modelo no devolvió texto")
	}
	return out.String(), nil
}

// DefaultModel vacío: los modelos de cada instalación de Ollama son distintos.
func (Ollama) DefaultModel() string { return "" }

func (Ollama) Generate(ctx context.Context, cfg Config, model, system, prompt string) (string, error) {
	if model == "" {
		return "", errors.New("escribe el modelo de Ollama: uno de los que lista tu instalación o Ollama Cloud")
	}
	ctx, cancel := context.WithTimeout(ctx, generateTimeout)
	defer cancel()
	body, err := json.Marshal(map[string]any{
		"model": model, "stream": false, "format": "json",
		"messages": []map[string]string{{"role": "system", "content": system}, {"role": "user", "content": prompt}},
	})
	if err != nil {
		return "", err
	}
	endpoint := strings.TrimRight(cfg["endpoint"], "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if t := cfg["token"]; t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", errors.New("no se pudo conectar con Ollama")
	}
	defer resp.Body.Close()
	var out struct {
		Message struct{ Content string } `json:"message"`
		Error   string                   `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&out); err != nil {
		return "", fmt.Errorf("Ollama respondió HTTP %d sin JSON", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Ollama respondió HTTP %d: %s", resp.StatusCode, out.Error)
	}
	if strings.TrimSpace(out.Message.Content) == "" {
		// Los modelos que razonan (p. ej. gpt-oss) pueden gastar la respuesta pensando y devolver texto vacío.
		return "", errors.New("el modelo no devolvió texto: prueba con otro modelo")
	}
	return out.Message.Content, nil
}
