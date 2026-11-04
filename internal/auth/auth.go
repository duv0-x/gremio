// Package auth es el login del panel (ADR-009): enlaces de un solo uso y sesiones, guardados como hash SHA-256.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"errors"
	"time"

	"github.com/duv0-x/gremio/internal/store"
)

const (
	LinkTTL    = 15 * time.Minute
	SessionTTL = 30 * 24 * time.Hour
	// La sesión se renueva al usarla, pero como mucho una vez al día, para no escribir en cada petición.
	renewAfter = 24 * time.Hour

	linksPerHour = 5
	linkInterval = time.Minute

	timeLayout = "2006-01-02T15:04:05.000Z"
)

var (
	ErrRateLimited = errors.New("ya se envió un enlace hace poco: espera un minuto (máximo 5 por hora)")
	ErrInvalidLink = errors.New("el enlace venció, ya se usó o se pidió uno más nuevo")
	ErrNoSession   = errors.New("sin sesión")
)

type Auth struct {
	st  *store.Store
	now func() time.Time
}

func New(st *store.Store) *Auth { return &Auth{st: st, now: time.Now} }

func (a *Auth) stamp(t time.Time) string { return t.UTC().Format(timeLayout) }

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hash(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

// RequestLink crea un enlace nuevo, que invalida al anterior, y devuelve su token. Respeta el límite de envíos.
func (a *Auth) RequestLink(ctx context.Context) (string, error) {
	token, err := newToken()
	if err != nil {
		return "", err
	}
	now := a.now()
	err = a.st.Tx(ctx, func(q *store.Queries) error {
		if n, err := q.CountLoginLinksSince(ctx, a.stamp(now.Add(-linkInterval))); err != nil {
			return err
		} else if n > 0 {
			return ErrRateLimited
		}
		if n, err := q.CountLoginLinksSince(ctx, a.stamp(now.Add(-time.Hour))); err != nil {
			return err
		} else if n >= linksPerHour {
			return ErrRateLimited
		}
		if err := q.DeleteLoginLinksBefore(ctx, a.stamp(now.Add(-time.Hour))); err != nil {
			return err
		}
		return q.CreateLoginLink(ctx, store.CreateLoginLinkParams{TokenHash: hash(token), CreatedAt: a.stamp(now)})
	})
	if err != nil {
		return "", err
	}
	return token, nil
}

// validLink devuelve el id del enlace si token es el último pedido, no se ha usado y no ha vencido.
func (a *Auth) validLink(ctx context.Context, q *store.Queries, token string) (int64, error) {
	l, err := q.LatestLoginLink(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrInvalidLink
	} else if err != nil {
		return 0, err
	}
	if subtle.ConstantTimeCompare(l.TokenHash, hash(token)) != 1 || l.UsedAt.Valid || l.CreatedAt <= a.stamp(a.now().Add(-LinkTTL)) {
		return 0, ErrInvalidLink
	}
	return l.ID, nil
}

// CheckLink dice si el enlace sirve, sin gastarlo: abrirlo (GET) no inicia sesión, para que los escáneres de
// correo no lo consuman.
func (a *Auth) CheckLink(ctx context.Context, token string) error {
	_, err := a.validLink(ctx, a.st.Read, token)
	return err
}

// UseLink gasta el enlace y abre una sesión; devuelve el token de la sesión para la cookie.
func (a *Auth) UseLink(ctx context.Context, token string) (string, error) {
	session, err := newToken()
	if err != nil {
		return "", err
	}
	now := a.now()
	err = a.st.Tx(ctx, func(q *store.Queries) error {
		id, err := a.validLink(ctx, q, token)
		if err != nil {
			return err
		}
		if n, err := q.UseLoginLink(ctx, store.UseLoginLinkParams{UsedAt: sql.NullString{String: a.stamp(now), Valid: true}, ID: id}); err != nil {
			return err
		} else if n != 1 {
			return ErrInvalidLink
		}
		if err := q.DeleteExpiredSessions(ctx, a.stamp(now)); err != nil {
			return err
		}
		return q.CreateSession(ctx, store.CreateSessionParams{TokenHash: hash(session), ExpiresAt: a.stamp(now.Add(SessionTTL))})
	})
	if err != nil {
		return "", err
	}
	return session, nil
}

// Session comprueba la sesión y la renueva si hace más de un día que no se renueva; renewed dice si hay que
// volver a mandar la cookie.
func (a *Auth) Session(ctx context.Context, token string) (renewed bool, err error) {
	if token == "" {
		return false, ErrNoSession
	}
	now := a.now()
	exp, err := a.st.Read.GetSessionExpiry(ctx, store.GetSessionExpiryParams{TokenHash: hash(token), ExpiresAt: a.stamp(now)})
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrNoSession
	} else if err != nil {
		return false, err
	}
	if exp > a.stamp(now.Add(SessionTTL-renewAfter)) {
		return false, nil
	}
	err = a.st.Write.RenewSession(ctx, store.RenewSessionParams{ExpiresAt: a.stamp(now.Add(SessionTTL)), TokenHash: hash(token)})
	return err == nil, err
}

func (a *Auth) Logout(ctx context.Context, token string) error {
	return a.st.Write.DeleteSession(ctx, hash(token))
}

// LogoutAll cierra todas las sesiones, en todos los dispositivos.
func (a *Auth) LogoutAll(ctx context.Context) error { return a.st.Write.DeleteAllSessions(ctx) }
