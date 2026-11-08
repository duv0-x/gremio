package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOllamaGenerate(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" || r.Header.Get("Authorization") != "Bearer tok" {
			http.Error(w, `{"error":"no"}`, http.StatusUnauthorized)
			return
		}
		json.NewDecoder(r.Body).Decode(&got)
		fmt.Fprint(w, `{"message":{"role":"assistant","content":"{\"title\":\"T\"}"}}`)
	}))
	defer srv.Close()
	cfg := Config{"endpoint": srv.URL, "token": "tok"}
	out, err := Ollama{}.Generate(context.Background(), cfg, "qwen", "sys", "hola")
	if err != nil || out != `{"title":"T"}` {
		t.Fatalf("%q %v", out, err)
	}
	if got["model"] != "qwen" || got["stream"] != false || got["format"] != "json" {
		t.Errorf("petición: %v", got)
	}
	if _, err := (Ollama{}).Generate(context.Background(), cfg, "", "s", "p"); err == nil {
		t.Error("sin modelo debía fallar")
	}
	if _, err := (Ollama{}).Generate(context.Background(), Config{"endpoint": srv.URL}, "qwen", "s", "p"); err == nil {
		t.Error("un 401 debía fallar")
	}
}

func TestClaudeGenerate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model  string `json:"model"`
			System []struct{ Text string }
		}
		json.NewDecoder(r.Body).Decode(&req)
		if r.URL.Path != "/v1/messages" || req.Model != "claude-opus-5-5" || len(req.System) != 1 || req.System[0].Text != "sys" {
			http.Error(w, `{"type":"error","error":{"type":"invalid_request_error","message":"no"}}`, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"m","type":"message","role":"assistant","model":"claude-opus-5-5","content":[{"type":"text","text":"{}"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	defer srv.Close()
	c := Claude{baseURL: srv.URL}
	out, err := c.Generate(context.Background(), Config{"api_key": "k"}, c.DefaultModel(), "sys", "hola")
	if err != nil || out != "{}" {
		t.Fatalf("%q %v", out, err)
	}
}
