package openai

// ChatCompletionRequest represents the OpenAI chat completions request payload.
type ChatCompletionRequest struct {
	Model     string           `json:"model"`
	Messages  []Message        `json:"messages"`
	MaxTokens int              `json:"max_tokens,omitempty"`
	Stream    bool             `json:"stream,omitempty"`
	N         int              `json:"n,omitempty"`
	Temperature float32        `json:"temperature,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}
