package main

import (
	"strings"
	"testing"
)

func TestHelpContainsNewFlags(t *testing.T) {
	h := buildHelp()
	for _, want := range []string{"--version", "--no-color", "OPENAI_API_KEY", "CLAUDE_API_KEY", "AIC_SUGGESTIONS", "AIC_PROVIDER"} {
		if !strings.Contains(h, want) {
			t.Fatalf("help missing %s", want)
		}
	}
}
