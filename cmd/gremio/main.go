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
	"strings"
	"syscall"
	"time"

	"github.com/duv0-x/gremio/internal/federation"
	"github.com/duv0-x/gremio/internal/instance"
	"github.com/duv0-x/gremio/internal/profile"
	"github.com/duv0-x/gremio/internal/store"
)

const usage = `uso: gremio <comando> [flags]

Comandos:
  serve        arranca el servidor
  admin init   crea al dueño de la instancia y su par de llaves
  admin note   publica un Note público para los seguidores (lo entrega serve)
  admin profile cambia nombre, resumen o avatar del perfil (avisa a los seguidores)
  admin import reemplaza el perfil por un JSON Resume (muestra qué cambia y pide confirmar)
  admin export escribe el perfil como JSON Resume por stdout
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
	case args[0] == "admin" && len(args) > 1 && args[1] == "note":
		return adminNote(ctx, args[2:])
	case args[0] == "admin" && len(args) > 1 && args[1] == "profile":
		return adminProfile(ctx, args[2:])
	case args[0] == "admin" && len(args) > 1 && args[1] == "import":
		return adminImport(ctx, args[2:])
	case args[0] == "admin" && len(args) > 1 && args[1] == "export":
		return adminExport(ctx, args[2:])
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

	go fed.RunDeliveries(ctx)
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

func adminNote(ctx context.Context, args []string) error {
	fs, dataDir := newFlags("admin note")
	text := fs.String("text", "", "texto del Note")
	if err := fs.Parse(args); err != nil {
		return err
	}
	st, err := store.Open(ctx, *dataDir)
	if err != nil {
		return err
	}
	defer st.Close()
	fed, err := federation.New(ctx, st, federation.Options{Version: version})
	if err != nil {
		return fmt.Errorf("%w (arranca `gremio serve --host …` una vez para fijar el host)", err)
	}
	id, queued, err := fed.PublishNote(ctx, *text)
	if err != nil {
		return err
	}
	slog.Info("note publicado", "id", id, "entregas_encoladas", queued, "nota", "las envía `gremio serve`")
	return nil
}

func adminProfile(ctx context.Context, args []string) error {
	fs, dataDir := newFlags("admin profile")
	name := fs.String("name", "", "nombre visible (basics.name)")
	summary := fs.String("summary", "", "resumen o bio (basics.summary)")
	avatar := fs.String("avatar", "", "archivo PNG, JPEG o WebP de hasta 2 MB")
	if err := fs.Parse(args); err != nil {
		return err
	}
	var u federation.ProfileUpdate
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "name":
			u.Name = name
		case "summary":
			u.Summary = summary
		}
	})
	if *avatar != "" {
		info, err := os.Stat(*avatar)
		if err != nil {
			return err
		}
		if info.Size() > federation.MaxAvatarBytes {
			return fmt.Errorf("el avatar pesa %d bytes; el máximo es %d", info.Size(), federation.MaxAvatarBytes)
		}
		if u.Avatar, err = os.ReadFile(*avatar); err != nil {
			return err
		}
	}
	if u.Name == nil && u.Summary == nil && u.Avatar == nil {
		return errors.New("nada que cambiar: usa --name, --summary o --avatar")
	}
	st, err := store.Open(ctx, *dataDir)
	if err != nil {
		return err
	}
	defer st.Close()
	fed, err := federation.New(ctx, st, federation.Options{Version: version})
	if err != nil {
		return fmt.Errorf("%w (arranca `gremio serve --host …` una vez para fijar el host)", err)
	}
	queued, err := fed.UpdateProfile(ctx, u)
	if err != nil {
		return err
	}
	slog.Info("perfil actualizado", "updates_encolados", queued, "nota", "los envía `gremio serve`")
	return nil
}

func adminImport(ctx context.Context, args []string) error {
	fs, dataDir := newFlags("admin import")
	yes := fs.Bool("yes", false, "no pedir confirmación")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("uso: gremio admin import [--yes] <perfil.json>")
	}
	data, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return err
	}
	next, warnings, err := profile.Parse(data)
	if err != nil {
		return err
	}
	st, err := store.Open(ctx, *dataDir)
	if err != nil {
		return err
	}
	defer st.Close()
	fed, err := federation.New(ctx, st, federation.Options{Version: version})
	if err != nil {
		return fmt.Errorf("%w (arranca `gremio serve --host …` una vez para fijar el host)", err)
	}
	cur, err := fed.CurrentProfile(ctx)
	if err != nil {
		return err
	}
	for _, w := range warnings {
		fmt.Fprintln(os.Stderr, "aviso:", w)
	}
	fmt.Fprintln(os.Stderr, "El perfil importado REEMPLAZA al actual (no se mezcla):")
	for _, line := range profile.Diff(cur, next) {
		fmt.Fprintln(os.Stderr, "  "+line)
	}
	if !*yes {
		fmt.Fprint(os.Stderr, "¿Reemplazar el perfil? [s/N] ")
		var answer string
		_, _ = fmt.Scanln(&answer)
		if a := strings.ToLower(strings.TrimSpace(answer)); a != "s" && a != "si" && a != "sí" {
			return errors.New("cancelado: el perfil no cambió")
		}
	}
	queued, err := fed.ReplaceProfile(ctx, next)
	if err != nil {
		return err
	}
	slog.Info("perfil importado", "updates_encolados", queued, "nota", "los envía `gremio serve`")
	return nil
}

func adminExport(ctx context.Context, args []string) error {
	fs, dataDir := newFlags("admin export")
	ext := fs.Bool("with-extensions", false, "incluye ids, evidencia, vistas y visibilidad (clave gremio); sin esto sale JSON Resume estándar")
	if err := fs.Parse(args); err != nil {
		return err
	}
	st, err := store.Open(ctx, *dataDir)
	if err != nil {
		return err
	}
	defer st.Close()
	doc, err := st.Read.GetProfile(ctx)
	if err != nil {
		return err
	}
	r, err := profile.Load(doc)
	if err != nil {
		return err
	}
	b, err := r.Export(*ext)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(b)
	return err
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
