package openai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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
	attempt := 0
	for {
		attempt++
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
		respBody, readErr := io.ReadAll(resp.Body)
		closeErr := resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read response body: %w", readErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close response body: %w", closeErr)
		}
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			bodyStr := string(respBody)
			if attempt < 3 {
				if strings.Contains(bodyStr, "Unsupported parameter: 'max_tokens'") {
					req.MaxCompletionTokens = req.MaxTokens
					req.MaxTokens = 0
					continue
				}
				if strings.Contains(bodyStr, "Unsupported value: 'temperature'") && req.Temperature != nil {
					// remove temperature to use provider default
					req.Temperature = nil
					continue
				}
			}
			return nil, fmt.Errorf("openai http %d: %s", resp.StatusCode, bodyStr)
		}
		var completion ChatCompletionResponse
		if err := json.Unmarshal(respBody, &completion); err != nil {
			return nil, fmt.Errorf("unmarshal response: %w", err)
		}
		completion.Raw = string(respBody)
		if completion.Error.Message != "" {
			return nil, fmt.Errorf("openai error: %s", completion.Error.Message)
		}
		// Handle empty content with finish_reason=length
		if len(completion.Choices) > 0 {
			allEmptyWithLength := true
			for _, choice := range completion.Choices {
				if choice.Message.Content != "" || choice.FinishReason != "length" {
					allEmptyWithLength = false
					break
				}
			}
			if allEmptyWithLength && req.MaxCompletionTokens > 0 {
				// Likely hit token limit before generating content. Retry with larger limit.
				req.MaxCompletionTokens *= 2        // Double the tokens and retry
				if req.MaxCompletionTokens > 8000 { // Safety cap
					return &completion, nil
				}
				continue
			}
		}
		return &completion, nil
	}
}
