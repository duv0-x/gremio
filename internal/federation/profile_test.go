package federation

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/duv0-x/gremio/internal/profile"
)

func testPNG(t *testing.T, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for x := 0; x < 8; x++ {
		for y := 0; y < 8; y++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func ptr(s string) *string { return &s }

func TestUpdateProfileConservaElRestoYPublicaIcon(t *testing.T) {
	a := newTestInstance(t, "alice")
	ctx := context.Background()
	if err := a.st.Write.UpdateProfile(ctx, `{"basics":{"name":"alice","email":"a@ejemplo.org"},"work":[{"name":"Acme"}]}`); err != nil {
		t.Fatal(err)
	}
	pic := testPNG(t, color.RGBA{R: 200, A: 255})
	if _, err := a.UpdateProfile(ctx, ProfileUpdate{Name: ptr("Alice Ejemplo"), Summary: ptr("  Platform Engineer  "), Avatar: pic}); err != nil {
		t.Fatal(err)
	}

	doc, _ := a.st.Read.GetProfile(ctx)
	var p map[string]any
	_ = json.Unmarshal([]byte(doc), &p)
	basics := p["basics"].(map[string]any)
	if basics["email"] != "a@ejemplo.org" || p["work"] == nil || basics["summary"] != "Platform Engineer" {
		t.Errorf("perfil = %s", doc)
	}

	actor := getJSON(t, string(a.actorID))
	icon, _ := actor["icon"].(map[string]any)
	if actor["name"] != "Alice Ejemplo" || actor["summary"] != "Platform Engineer" || icon["mediaType"] != "image/png" {
		t.Fatalf("actor = %v", actor)
	}
	resp, err := http.Get(icon["url"].(string))
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !bytes.Equal(got, pic) || resp.Header.Get("Content-Type") != "image/png" || resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("avatar: %d bytes, %v", len(got), resp.Header)
	}

	// Cambiar el avatar cambia la URL y borra el anterior.
	if _, err := a.UpdateProfile(ctx, ProfileUpdate{Avatar: testPNG(t, color.RGBA{B: 200, A: 255})}); err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(a.st.DataDir(), "media", "avatar-*"))
	if len(files) != 1 || strings.HasSuffix(icon["url"].(string), filepath.Base(files[0])) {
		t.Errorf("avatares en disco = %v", files)
	}
}

func TestAvatarInvalidoYMediaFueraDeLista(t *testing.T) {
	a := newTestInstance(t, "alice")
	ctx := context.Background()
	if _, err := a.UpdateProfile(ctx, ProfileUpdate{Avatar: []byte("<svg></svg>")}); err == nil {
		t.Error("un SVG no debería aceptarse")
	}
	if _, err := a.UpdateProfile(ctx, ProfileUpdate{Avatar: make([]byte, MaxAvatarBytes+1)}); err == nil {
		t.Error("un avatar de más de 2 MB no debería aceptarse")
	}
	_ = os.WriteFile(filepath.Join(a.st.DataDir(), "secreto.txt"), []byte("x"), 0o600)
	for _, p := range []string{"/media/gremio.db", "/media/..%2Fsecreto.txt", "/media/avatar-zz.png"} {
		resp, err := http.Get(a.url + p)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s: HTTP %d, quería 404", p, resp.StatusCode)
		}
	}
}

func TestUpdateProfileAvisaALosSeguidores(t *testing.T) {
	a, b := newTestInstance(t, "alice"), newTestInstance(t, "bob")
	b.follow(t, a, 1)
	waitFor(t, "Accept en el inbox de bob", func() bool { return b.inboxes.Load() == 1 })

	queued, err := a.UpdateProfile(context.Background(), ProfileUpdate{Name: ptr("Alice")})
	if err != nil || queued != 1 {
		t.Fatalf("UpdateProfile = %d, %v", queued, err)
	}
	waitFor(t, "Update en el sharedInbox de bob", func() bool { return b.shared.Load() == 1 })
}

func TestReplaceProfileConservaElAvatarYAvisa(t *testing.T) {
	a := newTestInstance(t, "alice")
	ctx := context.Background()
	if _, err := a.UpdateProfile(ctx, ProfileUpdate{Avatar: testPNG(t, color.RGBA{G: 200, A: 255})}); err != nil {
		t.Fatal(err)
	}
	before, _ := a.CurrentProfile(ctx)

	next, _, err := profile.Parse([]byte(`{"basics":{"name":"Alice Ejemplo","summary":"SRE"},"work":[{"name":"Acme","highlights":["Bajó el MTTR"]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.ReplaceProfile(ctx, next); err != nil {
		t.Fatal(err)
	}
	after, err := a.CurrentProfile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after.Basics.Image == "" || after.Basics.Image != before.Basics.Image || len(after.Work) != 1 || after.Work[0].Highlights[0].ID == "" {
		t.Errorf("perfil = %+v", after)
	}
	actor := getJSON(t, string(a.actorID))
	if actor["name"] != "Alice Ejemplo" || actor["summary"] != "SRE" || actor["icon"] == nil {
		t.Errorf("actor = %v", actor)
	}
}
