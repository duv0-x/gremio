package retos

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"strings"
	"testing"
)

// forgeTar arma un tar.gz como los de los forges: todo bajo un directorio raíz.
func forgeTar(t *testing.T, files map[string]string, extra ...*tar.Header) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: "repo-abc/" + name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		tw.Write([]byte(body))
	}
	for _, h := range extra {
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		tw.Write(make([]byte, h.Size))
	}
	tw.Close()
	gz.Close()
	return &buf
}

func TestEvaluate(t *testing.T) {
	files := map[string]string{
		"go.mod":                   "module x\n",
		"internal/api/api.go":      "package api\nfunc Healthz() {}\n",
		"internal/api/api_test.go": "package api\nfunc TestHealthz(t *testing.T) {}\n",
		"deploy/k8s/deploy.yaml":   "kind: Deployment\nreadinessProbe:\n",
		"deploy/k8s/svc.yaml":      "kind: Service\n",
		"docs/adr/ADR-001-algo.md": "# ADR\n",
		"secrets/.env.example":     "TOKEN=\n",
	}
	evil := []*tar.Header{
		{Name: "repo-abc/link.go", Linkname: "/etc/passwd", Typeflag: tar.TypeSymlink},
		{Name: "repo-abc/../escape_test.go", Mode: 0o644, Size: 3, Typeflag: tar.TypeReg},
	}
	checks := []Check{
		{ID: "gomod", Description: "módulo", Kind: "file", Path: "go.mod"},
		{ID: "tests", Description: "dos tests", Kind: "contains", Path: "**/*_test.go", Pattern: `func Test`, Min: 2},
		{ID: "probe", Description: "readiness", Kind: "contains", Path: "deploy/**/*.yaml", Pattern: `readinessProbe:`},
		{ID: "adr", Description: "un ADR", Kind: "file", Path: "docs/adr/ADR-*.md"},
		{ID: "noenv", Description: "sin .env", Kind: "absent", Path: "**/.env"},
		{ID: "nolink", Description: "symlink ignorado", Kind: "file", Path: "link.go"},
	}
	got, err := Evaluate(forgeTar(t, files, evil...), checks)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"gomod": true, "tests": false, "probe": true, "adr": true, "noenv": true, "nolink": false}
	for _, r := range got {
		if r.Passed != want[r.Check] {
			t.Errorf("%s: passed=%v, quería %v (%s)", r.Check, r.Passed, want[r.Check], r.Detail)
		}
	}
	if !strings.Contains(got[1].Detail, "1 de 2") {
		t.Errorf("detalle de tests: %q", got[1].Detail)
	}
}

func TestMatch(t *testing.T) {
	cases := []struct {
		pat, name string
		want      bool
	}{
		{"**/*_test.go", "a_test.go", true},
		{"**/*_test.go", "x/y/a_test.go", true},
		{"*.go", "x/a.go", false},
		{"deploy/**/*.yaml", "deploy/a.yaml", true},
		{"deploy/**/*.yaml", "deploy/k/a.yaml", true},
		{"deploy/**/*.yaml", "other/a.yaml", false},
	}
	for _, c := range cases {
		if got := match(c.pat, c.name); got != c.want {
			t.Errorf("match(%q, %q) = %v", c.pat, c.name, got)
		}
	}
}

func TestArchive(t *testing.T) {
	sha := strings.Repeat("a", 40)
	ok := map[string]string{
		"https://github.com/duv0-x/gremio":      "https://codeload.github.com/duv0-x/gremio/tar.gz/" + sha,
		"https://github.com/duv0-x/gremio.git/": "https://codeload.github.com/duv0-x/gremio/tar.gz/" + sha,
		"https://codeberg.org/ana/x":            "https://codeberg.org/ana/x/archive/" + sha + ".tar.gz",
		"https://gitlab.com/ana/x":              "https://gitlab.com/ana/x/-/archive/" + sha + "/x-" + sha + ".tar.gz",
	}
	for repo, want := range ok {
		if got, err := Archive(repo, sha); err != nil || got != want {
			t.Errorf("Archive(%q) = %q, %v", repo, got, err)
		}
	}
	for _, repo := range []string{
		"http://github.com/a/b", "https://evil.example/a/b", "https://github.com/a", "https://github.com/a/b/c",
		"https://u:p@github.com/a/b", "https://github.com:8443/a/b", "https://github.com/../b", "https://192.168.1.1/a/b",
	} {
		if _, err := Archive(repo, sha); err == nil {
			t.Errorf("Archive(%q) debía fallar", repo)
		}
	}
	if _, err := Archive("https://github.com/a/b", "main"); err == nil {
		t.Error("una rama no es un commit fijado")
	}
}

func TestCheckValidate(t *testing.T) {
	for _, c := range []Check{
		{ID: "a", Description: "d", Kind: "run", Path: "x"},
		{ID: "a", Description: "d", Kind: "contains", Path: "x"},
		{ID: "a", Description: "d", Kind: "file", Path: "../x"},
		{ID: "a", Description: "d", Kind: "file", Path: "/etc/passwd"},
		{ID: "a", Description: "d", Kind: "contains", Path: "x", Pattern: "("},
	} {
		if c.Validate() == nil {
			t.Errorf("%+v debía ser inválido", c)
		}
	}
}
