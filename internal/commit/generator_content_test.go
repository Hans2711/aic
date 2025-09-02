package commit

import (
	"strings"
	"testing"
)

func TestComposeUserContentWithSummary(t *testing.T) {
	full := "AAAA" + makeStr(20000) // ensure larger than truncated portion
	trunc := full[:16000]
	summary := "File changes: modified foo.go, added bar.go\nKey Impacts: updated API"
	uc := composeUserContent(full, trunc, summary)
	if len(uc) == 0 {
		t.Fatal("user content empty")
	}
	if !containsAll(uc, []string{"DIFF SUMMARY (model-generated)", "--- BEGIN TRUNCATED RAW DIFF ---", "--- END TRUNCATED RAW DIFF ---", "[TRUNCATED: showing first"}) {
		snippet := uc
		if len(snippet) > 400 {
			snippet = snippet[:400]
		}
		t.Fatalf("missing expected markers. snippet: %q", snippet)
	}
	// Ensure truncated diff content actually present and matches prefix of full diff
	if ucFind := findSegment(uc, trunc[:200]); ucFind == -1 {
		t.Fatalf("truncated diff segment not found in user content")
	}
}

func TestComposeUserContentNoSummary(t *testing.T) {
	full := makeStr(1000)
	uc := composeUserContent(full, full, "")
	if uc != full {
		t.Fatalf("expected raw trunc when no summary, got length %d vs %d", len(uc), len(full))
	}
}

func makeStr(n int) string {
	b := make([]byte, n)
	for i := 0; i < n; i++ {
		b[i] = byte('a' + (i % 26))
	}
	return string(b)
}

func containsAll(s string, subs []string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}

func findSegment(h, seg string) int { return strings.Index(h, seg) }
