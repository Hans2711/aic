package commit

import "testing"

func TestGenerateSuggestionsMockModeDefaultTrim(t *testing.T) {
    t.Setenv("AIC_MOCK", "1")
    cfg, _ := LoadConfig("")
    cfg.Suggestions = 2
    got, err := GenerateSuggestions(cfg, "")
    if err != nil { t.Fatalf("unexpected error: %v", err) }
    if len(got) != 2 { t.Fatalf("expected 2 suggestions, got %d: %v", len(got), got) }
}

func TestPromptAndOfferNonInteractive(t *testing.T) {
    t.Setenv("AIC_NON_INTERACTIVE", "1")
    // PromptUserSelect should pick the first when non-interactive
    msg, err := PromptUserSelect([]string{"first", "second"})
    if err != nil { t.Fatalf("unexpected error: %v", err) }
    if msg != "first" { t.Fatalf("expected 'first', got %q", msg) }

    // OfferCommit should not attempt to commit unless AIC_AUTO_COMMIT=1
    if err := OfferCommit(msg); err != nil {
        t.Fatalf("OfferCommit returned error in non-interactive without auto commit: %v", err)
    }
}

