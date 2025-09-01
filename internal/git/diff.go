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
