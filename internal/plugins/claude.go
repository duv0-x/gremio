package plugins

import (
	"context"
	"errors"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
)

// Claude usa la API de Anthropic con la API key del dueño (ADR-012 §4).
type Claude struct {
	baseURL string // solo para tests; vacío es la API de Anthropic
}

func (Claude) Info() Info {
	return Info{
		ID:          "claude",
		Name:        "Claude",
		Category:    "IA",
		Description: "Modelos Claude de Anthropic con tu API key.",
		Fields: []Field{
			{Key: "api_key", Label: "API key", Type: Secret, Required: true,
				Help: "Créala en console.anthropic.com, en API keys."},
		},
	}
}

// Client devuelve un cliente de la API con la key del plugin, para las capacidades que se agreguen.
func (c Claude) Client(cfg Config) anthropic.Client {
	opts := []option.RequestOption{option.WithAPIKey(cfg["api_key"])}
	if c.baseURL != "" {
		opts = append(opts, option.WithBaseURL(c.baseURL))
	}
	return anthropic.NewClient(opts...)
}

// Check lista los modelos de la cuenta: si la API los devuelve, la key sirve.
func (c Claude) Check(ctx context.Context, cfg Config) (string, error) {
	client := c.Client(cfg)
	n := 0
	iter := client.Models.ListAutoPaging(ctx, anthropic.ModelListParams{}, option.WithMaxRetries(0))
	for iter.Next() {
		n++
	}
	if err := iter.Err(); err != nil {
		var apierr *anthropic.Error
		if errors.As(err, &apierr) {
			if apierr.StatusCode == 401 {
				return "", errors.New("Anthropic rechazó la API key")
			}
			return "", fmt.Errorf("Anthropic respondió HTTP %d al listar los modelos", apierr.StatusCode)
		}
		return "", errors.New("no se pudo conectar con la API de Anthropic")
	}
	return fmt.Sprintf("API key válida · %d %s disponibles", n, plural(n, "modelo", "modelos")), nil
}
