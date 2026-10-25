package instance

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"testing"

	"github.com/duv0-x/gremio/internal/store"
)

func TestInit(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	for _, bad := range []string{"", "Ana", "ana-b", "ana.b", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"} {
		if err := Init(ctx, st, bad); err == nil {
			t.Errorf("Init(%q) debería fallar", bad)
		}
	}

	if err := Init(ctx, st, "duvan_b"); err != nil {
		t.Fatal(err)
	}
	if err := Init(ctx, st, "otro"); !errors.Is(err, ErrAlreadyInitialized) {
		t.Errorf("segundo Init: err = %v, quería ErrAlreadyInitialized", err)
	}

	inst, err := st.Read.GetInstance(ctx)
	if err != nil {
		t.Fatal(err)
	}
	priv, _ := pem.Decode([]byte(inst.PrivateKeyPem))
	pub, _ := pem.Decode([]byte(inst.PublicKeyPem))
	if priv == nil || pub == nil {
		t.Fatal("PEM inválido")
	}
	key, err := x509.ParsePKCS8PrivateKey(priv.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	pk, err := x509.ParsePKIXPublicKey(pub.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if !key.(*rsa.PrivateKey).PublicKey.Equal(pk) {
		t.Error("la llave pública no corresponde a la privada")
	}
}

func TestBindHostEsPermanente(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := BindHost(ctx, st, "https://gremio.ejemplo.org", ""); !errors.Is(err, ErrNotInitialized) {
		t.Errorf("sin dueño: err = %v, quería ErrNotInitialized", err)
	}
	if err := Init(ctx, st, "ana"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"gremio.ejemplo.org", "https://gremio.ejemplo.org/sub", "https://gremio.ejemplo.org/?x=1"} {
		if _, err := BindHost(ctx, st, bad, ""); err == nil {
			t.Errorf("BindHost(%q) debería fallar", bad)
		}
	}

	inst, err := BindHost(ctx, st, "https://gremio.ejemplo.org/", "")
	if err != nil {
		t.Fatal(err)
	}
	if inst.Host.String != "https://gremio.ejemplo.org" || inst.AccountDomain.String != "gremio.ejemplo.org" {
		t.Errorf("host = %q, account-domain = %q", inst.Host.String, inst.AccountDomain.String)
	}
	if _, err := BindHost(ctx, st, "https://gremio.ejemplo.org", "gremio.ejemplo.org"); err != nil {
		t.Errorf("mismos valores: %v", err)
	}
	if _, err := BindHost(ctx, st, "https://otro.ejemplo.org", ""); err == nil {
		t.Error("cambiar el host debería fallar")
	}
	if _, err := BindHost(ctx, st, "https://gremio.ejemplo.org", "ejemplo.org"); err == nil {
		t.Error("cambiar el account-domain debería fallar")
	}
}
