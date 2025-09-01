package provider

import "github.com/diesi/aic/internal/openai"

// OpenAI implements the Provider interface using the OpenAI API.
type OpenAI struct {
	client *openai.Client
}

// NewOpenAI creates a new OpenAI provider.
func NewOpenAI(apiKey string) *OpenAI {
	return &OpenAI{client: openai.NewClient(apiKey)}
}

// Chat sends a chat completion request to OpenAI and converts the result
// to a generic CompletionResponse.
func (o *OpenAI) Chat(req openai.ChatCompletionRequest) (*CompletionResponse, error) {
	resp, err := o.client.Chat(req)
	if err != nil {
		return nil, err
	}
	out := &CompletionResponse{Raw: resp.Raw}
	for _, c := range resp.Choices {
		out.Choices = append(out.Choices, c.Message.Content)
	}
	return out, nil
}
