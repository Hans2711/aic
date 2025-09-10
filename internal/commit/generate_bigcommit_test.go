package commit

import (
    "regexp"
    "strings"
    "testing"
)

func TestGenerateBigCommitMessage_Format(t *testing.T) {
    t.Setenv("AIC_MOCK", "1")
    // Avoid repo config reads
    t.Setenv("AIC_DISABLE_REPO_CONFIG", "1")

    cfg, err := LoadConfig("")
    if err != nil {
        t.Fatalf("LoadConfig error: %v", err)
    }
    cfg.BigCommit = true

    msg, err := GenerateBigCommitMessage(cfg, "")
    if err != nil {
        t.Fatalf("GenerateBigCommitMessage error: %v", err)
    }
    if strings.TrimSpace(msg) == "" {
        t.Fatalf("empty commit message")
    }
    // Normalize newlines
    msg = strings.ReplaceAll(msg, "\r\n", "\n")
    lines := strings.Split(msg, "\n")
    if len(lines) < 2 {
        t.Fatalf("expected at least 2 lines, got %d: %q", len(lines), msg)
    }
    // First line should be overall summary (no colon requirement, but must be non-empty)
    if strings.TrimSpace(lines[0]) == "" {
        t.Fatalf("first line (summary) is empty")
    }
    // Each subsequent line must match `<path>: <message>`
    re := regexp.MustCompile(`^[^:]+: .+`)
    for i := 1; i < len(lines); i++ {
        if strings.TrimSpace(lines[i]) == "" {
            t.Fatalf("line %d is empty", i+1)
        }
        if !re.MatchString(lines[i]) {
            t.Fatalf("line %d does not match '<path>: <message>': %q", i+1, lines[i])
        }
    }
}

