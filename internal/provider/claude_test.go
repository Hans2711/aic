package provider

import (
	"errors"
	"net/http"
	"testing"

	"github.com/diesi/aic/internal/openai"
)

type errReader struct {
	closed bool
	read   bool
}

func (e *errReader) Read(p []byte) (int, error) {
	if e.read {
		return 0, errors.New("read error")
	}
	e.read = true
	copy(p, []byte("partial"))
	return len("partial"), errors.New("read error")
}

func (e *errReader) Close() error {
	e.closed = true
	return nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestClaudeChatReadError(t *testing.T) {
	er := &errReader{}
	client := &Claude{
		APIKey: "test",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: er, Header: make(http.Header)}, nil
		})},
		BaseURL: "http://example.com",
	}
	_, err := client.Chat(openai.ChatCompletionRequest{Model: "claude-3-sonnet-20240229", Messages: []openai.Message{{Role: "user", Content: "hi"}}})
	if err == nil || err.Error() != "read response body: read error" {
		t.Fatalf("expected read error, got %v", err)
	}
	if !er.closed {
		t.Fatalf("response body not closed")
	}
}
