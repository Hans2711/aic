package config

import (
    "encoding/json"
    "fmt"
    "os"
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

