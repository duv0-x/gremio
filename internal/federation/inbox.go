package federation

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	ap "github.com/duv0-x/gremio/internal/goap/activitypub"
	"github.com/duv0-x/gremio/internal/store"
)

// postInbox procesa solo Follow, Undo{Follow} y Delete del Actor remoto (ADR-001);
// todo lo demás responde 202 y se descarta sin persistir.
func (s *Server) postInbox(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBody+1))
	if err != nil || len(body) > maxBody {
		http.Error(w, "cuerpo inválido o demasiado grande", http.StatusRequestEntityTooLarge)
		return
	}
	it, err := ap.UnmarshalJSON(body)
	if err != nil {
		http.Error(w, "JSON inválido", http.StatusBadRequest)
		return
	}
	act, err := ap.ToActivity(it)
	if err != nil || act.Actor == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	actor := act.Actor.GetLink().String()

	signer, err := s.verify(ctx, r, body)
	if err != nil {
		// Mastodon firma el Delete de una cuenta borrada con una llave que ya responde 410: sin poder
		// verificarlo, se acepta porque su único efecto es quitar a ese seguidor.
		if errors.Is(err, errGone) && ap.DeleteType.Match(act.Type) && act.Object != nil && act.Object.GetLink().String() == actor {
			s.removeFollower(ctx, actor, "delete sin llave")
			w.WriteHeader(http.StatusAccepted)
			return
		}
		slog.Warn("firma rechazada", "actor", actor, "err", err)
		http.Error(w, "firma inválida", http.StatusUnauthorized)
		return
	}
	if actor != signer {
		slog.Warn("actor distinto al firmante", "actor", actor, "firmante", signer)
		http.Error(w, "el actor no coincide con la firma", http.StatusUnauthorized)
		return
	}

	switch {
	case ap.FollowType.Match(act.Type):
		if act.Object == nil || act.Object.GetLink() != s.actorID {
			break
		}
		if err := s.acceptFollow(ctx, act); err != nil {
			slog.Error("follow", "actor", actor, "err", err)
			http.Error(w, "no se pudo procesar el follow", http.StatusBadGateway)
			return
		}
	case ap.UndoType.Match(act.Type):
		if inner, err := ap.ToActivity(act.Object); err == nil && ap.FollowType.Match(inner.Type) {
			s.removeFollower(ctx, actor, "undo")
		}
	case ap.DeleteType.Match(act.Type):
		if act.Object != nil && act.Object.GetLink().String() == actor {
			s.removeFollower(ctx, actor, "delete")
		}
	}
	w.WriteHeader(http.StatusAccepted)
}

// acceptFollow guarda al seguidor y le envía el Accept.
func (s *Server) acceptFollow(ctx context.Context, follow *ap.Activity) error {
	follower := follow.Actor.GetLink()
	remote, err := s.fetchActor(ctx, follower.String())
	if err != nil {
		return err
	}
	if remote.Inbox == nil {
		return fmt.Errorf("el actor %s no tiene inbox", follower)
	}
	inbox := remote.Inbox.GetLink().String()
	if _, err := s.checkURL(inbox); err != nil {
		return err
	}
	var shared sql.NullString
	if remote.Endpoints != nil && remote.Endpoints.SharedInbox != nil {
		if u := remote.Endpoints.SharedInbox.GetLink().String(); u != "" {
			if _, err := s.checkURL(u); err == nil {
				shared = sql.NullString{String: u, Valid: true}
			}
		}
	}
	if err := s.st.Write.UpsertFollower(ctx, store.UpsertFollowerParams{
		ActorIri: follower.String(), Inbox: inbox, SharedInbox: shared, FollowIri: follow.GetLink().String(),
	}); err != nil {
		return err
	}
	slog.Info("seguidor guardado", "actor", follower, "inbox", inbox, "shared_inbox", shared.String)

	accept := &ap.Activity{
		ID:     ap.ID(fmt.Sprintf("%s/accepts/%d", s.actorID, now().UnixNano())),
		Type:   ap.AcceptType,
		Actor:  s.actorID,
		Object: follow,
		To:     ap.ItemCollection{follower},
	}
	if err := s.enqueue(ctx, s.st.Write, []string{inbox}, accept); err != nil {
		return err
	}
	s.wake()
	return nil
}

func (s *Server) removeFollower(ctx context.Context, actor, reason string) {
	n, err := s.st.Write.DeleteFollower(ctx, actor)
	if err != nil {
		slog.Error("borrar seguidor", "actor", actor, "err", err)
		return
	}
	if n > 0 {
		slog.Info("seguidor eliminado", "actor", actor, "motivo", reason)
	}
}
