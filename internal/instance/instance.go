// Package instance gestiona al dueño único de la instancia, su par de llaves y su dominio (ADR-001).
package instance

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/url"
	"regexp"

	"github.com/duv0-x/gremio/internal/store"
)

// usernameRe sigue la regla de GoToSocial ([a-z0-9_]); el handle es permanente una vez que la instancia federa.
var usernameRe = regexp.MustCompile(`^[a-z0-9_]{1,30}$`)

var (
	// ErrAlreadyInitialized indica que la instancia ya tiene dueño.
	ErrAlreadyInitialized = errors.New("la instancia ya tiene dueño")
	// ErrNotInitialized indica que falta correr `gremio admin init`.
	ErrNotInitialized = errors.New("la instancia no tiene dueño: corre `gremio admin init --username <usuario>`")
)

// ValidateUsername comprueba el formato del usuario del dueño.
func ValidateUsername(username string) error {
	if !usernameRe.MatchString(username) {
		return fmt.Errorf("usuario %q inválido: usa entre 1 y 30 caracteres a-z, 0-9 o _", username)
	}
	return nil
}

// Init crea al dueño de la instancia con un par de llaves RSA 2048 nuevo y un perfil mínimo.
func Init(ctx context.Context, st *store.Store, username string) error {
	if err := ValidateUsername(username); err != nil {
		return err
	}
	if inst, err := st.Write.GetInstance(ctx); err == nil {
		return fmt.Errorf("%w: @%s", ErrAlreadyInitialized, inst.Username)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}
	priv, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return err
	}
	pub, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return err
	}
	doc, err := json.Marshal(map[string]any{"basics": map[string]string{"name": username}})
	if err != nil {
		return err
	}
	return st.Tx(ctx, func(q *store.Queries) error {
		if err := q.CreateInstance(ctx, store.CreateInstanceParams{
			Username:      username,
			PrivateKeyPem: string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: priv})),
			PublicKeyPem:  string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pub})),
		}); err != nil {
			return err
		}
		return q.CreateProfile(ctx, string(doc))
	})
}

// Load devuelve la instancia; ErrNotInitialized si todavía no tiene dueño.
func Load(ctx context.Context, q *store.Queries) (store.Instance, error) {
	inst, err := q.GetInstance(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return inst, ErrNotInitialized
	}
	return inst, err
}

// BindHost fija host y account-domain la primera vez que la instancia arranca y, desde entonces,
// rechaza cualquier otro valor: las demás instancias cachean el Actor y sus llaves para siempre (ADR-001).
// host es una URL base (https://ejemplo.org); accountDomain vacío significa el hostname de host.
func BindHost(ctx context.Context, st *store.Store, host, accountDomain string) (store.Instance, error) {
	u, err := url.Parse(host)
	if err != nil || u.Host == "" || (u.Path != "" && u.Path != "/") || u.RawQuery != "" {
		return store.Instance{}, fmt.Errorf("host %q inválido: usa una URL base como https://gremio.ejemplo.org", host)
	}
	base := u.Scheme + "://" + u.Host
	if accountDomain == "" {
		accountDomain = u.Hostname()
	}
	inst, err := Load(ctx, st.Write)
	if err != nil {
		return inst, err
	}
	if !inst.Host.Valid {
		if _, err := st.Write.BindInstanceHost(ctx, store.BindInstanceHostParams{
			Host:          sql.NullString{String: base, Valid: true},
			AccountDomain: sql.NullString{String: accountDomain, Valid: true},
		}); err != nil {
			return inst, err
		}
		return Load(ctx, st.Write)
	}
	if inst.Host.String != base || inst.AccountDomain.String != accountDomain {
		return inst, fmt.Errorf("la instancia ya federa como host %s y account-domain %s; no se pueden cambiar (pediste %s y %s)",
			inst.Host.String, inst.AccountDomain.String, base, accountDomain)
	}
	return inst, nil
}

// PrivateKey decodifica la llave privada guardada.
func PrivateKey(inst store.Instance) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(inst.PrivateKeyPem))
	if block == nil {
		return nil, errors.New("llave privada: PEM inválido")
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	rk, ok := k.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("llave privada: no es RSA")
	}
	return rk, nil
}
