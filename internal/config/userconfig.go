package config

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// UserConfig represents optional global configuration loaded from ~/.aic.json
// Currently supports:
//   - instructions: string appended to AI system prompts (team style presets)
type UserConfig struct {
	Instructions string `json:"instructions"`
}

// LoadUserConfig reads ~/.aic.json if present. Returns zero-value on any error.
// Errors are non-fatal; a short note is printed to stderr to assist debugging.
func LoadUserConfig() UserConfig {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return UserConfig{}
	}
	path := filepath.Join(home, ".aic.json")
	data, err := os.ReadFile(path)
	if err != nil {
		// Not present is fine; no noise.
		return UserConfig{}
	}
	var uc UserConfig
	if err := json.Unmarshal(data, &uc); err != nil {
		// Non-fatal parse issue; hint the user once.
		fmt.Fprintf(os.Stderr, "[aic] warning: cannot parse %s: %v\n", path, err)
		return UserConfig{}
	}
	uc.Instructions = strings.TrimSpace(uc.Instructions)
	return uc
}

// LoadRepoConfig reads .aic.json from the current Git repo root if present.
// Returns zero-value on any error.
func LoadRepoConfig() UserConfig {
    // Allow disabling repo-local config via env (useful for tests/CI)
    if Bool(EnvAICDisableRepoConfig) {
        return UserConfig{}
    }
    root := repoRoot()
    if root == "" {
        return UserConfig{}
    }
	path := filepath.Join(root, ".aic.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return UserConfig{}
	}
	var uc UserConfig
	if err := json.Unmarshal(data, &uc); err != nil {
		fmt.Fprintf(os.Stderr, "[aic] warning: cannot parse %s: %v\n", path, err)
		return UserConfig{}
	}
	uc.Instructions = strings.TrimSpace(uc.Instructions)
	return uc
}

// SaveRepoInstructions writes or updates .aic.json in the repo root with the provided instructions.
// If the file exists and is valid JSON, only the 'instructions' field is updated.
func SaveRepoInstructions(instructions string) error {
	root := repoRoot()
	if root == "" {
		return fmt.Errorf("not a git repository; cannot locate repo root")
	}
	path := filepath.Join(root, ".aic.json")
	instructions = strings.TrimSpace(instructions)
	existing := UserConfig{}
	if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
		_ = json.Unmarshal(b, &existing) // best-effort; ignore errors
	}
	existing.Instructions = instructions
	out, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}

// repoRoot returns the absolute path to the current repo's top-level directory, or "" if not in a repo.
func repoRoot() string {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
