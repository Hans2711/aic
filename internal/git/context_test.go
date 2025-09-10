package git

import "testing"

func TestFormatContext(t *testing.T) {
	got := formatContext("feat-123", "Add feature", []string{"Issue 1 desc", "Issue 2 desc"})
	want := "Branch: feat-123\nPR Title: Add feature\nIssue: Issue 1 desc\nIssue: Issue 2 desc"
	if got != want {
		t.Fatalf("unexpected format: %q", got)
	}
}

func TestFormatContextEmpty(t *testing.T) {
	if s := formatContext("", "", nil); s != "" {
		t.Fatalf("expected empty, got %q", s)
	}
}
