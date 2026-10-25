package federation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	ap "github.com/duv0-x/gremio/internal/goap/activitypub"
	"github.com/duv0-x/gremio/internal/profile"
	"github.com/duv0-x/gremio/internal/store"
)

// MaxAvatarBytes es el tamaño máximo de la foto de perfil.
const MaxAvatarBytes = 2 << 20

var (
	avatarExt = map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "image/webp": ".webp"}
	avatarRe  = regexp.MustCompile(`^avatar-[0-9a-f]{16}\.(png|jpg|webp)$`)
)

// ProfileUpdate son los cambios del perfil; un campo nil no se toca.
type ProfileUpdate struct {
	Name, Summary *string
	Avatar        []byte
}

// UpdateProfile cambia nombre, resumen y avatar del perfil JSON Resume sin tocar el resto del documento,
// y encola un Update{Person} para los seguidores (ADR-001). Devuelve cuántas entregas encoló.
func (s *Server) UpdateProfile(ctx context.Context, u ProfileUpdate) (int, error) {
	doc, err := s.st.Write.GetProfile(ctx)
	if err != nil {
		return 0, err
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(doc), &m); err != nil {
		return 0, err
	}
	basics, _ := m["basics"].(map[string]any)
	if basics == nil {
		basics = map[string]any{}
		m["basics"] = basics
	}
	if u.Name != nil {
		basics["name"] = strings.TrimSpace(*u.Name)
	}
	if u.Summary != nil {
		basics["summary"] = strings.TrimSpace(*u.Summary)
	}
	if u.Avatar != nil {
		url, err := s.saveAvatar(u.Avatar)
		if err != nil {
			return 0, err
		}
		basics["image"] = url
	}
	b, err := json.Marshal(m)
	if err != nil {
		return 0, err
	}
	return s.saveProfile(ctx, b)
}

// ReplaceProfile reemplaza el perfil completo por uno importado (ADR-003 §5) y avisa a los seguidores.
// La foto la gestiona la instancia: si el perfil nuevo no trae basics.image, se conserva la actual.
func (s *Server) ReplaceProfile(ctx context.Context, r *profile.Resume) (int, error) {
	if r.Basics.Image == "" {
		cur, err := s.CurrentProfile(ctx)
		if err != nil {
			return 0, err
		}
		r.Basics.Image = cur.Basics.Image
	}
	b, err := r.Export(true)
	if err != nil {
		return 0, err
	}
	return s.saveProfile(ctx, b)
}

// CurrentProfile devuelve el perfil guardado.
func (s *Server) CurrentProfile(ctx context.Context) (*profile.Resume, error) {
	doc, err := s.st.Read.GetProfile(ctx)
	if err != nil {
		return nil, err
	}
	return profile.Load(doc)
}

// saveProfile guarda el documento y encola un Update{Person} por inbox en la misma transacción.
func (s *Server) saveProfile(ctx context.Context, b []byte) (int, error) {
	var queued int
	err := s.st.Tx(ctx, func(q *store.Queries) error {
		if err := q.UpdateProfile(ctx, string(b)); err != nil {
			return err
		}
		a, err := s.actorFrom(string(b))
		if err != nil {
			return err
		}
		rows, err := q.ListFollowerInboxes(ctx)
		if err != nil {
			return err
		}
		inboxes := inboxesFor(rows)
		queued = len(inboxes)
		update := &ap.Activity{
			ID:     ap.ID(fmt.Sprintf("%s/updates/%d", s.actorID, now().UnixNano())),
			Type:   ap.UpdateType,
			Actor:  s.actorID,
			Object: a,
			To:     ap.ItemCollection{ap.PublicNS},
			CC:     ap.ItemCollection{s.actorID + "/followers"},
		}
		return s.enqueue(ctx, q, inboxes, update)
	})
	if err != nil {
		return 0, err
	}
	s.wake()
	return queued, nil
}

// saveAvatar valida la imagen por su contenido y la guarda con un nombre derivado de su hash: cambiar la
// foto cambia la URL, así que las demás instancias no se quedan con la vieja en caché.
func (s *Server) saveAvatar(data []byte) (string, error) {
	if len(data) == 0 || len(data) > MaxAvatarBytes {
		return "", fmt.Errorf("el avatar debe pesar entre 1 byte y %d MB", MaxAvatarBytes>>20)
	}
	ext, ok := avatarExt[http.DetectContentType(data)]
	if !ok {
		return "", errors.New("el avatar debe ser PNG, JPEG o WebP")
	}
	sum := sha256.Sum256(data)
	name := "avatar-" + hex.EncodeToString(sum[:8]) + ext
	dir := filepath.Join(s.st.DataDir(), "media")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, ".avatar-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name()) // no-op tras el rename
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmp.Name(), filepath.Join(dir, name)); err != nil {
		return "", err
	}
	// Las fotos anteriores ya no las referencia nadie.
	old, _ := filepath.Glob(filepath.Join(dir, "avatar-*"))
	for _, f := range old {
		if filepath.Base(f) != name {
			_ = os.Remove(f)
		}
	}
	return s.inst.Host.String + "/media/" + name, nil
}

// getMedia sirve solo archivos con nombre de avatar: nada fuera de <data-dir>/media es alcanzable.
func (s *Server) getMedia(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("file")
	if !avatarRe.MatchString(name) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable") // el nombre cambia con el contenido
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeFile(w, r, filepath.Join(s.st.DataDir(), "media", name))
}

func mediaTypeOf(url string) ap.MimeType {
	for typ, ext := range avatarExt {
		if strings.HasSuffix(url, ext) {
			return ap.MimeType(typ)
		}
	}
	return ""
}
