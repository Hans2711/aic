package provider

import "github.com/diesi/aic/internal/openai"

// Generic completion response for abstraction
type CompletionResponse struct {
	Choices []string
	Raw     string
}

// Provider defines the interface for AI providers.
type Provider interface {
	Chat(req openai.ChatCompletionRequest) (*CompletionResponse, error)
}
