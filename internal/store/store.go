// Package store abre la base SQLite de la instancia, aplica las migraciones y genera respaldos (ADR-006).
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"time"

	_ "github.com/ncruces/go-sqlite3/driver"
	"github.com/pressly/goose/v3"
)

//go:generate go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate

//go:embed migrations/*.sql
var migrations embed.FS

// FileName es el nombre de la base dentro del directorio de datos.
const FileName = "gremio.db"

// Store agrupa las dos conexiones: un solo escritor (evita SQLITE_BUSY entre escrituras) y un pool de lectura.
type Store struct {
	Write *Queries
	Read  *Queries

	writer, reader *sql.DB
	dataDir        string
}

// Open crea el directorio de datos si hace falta, abre la base y aplica las migraciones pendientes.
// Si la base ya existía y hay migraciones pendientes, antes hace un respaldo en <dataDir>/backups.
func Open(ctx context.Context, dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("directorio de datos %s: %w", dataDir, err)
	}
	path := filepath.Join(dataDir, FileName)
	info, err := os.Stat(path)
	existed := err == nil && info.Size() > 0

	writer, err := sql.Open("sqlite3", dsn(path, "_txlock=immediate&_pragma=busy_timeout(5000)&_pragma=journal_mode(wal)&_pragma=foreign_keys(1)&_pragma=synchronous(normal)"))
	if err != nil {
		return nil, err
	}
	writer.SetMaxOpenConns(1)
	s := &Store{writer: writer, dataDir: dataDir}
	if err := s.migrate(ctx, existed); err != nil {
		writer.Close()
		return nil, err
	}

	reader, err := sql.Open("sqlite3", dsn(path, "mode=ro&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"))
	if err != nil {
		writer.Close()
		return nil, err
	}
	s.reader = reader
	s.Write, s.Read = New(writer), New(reader)
	return s, nil
}

func dsn(path, query string) string {
	return (&url.URL{Scheme: "file", OmitHost: true, Path: path, RawQuery: query}).String()
}

func (s *Store) migrate(ctx context.Context, existed bool) error {
	fsys, err := fs.Sub(migrations, "migrations")
	if err != nil {
		return err
	}
	p, err := goose.NewProvider(goose.DialectSQLite3, s.writer, fsys)
	if err != nil {
		return err
	}
	pending, err := p.HasPending(ctx)
	if err != nil {
		return fmt.Errorf("leer versión de la base: %w (si otro proceso de gremio está migrando, espera a que termine)", err)
	}
	if !pending {
		return nil
	}
	if existed {
		dest, err := s.Backup(ctx, "")
		if err != nil {
			return fmt.Errorf("respaldo previo a migrar: %w", err)
		}
		slog.Info("respaldo previo a migrar", "archivo", dest)
	}
	results, err := p.Up(ctx)
	if err != nil {
		return fmt.Errorf("migrar: %w (si otro proceso de gremio está migrando, espera a que termine)", err)
	}
	for _, r := range results {
		slog.Info("migración aplicada", "versión", r.Source.Version, "archivo", r.Source.Path)
	}
	return nil
}

// Backup escribe una copia consistente de la base con VACUUM INTO, sin detener el servidor.
// Con dest vacío la guarda en <dataDir>/backups/gremio-<fecha>.db. Nunca sobrescribe un archivo existente.
func (s *Store) Backup(ctx context.Context, dest string) (string, error) {
	if dest == "" {
		dir := filepath.Join(s.dataDir, "backups")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return "", err
		}
		dest = filepath.Join(dir, "gremio-"+time.Now().UTC().Format("20060102T150405.000Z")+".db")
	}
	if _, err := os.Stat(dest); err == nil {
		return "", fmt.Errorf("%s ya existe", dest)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	if _, err := s.writer.ExecContext(ctx, "VACUUM INTO ?", dest); err != nil {
		return "", err
	}
	return dest, nil
}

// Ping comprueba que las dos conexiones responden.
func (s *Store) Ping(ctx context.Context) error {
	return errors.Join(s.writer.PingContext(ctx), s.reader.PingContext(ctx))
}

func (s *Store) Close() error {
	return errors.Join(s.reader.Close(), s.writer.Close())
}
