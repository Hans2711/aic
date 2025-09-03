package commit

import (
    "bytes"
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
            if err := cmd.Run(); err != nil {
                return err
            }
            // Non-interactive mode: do not prompt for push
            return nil
        }
        fmt.Printf("Non-interactive mode: skipping commit (set AIC_AUTO_COMMIT=1 to enable).\n")
        return nil
    }
    fmt.Printf("\n%s%s Commit with this message now?%s %s[Y|n]%s %s[default: Y]%s  %s(Alternative: copy to clipboard)%s: %s", 
        cli.ColorBold, cli.IconPrompt, cli.ColorReset, 
        cli.ColorYellow, cli.ColorReset, cli.ColorDim, cli.ColorReset,
        cli.ColorGray, cli.ColorReset, cli.ColorCyan)
    var commitChoice string
    fmt.Scanln(&commitChoice)
    if strings.ToLower(commitChoice) == "y" || commitChoice == "" {
        cmd := exec.Command("git", "commit", "-m", msg)
        cmd.Stdout = os.Stdout
        cmd.Stderr = os.Stderr
        if err := cmd.Run(); err != nil {
            return err
        }
        // After committing, offer to push to the current branch
        fmt.Printf("\n%s%s Push to current branch now?%s %s[Y|n]%s %s[default: Y]%s: %s", 
            cli.ColorBold, cli.IconPrompt, cli.ColorReset, cli.ColorYellow, cli.ColorReset, cli.ColorDim, cli.ColorReset, cli.ColorCyan)
        var pushChoice string
        fmt.Scanln(&pushChoice)
        if strings.ToLower(pushChoice) == "y" || pushChoice == "" {
            if err := pushCurrentBranch(); err != nil {
                fmt.Printf("%sPush failed:%s %v\n", cli.ColorYellow, cli.ColorReset, err)
            }
        }
        return nil
    }
    if copyToClipboard(msg) {
        fmt.Printf("%sMessage copied to clipboard.%s\n", cli.ColorGreen, cli.ColorReset)
    }
    return nil
}

// pushCurrentBranch tries to push the current branch to its upstream if configured,
// otherwise sets upstream to a likely remote (origin or first remote).
func pushCurrentBranch() error {
    // Detect current branch
    cur, err := gitOutput("rev-parse", "--abbrev-ref", "HEAD")
    if err != nil {
        return err
    }
    branch := strings.TrimSpace(cur)
    if branch == "HEAD" || branch == "" {
        return fmt.Errorf("cannot determine current branch (detached HEAD)")
    }

    // Check if upstream exists
    if _, err := gitOutput("rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}"); err == nil {
        // Upstream configured; a plain 'git push' should push to upstream with default settings
        cmd := exec.Command("git", "push")
        cmd.Stdout = os.Stdout
        cmd.Stderr = os.Stderr
        return cmd.Run()
    }

    // No upstream: find a reasonable remote
    remote, rerr := gitOutput("config", "--get", "branch."+branch+".remote")
    remote = strings.TrimSpace(remote)
    if rerr != nil || remote == "" {
        // Fallback to 'origin' if present
        remotes, _ := gitOutput("remote")
        cand := ""
        for _, line := range strings.Split(strings.TrimSpace(remotes), "\n") {
            line = strings.TrimSpace(line)
            if line == "" {
                continue
            }
            if cand == "" {
                cand = line
            }
            if line == "origin" {
                remote = line
                break
            }
        }
        if remote == "" {
            remote = cand
        }
    }
    if remote == "" {
        return fmt.Errorf("no Git remote configured")
    }

    // Set upstream on first push
    cmd := exec.Command("git", "push", "-u", remote, branch)
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    return cmd.Run()
}

// gitOutput runs `git ...args` and returns stdout as string.
func gitOutput(args ...string) (string, error) {
    cmd := exec.Command("git", args...)
    var out bytes.Buffer
    cmd.Stdout = &out
    cmd.Stderr = os.Stderr
    if err := cmd.Run(); err != nil {
        return "", err
    }
    return out.String(), nil
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
