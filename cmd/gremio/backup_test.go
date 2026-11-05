package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/duv0-x/gremio/internal/instance"
	"github.com/duv0-x/gremio/internal/store"
)

// El respaldo trae la base, el CV y media/, y restaurarlo en un directorio vacío deja una instancia que sirve lo
// mismo (gremio#101).
func TestBackupCompletoYRestauracion(t *testing.T) {
	ctx := context.Background()
	data := t.TempDir()
	st, err := store.Open(ctx, data)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := instance.Init(ctx, st, "ana"); err != nil {
		t.Fatal(err)
	}
	write := func(name, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(data, name)), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(data, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("cv.pdf", "%PDF-1.7 cv")
	write("media/avatar-0123456789abcdef.jpg", "foto")
	write("media/media-fedcba9876543210.png", "captura")
	write("media/.media-temporal", "a medio escribir")

	out, err := backupArchive(ctx, st, data, "")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(out) != filepath.Join(data, "backups") || filepath.Ext(out) != ".gz" {
		t.Errorf("destino por defecto: %s", out)
	}
	if info, err := os.Stat(out); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("el respaldo trae la llave privada: debe ser 0600 (%v, %v)", info.Mode(), err)
	}
	if _, err := backupArchive(ctx, st, data, out); err == nil {
		t.Error("no debe sobrescribir un respaldo existente")
	}

	// Restaurar es extraerlo en un directorio vacío.
	restored := t.TempDir()
	if b, err := exec.Command("tar", "-xzf", out, "-C", restored).CombinedOutput(); err != nil {
		t.Fatalf("tar: %v %s", err, b)
	}
	for name, want := range map[string]string{
		"cv.pdf":                            "%PDF-1.7 cv",
		"media/avatar-0123456789abcdef.jpg": "foto",
		"media/media-fedcba9876543210.png":  "captura",
	} {
		if got, err := os.ReadFile(filepath.Join(restored, name)); err != nil || string(got) != want {
			t.Errorf("%s restaurado: %q, %v", name, got, err)
		}
	}
	for _, name := range []string{"media/.media-temporal", "backups"} {
		if _, err := os.Stat(filepath.Join(restored, name)); err == nil {
			t.Errorf("%s no va en el respaldo", name)
		}
	}
	rs, err := store.Open(ctx, restored)
	if err != nil {
		t.Fatal(err)
	}
	defer rs.Close()
	if inst, err := rs.Read.GetInstance(ctx); err != nil || inst.Username != "ana" {
		t.Errorf("la base restaurada: %+v, %v", inst, err)
	}
}
