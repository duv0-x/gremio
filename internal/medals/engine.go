package medals

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/duv0-x/gremio/internal/dojo"
	"github.com/duv0-x/gremio/internal/plugins"
	"github.com/duv0-x/gremio/internal/profile"
	"github.com/duv0-x/gremio/internal/store"
)

// Repo es el repositorio cuya lista de contribuidores da Forjador del Gremio.
const Repo = "duv0-x/gremio"

// Engine otorga las medallas: las del dojo después de cada reto terminado, y las externas al arrancar y cada
// ExternalEvery en serve.
type Engine struct {
	st      *store.Store
	ledger  *dojo.Ledger
	plugins *plugins.Manager
	now     func() time.Time
}

func New(st *store.Store, ledger *dojo.Ledger, pm *plugins.Manager) *Engine {
	return &Engine{st: st, ledger: ledger, plugins: pm, now: func() time.Time { return time.Now().UTC() }}
}

// Subscribe engancha el motor al libro. Solo reacciona a los retos terminados: así ignora sus propios
// badgeAwarded y los retiros, y no entra en un loop.
func (en *Engine) Subscribe() {
	en.ledger.Subscribe(func(ctx context.Context, e dojo.Event) {
		if e.Type != dojo.ChallengeCompleted {
			return
		}
		if err := en.Sync(ctx); err != nil {
			slog.Error("medallas: no se pudieron evaluar", "err", err)
		}
	})
}

// Sync otorga las medallas del dojo que el libro ya cumple.
func (en *Engine) Sync(ctx context.Context) error {
	events, err := en.ledger.List(ctx)
	if err != nil {
		return err
	}
	c, err := en.context(ctx)
	if err != nil {
		return err
	}
	for _, e := range Evaluate(events, c) {
		if _, err := en.ledger.Append(ctx, e); err != nil {
			return err
		}
	}
	return nil
}

// context arma el Context de las reglas desde el perfil. Sin perfil, todo en UTC y sin atributos.
func (en *Engine) context(ctx context.Context) (Context, error) {
	doc, err := en.st.Read.GetProfile(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return Context{}, nil
	} else if err != nil {
		return Context{}, err
	}
	r, err := profile.Load(doc)
	if err != nil {
		return Context{}, err
	}
	c := Context{AttrOf: map[string]string{}}
	if tz := r.Gremio.Timezone; tz != "" {
		c.Location, _ = time.LoadLocation(tz) // Normalize ya validó la zona
	}
	for _, a := range r.Gremio.Attributes {
		for _, sub := range a.Subs {
			if sub.DojoSkill != "" {
				c.AttrOf[sub.DojoSkill] = a.Code
			}
		}
	}
	return c, nil
}

// CheckExternal repite el chequeo de identidad de GitHub (el panel ve el resultado) y, si la identidad se
// comprueba, otorga Forjador cuando el usuario figura entre los contribuidores de Repo.
func (en *Engine) CheckExternal(ctx context.Context) error {
	cfg, err := en.plugins.Config(ctx, "github")
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	} else if err != nil {
		return err
	}
	if _, err := en.plugins.Recheck(ctx, "github"); err != nil {
		return nil // identidad sin comprobar: el estado del plugin ya guarda el error
	}
	events, err := en.ledger.List(ctx)
	if err != nil {
		return err
	}
	for _, a := range Awarded(events) {
		if a.ID == Forjador {
			return nil
		}
	}
	p, _ := en.plugins.Find("github")
	gh := p.(plugins.GitHub)
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	ok, url, err := gh.IsContributor(cctx, cfg, Repo)
	if err != nil || !ok {
		return err
	}
	_, err = en.ledger.Append(ctx, award(Forjador, en.now(), &dojo.Proof{User: cfg["username"], URL: url}))
	return err
}

// ExternalEvery es cada cuánto serve repite las reglas externas. Sin token, GitHub deja 60 consultas por hora.
const ExternalEvery = 6 * time.Hour

// Run evalúa todo al arrancar (recupera las medallas que el catálogo sumó) y repite las externas cada
// ExternalEvery hasta que ctx termine.
func (en *Engine) Run(ctx context.Context) {
	tick := time.NewTicker(ExternalEvery)
	defer tick.Stop()
	if err := en.Sync(ctx); err != nil {
		slog.Error("medallas: no se pudieron evaluar", "err", err)
	}
	for {
		if err := en.CheckExternal(ctx); err != nil {
			slog.Warn("medallas: no se pudieron revisar las externas", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// Medals devuelve las medallas ganadas para mostrar. Una externa queda "sin comprobar" si el plugin GitHub está
// deshabilitado, su último chequeo falló o ahora apunta a otro usuario (ADR-015 §3): no se borra, porque registra
// un hecho pasado.
func (en *Engine) Medals(ctx context.Context) ([]Award, error) {
	events, err := en.ledger.List(ctx)
	if err != nil {
		return nil, err
	}
	awards := Awarded(events)
	states, err := en.plugins.List(ctx)
	if err != nil {
		return nil, err
	}
	var gh plugins.State
	for _, s := range states {
		if s.Info.ID == "github" {
			gh = s
		}
	}
	for i, a := range awards {
		if a.External {
			user := ""
			if a.Proof != nil {
				user = a.Proof.User
			}
			awards[i].Unverified = !gh.Enabled || strings.HasPrefix(gh.Status, "Error: ") || !strings.EqualFold(gh.Values["username"], user)
		}
	}
	return awards, nil
}
