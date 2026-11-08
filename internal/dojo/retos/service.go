package retos

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/duv0-x/gremio/internal/dojo"
	"github.com/duv0-x/gremio/internal/store"
)

const timeLayout = "2006-01-02T15:04:05.000Z"

// Estados de una ejecución (ADR-002 §5). Tomar un reto la deja en in_progress: draft queda para los retos propios.
const (
	StatusInProgress = "in_progress"
	StatusDone       = "done"
	StatusAbandoned  = "abandoned"
)

// Errores que el panel muestra tal cual.
var (
	ErrNotFound  = errors.New("no existe esa ejecución")
	ErrState     = errors.New("la ejecución ya no admite ese cambio")
	ErrUnchecked = errors.New("sin verificación aprobada no hay ganancia: corre los checks y que pasen todos")
)

// Execution es el trabajo de una persona sobre un reto, con el enunciado que tomó (Snapshot).
type Execution struct {
	ID            string
	Challenge     string // id de linaje: "catalog:<slug>" o la URL del reto propio
	Version       int
	Snapshot      Challenge
	Status        string
	Delivery      Delivery
	Verification  *Verification
	StartedAt     time.Time
	CompletedAt   time.Time // cero si no terminó
	PublishedAt   time.Time // cero si es privada
	RetiredAt     time.Time // cero si no se retiró
	StartedHash   string
	CompletedHash string
}

// Delivery es la entrega: el repo con el commit fijado, la demo y el informe. Notes es privada.
type Delivery struct {
	Repo, Commit, Demo, Report, Notes string
}

func (e Execution) Published() bool { return !e.PublishedAt.IsZero() && e.RetiredAt.IsZero() }
func (e Execution) Retired() bool   { return !e.RetiredAt.IsZero() }
func (e Execution) Open() bool      { return e.Status == StatusInProgress || e.Status == "draft" }

// Slug es el slug del catálogo, si la ejecución es de un reto del catálogo.
func (e Execution) Slug() (string, bool) { return strings.CutPrefix(e.Challenge, CatalogPrefix) }

// Service guarda las ejecuciones y escribe en el libro del dojo al publicarlas.
type Service struct {
	st     *store.Store
	ledger *dojo.Ledger
	Client *http.Client // para bajar las entregas
	Now    func() time.Time
}

func NewService(st *store.Store, ledger *dojo.Ledger) *Service {
	return &Service{st: st, ledger: ledger, Client: Client, Now: time.Now}
}

func (s *Service) now() string { return s.Now().UTC().Format(timeLayout) }

// Take toma un reto del catálogo: copia el enunciado de su última versión (ADR-002 §3).
func (s *Service) Take(ctx context.Context, slug string) (Execution, error) {
	entry, ok := Find(slug)
	if !ok {
		return Execution{}, fmt.Errorf("el reto %s no está en el catálogo", slug)
	}
	c := entry.Latest()
	doc, err := json.Marshal(c)
	if err != nil {
		return Execution{}, err
	}
	id, now := strings.ToLower(rand.Text()[:16]), s.now()
	err = s.st.Write.CreateExecution(ctx, store.CreateExecutionParams{
		ID: id, Challenge: CatalogPrefix + slug, ChallengeVersion: int64(c.Version), Snapshot: string(doc),
		StartedAt: now, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		return Execution{}, err
	}
	return s.Get(ctx, id)
}

func (s *Service) Get(ctx context.Context, id string) (Execution, error) {
	row, err := s.st.Read.GetExecution(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Execution{}, ErrNotFound
	}
	if err != nil {
		return Execution{}, err
	}
	return decode(row)
}

func (s *Service) List(ctx context.Context) ([]Execution, error) {
	rows, err := s.st.Read.ListExecutions(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Execution, 0, len(rows))
	for _, r := range rows {
		e, err := decode(r)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

// Deliver guarda la entrega y descarta la verificación anterior: los checks corren contra este commit.
func (s *Service) Deliver(ctx context.Context, id string, d Delivery) error {
	d.Repo, d.Commit = strings.TrimSpace(d.Repo), strings.ToLower(strings.TrimSpace(d.Commit))
	if d.Repo != "" || d.Commit != "" {
		if _, err := Archive(d.Repo, d.Commit); err != nil {
			return err
		}
	}
	for _, u := range []string{d.Demo, d.Report} {
		if u != "" && !strings.HasPrefix(u, "https://") {
			return errors.New("la demo y el informe tienen que ser enlaces https")
		}
	}
	n, err := s.st.Write.SaveDelivery(ctx, store.SaveDeliveryParams{
		Repo: d.Repo, CommitSha: d.Commit, Demo: strings.TrimSpace(d.Demo), Report: strings.TrimSpace(d.Report),
		Notes: d.Notes, UpdatedAt: s.now(), ID: id,
	})
	return rowsErr(n, err)
}

// Verify corre los checks del enunciado tomado contra la entrega y guarda el resultado, pase o no.
func (s *Service) Verify(ctx context.Context, id string) (Verification, error) {
	e, err := s.Get(ctx, id)
	if err != nil {
		return Verification{}, err
	}
	if !e.Open() {
		return Verification{}, ErrState
	}
	if e.Delivery.Repo == "" {
		return Verification{}, errors.New("primero guarda la entrega con el repo y el commit")
	}
	if len(e.Snapshot.Checks) == 0 {
		return Verification{}, errors.New("este reto no tiene checks: no se puede verificar")
	}
	v, err := Run(ctx, s.Client, e.Delivery.Repo, e.Delivery.Commit, e.Snapshot.Checks)
	if err != nil {
		return v, err
	}
	doc, err := json.Marshal(v)
	if err != nil {
		return v, err
	}
	n, err := s.st.Write.SetVerification(ctx, store.SetVerificationParams{
		Verification: sql.NullString{String: string(doc), Valid: true}, UpdatedAt: s.now(), ID: id,
	})
	return v, rowsErr(n, err)
}

// Trust es la confianza que tendría la ejecución con su verificación actual. Para un reto del catálogo usa la
// versión que viaja en el binario, no la copia: el revisor puede llegar después en el mismo archivo.
func (s *Service) Trust(e Execution) (dojo.Trust, error) {
	v := Verification{}
	if e.Verification != nil {
		v = *e.Verification
	}
	if slug, ok := e.Slug(); ok {
		if entry, ok := Find(slug); ok {
			if c, ok := entry.Version(e.Version); ok {
				return Trust(c, true, v)
			}
		}
	}
	return Trust(e.Snapshot, false, v)
}

// ExpectedTrust es la confianza que daría la ejecución si la verificación pasa: para "cuánto sumaría".
func (s *Service) ExpectedTrust(e Execution) dojo.Trust {
	e.Verification = &Verification{Passed: true}
	t, _ := s.Trust(e)
	return t
}

// Complete termina la ejecución. Exige la verificación aprobada; si ya es pública, entra challengeCompleted al libro.
func (s *Service) Complete(ctx context.Context, id string, urls URLs) (Execution, error) {
	e, err := s.Get(ctx, id)
	if err != nil {
		return e, err
	}
	if !e.Open() {
		return e, ErrState
	}
	if e.Verification == nil || !e.Verification.Passed {
		return e, ErrUnchecked
	}
	now := s.now()
	if n, err := s.st.Write.CompleteExecution(ctx, store.CompleteExecutionParams{CompletedAt: sql.NullString{String: now, Valid: true}, UpdatedAt: now, ID: id}); err != nil || n == 0 {
		return e, rowsErr(n, err)
	}
	if e, err = s.Get(ctx, id); err != nil || !e.Published() {
		return e, err
	}
	return s.syncBook(ctx, e, urls)
}

// Publish hace pública la ejecución (ADR-002 §5): entra challengeStarted con su fecha de inicio y, si ya
// terminó, challengeCompleted. Es idempotente: si se cortó a la mitad, volver a publicar completa lo que falta.
func (s *Service) Publish(ctx context.Context, id string, urls URLs) (Execution, error) {
	e, err := s.Get(ctx, id)
	if err != nil {
		return e, err
	}
	if e.Retired() || e.Status == StatusAbandoned {
		return e, ErrState
	}
	if e.PublishedAt.IsZero() {
		now := s.now()
		if n, err := s.st.Write.PublishExecution(ctx, store.PublishExecutionParams{PublishedAt: sql.NullString{String: now, Valid: true}, UpdatedAt: now, ID: id}); err != nil || n == 0 {
			return e, rowsErr(n, err)
		}
		if e, err = s.Get(ctx, id); err != nil {
			return e, err
		}
	}
	return s.syncBook(ctx, e, urls)
}

// URLs arma las páginas públicas que enlaza el libro.
type URLs struct {
	Host string // https://dominio, sin barra final
}

func (u URLs) Execution(id string) string { return u.Host + "/dojo/ejecuciones/" + id }

// ChallengeVersion es la página pública de la versión de un reto del catálogo.
func (u URLs) ChallengeVersion(slug string, v int) string {
	return fmt.Sprintf("%s/dojo/catalogo/%s/v%d", u.Host, slug, v)
}

func (u URLs) challenge(e Execution) string {
	if slug, ok := e.Slug(); ok {
		return u.ChallengeVersion(slug, e.Version)
	}
	return e.Challenge
}

// Event arma el evento del libro de la ejecución. Para challengeCompleted lleva la confianza y la prueba.
func (s *Service) Event(e Execution, t dojo.Type, urls URLs) (dojo.Event, error) {
	ev := dojo.Event{
		Type: t, Challenge: e.Challenge, Version: e.Version, Title: e.Snapshot.Title,
		URL: urls.challenge(e), Execution: urls.Execution(e.ID), At: e.StartedAt,
	}
	if t != dojo.ChallengeCompleted {
		return ev, nil
	}
	ev.At, ev.Level, ev.Skills = e.CompletedAt, e.Snapshot.Level, e.Snapshot.Skills
	ev.Proof = &dojo.Proof{Commit: e.Delivery.Commit, URL: urls.Execution(e.ID) + "#verificacion"}
	var err error
	ev.Trust, err = s.Trust(e)
	return ev, err
}

// syncBook agrega al libro los eventos que le faltan a una ejecución pública. Antes de agregar busca en el libro
// uno de la misma ejecución y tipo, por si una corrida anterior se cortó entre Append y guardar su hash.
func (s *Service) syncBook(ctx context.Context, e Execution, urls URLs) (Execution, error) {
	want := []dojo.Type{dojo.ChallengeStarted}
	if e.Status == StatusDone {
		want = append(want, dojo.ChallengeCompleted)
	}
	have := map[dojo.Type]*string{dojo.ChallengeStarted: &e.StartedHash, dojo.ChallengeCompleted: &e.CompletedHash}
	var book []dojo.Event
	for _, t := range want {
		if *have[t] != "" {
			continue
		}
		if book == nil {
			var err error
			if book, err = s.ledger.List(ctx); err != nil {
				return e, err
			}
		}
		if h := findEvent(book, urls.Execution(e.ID), t); h != "" {
			*have[t] = h
			continue
		}
		ev, err := s.Event(e, t, urls)
		if err != nil {
			return e, err
		}
		if ev, err = s.ledger.Append(ctx, ev); err != nil {
			return e, err
		}
		*have[t] = ev.Hash
	}
	err := s.st.Write.SetExecutionHashes(ctx, store.SetExecutionHashesParams{
		StartedHash: null(e.StartedHash), CompletedHash: null(e.CompletedHash), UpdatedAt: s.now(), ID: e.ID,
	})
	return e, err
}

func findEvent(book []dojo.Event, execution string, t dojo.Type) string {
	for _, ev := range book {
		if ev.Type == t && ev.Execution == execution && !ev.Tombstone {
			return ev.Hash
		}
	}
	return ""
}

// Abandon deja la ejecución abandonada. Si era pública, su challengeStarted queda: empezó de verdad.
func (s *Service) Abandon(ctx context.Context, id string) error {
	n, err := s.st.Write.AbandonExecution(ctx, store.AbandonExecutionParams{UpdatedAt: s.now(), ID: id})
	return rowsErr(n, err)
}

// Retire retira una ejecución publicada (ADR-013 §6): sus eventos pasan a tombstone y la página deja de mostrarse.
// El reto no vuelve a sumar aunque se rehaga.
func (s *Service) Retire(ctx context.Context, id string) error {
	e, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if !e.Published() {
		return ErrState
	}
	at := s.Now()
	for _, h := range []string{e.CompletedHash, e.StartedHash} {
		if h == "" {
			continue
		}
		if _, err := s.ledger.Retire(ctx, h, at); err != nil && !errors.Is(err, dojo.ErrAlreadyRetired) {
			return err
		}
	}
	now := s.now()
	n, err := s.st.Write.RetireExecution(ctx, store.RetireExecutionParams{RetiredAt: sql.NullString{String: now, Valid: true}, UpdatedAt: now, ID: id})
	return rowsErr(n, err)
}

// Explain dice, por habilidad, cuánto sumaría la ejecución si se terminara y publicara hoy, y por qué no si no suma.
func (s *Service) Explain(ctx context.Context, e Execution, urls URLs) ([]dojo.Outcome, error) {
	book, err := s.ledger.List(ctx)
	if err != nil {
		return nil, err
	}
	e.CompletedAt = s.Now().UTC()
	ev, _ := s.Event(e, dojo.ChallengeCompleted, urls)
	ev.Trust = s.ExpectedTrust(e)
	// Una ejecución ya terminada y publicada está en el libro: se explica sin ella.
	if e.CompletedHash != "" {
		book = without(book, e.CompletedHash)
	}
	return dojo.Explain(book, ev, e.CompletedAt), nil
}

func without(book []dojo.Event, hash string) []dojo.Event {
	out := make([]dojo.Event, 0, len(book))
	for _, ev := range book {
		if ev.Hash != hash {
			out = append(out, ev)
		}
	}
	return out
}

func decode(r store.Execution) (Execution, error) {
	e := Execution{
		ID: r.ID, Challenge: r.Challenge, Version: int(r.ChallengeVersion), Status: r.Status,
		Delivery:    Delivery{Repo: r.Repo, Commit: r.CommitSha, Demo: r.Demo, Report: r.Report, Notes: r.Notes},
		StartedHash: r.StartedHash.String, CompletedHash: r.CompletedHash.String,
	}
	if err := json.Unmarshal([]byte(r.Snapshot), &e.Snapshot); err != nil {
		return e, fmt.Errorf("ejecución %s: enunciado: %w", r.ID, err)
	}
	if r.Verification.Valid {
		e.Verification = &Verification{}
		if err := json.Unmarshal([]byte(r.Verification.String), e.Verification); err != nil {
			return e, fmt.Errorf("ejecución %s: verificación: %w", r.ID, err)
		}
	}
	e.StartedAt = parse(r.StartedAt)
	e.CompletedAt, e.PublishedAt, e.RetiredAt = parse(r.CompletedAt.String), parse(r.PublishedAt.String), parse(r.RetiredAt.String)
	return e, nil
}

func parse(s string) time.Time {
	t, _ := time.Parse(timeLayout, s)
	return t
}

func null(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }

func rowsErr(n int64, err error) error {
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrState
	}
	return nil
}
