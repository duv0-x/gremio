// Command gremio es el servidor de una instancia de Gremio (ADR-007).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/duv0-x/gremio/internal/activity"
	"github.com/duv0-x/gremio/internal/dojo"
	"github.com/duv0-x/gremio/internal/federation"
	"github.com/duv0-x/gremio/internal/instance"
	"github.com/duv0-x/gremio/internal/mail"
	"github.com/duv0-x/gremio/internal/profile"
	"github.com/duv0-x/gremio/internal/store"
	"github.com/duv0-x/gremio/internal/web"
)

const usage = `uso: gremio <comando> [flags]

Comandos:
  serve        arranca el servidor
  admin init   crea al dueño de la instancia y su par de llaves
  admin set-email cambia el correo al que llegan los enlaces para entrar al panel
  admin profile cambia nombre, resumen o avatar del perfil (avisa a los seguidores)
  admin import reemplaza el perfil por un JSON Resume (muestra qué cambia y pide confirmar)
  admin export escribe el perfil como JSON Resume por stdout
  admin cv-pdf publica el PDF del CV para el botón "Descargar CV" (reemplaza el anterior)
  admin media  guarda una imagen (p. ej. la captura de un proyecto) e imprime su URL
  backup       respaldo completo en .tar.gz: la base (copia consistente), el CV en PDF y media/
  healthcheck  pide /readyz al servidor local (el HEALTHCHECK de la imagen, que no tiene curl)

Variables de entorno: GREMIO_DATA_DIR (directorio de datos), GREMIO_ADDR (dirección de escucha),
GREMIO_HOST (URL pública) y GREMIO_ACCOUNT_DOMAIN (dominio del handle). El login del panel envía el
enlace por SMTP con TLS: GREMIO_SMTP_HOST, GREMIO_SMTP_PORT, GREMIO_SMTP_USERNAME, GREMIO_SMTP_FROM y
GREMIO_SMTP_PASSWORD (la clave solo por entorno).
Usa "gremio <comando> -h" para ver sus flags.
`

// version se fija al compilar con -ldflags "-X main.version=1.2.3" (sin la "v" del tag, como lo hace GoReleaser).
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
	case args[0] == "admin" && len(args) > 1 && args[1] == "set-email":
		return adminSetEmail(ctx, args[2:])
	case args[0] == "admin" && len(args) > 1 && args[1] == "profile":
		return adminProfile(ctx, args[2:])
	case args[0] == "admin" && len(args) > 1 && args[1] == "import":
		return adminImport(ctx, args[2:])
	case args[0] == "admin" && len(args) > 1 && args[1] == "export":
		return adminExport(ctx, args[2:])
	case args[0] == "admin" && len(args) > 1 && args[1] == "cv-pdf":
		return adminCVPDF(ctx, args[2:])
	case args[0] == "admin" && len(args) > 1 && args[1] == "media":
		return adminMedia(ctx, args[2:])
	case args[0] == "backup":
		return backup(ctx, args[1:])
	case args[0] == "healthcheck":
		return healthcheck(ctx, args[1:])
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
	smtp := mail.SMTP{Password: os.Getenv("GREMIO_SMTP_PASSWORD")}
	fs.StringVar(&smtp.Host, "smtp-host", envOr("GREMIO_SMTP_HOST", ""), "servidor SMTP para el enlace del panel")
	fs.IntVar(&smtp.Port, "smtp-port", 587, "puerto SMTP: 587 con STARTTLS o 465 con TLS (o GREMIO_SMTP_PORT)")
	fs.StringVar(&smtp.Username, "smtp-username", envOr("GREMIO_SMTP_USERNAME", ""), "usuario SMTP; la clave va en GREMIO_SMTP_PASSWORD")
	fs.StringVar(&smtp.From, "smtp-from", envOr("GREMIO_SMTP_FROM", ""), "remitente, p. ej. Gremio <gremio@ejemplo.org>")
	if p := os.Getenv("GREMIO_SMTP_PORT"); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil {
			return fmt.Errorf("GREMIO_SMTP_PORT=%q: %w", p, err)
		}
		smtp.Port = n
	}
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

	// Un solo libro del dojo para toda la instancia: los suscriptores (medallas, actividad) se registran sobre este.
	ledger := dojo.NewLedger(st)
	ledger.Subscribe(activity.Subscriber(fed, ledger, inst.Host.String+"/@"+inst.Username))

	mux := http.NewServeMux()
	fed.Register(mux)
	var mailer web.Mailer
	if smtp.Configured() {
		mailer = smtp
	} else {
		slog.Warn("SMTP sin configurar: no se puede entrar al panel", "arreglo", "GREMIO_SMTP_HOST y GREMIO_SMTP_FROM")
	}
	if !inst.OwnerEmail.Valid {
		slog.Warn("la instancia no tiene correo del dueño: no se puede entrar al panel", "arreglo", "gremio admin set-email <correo>")
	}
	web.New(st, version, mailer, fed, ledger).Register(mux)
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
	email := fs.String("email", "", "correo del dueño, al que llegan los enlaces para entrar al panel")
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
	if *email != "" {
		if err := instance.SetOwnerEmail(ctx, st, *email); err != nil {
			return err
		}
	}
	slog.Info("instancia creada", "usuario", *username)
	return nil
}

func adminSetEmail(ctx context.Context, args []string) error {
	fs, dataDir := newFlags("admin set-email")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("uso: gremio admin set-email <correo>")
	}
	st, err := store.Open(ctx, *dataDir)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := instance.SetOwnerEmail(ctx, st, fs.Arg(0)); err != nil {
		return err
	}
	slog.Info("correo del dueño actualizado", "nota", "no inicia sesión: pide el enlace en /login")
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
	out := fs.String("out", "", "archivo .tar.gz de destino (por defecto <data-dir>/backups/gremio-<fecha>.tar.gz)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	st, err := store.Open(ctx, *dataDir)
	if err != nil {
		return err
	}
	defer st.Close()
	dest, err := backupArchive(ctx, st, *dataDir, *out)
	if err != nil {
		return err
	}
	slog.Info("respaldo creado", "archivo", dest, "restaurar", "tar -xzf <archivo> -C <data-dir vacío>")
	return nil
}

func adminCVPDF(ctx context.Context, args []string) error {
	fs, dataDir := newFlags("admin cv-pdf")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("uso: gremio admin cv-pdf <archivo.pdf>")
	}
	data, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return err
	}
	st, err := store.Open(ctx, *dataDir) // valida que el directorio sea el de una instancia
	if err != nil {
		return err
	}
	defer st.Close()
	if err := web.SaveCV(st.DataDir(), data); err != nil {
		return err
	}
	slog.Info("CV publicado", "bytes", len(data), "url", "/@<usuario>/cv.pdf")
	return nil
}

// healthcheck pide /readyz al serve local y falla si no responde 200. La imagen OCI no tiene shell ni curl.
func healthcheck(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("healthcheck", flag.ContinueOnError)
	addr := fs.String("addr", envOr("GREMIO_ADDR", "127.0.0.1:8080"), "dirección de escucha del servidor")
	if err := fs.Parse(args); err != nil {
		return err
	}
	host, port, err := net.SplitHostPort(*addr)
	if err != nil {
		return err
	}
	if ip := net.ParseIP(host); host == "" || ip != nil && ip.IsUnspecified() {
		host = "127.0.0.1" // escucha en todas las interfaces: se le pregunta por loopback
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+net.JoinHostPort(host, port)+"/readyz", nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("readyz: %s", resp.Status)
	}
	return nil
}

// adminMedia guarda imágenes en <data-dir>/media e imprime sus URLs, para usarlas en el perfil (projects[].image).
func adminMedia(ctx context.Context, args []string) error {
	fs, dataDir := newFlags("admin media")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return errors.New("uso: gremio admin media <imagen>…")
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
	for _, path := range fs.Args() {
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		if info.Size() > federation.MaxAvatarBytes {
			return fmt.Errorf("%s pesa %d bytes; el máximo es %d", path, info.Size(), federation.MaxAvatarBytes)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		url, err := fed.SaveMedia(data)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		fmt.Printf("%s\t%s\n", path, url)
	}
	return nil
}
