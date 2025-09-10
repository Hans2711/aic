package commit

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/diesi/aic/internal/openai"
)

func TestGenerateCombinedSuggestions_UsesHighTempAndDistinctPrompt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		body, _ := io.ReadAll(r.Body)
		var req openai.ChatCompletionRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("unmarshal request: %v", err)
		}
		// check temperature and system prompt
		if req.Temperature == nil || *req.Temperature != 0.7 {
			t.Errorf("temperature = %v, want 0.7", req.Temperature)
		}
		found := false
		for _, m := range req.Messages {
			if m.Role == "system" && strings.Contains(m.Content, "distinct wording and sentence structure") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("system prompt missing distinct wording phrase: %#v", req.Messages)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"content":"first"},"finish_reason":"stop"}]}`)
	}))
	defer srv.Close()

	t.Setenv("CUSTOM_BASE_URL", srv.URL)
	cfg := Config{Provider: "custom", Model: "test-model", Suggestions: 1}
	msgs, err := GenerateCombinedSuggestions(cfg, "", []string{"a", "b"})
	if err != nil {
		t.Fatalf("GenerateCombinedSuggestions error: %v", err)
	}
	if len(msgs) == 0 {
		t.Fatalf("expected at least one suggestion")
	}
}
