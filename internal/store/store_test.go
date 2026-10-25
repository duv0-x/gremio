package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func open(t *testing.T, dir string) *Store {
	t.Helper()
	s, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func backups(t *testing.T, dir string) []os.DirEntry {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(dir, "backups"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return entries
}

func TestOpenMigraYReabreSinRespaldo(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	ctx := context.Background()
	if err := s.Write.CreateInstance(ctx, CreateInstanceParams{Username: "ana", PrivateKeyPem: "priv", PublicKeyPem: "pub"}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	// Base nueva: no hay respaldo. Reabrir sin migraciones pendientes: tampoco.
	s = open(t, dir)
	if n := len(backups(t, dir)); n != 0 {
		t.Errorf("respaldos = %d, quería 0", n)
	}
	inst, err := s.Read.GetInstance(ctx)
	if err != nil || inst.Username != "ana" {
		t.Errorf("GetInstance = %+v, %v", inst, err)
	}
}

func TestSoloUnaInstancia(t *testing.T) {
	s := open(t, t.TempDir())
	ctx := context.Background()
	p := CreateInstanceParams{Username: "ana", PrivateKeyPem: "priv", PublicKeyPem: "pub"}
	if err := s.Write.CreateInstance(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := s.Write.CreateInstance(ctx, p); err == nil {
		t.Error("una segunda fila en instance debería fallar")
	}
}

func TestLecturaEsDeSoloLectura(t *testing.T) {
	s := open(t, t.TempDir())
	if _, err := s.reader.Exec(`DELETE FROM instance`); err == nil {
		t.Error("el pool de lectura no debería poder escribir")
	}
}

func TestRespaldoAntesDeMigrarUnaBaseExistente(t *testing.T) {
	dir := t.TempDir()
	// Una base existente sin las migraciones de Gremio aplicadas.
	db, err := sql.Open("sqlite3", dsn(filepath.Join(dir, FileName), ""))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE previa (x INTEGER); INSERT INTO previa VALUES (42)`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	open(t, dir)
	entries := backups(t, dir)
	if len(entries) != 1 {
		t.Fatalf("respaldos = %d, quería 1", len(entries))
	}
	bk, err := sql.Open("sqlite3", dsn(filepath.Join(dir, "backups", entries[0].Name()), "mode=ro"))
	if err != nil {
		t.Fatal(err)
	}
	defer bk.Close()
	var x int
	if err := bk.QueryRow(`SELECT x FROM previa`).Scan(&x); err != nil || x != 42 {
		t.Errorf("respaldo: x = %d, %v", x, err)
	}
	var n int
	if err := bk.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name = 'instance'`).Scan(&n); err != nil || n != 0 {
		t.Errorf("el respaldo debe ser de antes de migrar: tablas instance = %d, %v", n, err)
	}
}

func TestBackupNoSobrescribe(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	ctx := context.Background()
	if err := s.Write.CreateInstance(ctx, CreateInstanceParams{Username: "ana", PrivateKeyPem: "priv", PublicKeyPem: "pub"}); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "copia.db")
	if _, err := s.Backup(ctx, dest); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Backup(ctx, dest); err == nil {
		t.Error("Backup sobre un archivo existente debería fallar")
	}

	bk, err := sql.Open("sqlite3", dsn(dest, "mode=ro"))
	if err != nil {
		t.Fatal(err)
	}
	defer bk.Close()
	var user string
	if err := bk.QueryRow(`SELECT username FROM instance`).Scan(&user); err != nil || user != "ana" {
		t.Errorf("copia: username = %q, %v", user, err)
	}
}
