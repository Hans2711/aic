package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/diesi/aic/internal/analyze"
	"github.com/diesi/aic/internal/cli"
	"github.com/diesi/aic/internal/commit"
	"github.com/diesi/aic/internal/config"
	"github.com/diesi/aic/internal/git"
	"github.com/diesi/aic/internal/version"
	"strconv"
)

func main() {
	// Environment variables are used directly; no .env file loading.
	var systemAddition string
	var hookFile string
	args := os.Args[1:]

	// Subcommand: analyze
	if len(args) > 0 && (args[0] == "analyze" || args[0] == "analyse") {
		runAnalyze(args[1:])
		return
	}

	// Subcommand: update
	if len(args) > 0 && (args[0] == "update") {
		runUpdate()
		return
	}

	// Soft warning for unknown/unused AIC_* variables to catch typos/misconfig
	config.WarnUnknownAICEnv()

	// Simple, robust flag parsing (skips consumed values)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-h" || arg == "--help" || arg == "help":
			fmt.Print(buildHelp())
			return
		case arg == "--version" || arg == "-v":
			fmt.Printf("aic %s\n", version.Get())
			return
		case arg == "--no-color":
			cli.DisableColors()
		case strings.HasPrefix(arg, "--hook="):
			hookFile = strings.TrimPrefix(arg, "--hook=")
		case arg == "--hook":
			if i+1 < len(args) {
				hookFile = args[i+1]
				i++
			}
		case strings.HasPrefix(arg, "-s="):
			systemAddition = strings.TrimPrefix(arg, "-s=")
		case arg == "-s":
			if i+1 < len(args) {
				systemAddition = args[i+1]
				i++
			}
		}
	}

	cfg, err := commit.LoadConfig(systemAddition)
	if err != nil {
		fatal(err)
	}

	// Show which staged files are included in the diff (for transparency)
	if files, err := git.StagedFiles(); err == nil && len(files) > 0 {
		fmt.Printf("%s%s Staged changes:%s\n", cli.ColorGray, cli.ColorBold, cli.ColorReset)
		for _, f := range files {
			fmt.Printf("  %s- %s%s\n", cli.ColorYellow, f, cli.ColorReset)
		}
	}

	stop := cli.Spinner(fmt.Sprintf("Requesting %d suggestions from %s", cfg.Suggestions, cfg.Model))
	var apiKey string
	switch cfg.Provider {
	case "claude":
		apiKey = config.Get(config.EnvClaudeAPIKey)
	case "gemini":
		apiKey = config.Get(config.EnvGeminiAPIKey)
	case "custom":
		apiKey = config.Get(config.EnvCustomAPIKey) // may be empty for local servers
	default:
		apiKey = config.Get(config.EnvOpenAIAPIKey)
	}
	suggestions, err := commit.GenerateSuggestions(cfg, apiKey)
	stop(err == nil)
	if err != nil {
		if isInvalidKeyErr(err) {
			switch cfg.Provider {
			case "claude":
				fmt.Fprintln(os.Stderr, "Hint: Ensure your real CLAUDE_API_KEY is exported (export CLAUDE_API_KEY=sk-...)")
			case "gemini":
				fmt.Fprintln(os.Stderr, "Hint: Ensure your real GEMINI_API_KEY is exported (export GEMINI_API_KEY=sk-...)")
			default:
				fmt.Fprintln(os.Stderr, "Hint: Ensure your real OPENAI_API_KEY is exported (export OPENAI_API_KEY=sk-...)")
			}
		}
		fatal(err)
	}
	msg, err := commit.PromptUserSelect(cfg, suggestions)
	if err != nil {
		fatal(err)
	}
	// If invoked as a Git hook, write the message to the given file and exit.
	if hookFile != "" {
		if err := os.WriteFile(hookFile, []byte(msg+"\n"), 0644); err != nil {
			fatal(fmt.Errorf("failed to write hook message file: %w", err))
		}
		return
	}
	if err := commit.OfferCommit(msg); err != nil {
		fatal(err)
	}
}

func runAnalyze(args []string) {
	// Defaults
	limit := 1000
	// Allow simple flag: --limit N
	for i := 0; i < len(args); i++ {
		if args[i] == "--limit" && i+1 < len(args) {
			if n, err := strconv.Atoi(args[i+1]); err == nil && n > 0 {
				limit = n
			}
			i++
		}
	}
	// Build a config without extra instructions to avoid biasing analysis
	cfg, err := commit.LoadConfig("")
	if err != nil {
		fatal(err)
	}
	var apiKey string
	switch cfg.Provider {
	case "claude":
		apiKey = config.Get(config.EnvClaudeAPIKey)
	case "gemini":
		apiKey = config.Get(config.EnvGeminiAPIKey)
	case "custom":
		apiKey = config.Get(config.EnvCustomAPIKey)
	default:
		apiKey = config.Get(config.EnvOpenAIAPIKey)
	}
	res, err := analyze.Analyze(limit, cfg, apiKey)
	if err != nil {
		fatal(err)
	}
	if err := config.SaveRepoInstructions(res.Instructions); err != nil {
		fatal(err)
	}
	_ = config.SaveRepoStyle(res.Instructions, res.Embedding)
	fmt.Printf("%s%s Wrote repo .aic.json%s\n", cli.ColorGray, cli.ColorBold, cli.ColorReset)
	fmt.Printf("  %sAnalyzed %d commit subjects and generated style instructions.%s\n", cli.ColorDim, res.SampleTotal, cli.ColorReset)
}

func buildHelp() string {
	// Core env rows come from config, then CLI flags, then custom-provider env rows.
	rows := make([][2]string, 0, 32)
	rows = append(rows, config.HelpEnvRowsCore()...)
	rows = append(rows,
		[2]string{"--version / -v", "Show version and exit"},
		[2]string{"--no-color", "Disable colored output (alias: AIC_NO_COLOR=1)"},
		[2]string{"--hook <file>", "Hook mode: write selected message to file and exit"},
		[2]string{"analyze [--limit N]", "Infer repo commit style and write .aic.json"},
	)
	rows = append(rows, config.HelpEnvRowsCustom()...)
	// Commands
	rows = append(rows,
		[2]string{"update", "Update aic to the latest version"},
	)
	maxVar := 0
	for _, r := range rows {
		if len(r[0]) > maxVar {
			maxVar = len(r[0])
		}
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s%s aic%s – %sAI-assisted git commit message generator%s\n\n", cli.ColorBold, cli.ColorCyan, cli.ColorReset, cli.ColorMagenta, cli.ColorReset))
	b.WriteString(fmt.Sprintf("%sUsage%s:\n", cli.ColorBold, cli.ColorReset))
	b.WriteString("  aic [-s \"extra instruction\"] [--version] [--no-color]\n\n")
	b.WriteString(fmt.Sprintf("%sDescription%s:\n", cli.ColorBold, cli.ColorReset))
	b.WriteString("  Generates concise, natural-language Git commit messages based on your staged changes.\n")
	b.WriteString("  It requests suggestions from an AI model, lets you choose one, then offers to commit.\n")
	b.WriteString("  Also includes 'aic analyze' to infer repo style and write .aic.json presets.\n\n")
	b.WriteString(fmt.Sprintf("%sArguments & Environment%s:\n", cli.ColorBold, cli.ColorReset))
	for _, r := range rows {
		pad := strings.Repeat(" ", maxVar-len(r[0]))
		color := cli.ColorCyan
		if strings.Contains(r[1], "required") {
			color = cli.ColorRed
		}
		b.WriteString(fmt.Sprintf("  %s%s%s%s  %s%s%s\n", cli.ColorBold, r[0], cli.ColorReset, pad, color, r[1], cli.ColorReset))
	}
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("%sExample%s:\n", cli.ColorBold, cli.ColorReset))
	b.WriteString("  aic -s \"Refactor auth logic\"\n")
	return b.String()
}
func runUpdate() {
    if runtime.GOOS == "windows" {
        fatal(fmt.Errorf("update is not supported on Windows; please reinstall from the latest release or rebuild from source"))
    }
    // Try repo at ~/aic first
    home := os.Getenv("HOME")
    repoHome := filepath.Join(home, "aic")
    if dirExists(filepath.Join(repoHome, ".git")) {
        if err := gitPull(repoHome); err != nil {
            fmt.Fprintf(os.Stderr, "%sFailed to pull in %s: %v%s\n", cli.ColorRed, repoHome, err, cli.ColorReset)
        } else {
            // Re-run installer to refresh links if needed
            _ = runInstaller(filepath.Join(repoHome, "scripts", "install.sh"))
            fmt.Printf("%sUpdated from %s and refreshed installation.%s\n", cli.ColorGreen, repoHome, cli.ColorReset)
            return
        }
    }

    // Try to discover repo from executable path by walking up to find .git
    if repoExec := findGitRootFromExecutable(); repoExec != "" {
        if err := gitPull(repoExec); err != nil {
            fmt.Fprintf(os.Stderr, "%sFailed to pull in %s: %v%s\n", cli.ColorRed, repoExec, err, cli.ColorReset)
        } else {
            _ = runInstaller(filepath.Join(repoExec, "scripts", "install.sh"))
            fmt.Printf("%sUpdated from %s and refreshed installation.%s\n", cli.ColorGreen, repoExec, cli.ColorReset)
            return
        }
    }

    // Fallback: remote installer (fresh or repair)
    fmt.Printf("%sNo local repo found; fetching installer...%s\n", cli.ColorDim, cli.ColorReset)
    cmd := exec.Command("bash", "-lc", "(curl -fsSL https://raw.githubusercontent.com/Hans2711/aic/master/scripts/install.sh || wget -qO- https://raw.githubusercontent.com/Hans2711/aic/master/scripts/install.sh) | bash")
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    if err := cmd.Run(); err != nil {
        fatal(fmt.Errorf("update failed: %w", err))
    }
}

func gitPull(repo string) error {
    // Fast-forward only to avoid unintended merges
    cmd := exec.Command("git", "-C", repo, "pull", "--ff-only", "--rebase=false")
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    return cmd.Run()
}

func runInstaller(path string) error {
    if !fileExists(path) {
        return fmt.Errorf("installer not found: %s", path)
    }
    cmd := exec.Command("bash", path)
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    return cmd.Run()
}

func findGitRootFromExecutable() string {
    exe, err := os.Executable()
    if err != nil {
        return ""
    }
    real, err := filepath.EvalSymlinks(exe)
    if err != nil {
        real = exe
    }
    dir := filepath.Dir(real)
    // Walk upward until root looking for .git
    for i := 0; i < 10; i++ { // cap to reasonable depth
        if dirExists(filepath.Join(dir, ".git")) {
            return dir
        }
        parent := filepath.Dir(dir)
        if parent == dir {
            break
        }
        dir = parent
    }
    return ""
}

func dirExists(path string) bool {
    st, err := os.Stat(path)
    if err != nil {
        return false
    }
    return st.IsDir()
}

func fileExists(path string) bool {
    st, err := os.Stat(path)
    if err != nil {
        return false
    }
    return !st.IsDir()
}
func fatal(err error) {
	// Provide nicer categorized errors
	banner := fmt.Sprintf("%s%s %sERROR%s", cli.ColorBold, cli.ColorRed, cli.IconError, cli.ColorReset)
	hintLines := []string{}
	msg := err.Error()
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "no staged changes"):
		hintLines = append(hintLines, fmt.Sprintf("%s%s%s Stage changes first, e.g.: %sgit add -p%s", cli.ColorYellow, cli.IconInfo, cli.ColorReset, cli.ColorGreen, cli.ColorReset))
	case strings.Contains(lower, "not a git repository"):
		hintLines = append(hintLines, fmt.Sprintf("%s%s%s Run %sgit init%s or cd into a repo.", cli.ColorYellow, cli.IconInfo, cli.ColorReset, cli.ColorGreen, cli.ColorReset))
	case strings.Contains(lower, "missing openai_api_key"):
		hintLines = append(hintLines, fmt.Sprintf("%s%s%s Export your key: %sexport OPENAI_API_KEY=sk-***%s", cli.ColorYellow, cli.IconInfo, cli.ColorReset, cli.ColorGreen, cli.ColorReset))
	case strings.Contains(lower, "missing claude_api_key"):
		hintLines = append(hintLines, fmt.Sprintf("%s%s%s Export your key: %sexport CLAUDE_API_KEY=sk-***%s", cli.ColorYellow, cli.IconInfo, cli.ColorReset, cli.ColorGreen, cli.ColorReset))
	case strings.Contains(lower, "missing gemini_api_key"):
		hintLines = append(hintLines, fmt.Sprintf("%s%s%s Export your key: %sexport GEMINI_API_KEY=sk-***%s", cli.ColorYellow, cli.IconInfo, cli.ColorReset, cli.ColorGreen, cli.ColorReset))
	case isRateLimitErr(lower):
		hintLines = append(hintLines, fmt.Sprintf("%s%s%s Rate limits; wait or lower suggestions (AIC_SUGGESTIONS=3).", cli.ColorYellow, cli.IconInfo, cli.ColorReset))
	}

	// Add generic retry suggestion for transient network errors
	if strings.Contains(lower, "timeout") || strings.Contains(lower, "temporarily") || strings.Contains(lower, "connection refused") {
		hintLines = append(hintLines, fmt.Sprintf("%s%s%s Network issue – retry shortly.", cli.ColorYellow, cli.IconInfo, cli.ColorReset))
	}
	ts := time.Now().Format("15:04:05")
	fmt.Fprintf(os.Stderr, "%s %s%s%s\n  %s%v%s\n", banner, cli.ColorDim, ts, cli.ColorReset, cli.ColorRed, msg, cli.ColorReset)
	if len(hintLines) > 0 {
		for _, h := range hintLines {
			fmt.Fprintln(os.Stderr, "  "+h)
		}
	}
	// Provide debug env hint if user wants more
	if !config.Bool(config.EnvAICDebug) {
		fmt.Fprintf(os.Stderr, "  %s%s%s Set AIC_DEBUG=1 for verbose response details.%s\n", cli.ColorDim, cli.IconInfo, cli.ColorReset, cli.ColorReset)
	}
	os.Exit(1)
}

func isInvalidKeyErr(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(strings.ToLower(s), "invalid_api_key") || strings.Contains(strings.ToLower(s), "incorrect api key")
}

func isRateLimitErr(s string) bool {
	if s == "" {
		return false
	}
	s = strings.ToLower(s)
	return strings.Contains(s, "rate limit") || strings.Contains(s, "too many requests")
}
