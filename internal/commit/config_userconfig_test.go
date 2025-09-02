package commit

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_MergesUserInstructionsAndCLI(t *testing.T) {
    td := t.TempDir()
    t.Setenv("AIC_DISABLE_REPO_CONFIG", "1")
    t.Setenv("HOME", td)

	// Create ~/.aic.json with instructions
	cfgPath := filepath.Join(td, ".aic.json")
	contents := `{"instructions": "global style prefs"}`
	if err := os.WriteFile(cfgPath, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", cfgPath, err)
	}

	// Merge with CLI addition
	cfg, err := LoadConfig("and cli hint")
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if got, want := cfg.SystemAddition, "global style prefs and cli hint"; got != want {
		t.Fatalf("merge mismatch: got %q want %q", got, want)
	}

	// When CLI addition is empty, should use only global
	cfg2, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if got, want := cfg2.SystemAddition, "global style prefs"; got != want {
		t.Fatalf("want only global instructions; got %q", got)
	}
}

func TestLoadConfig_NoUserConfig_UsesCLIOnly(t *testing.T) {
    td := t.TempDir()
    t.Setenv("AIC_DISABLE_REPO_CONFIG", "1")
    t.Setenv("HOME", td) // no .aic.json

	cfg, err := LoadConfig("cli only")
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if got, want := cfg.SystemAddition, "cli only"; got != want {
		t.Fatalf("expected CLI-only instructions; got %q", got)
	}
}
