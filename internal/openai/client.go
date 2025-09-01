package openai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is a minimal OpenAI API client.
type Client struct {
	APIKey     string
	HTTPClient *http.Client
	BaseURL    string
}

func NewClient(apiKey string) *Client {
	return &Client{
		APIKey:     apiKey,
		HTTPClient: &http.Client{Timeout: 60 * time.Second},
		BaseURL:    "https://api.openai.com/v1",
	}
}

func (c *Client) Chat(req ChatCompletionRequest) (*ChatCompletionResponse, error) {
	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	endpoint := c.BaseURL + "/chat/completions"
	httpReq, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("openai http %d: %s", resp.StatusCode, string(respBody))
	}
	var completion ChatCompletionResponse
	if err := json.Unmarshal(respBody, &completion); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	if completion.Error.Message != "" {
		return nil, fmt.Errorf("openai error: %s", completion.Error.Message)
	}
	return &completion, nil
}
