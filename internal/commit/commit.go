package commit

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/diesi/aic/internal/cli"
	"github.com/diesi/aic/internal/config"
)

// OfferCommit asks to commit or copy to clipboard.
func OfferCommit(msg string) error {
	fmt.Printf("\n%sSelected commit message:%s\n  %s%s%s\n", cli.ColorBold, cli.ColorReset, cli.ColorGreen, msg, cli.ColorReset)
	if config.Bool(config.EnvAICNonInteractive) {
		// In CI/test mode, don't attempt to commit unless explicitly allowed
		if config.Bool(config.EnvAICAutoCommit) {
			cmd := exec.Command("git", "commit", "-m", msg)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			return cmd.Run()
		}
		fmt.Printf("Non-interactive mode: skipping commit (set AIC_AUTO_COMMIT=1 to enable).\n")
		return nil
	}
	fmt.Printf("\n%s%s Commit with this message now?%s %s[Y|n]%s %s[default: Y]%s: %s", cli.ColorBold, cli.IconPrompt, cli.ColorReset, cli.ColorYellow, cli.ColorReset, cli.ColorDim, cli.ColorReset, cli.ColorCyan)
	var commitChoice string
	fmt.Scanln(&commitChoice)
	if strings.ToLower(commitChoice) == "y" || commitChoice == "" {
		cmd := exec.Command("git", "commit", "-m", msg)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
	if copyToClipboard(msg) {
		fmt.Printf("%sMessage copied to clipboard.%s\n", cli.ColorGreen, cli.ColorReset)
	}
	return nil
}

type clipboardTool struct {
	name string
	args []string
}

var clipboardTools = []clipboardTool{
	{name: "pbcopy"},
	{name: "wl-copy"},
	{name: "xclip", args: []string{"-selection", "clipboard"}},
	{name: "clip"},
}

// copyToClipboard tries a series of common clipboard tools until one succeeds.
func copyToClipboard(msg string) bool {
	run := func(name string, args ...string) error {
		c := exec.Command(name, args...)
		c.Stdin = strings.NewReader(msg)
		return c.Run()
	}
	return tryClipboard(msg, exec.LookPath, run)
}

// tryClipboard attempts available clipboard tools using provided lookPath and run helpers.
func tryClipboard(msg string, lookPath func(string) (string, error), run func(string, ...string) error) bool {
	for _, t := range clipboardTools {
		if _, err := lookPath(t.name); err == nil {
			if run(t.name, t.args...) == nil {
				return true
			}
		}
	}
	return false
}
