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

// Gemini implements the Provider interface using Google's Gemini API.
type Gemini struct {
    APIKey     string
    HTTPClient *http.Client
    BaseURL    string
}

// NewGemini creates a new Gemini provider.
func NewGemini(apiKey string) *Gemini {
	return &Gemini{
		APIKey:     apiKey,
		HTTPClient: &http.Client{Timeout: 60 * time.Second},
		BaseURL:    "https://generativelanguage.googleapis.com/v1beta",
	}
}

func mapRole(role string) string {
	if role == "assistant" {
		return "model"
	}
	return role
}

// Chat sends a chat completion request to Gemini and converts the result to a generic CompletionResponse.
func (g *Gemini) Chat(req openai.ChatCompletionRequest) (*CompletionResponse, error) {
    // Build messages once; only maxOutputTokens may change across attempts
    messages := []map[string]any{}
    var system string
    for _, m := range req.Messages {
        if m.Role == "system" && system == "" {
            system = m.Content
            continue
        }
        messages = append(messages, map[string]any{
            "role":  mapRole(m.Role),
            "parts": []map[string]string{{"text": m.Content}},
        })
    }

    // Determine initial max output tokens
    maxOut := 256
    if req.MaxTokens > 0 {
        maxOut = req.MaxTokens
    } else if req.MaxCompletionTokens > 0 {
        maxOut = req.MaxCompletionTokens
    }

    // Up to 3 attempts: increase output tokens if we hit MAX_TOKENS with empty content
    for attempt := 1; attempt <= 3; attempt++ {
        body := map[string]any{
            "contents": messages,
        }
        if system != "" {
            body["systemInstruction"] = map[string]any{
                "parts": []map[string]any{{"text": system}},
            }
        }
        genConfig := map[string]any{
            "maxOutputTokens": maxOut,
        }
        if req.Temperature != nil {
            genConfig["temperature"] = req.Temperature
        }
        if req.N > 0 {
            genConfig["candidateCount"] = req.N
        }
        // Encourage plain text content
        genConfig["responseMimeType"] = "text/plain"
        body["generationConfig"] = genConfig

        bodyBytes, err := json.Marshal(body)
        if err != nil {
            return nil, fmt.Errorf("marshal request: %w", err)
        }
        endpoint := fmt.Sprintf("%s/models/%s:generateContent?key=%s", g.BaseURL, req.Model, g.APIKey)
        httpReq, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(bodyBytes))
        if err != nil {
            return nil, fmt.Errorf("new request: %w", err)
        }
        httpReq.Header.Set("content-type", "application/json")
        resp, err := g.HTTPClient.Do(httpReq)
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
            return nil, fmt.Errorf("gemini http %d: %s", resp.StatusCode, string(respBody))
        }
        var completion struct {
            Candidates []struct {
                Content struct {
                    Parts []struct {
                        Text string `json:"text"`
                    } `json:"parts"`
                } `json:"content"`
                FinishReason string `json:"finishReason"`
            } `json:"candidates"`
            Error struct {
                Message string `json:"message"`
            } `json:"error"`
        }
        if err := json.Unmarshal(respBody, &completion); err != nil {
            return nil, fmt.Errorf("unmarshal response: %w", err)
        }
        if completion.Error.Message != "" {
            return nil, fmt.Errorf("gemini error: %s", completion.Error.Message)
        }
        choices := make([]string, 0, len(completion.Candidates))
        allEmpty := true
        hitMaxTokens := false
        for _, c := range completion.Candidates {
            text := ""
            for _, p := range c.Content.Parts {
                text += p.Text
            }
            if strings.TrimSpace(text) != "" {
                allEmpty = false
            }
            if strings.EqualFold(c.FinishReason, "MAX_TOKENS") {
                hitMaxTokens = true
            }
            choices = append(choices, text)
        }
        if !allEmpty || !hitMaxTokens || attempt == 3 || maxOut >= 2048 {
            return &CompletionResponse{Choices: choices, Raw: string(respBody)}, nil
        }
        // Retry with larger output budget
        if maxOut < 2048 {
            maxOut *= 2
            if maxOut > 2048 {
                maxOut = 2048
            }
        }
    }
    // Unreachable due to returns in loop
    return nil, fmt.Errorf("unexpected gemini Chat loop exit")
}
