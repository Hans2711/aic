package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/diesi/aic/internal/cli"
	"github.com/diesi/aic/internal/commit"
)

func main() {
	// Environment variables are used directly; no .env file loading.
	var systemAddition string
	args := os.Args[1:]

	// Simple flag parsing
	for i, arg := range args {
		if arg == "-h" || arg == "--help" || arg == "help" {
			printHelp()
			return
		}
		if arg == "-s" {
			if i+1 < len(args) {
				systemAddition = args[i+1]
				// basic: assumes value isn't another flag
			}
		}
	}

	cfg, err := commit.LoadConfig(systemAddition)
	if err != nil {
		fatal(err)
	}

	stop := cli.Spinner(fmt.Sprintf("Requesting %d suggestions from %s", cfg.Suggestions, cfg.Model))
	suggestions, err := commit.GenerateSuggestions(cfg, os.Getenv("OPENAI_API_KEY"))
	stop()
	if err != nil {
		if isInvalidKeyErr(err) {
			fmt.Fprintln(os.Stderr, "Hint: Ensure your real OPENAI_API_KEY is exported (export OPENAI_API_KEY=sk-...)")
		}
		fatal(err)
	}
	msg, err := commit.PromptUserSelect(suggestions)
	if err != nil {
		fatal(err)
	}
	if err := commit.OfferCommit(msg); err != nil {
		fatal(err)
	}
}

func printHelp() {
	fmt.Println(`aic - AI-assisted git commit message generator.

Usage:
  aic [-s "extra instruction"]

Description:
  Generates conventional-style git commit messages based on your staged changes.
  It requests suggestions from an AI model, lets you choose one, and then offers
  to perform the commit or copy the message to your clipboard.

Arguments:
  -s <instruction>   An optional instruction to provide additional context
                     to the AI, like "focus on backend changes".

Environment Variables:
  OPENAI_API_KEY      (Required) Your OpenAI API key.
  AIC_MODEL           (Optional) The model to use (e.g., gpt-4o, gpt-5-2025-08-07).
                      Default: gpt-4o-mini.
  AIC_SUGGESTIONS     (Optional) The number of suggestions to request (1-15).
                      Default: 5.
  AIC_PROVIDER        (Optional) AI provider to use. Default: openai.
  AIC_DEBUG           (Optional) Set to "1" to print raw API responses on failure.

Example:
  aic -s "This is a refactor of the authentication logic"`)
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	os.Exit(1)
}

func isInvalidKeyErr(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(strings.ToLower(s), "invalid_api_key") || strings.Contains(strings.ToLower(s), "incorrect api key")
}
