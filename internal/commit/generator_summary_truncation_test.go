package commit

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// Ensure firstNRunes truncates on rune boundaries and maintains valid UTF-8.
func TestFirstNRunesUTF8Boundary(t *testing.T) {
	base := strings.Repeat("a", 47999)
	multi := "汉" // multi-byte character (3 bytes)
	tail := "b"
	diff := base + multi + tail // total runes: 48001

	truncated := firstNRunes(diff, 48000)

	if !utf8.ValidString(truncated) {
		t.Fatalf("truncated string is not valid UTF-8")
	}
	if strings.HasSuffix(truncated, tail) {
		t.Fatalf("expected tail character to be truncated")
	}
	if !strings.HasSuffix(truncated, multi) {
		t.Fatalf("expected multi-byte character to be retained")
	}
	if n := utf8.RuneCountInString(truncated); n != 48000 {
		t.Fatalf("expected %d runes, got %d", 48000, n)
	}
}
