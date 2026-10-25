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
		if err := Init(ctx, st.Write, bad); err == nil {
			t.Errorf("Init(%q) debería fallar", bad)
		}
	}

	if err := Init(ctx, st.Write, "duvan_b"); err != nil {
		t.Fatal(err)
	}
	if err := Init(ctx, st.Write, "otro"); !errors.Is(err, ErrAlreadyInitialized) {
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
