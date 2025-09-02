package provider

import (
	"errors"
	"net/http"
	"testing"

	"github.com/diesi/aic/internal/openai"
)

type errReader2 struct {
	closed bool
	read   bool
}

func (e *errReader2) Read(p []byte) (int, error) {
	if e.read {
		return 0, errors.New("read error")
	}
	e.read = true
	copy(p, []byte("partial"))
	return len("partial"), errors.New("read error")
}

func (e *errReader2) Close() error {
	e.closed = true
	return nil
}

type roundTripFunc2 func(*http.Request) (*http.Response, error)

func (f roundTripFunc2) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestGeminiChatReadError(t *testing.T) {
	er := &errReader2{}
	client := &Gemini{
		APIKey: "test",
		HTTPClient: &http.Client{Transport: roundTripFunc2(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: er, Header: make(http.Header)}, nil
		})},
		BaseURL: "http://example.com",
	}
	_, err := client.Chat(openai.ChatCompletionRequest{Model: "gemini-1.5-flash", Messages: []openai.Message{{Role: "user", Content: "hi"}}})
	if err == nil || err.Error() != "read response body: read error" {
		t.Fatalf("expected read error, got %v", err)
	}
	if !er.closed {
		t.Fatalf("response body not closed")
	}
}
