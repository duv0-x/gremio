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

	"github.com/duv0-x/gremio/internal/store"
)

// GitHubAPI es la API pública de GitHub.
const GitHubAPI = "https://api.github.com"

// GitHub demuestra la identidad del dueño en GitHub para las medallas externas (ADR-015 §3). Que un correo
// coincida con los commits no prueba nada: la prueba es que el sitio web (blog) del perfil de GitHub apunte a la
// instancia, algo que solo puede poner el dueño de esa cuenta.
type GitHub struct {
	API      string                                        // vacío es GitHubAPI; los tests usan un httptest
	Instance func(context.Context) (store.Instance, error) // la instancia, para comparar su host con el blog
}

func (GitHub) Info() Info {
	return Info{
		ID:          "github",
		Name:        "GitHub",
		Category:    "Identidad",
		Description: "Demuestra tu cuenta de GitHub para ganar medallas externas, como Forjador del Gremio.",
		Fields: []Field{
			{Key: "username", Label: "Usuario", Type: Text, Required: true,
				Help: "Tu usuario de GitHub. En tu perfil de GitHub, el campo Website tiene que ser la URL de esta instancia."},
			{Key: "token", Label: "Token", Type: Secret,
				Help: "Opcional: un token sin permisos solo sube la cuota de la API de GitHub."},
		},
	}
}

// Check pide el perfil del usuario y exige que su blog apunte a la instancia.
func (g GitHub) Check(ctx context.Context, cfg Config) (string, error) {
	inst, err := g.Instance(ctx)
	if err != nil {
		return "", err
	}
	user := cfg["username"]
	var u struct{ Login, Blog string }
	if err := g.get(ctx, cfg, "/users/"+url.PathEscape(user), &u); err != nil {
		return "", err
	}
	if !blogPointsTo(u.Blog, inst.Host.String, inst.Username) {
		return "", fmt.Errorf("el sitio web del perfil de GitHub de %s es %q: tiene que ser %s", u.Login, u.Blog, inst.Host.String)
	}
	return "Identidad comprobada como " + u.Login, nil
}

// IsContributor dice si el usuario configurado figura entre los contribuidores de repo ("dueño/nombre"). Devuelve
// también la URL consultada, que es la prueba que guarda la medalla.
func (g GitHub) IsContributor(ctx context.Context, cfg Config, repo string) (bool, string, error) {
	const perPage = 100
	path := "/repos/" + repo + "/contributors?per_page=100"
	for page := 1; page <= 10; page++ { // 1000 contribuidores alcanzan de sobra
		var list []struct{ Login string }
		if err := g.get(ctx, cfg, fmt.Sprintf("%s&page=%d", path, page), &list); err != nil {
			return false, "", err
		}
		for _, c := range list {
			if strings.EqualFold(c.Login, cfg["username"]) {
				return true, g.api() + path, nil
			}
		}
		if len(list) < perPage {
			break
		}
	}
	return false, g.api() + path, nil
}

func (g GitHub) api() string {
	if g.API != "" {
		return g.API
	}
	return GitHubAPI
}

func (g GitHub) get(ctx context.Context, cfg Config, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.api()+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if t := cfg["token"]; t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return errors.New("no se pudo conectar con GitHub")
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return fmt.Errorf("GitHub no encontró %s", strings.SplitN(path, "?", 2)[0])
	case http.StatusUnauthorized:
		return errors.New("GitHub rechazó el token")
	case http.StatusForbidden, http.StatusTooManyRequests:
		return errors.New("GitHub limitó las consultas: agrega un token o prueba más tarde")
	default:
		return fmt.Errorf("GitHub respondió HTTP %d", resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(out); err != nil {
		return fmt.Errorf("respuesta ilegible de GitHub: %w", err)
	}
	return nil
}

// blogPointsTo acepta https://<host> o https://<host>/@usuario, con o sin barra final. GitHub guarda el campo tal
// como lo escribe el dueño, así que sin esquema se asume el de la instancia.
func blogPointsTo(blog, host, username string) bool {
	h, err := url.Parse(host)
	if err != nil || h.Host == "" {
		return false
	}
	blog = strings.TrimSpace(blog)
	if !strings.Contains(blog, "://") {
		blog = h.Scheme + "://" + blog
	}
	b, err := url.Parse(blog)
	if err != nil || b.Scheme != h.Scheme || !strings.EqualFold(b.Host, h.Host) || b.RawQuery != "" || b.Fragment != "" {
		return false
	}
	p := strings.TrimSuffix(b.Path, "/")
	return p == "" || p == "/@"+username
}
