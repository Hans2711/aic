package analyze

import (
	"bytes"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/diesi/aic/internal/commit"
	"github.com/diesi/aic/internal/openai"
	"github.com/diesi/aic/internal/provider"
)

// Result summarizes AI-generated instructions and sample count.
type Result struct {
	Instructions string
	SampleTotal  int
}

var ccRe = regexp.MustCompile(`^([a-z]+)(\([^\)]+\))?:\s+(.+)$`)

// Analyze collects recent commit subjects and asks the configured AI provider to
// synthesize clear, prescriptive commit-style instructions for this repository.
// limit defines how many commits to inspect.
func Analyze(limit int, cfg commit.Config, apiKey string) (Result, error) {
	subjects, err := collectSubjects(limit)
	if err != nil {
		return Result{}, err
	}
	instr, err := generateInstructions(cfg, apiKey, subjects)
	if err != nil {
		return Result{}, err
	}
	return Result{Instructions: instr, SampleTotal: len(subjects)}, nil
}

// collectSubjects returns recent non-merge commit subjects (one per line, trimmed).
func collectSubjects(limit int) ([]string, error) {
	if limit <= 0 {
		limit = 500
	}
	args := []string{"log", fmt.Sprintf("-n%d", limit), "--pretty=%s", "--no-merges"}
	cmd := exec.Command("git", args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git log failed: %v: %s", err, out.String())
	}
	lines := strings.Split(out.String(), "\n")
	subs := make([]string, 0, len(lines))
	for _, raw := range lines {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}
		subs = append(subs, s)
	}
	return subs, nil
}

// generateInstructions prompts the AI model to output a single, concise
// instruction string for commit style based on the given subjects.
func generateInstructions(cfg commit.Config, apiKey string, subjects []string) (string, error) {
	if len(subjects) == 0 {
		// With no commits, fall back to a generic instruction set
		return "Use Conventional Commits (feat|fix|docs|refactor|chore|test|perf|build|ci|style). Imperative mood, subject <=72 chars, scope optional, no trailing period.", nil
	}

	var p provider.Provider
	switch cfg.Provider {
	case "claude":
		p = provider.NewClaude(apiKey)
	case "gemini":
		p = provider.NewGemini(apiKey)
	case "custom":
		p = provider.NewCustom(apiKey)
	default:
		p = provider.NewOpenAI(apiKey)
	}

	// Prepare the prompt. Ask for a single-line or compact, sentence-like
	// instruction set that we can store in .aic.json under "instructions".
	system := "You analyze Git commit history and produce a concise, prescriptive style guide for future commit messages. " +
		"Infer conventions actually used (types like feat|fix|docs|refactor|chore|test|perf|build|ci|style; whether scope is used; whether subjects end with a period; imperative mood; <=72 char subject). " +
		"Output only the final instruction text suitable for a config file; do not include examples, lists, or the analyzed messages."
	// Join subjects in a compact block. We only pass subjects, not bodies.
	user := "Recent commit subjects (one per line):\n" + strings.Join(subjects, "\n")

	temp := float32(0.3)
	req := openai.ChatCompletionRequest{
		Model:       cfg.Model,
		Messages:    []openai.Message{{Role: "system", Content: system}, {Role: "user", Content: user}},
		MaxTokens:   280,
		N:           1,
		Temperature: &temp,
	}
	resp, err := p.Chat(req)
	if err != nil {
		return "", err
	}
	if resp == nil || len(resp.Choices) == 0 {
		return "", fmt.Errorf("no output from provider")
	}
	out := strings.TrimSpace(resp.Choices[0])
	// Ensure it's a single line (config field); keep punctuation.
	out = strings.ReplaceAll(out, "\n", " ")
	out = strings.TrimSpace(out)
	if out == "" {
		return "", fmt.Errorf("empty instructions from provider")
	}
	return out, nil
}
