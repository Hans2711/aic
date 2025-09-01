package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/diesi/aic/internal/cli"
	"github.com/diesi/aic/internal/commit"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] == "help" || os.Args[1] == "--help" || os.Args[1] == "-h" {
		printHelp()
		return
	}
	cmd := os.Args[1]
	switch cmd {
	case "aic", "ai-commit":
		var systemAddition string
		if len(os.Args) > 2 && os.Args[2] == "-s" && len(os.Args) > 3 { systemAddition = os.Args[3] }
		cfg, _ := commit.LoadConfig(systemAddition)
		apiKey := os.Getenv("OPENAI_API_KEY")
		if apiKey == "" { fatal(errors.New("OPENAI_API_KEY env var missing")) }
		stop := cli.Spinner(fmt.Sprintf("Requesting %d suggestions from %s", cfg.Suggestions, cfg.Model))
		suggestions, err := commit.GenerateSuggestions(cfg, apiKey)
		stop()
		if err != nil { fatal(err) }
		msg, err := commit.PromptUserSelect(suggestions)
		if err != nil { fatal(err) }
		if err := commit.OfferCommit(msg); err != nil { fatal(err) }
	default:
		fmt.Printf("Unknown command: %s\n", cmd)
		printHelp()
	}
}

func printHelp() {
	fmt.Println("aic - AI assisted git commit message generator")
	fmt.Println()
	fmt.Println("Usage: aic aic [-s <additional system instruction>]")
	fmt.Println()
	fmt.Println("Environment variables:")
	fmt.Println("  OPENAI_API_KEY      (required) OpenAI API key")
	fmt.Println("  AIC_MODEL           (optional) Model name, default gpt-4o-mini")
	fmt.Println("  AIC_SUGGESTIONS     (optional) Number of suggestions (1-15), default 5")
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	os.Exit(1)
}
