package provider

import (
    "bytes"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "regexp"
    "strings"
    "time"

    "github.com/diesi/aic/internal/config"
    "github.com/diesi/aic/internal/openai"
)

// Custom implements the Provider interface using a configurable OpenAI-compatible server.
// It targets a base URL and allows overriding individual endpoint paths via env vars.
// Only Chat is required by this app; other paths exist for completeness and future use.
type Custom struct {
    APIKey     string
    HTTPClient *http.Client
    BaseURL    string
    // Paths (joined with BaseURL)
    ChatCompletionsPath string
    CompletionsPath     string
    EmbeddingsPath      string
    ModelsPath          string
}

// envOr returns v if non-empty, otherwise def.
func envOr(v, def string) string { if strings.TrimSpace(v) != "" { return v }; return def }

// NewCustom creates a new Custom provider using environment configuration.
// If apiKey is empty, no Authorization header is sent.
func NewCustom(apiKey string) *Custom {
    base := envOr(config.Get(config.EnvCustomBaseURL), "http://127.0.0.1:1234")
    return &Custom{
        APIKey:              apiKey,
        HTTPClient:          &http.Client{Timeout: 60 * time.Second},
        BaseURL:             strings.TrimRight(base, "/"),
        ChatCompletionsPath: envOr(config.Get(config.EnvCustomChatCompletionsPath), "/v1/chat/completions"),
        CompletionsPath:     envOr(config.Get(config.EnvCustomCompletionsPath), "/v1/completions"),
        EmbeddingsPath:      envOr(config.Get(config.EnvCustomEmbeddingsPath), "/v1/embeddings"),
        ModelsPath:          envOr(config.Get(config.EnvCustomModelsPath), "/v1/models"),
    }
}

func (c *Custom) endpoint(path string) string {
    if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
        return path
    }
    if !strings.HasPrefix(path, "/") {
        path = "/" + path
    }
    return c.BaseURL + path
}

// ensureModel populates req.Model by querying the models endpoint when needed.
func (c *Custom) ensureModel(req *openai.ChatCompletionRequest) error {
    m := strings.TrimSpace(req.Model)
    if m != "" && strings.ToLower(m) != "auto" {
        return nil
    }
    // GET models
    url := c.endpoint(c.ModelsPath)
    httpReq, err := http.NewRequest("GET", url, nil)
    if err != nil {
        return fmt.Errorf("new request: %w", err)
    }
    if c.APIKey != "" {
        httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
    }
    resp, err := c.HTTPClient.Do(httpReq)
    if err != nil {
        return fmt.Errorf("request failed: %w", err)
    }
    body, readErr := io.ReadAll(resp.Body)
    closeErr := resp.Body.Close()
    if readErr != nil {
        return fmt.Errorf("read response body: %w", readErr)
    }
    if closeErr != nil {
        return fmt.Errorf("close response body: %w", closeErr)
    }
    if resp.StatusCode < 200 || resp.StatusCode > 299 {
        return fmt.Errorf("custom models http %d: %s", resp.StatusCode, string(body))
    }
    // Try OpenAI-compatible shape: { data: [ { id: "..." }, ... ] }
    var models struct {
        Data []struct{ ID string `json:"id"` } `json:"data"`
    }
    if err := json.Unmarshal(body, &models); err == nil && len(models.Data) > 0 && strings.TrimSpace(models.Data[0].ID) != "" {
        req.Model = models.Data[0].ID
        return nil
    }
    // Fallback: try simple array [ { "id": "..." } ]
    var arr []struct{ ID string `json:"id"` }
    if err := json.Unmarshal(body, &arr); err == nil && len(arr) > 0 && strings.TrimSpace(arr[0].ID) != "" {
        req.Model = arr[0].ID
        return nil
    }
    return fmt.Errorf("could not determine model from /models response")
}

// Chat sends a chat completion request to the custom server and maps the response.
func (c *Custom) Chat(req openai.ChatCompletionRequest) (*CompletionResponse, error) {
    attempt := 0
    for {
        attempt++
        // Auto-resolve model if needed
        if err := c.ensureModel(&req); err != nil {
            return nil, err
        }
        bodyBytes, err := json.Marshal(req)
        if err != nil {
            return nil, fmt.Errorf("marshal request: %w", err)
        }
        endpoint := c.endpoint(c.ChatCompletionsPath)
        httpReq, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(bodyBytes))
        if err != nil {
            return nil, fmt.Errorf("new request: %w", err)
        }
        if c.APIKey != "" {
            httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
        }
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
            // try minor compatibility adjustments based on error text
            if attempt < 3 {
                if strings.Contains(bodyStr, "Unsupported parameter: 'max_tokens'") {
                    req.MaxCompletionTokens = req.MaxTokens
                    req.MaxTokens = 0
                    continue
                }
                if strings.Contains(bodyStr, "Unsupported value: 'temperature'") && req.Temperature != nil {
                    req.Temperature = nil
                    continue
                }
            }
            return nil, fmt.Errorf("custom http %d: %s", resp.StatusCode, bodyStr)
        }
        var completion openai.ChatCompletionResponse
        if err := json.Unmarshal(respBody, &completion); err != nil {
            return nil, fmt.Errorf("unmarshal response: %w", err)
        }
        completion.Raw = string(respBody)
        if completion.Error.Message != "" {
            return nil, fmt.Errorf("custom error: %s", completion.Error.Message)
        }
        // If server splits reasoning vs answer across choices, merge and strip reasoning blocks.
        joined := ""
        for i, ch := range completion.Choices {
            if i > 0 { joined += "\n" }
            joined += ch.Message.Content
        }
        if s := stripReasoning(joined); strings.TrimSpace(s) != "" {
            return &CompletionResponse{Choices: []string{strings.TrimSpace(s)}, Raw: completion.Raw}, nil
        }
        // Handle empty content with finish_reason=length similarly
        if len(completion.Choices) > 0 {
            allEmptyWithLength := true
            for _, choice := range completion.Choices {
                if choice.Message.Content != "" || choice.FinishReason != "length" {
                    allEmptyWithLength = false
                    break
                }
            }
            if allEmptyWithLength && req.MaxCompletionTokens > 0 {
                req.MaxCompletionTokens *= 2
                if req.MaxCompletionTokens > 8000 {
                    // Safety cap; return as-is
                    out := &CompletionResponse{Raw: completion.Raw}
                    for _, ch := range completion.Choices {
                        out.Choices = append(out.Choices, ch.Message.Content)
                    }
                    return out, nil
                }
                continue
            }
        }
        // Map to generic response with per-choice think stripping
        out := &CompletionResponse{Raw: completion.Raw}
        for _, ch := range completion.Choices {
            cleaned := strings.TrimSpace(stripReasoning(ch.Message.Content))
            if cleaned == "" { continue }
            out.Choices = append(out.Choices, cleaned)
        }
        return out, nil
    }
}

var (
    // Remove balanced <think>...</think>
    thinkBalancedRe = regexp.MustCompile(`(?is)<think\b[^>]*>.*?</think>`)
    // Remove standalone opening/closing think tags
    thinkTagOnlyRe  = regexp.MustCompile(`(?is)</?think\b[^>]*>`) 
)

func stripReasoning(s string) string {
    // First drop balanced blocks
    s = thinkBalancedRe.ReplaceAllString(s, "")
    // Then remove any stray open/close tags left
    s = thinkTagOnlyRe.ReplaceAllString(s, "")
    return strings.TrimSpace(s)
}
