package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/diesi/aic/internal/openai"
)

// Claude implements the Provider interface using Anthropic's Claude API.
type Claude struct {
	APIKey     string
	HTTPClient *http.Client
	BaseURL    string
}

// NewClaude creates a new Claude provider.
func NewClaude(apiKey string) *Claude {
	return &Claude{
		APIKey:     apiKey,
		HTTPClient: &http.Client{Timeout: 60 * time.Second},
		BaseURL:    "https://api.anthropic.com/v1",
	}
}

func (c *Claude) Chat(req openai.ChatCompletionRequest) (*CompletionResponse, error) {
	count := req.N
	if count < 1 {
		count = 1
	}
	choices := make([]string, 0, count)
	rawParts := make([]string, 0, count)

	for i := 0; i < count; i++ {
		messages := []map[string]string{}
		var system string
		for _, m := range req.Messages {
			if m.Role == "system" && system == "" {
				system = m.Content
				continue
			}
			messages = append(messages, map[string]string{"role": m.Role, "content": m.Content})
		}
		body := map[string]any{
			"model":    req.Model,
			"messages": messages,
		}
		if system != "" {
			body["system"] = system
		}
		if req.MaxTokens > 0 {
			body["max_tokens"] = req.MaxTokens
		} else if req.MaxCompletionTokens > 0 {
			body["max_tokens"] = req.MaxCompletionTokens
		} else {
			body["max_tokens"] = 256
		}
		if req.Temperature != nil {
			body["temperature"] = req.Temperature
		}

		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		endpoint := c.BaseURL + "/messages"
		httpReq, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(bodyBytes))
		if err != nil {
			return nil, fmt.Errorf("new request: %w", err)
		}
		httpReq.Header.Set("x-api-key", c.APIKey)
		httpReq.Header.Set("content-type", "application/json")
		httpReq.Header.Set("anthropic-version", "2023-06-01")
		resp, err := c.HTTPClient.Do(httpReq)
		if err != nil {
			return nil, fmt.Errorf("request failed: %w", err)
		}
		respBody, readErr := io.ReadAll(resp.Body)
		closeErr := resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read response body: %w", readErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close response body: %w", closeErr)
		}
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return nil, fmt.Errorf("claude http %d: %s", resp.StatusCode, string(respBody))
		}
		var completion struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(respBody, &completion); err != nil {
			return nil, fmt.Errorf("unmarshal response: %w", err)
		}
		if completion.Error.Message != "" {
			return nil, fmt.Errorf("claude error: %s", completion.Error.Message)
		}
		text := ""
		for _, c := range completion.Content {
			if c.Type == "text" {
				text += c.Text
			}
		}
		choices = append(choices, text)
		rawParts = append(rawParts, string(respBody))
	}

	return &CompletionResponse{Choices: choices, Raw: strings.Join(rawParts, "\n")}, nil
}
