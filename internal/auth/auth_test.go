package auth

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/duv0-x/gremio/internal/store"
)

func newAuth(t *testing.T) (*Auth, *time.Time, string) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	a := New(st)
	a.now = func() time.Time { return now }
	return a, &now, dir
}

func TestEnlaceDeUnSoloUso(t *testing.T) {
	ctx := context.Background()
	a, _, dir := newAuth(t)
	tok, err := a.RequestLink(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.CheckLink(ctx, tok); err != nil {
		t.Fatalf("abrir el enlace (GET) debe ser válido: %v", err)
	}
	if err := a.CheckLink(ctx, tok+"x"); !errors.Is(err, ErrInvalidLink) {
		t.Errorf("token alterado: %v", err)
	}
	// CheckLink no lo gasta: sigue sirviendo para entrar, pero una sola vez.
	session, err := a.UseLink(ctx, tok)
	if err != nil || session == "" {
		t.Fatalf("UseLink: %q %v", session, err)
	}
	if _, err := a.UseLink(ctx, tok); !errors.Is(err, ErrInvalidLink) {
		t.Errorf("segundo uso: %v", err)
	}

	// La base guarda solo hashes, nunca los tokens.
	a.st.Close()
	db, _ := os.ReadFile(filepath.Join(dir, store.FileName))
	wal, _ := os.ReadFile(filepath.Join(dir, store.FileName+"-wal"))
	for _, raw := range []string{tok, session} {
		if bytes.Contains(db, []byte(raw)) || bytes.Contains(wal, []byte(raw)) {
			t.Errorf("la base contiene el token %q en claro", raw)
		}
	}
}

func TestEnlaceVenceYSeReemplaza(t *testing.T) {
	ctx := context.Background()
	a, now, _ := newAuth(t)
	tok, _ := a.RequestLink(ctx)
	*now = now.Add(LinkTTL)
	if _, err := a.UseLink(ctx, tok); !errors.Is(err, ErrInvalidLink) {
		t.Errorf("a los 15 min debe estar vencido: %v", err)
	}

	*now = now.Add(time.Minute)
	old, _ := a.RequestLink(ctx)
	*now = now.Add(time.Minute)
	fresh, err := a.RequestLink(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.UseLink(ctx, old); !errors.Is(err, ErrInvalidLink) {
		t.Errorf("pedir uno nuevo invalida el anterior: %v", err)
	}
	if _, err := a.UseLink(ctx, fresh); err != nil {
		t.Errorf("el último sirve: %v", err)
	}
}

func TestLimiteDeEnvios(t *testing.T) {
	ctx := context.Background()
	a, now, _ := newAuth(t)
	if _, err := a.RequestLink(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := a.RequestLink(ctx); !errors.Is(err, ErrRateLimited) {
		t.Errorf("1 por minuto: %v", err)
	}
	for range 4 {
		*now = now.Add(61 * time.Second)
		if _, err := a.RequestLink(ctx); err != nil {
			t.Fatal(err)
		}
	}
	*now = now.Add(61 * time.Second)
	if _, err := a.RequestLink(ctx); !errors.Is(err, ErrRateLimited) {
		t.Errorf("5 por hora: %v", err)
	}
	*now = now.Add(time.Hour)
	if _, err := a.RequestLink(ctx); err != nil {
		t.Errorf("pasada la hora se puede pedir otra vez: %v", err)
	}
}

func TestSesion(t *testing.T) {
	ctx := context.Background()
	a, now, _ := newAuth(t)
	tok, _ := a.RequestLink(ctx)
	s, err := a.UseLink(ctx, tok)
	if err != nil {
		t.Fatal(err)
	}
	if renewed, err := a.Session(ctx, s); err != nil || renewed {
		t.Errorf("sesión nueva: renewed=%v err=%v", renewed, err)
	}
	if _, err := a.Session(ctx, "otra"); !errors.Is(err, ErrNoSession) {
		t.Errorf("token desconocido: %v", err)
	}
	// Se renueva como mucho una vez al día, y al renovarse vuelve a durar 30 días.
	*now = now.Add(2 * time.Hour)
	if renewed, _ := a.Session(ctx, s); renewed {
		t.Error("no debe renovarse antes de un día")
	}
	*now = now.Add(29 * 24 * time.Hour)
	if renewed, err := a.Session(ctx, s); err != nil || !renewed {
		t.Errorf("a los 29 días se renueva: renewed=%v err=%v", renewed, err)
	}
	*now = now.Add(29 * 24 * time.Hour)
	if _, err := a.Session(ctx, s); err != nil {
		t.Errorf("renovada, sigue viva: %v", err)
	}
	*now = now.Add(SessionTTL)
	if _, err := a.Session(ctx, s); !errors.Is(err, ErrNoSession) {
		t.Errorf("vencida: %v", err)
	}
}

func TestCerrarSesiones(t *testing.T) {
	ctx := context.Background()
	a, now, _ := newAuth(t)
	var sessions []string
	for range 2 {
		tok, _ := a.RequestLink(ctx)
		s, err := a.UseLink(ctx, tok)
		if err != nil {
			t.Fatal(err)
		}
		sessions = append(sessions, s)
		*now = now.Add(2 * time.Minute)
	}
	if err := a.Logout(ctx, sessions[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Session(ctx, sessions[0]); !errors.Is(err, ErrNoSession) {
		t.Error("cerrar sesión borra esa sesión")
	}
	if _, err := a.Session(ctx, sessions[1]); err != nil {
		t.Error("y no las demás")
	}
	if err := a.LogoutAll(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Session(ctx, sessions[1]); !errors.Is(err, ErrNoSession) {
		t.Error("cerrar todas")
	}
}
