package commit

import (
    "bytes"
    "fmt"
    "os"
    "os/exec"
    "regexp"
    "strconv"
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
            } else {
                // After successful push, offer to bump and push a tag (default: No)
                fmt.Printf("\n%s%s Increment latest tag?%s %s[y|N]%s %s[default: N]%s: %s",
                    cli.ColorBold, cli.IconPrompt, cli.ColorReset,
                    cli.ColorYellow, cli.ColorReset, cli.ColorDim, cli.ColorReset, cli.ColorCyan)
                var tagChoice string
                fmt.Scanln(&tagChoice)
                if strings.ToLower(tagChoice) == "y" {
                    // Determine latest semver-like tag
                    latest, vPrefix, maj, min, pat, err := latestSemverTag()
                    if err != nil || latest == "" {
                        fmt.Printf("%sNo existing semver-like tag found (e.g., v1.2.3). Skipping tagging.%s\n", cli.ColorYellow, cli.ColorReset)
                        return nil
                    }
                    // Build candidate increments (Major, Minor, Patch)
                    majTag := formatTag(vPrefix, maj+1, 0, 0)
                    minTag := formatTag(vPrefix, maj, min+1, 0)
                    patTag := formatTag(vPrefix, maj, min, pat+1)
                    options := []string{
                        fmt.Sprintf("Major -> %s (from %s)", majTag, latest),
                        fmt.Sprintf("Minor -> %s (from %s)", minTag, latest),
                        fmt.Sprintf("Patch -> %s (from %s)", patTag, latest),
                    }
                    idx, err := promptSimpleSelect("Select version bump", options)
                    if err != nil {
                        // If selection canceled or failed, do nothing further
                        return nil
                    }
                    newTag := ""
                    switch idx {
                    case 0:
                        newTag = majTag
                    case 1:
                        newTag = minTag
                    case 2:
                        newTag = patTag
                    }
                    if newTag == "" {
                        return nil
                    }
                    // Create the tag
                    if err := createTag(newTag); err != nil {
                        fmt.Printf("%sFailed to create tag:%s %v\n", cli.ColorYellow, cli.ColorReset, err)
                        return nil
                    }
                    // Push the tag
                    if err := pushTag(newTag); err != nil {
                        fmt.Printf("%sFailed to push tag '%s':%s %v\n", cli.ColorYellow, newTag, cli.ColorReset, err)
                        return nil
                    }
                    fmt.Printf("%sPushed tag:%s %s\n", cli.ColorGreen, cli.ColorReset, newTag)
                }
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

// latestSemverTag returns the most recent semver-like tag (sorted by version),
// whether it used a 'v' prefix, and its numeric components.
func latestSemverTag() (string, bool, int, int, int, error) {
    out, err := gitOutput("tag", "-l", "--sort=-v:refname")
    if err != nil {
        return "", false, 0, 0, 0, err
    }
    lines := strings.Split(strings.TrimSpace(out), "\n")
    rx := regexp.MustCompile(`^(v)?(\d+)\.(\d+)\.(\d+)$`)
    for _, line := range lines {
        line = strings.TrimSpace(line)
        if line == "" {
            continue
        }
        m := rx.FindStringSubmatch(line)
        if m == nil {
            continue
        }
        vPref := m[1] == "v"
        maj, _ := strconv.Atoi(m[2])
        min, _ := strconv.Atoi(m[3])
        pat, _ := strconv.Atoi(m[4])
        return line, vPref, maj, min, pat, nil
    }
    return "", false, 0, 0, 0, fmt.Errorf("no semver-like tags found")
}

func formatTag(vPrefix bool, major, minor, patch int) string {
    if vPrefix {
        return fmt.Sprintf("v%d.%d.%d", major, minor, patch)
    }
    return fmt.Sprintf("%d.%d.%d", major, minor, patch)
}

func createTag(tag string) error {
    cmd := exec.Command("git", "tag", tag)
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    return cmd.Run()
}

func pushTag(tag string) error {
    // Try to detect upstream remote of current branch
    if up, err := gitOutput("rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}"); err == nil {
        up = strings.TrimSpace(up)
        if up != "" && strings.Contains(up, "/") {
            remote := strings.SplitN(up, "/", 2)[0]
            cmd := exec.Command("git", "push", remote, tag)
            cmd.Stdout = os.Stdout
            cmd.Stderr = os.Stderr
            return cmd.Run()
        }
    }
    // Fallback: resolve remote from branch config or remotes list
    cur, err := gitOutput("rev-parse", "--abbrev-ref", "HEAD")
    branch := strings.TrimSpace(cur)
    remote := ""
    if err == nil && branch != "" && branch != "HEAD" {
        if r, e := gitOutput("config", "--get", "branch."+branch+".remote"); e == nil {
            remote = strings.TrimSpace(r)
        }
    }
    if remote == "" {
        if rems, e := gitOutput("remote"); e == nil {
            for _, line := range strings.Split(strings.TrimSpace(rems), "\n") {
                line = strings.TrimSpace(line)
                if line == "" {
                    continue
                }
                if remote == "" {
                    remote = line
                }
                if line == "origin" {
                    remote = line
                    break
                }
            }
        }
    }
    if remote != "" {
        cmd := exec.Command("git", "push", remote, tag)
        cmd.Stdout = os.Stdout
        cmd.Stderr = os.Stderr
        return cmd.Run()
    }
    // Last resort: push all tags to default remote
    cmd := exec.Command("git", "push", "--tags")
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    return cmd.Run()
}

// promptSimpleSelect renders a minimal interactive selector (no multi-select/combine)
// with the same key bindings (1-9/0, arrows, j/k, Enter). Returns the selected index.
func promptSimpleSelect(title string, options []string) (int, error) {
    n := len(options)
    if n == 0 {
        return 0, fmt.Errorf("no options")
    }
    // If STDIN is not a TTY, fall back to simple prompt
    if fi, err := os.Stdin.Stat(); err == nil && (fi.Mode()&os.ModeCharDevice) == 0 {
        fmt.Printf("%s%s %s:%s\n", cli.ColorGray, cli.ColorBold, title, cli.ColorReset)
        for i := 0; i < n; i++ {
            fmt.Printf("  %s[%d]%s %s%s%s\n", cli.ColorYellow, i+1, cli.ColorReset, cli.ColorCyan, options[i], cli.ColorReset)
        }
        fmt.Printf("\n%s%s Choose %s[1-%d]%s %s[default: 1]%s: %s", cli.ColorBold, cli.IconPrompt, cli.ColorYellow, n, cli.ColorReset, cli.ColorDim, cli.ColorReset, cli.ColorCyan)
        var input string
        fmt.Scanln(&input)
        fmt.Printf("%s", cli.ColorReset)
        if input == "" {
            return 0, nil
        }
        if v, err := strconv.Atoi(input); err == nil && v >= 1 && v <= n {
            return v - 1, nil
        }
        return 0, nil
    }
    // Interactive TTY mode
    restore, err := enableCBreak()
    if err != nil {
        // Fallback to simple prompt
        fmt.Printf("%s%s %s:%s\n", cli.ColorGray, cli.ColorBold, title, cli.ColorReset)
        for i := 0; i < n; i++ {
            fmt.Printf("  %s[%d]%s %s%s%s\n", cli.ColorYellow, i+1, cli.ColorReset, cli.ColorCyan, options[i], cli.ColorReset)
        }
        fmt.Printf("\n%s%s Choose %s[1-%d]%s %s[default: 1]%s: %s", cli.ColorBold, cli.IconPrompt, cli.ColorYellow, n, cli.ColorReset, cli.ColorDim, cli.ColorReset, cli.ColorCyan)
        var input string
        fmt.Scanln(&input)
        fmt.Printf("%s", cli.ColorReset)
        if input == "" {
            return 0, nil
        }
        if v, err := strconv.Atoi(input); err == nil && v >= 1 && v <= n {
            return v - 1, nil
        }
        return 0, nil
    }
    defer restore()

    selected := 0
    render := func() {
        fmt.Printf("%s%s %s:%s\n", cli.ColorGray, cli.ColorBold, title, cli.ColorReset)
        for i := 0; i < n; i++ {
            prefix := "  "
            lineColorStart := cli.ColorCyan
            lineColorEnd := cli.ColorReset
            idxLabel := fmt.Sprintf("%d", i+1)
            if i == selected {
                prefix = fmt.Sprintf("%s> %s", cli.ColorYellow, cli.ColorReset)
                lineColorStart = cli.ColorGreen + cli.ColorBold
            }
            fmt.Printf("%s[%s] %s%s%s\n", prefix, idxLabel, lineColorStart, options[i], lineColorEnd)
        }
        fmt.Printf("%sUse ↑/↓ or j/k, numbers to pick, Enter to confirm.%s\n", cli.ColorDim, cli.ColorReset)
    }

    render()
    in := make([]byte, 3)
    backLines := n + 2
    moveUp := func(lines int) { if lines > 0 { fmt.Printf("\033[%dA", lines) } }
    clearLine := func() { fmt.Printf("\033[2K\r") }
    for {
        _, err := os.Stdin.Read(in[:1])
        if err != nil {
            break
        }
        b := in[0]
        if b == 0 {
            continue
        }
        switch b {
        case 3: // Ctrl+C
            return 0, fmt.Errorf("selection canceled")
        case '\r', '\n':
            return selected, nil
        case 'k':
            if selected > 0 { selected-- }
        case 'j':
            if selected < n-1 { selected++ }
        case 27: // ESC sequence
            os.Stdin.Read(in[1:2])
            if in[1] != '[' { continue }
            os.Stdin.Read(in[2:3])
            switch in[2] {
            case 'A':
                if selected > 0 { selected-- }
            case 'B':
                if selected < n-1 { selected++ }
            }
        default:
            if b >= '1' && b <= '9' {
                v := int(b - '0')
                if v >= 1 && v <= n { return v-1, nil }
            } else if b == '0' && n == 10 {
                return 9, nil
            }
        }
        // re-render
        moveUp(backLines)
        for i := 0; i < backLines; i++ {
            clearLine()
            if i < backLines-1 { fmt.Printf("\n") }
        }
        moveUp(backLines - 1)
        render()
    }
    return selected, nil
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
