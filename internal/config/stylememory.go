package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type repoStyle struct {
	Instructions string    `json:"instructions"`
	Embedding    []float64 `json:"embedding,omitempty"`
}

type styleMemory struct {
	Repos map[string]repoStyle `json:"repos"`
}

func styleMemoryPath() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ""
	}
	return filepath.Join(home, ".aic-styles.json")
}

// LoadRepoStyle returns the stored style instructions for the current repository, if any.
func LoadRepoStyle() UserConfig {
	root := repoRoot()
	if root == "" {
		return UserConfig{}
	}
	path := styleMemoryPath()
	if path == "" {
		return UserConfig{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return UserConfig{}
	}
	var sm styleMemory
	if err := json.Unmarshal(data, &sm); err != nil {
		return UserConfig{}
	}
	if sm.Repos == nil {
		return UserConfig{}
	}
	rs, ok := sm.Repos[root]
	if !ok {
		return UserConfig{}
	}
	return UserConfig{Instructions: strings.TrimSpace(rs.Instructions)}
}

// SaveRepoStyle stores instructions and optional embedding for the current repo in the style memory file.
func SaveRepoStyle(instructions string, embedding []float64) error {
	root := repoRoot()
	if root == "" {
		return fmt.Errorf("not a git repository; cannot locate repo root")
	}
	path := styleMemoryPath()
	if path == "" {
		return fmt.Errorf("cannot determine home directory")
	}
	instructions = strings.TrimSpace(instructions)
	var sm styleMemory
	if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
		_ = json.Unmarshal(b, &sm) // best effort
	}
	if sm.Repos == nil {
		sm.Repos = map[string]repoStyle{}
	}
	sm.Repos[root] = repoStyle{Instructions: instructions, Embedding: embedding}
	out, err := json.MarshalIndent(sm, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}
