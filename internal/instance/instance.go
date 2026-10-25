// Package instance gestiona al dueño único de la instancia y su par de llaves (ADR-001).
package instance

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"database/sql"
	"encoding/pem"
	"errors"
	"fmt"
	"regexp"

	"github.com/duv0-x/gremio/internal/store"
)

// usernameRe sigue la regla de GoToSocial ([a-z0-9_]); el handle es permanente una vez que la instancia federa.
var usernameRe = regexp.MustCompile(`^[a-z0-9_]{1,30}$`)

// ErrAlreadyInitialized indica que la instancia ya tiene dueño.
var ErrAlreadyInitialized = errors.New("la instancia ya tiene dueño")

// ValidateUsername comprueba el formato del usuario del dueño.
func ValidateUsername(username string) error {
	if !usernameRe.MatchString(username) {
		return fmt.Errorf("usuario %q inválido: usa entre 1 y 30 caracteres a-z, 0-9 o _", username)
	}
	return nil
}

// Init crea al dueño de la instancia con un par de llaves RSA 2048 nuevo.
func Init(ctx context.Context, q *store.Queries, username string) error {
	if err := ValidateUsername(username); err != nil {
		return err
	}
	if inst, err := q.GetInstance(ctx); err == nil {
		return fmt.Errorf("%w: @%s", ErrAlreadyInitialized, inst.Username)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return err
	}
	return q.CreateInstance(ctx, store.CreateInstanceParams{
		Username:      username,
		PrivateKeyPem: string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: mustPKCS8(key)})),
		PublicKeyPem:  string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})),
	})
}

func mustPKCS8(key *rsa.PrivateKey) []byte {
	b, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		panic(err) // solo falla con tipos de llave no soportados; RSA siempre lo está
	}
	return b
}
