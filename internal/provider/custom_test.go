package provider

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/diesi/aic/internal/openai"
)

type errReaderCustom struct {
	closed bool
	read   bool
}

func (e *errReaderCustom) Read(p []byte) (int, error) {
	if e.read {
		return 0, errors.New("read error")
	}
	e.read = true
	copy(p, []byte("partial"))
	return len("partial"), errors.New("read error")
}

func (e *errReaderCustom) Close() error { e.closed = true; return nil }

type roundTripFuncCustom func(*http.Request) (*http.Response, error)

func (f roundTripFuncCustom) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCustomChatReadError(t *testing.T) {
	er := &errReaderCustom{}
	p := NewCustom("")
	p.HTTPClient = &http.Client{Transport: roundTripFuncCustom(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: er, Header: make(http.Header)}, nil
	})}
	_, err := p.Chat(openai.ChatCompletionRequest{Model: "test-model", Messages: []openai.Message{{Role: "user", Content: "hi"}}})
	if err == nil || err.Error() != "read response body: read error" {
		t.Fatalf("expected read error, got %v", err)
	}
	if !er.closed {
		t.Fatalf("response body not closed")
	}
}

func TestCustomModelAuto(t *testing.T) {
	p := NewCustom("")
	modelsURL := p.endpoint(p.ModelsPath)
	chatURL := p.endpoint(p.ChatCompletionsPath)
	var gotModel string
	p.HTTPClient = &http.Client{Transport: roundTripFuncCustom(func(r *http.Request) (*http.Response, error) {
		switch r.URL.String() {
		case modelsURL:
			body := io.NopCloser(strings.NewReader(`{"data":[{"id":"local-model-1"},{"id":"local-model-2"}]}`))
			return &http.Response{StatusCode: 200, Body: body, Header: make(http.Header)}, nil
		case chatURL:
			// Capture request body to verify chosen model
			b, _ := io.ReadAll(r.Body)
			_ = r.Body.Close()
			if strings.Contains(string(b), "\"model\":\"local-model-1\"") {
				gotModel = "local-model-1"
			}
			body := io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`))
			return &http.Response{StatusCode: 200, Body: body, Header: make(http.Header)}, nil
		default:
			return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("not found")), Header: make(http.Header)}, nil
		}
	})}
	// Pass empty model to force auto
	_, err := p.Chat(openai.ChatCompletionRequest{Model: "", Messages: []openai.Message{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotModel != "local-model-1" {
		t.Fatalf("expected local-model-1 to be used, got %q", gotModel)
	}
}

func TestCustomStripsThinkAndMergesChoices(t *testing.T) {
	p := NewCustom("")
	chatURL := p.endpoint(p.ChatCompletionsPath)
	p.HTTPClient = &http.Client{Transport: roundTripFuncCustom(func(r *http.Request) (*http.Response, error) {
		if r.Method == "GET" {
			// models call (from ensureModel) – return a default
			body := io.NopCloser(strings.NewReader(`{"data":[{"id":"m1"}]}`))
			return &http.Response{StatusCode: 200, Body: body, Header: make(http.Header)}, nil
		}
		if r.URL.String() == chatURL {
			// two choices: first with <think> only, second with final text
			body := io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"<think>thinking...</think>"},"finish_reason":"stop"},{"message":{"content":"feat: improve tests"},"finish_reason":"stop"}]}`))
			return &http.Response{StatusCode: 200, Body: body, Header: make(http.Header)}, nil
		}
		return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("not found")), Header: make(http.Header)}, nil
	})}
	resp, err := p.Chat(openai.ChatCompletionRequest{Model: "auto", Messages: []openai.Message{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Choices) != 1 || strings.TrimSpace(resp.Choices[0]) != "feat: improve tests" {
		t.Fatalf("unexpected choices: %#v", resp.Choices)
	}
}

func TestCustomStripsUnbalancedThinkTag(t *testing.T) {
	p := NewCustom("")
	chatURL := p.endpoint(p.ChatCompletionsPath)
	p.HTTPClient = &http.Client{Transport: roundTripFuncCustom(func(r *http.Request) (*http.Response, error) {
		if r.Method == "GET" {
			body := io.NopCloser(strings.NewReader(`{"data":[{"id":"m1"}]}`))
			return &http.Response{StatusCode: 200, Body: body, Header: make(http.Header)}, nil
		}
		if r.URL.String() == chatURL {
			body := io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"<think>"},"finish_reason":"stop"},{"message":{"content":"feat: add feature"},"finish_reason":"stop"}]}`))
			return &http.Response{StatusCode: 200, Body: body, Header: make(http.Header)}, nil
		}
		return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("not found")), Header: make(http.Header)}, nil
	})}
	resp, err := p.Chat(openai.ChatCompletionRequest{Model: "auto", Messages: []openai.Message{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Choices) != 1 || strings.TrimSpace(resp.Choices[0]) != "feat: add feature" {
		t.Fatalf("unexpected choices: %#v", resp.Choices)
	}
}

func TestCustomChatEndpointUsesEnv(t *testing.T) {
	t.Setenv("CUSTOM_BASE_URL", "http://localhost:1234/")
	t.Setenv("CUSTOM_CHAT_COMPLETIONS_PATH", "/vX/chat")
	p := NewCustom("")
	var gotURL string
	p.HTTPClient = &http.Client{Transport: roundTripFuncCustom(func(r *http.Request) (*http.Response, error) {
		gotURL = r.URL.String()
		body := io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`))
		return &http.Response{StatusCode: 200, Body: body, Header: make(http.Header)}, nil
	})}
	_, err := p.Chat(openai.ChatCompletionRequest{Model: "test-model", Messages: []openai.Message{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "http://localhost:1234/vX/chat"
	if gotURL != want {
		t.Fatalf("endpoint mismatch: got %q, want %q", gotURL, want)
	}
}
