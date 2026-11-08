package retos

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/duv0-x/gremio/internal/dojo"
	"github.com/duv0-x/gremio/internal/store"
)

// Own es un reto propio (ADR-002 §1 y §4): un borrador editable y sus versiones publicadas, que son inmutables.
// Cuenta como práctica propia, N2 como máximo (ADR-013 §4), aunque lo haya generado una IA o tenga checks.
type Own struct {
	ID       string
	Draft    Challenge
	Latest   *Challenge // última versión publicada; nil si nunca se publicó
	Versions int
}

// CreateOwn guarda un borrador nuevo y devuelve su id. El borrador no tiene que ser válido todavía: se valida al
// publicar, y el panel muestra lo que falta.
func (s *Service) CreateOwn(ctx context.Context, c Challenge) (string, error) {
	c.ID = strings.ToLower(rand.Text()[:12])
	doc, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	now := s.now()
	return c.ID, s.st.Write.CreateChallenge(ctx, store.CreateChallengeParams{
		ID: c.ID, Draft: sql.NullString{String: string(doc), Valid: true}, CreatedAt: now, UpdatedAt: now,
	})
}

// SaveOwn reemplaza el borrador. El id no cambia aunque el formulario traiga otro.
func (s *Service) SaveOwn(ctx context.Context, id string, c Challenge) error {
	c.ID = id
	doc, err := json.Marshal(c)
	if err != nil {
		return err
	}
	n, err := s.st.Write.SaveChallengeDraft(ctx, store.SaveChallengeDraftParams{
		Draft: sql.NullString{String: string(doc), Valid: true}, UpdatedAt: s.now(), ID: id,
	})
	if n == 0 && err == nil {
		return ErrNotFound
	}
	return err
}

func (s *Service) GetOwn(ctx context.Context, id string) (Own, error) {
	row, err := s.st.Read.GetChallenge(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Own{}, ErrNotFound
	}
	if err != nil {
		return Own{}, err
	}
	o := Own{ID: row.ID}
	if row.Draft.Valid {
		if err := json.Unmarshal([]byte(row.Draft.String), &o.Draft); err != nil {
			return o, fmt.Errorf("reto %s: borrador: %w", id, err)
		}
	}
	v, err := s.st.Read.LatestChallengeVersion(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return o, nil
	}
	if err != nil {
		return o, err
	}
	var c Challenge
	if err := json.Unmarshal([]byte(v.Document), &c); err != nil {
		return o, fmt.Errorf("reto %s v%d: %w", id, v.Version, err)
	}
	o.Latest, o.Versions = &c, int(v.Version)
	return o, nil
}

func (s *Service) ListOwn(ctx context.Context) ([]Own, error) {
	rows, err := s.st.Read.ListChallenges(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Own, 0, len(rows))
	for _, r := range rows {
		o, err := s.GetOwn(ctx, r.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, nil
}

// OwnVersion devuelve una versión publicada de un reto propio.
func (s *Service) OwnVersion(ctx context.Context, id string, n int) (Challenge, error) {
	v, err := s.st.Read.GetChallengeVersion(ctx, store.GetChallengeVersionParams{ChallengeID: id, Version: int64(n)})
	if errors.Is(err, sql.ErrNoRows) {
		return Challenge{}, ErrNotFound
	}
	if err != nil {
		return Challenge{}, err
	}
	var c Challenge
	return c, json.Unmarshal([]byte(v.Document), &c)
}

// PublishOwn valida el borrador y lo publica como versión nueva, inmutable (ADR-002 §3). El autor es el dueño:
// la IA nunca figura como autora (ADR-002 §4).
func (s *Service) PublishOwn(ctx context.Context, id, author string) (int, error) {
	o, err := s.GetOwn(ctx, id)
	if err != nil {
		return 0, err
	}
	c := o.Draft
	c.ID, c.Version, c.Author, c.Reviewer = id, o.Versions+1, author, ""
	if c.License == "" {
		c.License = LicenseDefault
	}
	if err := c.Validate(); err != nil {
		return 0, err
	}
	for _, s := range c.Skills {
		if _, ok := dojo.FindSkill(s.Skill); !ok {
			return 0, fmt.Errorf("la habilidad %q no está en el catálogo de habilidades", s.Skill)
		}
	}
	doc, err := json.Marshal(c)
	if err != nil {
		return 0, err
	}
	err = s.st.Write.CreateChallengeVersion(ctx, store.CreateChallengeVersionParams{
		ChallengeID: id, Version: int64(c.Version), Document: string(doc), PublishedAt: s.now(),
	})
	return c.Version, err
}

// TakeOwn toma la última versión publicada de un reto propio. Su id de linaje es su URL pública, o el de su
// origen si es copia de otro (basedOn), para que las copias sumen una sola vez (ADR-013 §3).
func (s *Service) TakeOwn(ctx context.Context, id string, urls URLs) (Execution, error) {
	o, err := s.GetOwn(ctx, id)
	if err != nil {
		return Execution{}, err
	}
	if o.Latest == nil {
		return Execution{}, errors.New("publica el reto antes de tomarlo")
	}
	lineage := urls.Own(id)
	if o.Latest.BasedOn != "" {
		lineage = o.Latest.BasedOn
	}
	return s.take(ctx, lineage, *o.Latest)
}
