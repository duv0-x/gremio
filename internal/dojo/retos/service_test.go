package retos

import (
	"bytes"
	"context"
	"errors"
	"io"
	"maps"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/duv0-x/gremio/internal/dojo"
	"github.com/duv0-x/gremio/internal/store"
)

// fakeForge responde cualquier GET con el tarball dado, como codeload.github.com.
type fakeForge struct{ body []byte }

func (f fakeForge) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(bytes.NewReader(f.body)), Header: http.Header{}}, nil
}

// k8sSolution satisface los checks del reto real k8s-despliegue-resiliente.
var k8sSolution = map[string]string{
	"base/deployment.yaml":             "kind: Deployment\nspec:\n  template:\n    spec:\n      securityContext:\n        runAsNonRoot: true\n      topologySpreadConstraints:\n      containers:\n      - image: ghcr.io/ana/app:1.2.3\n        readinessProbe:\n        livenessProbe:\n        resources:\n          requests:\n          limits:\n",
	"base/hpa.yaml":                    "kind: HorizontalPodAutoscaler\n",
	"base/pdb.yaml":                    "kind: PodDisruptionBudget\n",
	"base/netpol.yaml":                 "kind: NetworkPolicy\n",
	"overlays/qa/kustomization.yaml":   "resources: [../../base]\n",
	"overlays/prod/kustomization.yaml": "resources: [../../base]\n",
	".github/workflows/ci.yml":         "run: kustomize build overlays/prod | kubeconform -strict\n",
	"docs/adr/ADR-001-capacidad.md":    "# Capacidad\n",
	"README.md":                        "# app\n",
	"LICENSE":                          "MIT\n",
	"go.mod":                           "module app\n",
	"main_test.go":                     "package main\nfunc TestX(t *testing.T) {}\n",
}

func newService(t *testing.T, files map[string]string) (*Service, *dojo.Ledger, context.Context) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	saved := Catalog
	Catalog = maps.Clone(saved)
	t.Cleanup(func() { Catalog = saved })
	if err := AddDir("testdata/catalog"); err != nil {
		t.Fatal(err)
	}
	ledger := dojo.NewLedger(st)
	s := NewService(st, ledger)
	s.Client = &http.Client{Transport: fakeForge{forgeTar(t, files).Bytes()}}
	day := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { return day }
	return s, ledger, ctx
}

func cycle(t *testing.T, s *Service, ctx context.Context, slug string) Execution {
	t.Helper()
	urls := URLs{Host: "https://duv0-x.cloud"}
	e, err := s.Take(ctx, slug)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Complete(ctx, e.ID, urls); !errors.Is(err, ErrUnchecked) {
		t.Fatalf("terminar sin verificar: %v", err)
	}
	if err := s.Deliver(ctx, e.ID, Delivery{Repo: "https://github.com/ana/app", Commit: strings.Repeat("a", 40)}); err != nil {
		t.Fatal(err)
	}
	v, err := s.Verify(ctx, e.ID)
	if err != nil || !v.Passed {
		t.Fatalf("verificar: %v %+v", err, v.Results)
	}
	if _, err := s.Complete(ctx, e.ID, urls); err != nil {
		t.Fatal(err)
	}
	if e, err = s.Publish(ctx, e.ID, urls); err != nil {
		t.Fatal(err)
	}
	if e.StartedHash == "" || e.CompletedHash == "" {
		t.Fatalf("faltan eventos en el libro: %+v", e)
	}
	// Publicar de nuevo no duplica eventos.
	if _, err := s.Publish(ctx, e.ID, urls); err != nil {
		t.Fatal(err)
	}
	return e
}

func TestCycleReviewedReachesN3(t *testing.T) {
	s, ledger, ctx := newService(t, k8sSolution)
	e := cycle(t, s, ctx, "qa-revisado-ficticio")
	book, err := ledger.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(book) != 2 {
		t.Fatalf("el libro tiene %d eventos, quería 2 (started y completed)", len(book))
	}
	if book[1].Trust != dojo.Reproducible || book[1].Proof.Commit != e.Delivery.Commit {
		t.Errorf("evento completed: %+v", book[1])
	}
	// N3 reproducible: entrada 60 + 0.2 × (80 − 60) = 64 (ADR-013 §3).
	if got := dojo.Training(book, "kubernetes", s.Now()).Shown(); got != 64 {
		t.Errorf("Training kubernetes = %d, quería 64", got)
	}
	// Ya hecho: Explain dice que no vuelve a sumar.
	e2, _ := s.Take(ctx, "qa-revisado-ficticio")
	out, err := s.Explain(ctx, e2, URLs{Host: "https://duv0-x.cloud"})
	if err != nil || len(out) == 0 || out[0].Reason == "" {
		t.Errorf("Explain de un reto repetido: %+v %v", out, err)
	}

	if err := s.Retire(ctx, e.ID); err != nil {
		t.Fatal(err)
	}
	book, _ = ledger.List(ctx)
	if got := dojo.Training(book, "kubernetes", s.Now()); !got.Empty() && got.Value != 0 {
		t.Errorf("después de retirar, Training = %v", got.Value)
	}
}

func TestCycleInReviewCapsAtN2(t *testing.T) {
	s, ledger, ctx := newService(t, k8sSolution)
	cycle(t, s, ctx, "k8s-despliegue-resiliente")
	book, _ := ledger.List(ctx)
	if book[1].Trust != dojo.SelfPractice {
		t.Fatalf("un reto en revisión es práctica propia, no %q", book[1].Trust)
	}
	// N3 tapado a N2: 40 + 0.2 × (60 − 40) = 44.
	if got := dojo.Training(book, "kubernetes", s.Now()).Shown(); got != 44 {
		t.Errorf("Training kubernetes = %d, quería 44", got)
	}
}

func TestPrivateExecutionStaysOutOfBook(t *testing.T) {
	s, ledger, ctx := newService(t, k8sSolution)
	e, _ := s.Take(ctx, "qa-revisado-ficticio")
	s.Deliver(ctx, e.ID, Delivery{Repo: "https://github.com/ana/app", Commit: strings.Repeat("b", 40)})
	if _, err := s.Verify(ctx, e.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Complete(ctx, e.ID, URLs{Host: "https://x"}); err != nil {
		t.Fatal(err)
	}
	if book, _ := ledger.List(ctx); len(book) != 0 {
		t.Fatalf("una ejecución privada no entra al libro: %d eventos", len(book))
	}
	if err := s.Deliver(ctx, e.ID, Delivery{}); !errors.Is(err, ErrState) {
		t.Errorf("una ejecución terminada no cambia su entrega: %v", err)
	}
}

func TestFailingChecksBlockCompletion(t *testing.T) {
	s, _, ctx := newService(t, map[string]string{"README.md": "x"})
	e, _ := s.Take(ctx, "qa-revisado-ficticio")
	s.Deliver(ctx, e.ID, Delivery{Repo: "https://github.com/ana/app", Commit: strings.Repeat("c", 40)})
	if v, err := s.Verify(ctx, e.ID); err != nil || v.Passed {
		t.Fatalf("verify: %v %v", v.Passed, err)
	}
	if _, err := s.Complete(ctx, e.ID, URLs{}); !errors.Is(err, ErrUnchecked) {
		t.Errorf("con checks fallidos no se termina: %v", err)
	}
}

// Un reto propio N3 con checks que pasan cuenta como práctica propia: N2 como máximo (ADR-013 §4).
func TestOwnChallengeCapsAtN2(t *testing.T) {
	s, ledger, ctx := newService(t, k8sSolution)
	c := Catalog["qa-revisado-ficticio"].Latest()
	c.Reviewer, c.Author = "alguien", "otra" // nada de esto vale fuera del catálogo
	id, err := s.CreateOwn(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.TakeOwn(ctx, id, URLs{Host: "https://x"}); err == nil {
		t.Fatal("un reto sin publicar no se toma")
	}
	if v, err := s.PublishOwn(ctx, id, "root"); err != nil || v != 1 {
		t.Fatalf("publicar: %d %v", v, err)
	}
	o, _ := s.GetOwn(ctx, id)
	if o.Latest.Author != "root" || o.Latest.Reviewer != "" {
		t.Errorf("el autor es el dueño y un reto propio no tiene revisor: %+v", o.Latest)
	}
	urls := URLs{Host: "https://duv0-x.cloud"}
	e, err := s.TakeOwn(ctx, id, urls)
	if err != nil {
		t.Fatal(err)
	}
	if e.Challenge != urls.Own(id) || ChallengePath(e) != "/dojo/retos/"+id+"/v1" {
		t.Errorf("linaje %q, ruta %q", e.Challenge, ChallengePath(e))
	}
	s.Deliver(ctx, e.ID, Delivery{Repo: "https://github.com/ana/app", Commit: strings.Repeat("d", 40)})
	if _, err := s.Verify(ctx, e.ID); err != nil {
		t.Fatal(err)
	}
	s.Complete(ctx, e.ID, urls)
	if _, err := s.Publish(ctx, e.ID, urls); err != nil {
		t.Fatal(err)
	}
	book, _ := ledger.List(ctx)
	if book[1].Trust != dojo.SelfPractice {
		t.Fatalf("confianza %q", book[1].Trust)
	}
	if got := dojo.Training(book, "kubernetes", s.Now()).Shown(); got != 44 {
		t.Errorf("Training = %d, quería 44", got)
	}
	// Una versión nueva queda aparte y la anterior no cambia.
	c.Title = "Otro título"
	s.SaveOwn(ctx, id, c)
	if v, _ := s.PublishOwn(ctx, id, "root"); v != 2 {
		t.Errorf("segunda versión: %d", v)
	}
	if v1, _ := s.OwnVersion(ctx, id, 1); v1.Title == "Otro título" {
		t.Error("la versión 1 cambió")
	}
}

func TestParseDraft(t *testing.T) {
	text := "Aquí va:\n```json\n" + `{"id":"x","title":"T","author":"claude","reviewer":"claude","origin":"human","level":5,
	"skills":[{"skill":"go","level":1},{"skill":"git","level":1},{"skill":"sql","level":1},{"skill":"bash","level":1}]}` + "\n```"
	c, err := ParseDraft(text, "ollama:qwen")
	if err != nil {
		t.Fatal(err)
	}
	if c.Origin != "ai" || c.Model != "ollama:qwen" || c.Author != "" || c.Reviewer != "" || c.ID != "" || len(c.Skills) != 3 {
		t.Errorf("la IA no fija autoría, revisión ni id: %+v", c)
	}
	if c.Validate() == nil {
		t.Error("un nivel 5 no valida: se corrige en el panel")
	}
	if _, err := ParseDraft("no sé", "m"); err == nil {
		t.Error("sin JSON debía fallar")
	}
}
