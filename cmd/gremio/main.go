// Command gremio es el servidor de una instancia de Gremio (ADR-007).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/duv0-x/gremio/internal/federation"
	"github.com/duv0-x/gremio/internal/instance"
	"github.com/duv0-x/gremio/internal/store"
)

const usage = `uso: gremio <comando> [flags]

Comandos:
  serve        arranca el servidor
  admin init   crea al dueño de la instancia y su par de llaves
  backup       copia consistente de la base

Variables de entorno: GREMIO_DATA_DIR (directorio de datos), GREMIO_ADDR (dirección de escucha),
GREMIO_HOST (URL pública) y GREMIO_ACCOUNT_DOMAIN (dominio del handle).
Usa "gremio <comando> -h" para ver sus flags.
`

// version se fija al compilar con -ldflags "-X main.version=v1.2.3".
var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return errors.New("falta el comando")
	}
	switch {
	case args[0] == "serve":
		return serve(ctx, args[1:])
	case args[0] == "admin" && len(args) > 1 && args[1] == "init":
		return adminInit(ctx, args[2:])
	case args[0] == "backup":
		return backup(ctx, args[1:])
	default:
		fmt.Fprint(os.Stderr, usage)
		return fmt.Errorf("comando desconocido: %v", args)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func newFlags(name string) (*flag.FlagSet, *string) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	dataDir := fs.String("data-dir", envOr("GREMIO_DATA_DIR", "data"), "directorio de datos (base, respaldos)")
	return fs, dataDir
}

func serve(ctx context.Context, args []string) error {
	fs, dataDir := newFlags("serve")
	addr := fs.String("addr", envOr("GREMIO_ADDR", "127.0.0.1:8080"), "dirección de escucha")
	host := fs.String("host", envOr("GREMIO_HOST", ""), "URL pública de la instancia, p. ej. https://gremio.ejemplo.org (permanente)")
	accountDomain := fs.String("account-domain", envOr("GREMIO_ACCOUNT_DOMAIN", ""), "dominio del handle @usuario@dominio; por defecto el de --host (permanente)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *host == "" {
		return errors.New("falta --host (o GREMIO_HOST): la URL pública con la que federa la instancia")
	}
	st, err := store.Open(ctx, *dataDir)
	if err != nil {
		return err
	}
	defer st.Close()
	inst, err := instance.BindHost(ctx, st, *host, *accountDomain)
	if err != nil {
		return err
	}
	fed, err := federation.New(ctx, st, federation.Options{Version: version})
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	fed.Register(mux)
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := st.Ping(r.Context()); err != nil {
			http.Error(w, "base no disponible", http.StatusServiceUnavailable)
			return
		}
		fmt.Fprintln(w, "ok")
	})
	srv := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}

	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	slog.Info("escuchando", "addr", *addr, "actor", inst.Host.String+"/users/"+inst.Username,
		"handle", "@"+inst.Username+"@"+inst.AccountDomain.String, "version", version)
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		slog.Info("apagando")
		return srv.Shutdown(shutdownCtx)
	}
}

func adminInit(ctx context.Context, args []string) error {
	fs, dataDir := newFlags("admin init")
	username := fs.String("username", "", "usuario del dueño (a-z, 0-9, _); es permanente una vez que la instancia federa")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := instance.ValidateUsername(*username); err != nil {
		return err
	}
	st, err := store.Open(ctx, *dataDir)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := instance.Init(ctx, st, *username); err != nil {
		return err
	}
	slog.Info("instancia creada", "usuario", *username)
	return nil
}

func backup(ctx context.Context, args []string) error {
	fs, dataDir := newFlags("backup")
	out := fs.String("out", "", "archivo de destino (por defecto <data-dir>/backups/gremio-<fecha>.db)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	st, err := store.Open(ctx, *dataDir)
	if err != nil {
		return err
	}
	defer st.Close()
	dest, err := st.Backup(ctx, *out)
	if err != nil {
		return err
	}
	slog.Info("respaldo creado", "archivo", dest)
	return nil
}
