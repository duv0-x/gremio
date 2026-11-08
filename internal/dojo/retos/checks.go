package retos

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"
)

// Check es una comprobación reproducible sobre el árbol del repo en el commit fijado (ADR-013 §4). Son
// declarativas a propósito: Gremio nunca ejecuta código de la entrega. Un check que corre tests (el resultado
// del CI de un workflow fijado, con su attestation) es el camino a N4 y será otro Kind.
type Check struct {
	ID          string `json:"id"`
	Description string `json:"description"` // qué demuestra, para la persona que lo lee
	// Kind: "file" (al menos Min archivos coinciden con Path), "contains" (al menos Min archivos de Path
	// contienen Pattern) o "absent" (ningún archivo de Path existe o, con Pattern, ninguno lo contiene).
	Kind    string `json:"kind"`
	Path    string `json:"path"`              // glob sobre la ruta relativa; "**" cruza directorios
	Pattern string `json:"pattern,omitempty"` // expresión regular RE2
	Min     int    `json:"min,omitempty"`     // por defecto 1
}

// Result es el resultado de un check.
type Result struct {
	Check  string `json:"check"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail"`
}

// Verification es lo que queda de una corrida: con qué commit, cuándo y qué dio cada check.
type Verification struct {
	Repo    string    `json:"repo"`
	Commit  string    `json:"commit"`
	Archive string    `json:"archive"` // URL exacta que se descargó
	At      time.Time `json:"at"`
	Results []Result  `json:"results"`
	Passed  bool      `json:"passed"`
}

// Límites al leer una entrega ajena.
const (
	maxArchive = 50 << 20  // tarball comprimido
	maxTotal   = 200 << 20 // contenido descomprimido
	maxFile    = 1 << 20   // un archivo más grande no se revisa con Pattern
	maxEntries = 50_000
	timeout    = 60 * time.Second
)

var (
	commitRe  = regexp.MustCompile(`^[0-9a-f]{40}$`)
	segmentRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
)

// Validate revisa un check antes de aceptarlo en un reto.
func (c Check) Validate() error {
	if c.ID == "" || c.Description == "" {
		return errors.New("el check necesita id y descripción")
	}
	if c.Path == "" || strings.HasPrefix(c.Path, "/") || strings.Contains(c.Path, "..") {
		return fmt.Errorf("check %s: ruta %q inválida", c.ID, c.Path)
	}
	if _, err := path.Match(strings.ReplaceAll(c.Path, "**", "x"), ""); err != nil {
		return fmt.Errorf("check %s: glob %q: %w", c.ID, c.Path, err)
	}
	if c.Min < 0 {
		return fmt.Errorf("check %s: min negativo", c.ID)
	}
	switch c.Kind {
	case "file", "absent":
	case "contains":
		if c.Pattern == "" {
			return fmt.Errorf("check %s: contains necesita pattern", c.ID)
		}
	default:
		return fmt.Errorf("check %s: kind %q no soportado", c.ID, c.Kind)
	}
	if c.Pattern != "" {
		if _, err := regexp.Compile(c.Pattern); err != nil {
			return fmt.Errorf("check %s: pattern: %w", c.ID, err)
		}
	}
	return nil
}

func segment(s string) bool { return segmentRe.MatchString(s) && strings.Trim(s, ".") != "" }

// Archive valida el repo y el commit y devuelve la URL del tarball de ese commit. Solo https y forges conocidos:
// la instancia no descarga de cualquier lado (SSRF) y el commit fijo hace la corrida reproducible.
func Archive(repo, commit string) (string, error) {
	if !commitRe.MatchString(commit) {
		return "", errors.New("el commit tiene que ser el SHA completo de 40 caracteres en minúsculas")
	}
	u, err := url.Parse(repo)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("el repo tiene que ser una URL https sin usuario, puerto ni parámetros")
	}
	parts := strings.Split(strings.TrimSuffix(strings.Trim(u.Path, "/"), ".git"), "/")
	if len(parts) != 2 || !segment(parts[0]) || !segment(parts[1]) {
		return "", errors.New("el repo tiene que tener la forma https://<forge>/<dueño>/<repo>")
	}
	owner, name := parts[0], parts[1]
	switch u.Host {
	case "github.com":
		return "https://codeload.github.com/" + owner + "/" + name + "/tar.gz/" + commit, nil
	case "codeberg.org":
		return "https://codeberg.org/" + owner + "/" + name + "/archive/" + commit + ".tar.gz", nil
	case "gitlab.com":
		return "https://gitlab.com/" + owner + "/" + name + "/-/archive/" + commit + "/" + name + "-" + commit + ".tar.gz", nil
	}
	return "", fmt.Errorf("forge %q no soportado: usa github.com, codeberg.org o gitlab.com", u.Host)
}

var archiveHosts = map[string]bool{"codeload.github.com": true, "codeberg.org": true, "gitlab.com": true}

// Client es el cliente HTTP para bajar entregas: timeout fijo y redirecciones solo a forges conocidos.
var Client = &http.Client{
	Timeout: timeout,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || req.URL.Scheme != "https" || !archiveHosts[req.URL.Host] {
			return errors.New("redirección fuera de los forges conocidos")
		}
		return nil
	},
}

// Run baja el repo en el commit fijado y corre los checks. Un error es un problema para bajar o leer la entrega;
// un check que no pasa no es un error, queda en el resultado.
func Run(ctx context.Context, client *http.Client, repo, commit string, checks []Check) (Verification, error) {
	v := Verification{Repo: repo, Commit: commit, At: time.Now().UTC().Truncate(time.Second)}
	archive, err := Archive(repo, commit)
	if err != nil {
		return v, err
	}
	v.Archive = archive
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, archive, nil)
	if err != nil {
		return v, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return v, fmt.Errorf("bajar la entrega: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return v, fmt.Errorf("bajar la entrega: %s respondió %s (¿el repo es público y el commit existe?)", archive, resp.Status)
	}
	v.Results, err = Evaluate(io.LimitReader(resp.Body, maxArchive+1), checks)
	if err != nil {
		return v, err
	}
	v.Passed = len(v.Results) > 0
	for _, r := range v.Results {
		v.Passed = v.Passed && r.Passed
	}
	return v, nil
}

// Evaluate lee un tar.gz de un forge (todo bajo un directorio raíz que se descarta) y corre los checks. Solo
// mira archivos regulares: los symlinks, hardlinks y rutas con ".." se ignoran, y nada se escribe a disco.
func Evaluate(r io.Reader, checks []Check) ([]Result, error) {
	type state struct {
		re    *regexp.Regexp
		count int
		hits  []string
	}
	st := make([]state, len(checks))
	for i, c := range checks {
		if err := c.Validate(); err != nil {
			return nil, err
		}
		if c.Pattern != "" {
			st[i].re = regexp.MustCompile(c.Pattern)
		}
	}
	counted := &countingReader{r: r}
	gz, err := gzip.NewReader(counted)
	if err != nil {
		return nil, fmt.Errorf("la entrega no es un tar.gz: %w", err)
	}
	tr := tar.NewReader(io.LimitReader(gz, maxTotal))
	for n := 0; ; n++ {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil && counted.n <= maxArchive {
			return nil, fmt.Errorf("leer la entrega: %w", err)
		}
		if counted.n > maxArchive {
			return nil, fmt.Errorf("la entrega pasa de %d MiB comprimida", maxArchive>>20)
		}
		if n >= maxEntries {
			return nil, fmt.Errorf("la entrega tiene más de %d archivos", maxEntries)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		name, ok := repoPath(hdr.Name)
		if !ok {
			continue
		}
		var body []byte
		for i, c := range checks {
			if !match(c.Path, name) {
				continue
			}
			if st[i].re == nil {
				st[i].count++
				st[i].hits = append(st[i].hits, name)
				continue
			}
			if body == nil {
				if hdr.Size > maxFile {
					continue
				}
				if body, err = io.ReadAll(io.LimitReader(tr, maxFile)); err != nil {
					return nil, fmt.Errorf("leer %s: %w", name, err)
				}
			}
			if st[i].re.Match(body) {
				st[i].count++
				st[i].hits = append(st[i].hits, name)
			}
		}
	}
	if counted.n > maxArchive {
		return nil, fmt.Errorf("la entrega pasa de %d MiB comprimida", maxArchive>>20)
	}
	results := make([]Result, len(checks))
	for i, c := range checks {
		min := max(c.Min, 1)
		res := Result{Check: c.ID}
		switch c.Kind {
		case "absent":
			res.Passed = st[i].count == 0
		default:
			res.Passed = st[i].count >= min
		}
		res.Detail = detail(c, st[i].count, min, st[i].hits)
		results[i] = res
	}
	return results, nil
}

func detail(c Check, count, min int, hits []string) string {
	if len(hits) > 3 {
		hits = append(hits[:3:3], "…")
	}
	list := strings.Join(hits, ", ")
	switch c.Kind {
	case "absent":
		if count == 0 {
			return "ningún archivo de " + c.Path
		}
		return fmt.Sprintf("encontrado en %s", list)
	default:
		if count == 0 {
			return fmt.Sprintf("ningún archivo de %s (se piden %d)", c.Path, min)
		}
		return fmt.Sprintf("%d de %d: %s", count, min, list)
	}
}

// repoPath quita el directorio raíz del tarball y descarta rutas absolutas o que salen del repo.
func repoPath(name string) (string, bool) {
	_, rest, ok := strings.Cut(name, "/")
	if !ok || rest == "" || strings.HasPrefix(rest, "/") || path.Clean(rest) != rest {
		return "", false
	}
	for _, seg := range strings.Split(rest, "/") {
		if seg == ".." {
			return "", false
		}
	}
	return rest, true
}

// match compara un glob de path.Match por segmentos; "**" coincide con cero o más directorios.
func match(pattern, name string) bool {
	return matchSegs(strings.Split(pattern, "/"), strings.Split(name, "/"))
}

func matchSegs(pat, segs []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			for i := 0; i <= len(segs); i++ {
				if matchSegs(pat[1:], segs[i:]) {
					return true
				}
			}
			return false
		}
		if len(segs) == 0 {
			return false
		}
		if ok, _ := path.Match(pat[0], segs[0]); !ok {
			return false
		}
		pat, segs = pat[1:], segs[1:]
	}
	return len(segs) == 0
}

type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}
