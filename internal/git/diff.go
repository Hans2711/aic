package git

import (
    "bytes"
    "errors"
    "fmt"
    "os/exec"
    "strings"
)

// StagedDiff returns the staged (cached) git diff using minimal unified output.
func StagedDiff() (string, error) {
	// First ensure we are inside a git work tree.
	if err := insideRepo(); err != nil {
		return "", err
	}

	variants := [][]string{
		{"diff", "--cached", "--minimal", "--unified=0", "--no-prefix", "--color=never"},
		{"diff", "--staged", "--minimal", "--unified=0", "--no-prefix", "--color=never"}, // alias
		{"diff", "--minimal", "--unified=0", "--no-prefix", "--color=never"},              // fallback (includes unstaged)
	}

	var lastErr error
	for i, args := range variants {
		cmd := exec.Command("git", args...)
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		if err := cmd.Run(); err != nil {
			// If first two (with cached/staged) fail due to unknown option, keep going.
			if strings.Contains(out.String(), "unknown option") && i < 2 {
				lastErr = fmt.Errorf("git diff variant failed (%v): %w: %s", args, err, out.String())
				continue
			}
			return "", fmt.Errorf("git diff failed (%v): %w: %s", args, err, out.String())
		}
		return out.String(), nil
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", errors.New("failed to obtain git diff")
}

func insideRepo() error {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("not a git repository (run 'git init'): %w: %s", err, out.String())
	}
	val := strings.TrimSpace(out.String())
	if val != "true" {
		return errors.New("not inside a git work tree")
	}
    return nil
}

// StagedFiles returns the list of files included in the staged diff.
// It prefers --cached/--staged and falls back to name-only without it (which may include unstaged).
func StagedFiles() ([]string, error) {
    if err := insideRepo(); err != nil {
        return nil, err
    }

    variants := [][]string{
        {"diff", "--name-only", "--cached"},
        {"diff", "--name-only", "--staged"}, // alias
        {"diff", "--name-only"},              // fallback (may include unstaged)
    }

    var outStr string
    for i, args := range variants {
        cmd := exec.Command("git", args...)
        var out bytes.Buffer
        cmd.Stdout = &out
        cmd.Stderr = &out
        if err := cmd.Run(); err != nil {
            if strings.Contains(out.String(), "unknown option") && i < 2 {
                // try next variant on unknown option
                continue
            }
            return nil, fmt.Errorf("git diff --name-only failed (%v): %w: %s", args, err, out.String())
        }
        outStr = out.String()
        break
    }
    if strings.TrimSpace(outStr) == "" {
        return []string{}, nil
    }
    lines := strings.Split(outStr, "\n")
    files := make([]string, 0, len(lines))
    for _, ln := range lines {
        ln = strings.TrimSpace(ln)
        if ln == "" {
            continue
        }
        files = append(files, ln)
    }
    return files, nil
}
