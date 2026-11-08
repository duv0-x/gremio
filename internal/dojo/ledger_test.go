package dojo

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/duv0-x/gremio/internal/store"
)

func completed(challenge string, at time.Time) Event {
	return Event{
		Type: ChallengeCompleted, At: at, Challenge: challenge, Version: 1, Level: N3,
		Skills: []SkillLevel{{Skill: "kubernetes", Level: N3}}, Trust: Reproducible,
		Proof: &Proof{Commit: "abc123", URL: "https://ci.example/run/1"},
	}
}

func TestLedger(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	l := NewLedger(st)
	var got []Type
	l.Subscribe(func(_ context.Context, e Event) { got = append(got, e.Type) })

	day := time.Date(2026, 11, 7, 10, 30, 0, 0, time.UTC)
	if _, err := l.Append(ctx, Event{Type: ChallengeCompleted, At: day, Challenge: "x"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("un completado sin nivel ni confianza debe ser inválido: %v", err)
	}
	a, err := l.Append(ctx, completed("catalog:k8s-operator", day))
	if err != nil {
		t.Fatal(err)
	}
	if a.Prev != "" || len(a.Hash) != 64 {
		t.Fatalf("primer evento: prev %q hash %q", a.Prev, a.Hash)
	}
	b, err := l.Append(ctx, Event{Type: BadgeAwarded, At: day, Badge: &Badge{ID: "primer-reto"}})
	if err != nil {
		t.Fatal(err)
	}
	if b.Prev != a.Hash {
		t.Fatalf("el segundo evento debe enlazar al primero")
	}

	r, err := l.Retire(ctx, a.Hash, day.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if r.Retires != a.Hash || r.Challenge != "catalog:k8s-operator" || r.Prev != b.Hash {
		t.Fatalf("retired: %+v", r)
	}
	if _, err := l.Retire(ctx, a.Hash, day); !errors.Is(err, ErrAlreadyRetired) {
		t.Fatalf("retirar dos veces: %v", err)
	}
	if _, err := l.Retire(ctx, "nada", day); !errors.Is(err, ErrNotFound) {
		t.Fatalf("retirar un hash inexistente: %v", err)
	}

	book, err := l.Book(ctx, "@root@duv0-x.cloud")
	if err != nil {
		t.Fatal(err)
	}
	if len(book.Events) != 3 || book.Head != r.Hash || book.Rules != Rules {
		t.Fatalf("libro: %+v", book)
	}
	tomb := book.Events[0]
	if !tomb.Tombstone || tomb.Proof != nil || tomb.Skills != nil || tomb.Challenge != "catalog:k8s-operator" || tomb.Hash != a.Hash {
		t.Fatalf("la tombstone conserva solo reto, fecha y hash: %+v", tomb)
	}
	if err := VerifyChain(book.Events); err != nil {
		t.Fatal(err)
	}
	book.Events[1].At = day.Add(-24 * time.Hour)
	if err := VerifyChain(book.Events); err == nil {
		t.Fatal("un evento reescrito debe romper la cadena")
	}
	if want := []Type{ChallengeCompleted, BadgeAwarded, Retired}; len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("suscriptores: %v", got)
	}
}

func TestMatchSkill(t *testing.T) {
	if s, ok := MatchSkill("  EKS "); !ok || s.ID != "kubernetes" {
		t.Fatalf("EKS: %+v %v", s, ok)
	}
	seen := map[string]bool{}
	for _, s := range Skills {
		if seen[s.ID] || s.Volatility.HalfLife() == 0 || s.Why == "" {
			t.Errorf("habilidad %s: id repetido, volatilidad o justificación inválidas", s.ID)
		}
		seen[s.ID] = true
		for _, a := range s.Aliases {
			if m, _ := MatchSkill(a); m.ID != s.ID {
				t.Errorf("el alias %q de %s lo reconoce %s", a, s.ID, m.ID)
			}
		}
	}
}
