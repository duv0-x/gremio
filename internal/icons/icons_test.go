package icons

import "testing"

func TestIconos(t *testing.T) {
	if !Exists("bot") || Exists("no-existe") {
		t.Fatal("Exists")
	}
	if got := Inner("bot"); got[:22] != `<path d="M12 8V4H8"/><` {
		t.Errorf("Inner(bot) = %q", got)
	}
	if Inner("no-existe") != "" {
		t.Error("un ícono inexistente debe quedar vacío")
	}
}
