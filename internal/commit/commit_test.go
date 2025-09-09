package commit

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestBuildTagMessage verifies that commit subjects between tags are collected.
func TestBuildTagMessage(t *testing.T) {
	dir := t.TempDir()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer os.Chdir(orig)
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	run := func(cmd string, args ...string) {
		c := exec.Command(cmd, args...)
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("%s %v: %v\n%s", cmd, args, err, out)
		}
	}
	run("git", "init")
	run("git", "config", "user.email", "test@example.com")
	run("git", "config", "user.name", "test")
	if err := os.WriteFile("file.txt", []byte("first"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	run("git", "add", "file.txt")
	run("git", "commit", "-m", "first commit")
	run("git", "tag", "v1.0.0")
	if err := os.WriteFile("file.txt", []byte("second"), 0644); err != nil {
		t.Fatalf("write file2: %v", err)
	}
	run("git", "add", "file.txt")
	run("git", "commit", "-m", "second commit")
	msg, err := buildTagMessage("v1.0.0")
	if err != nil {
		t.Fatalf("buildTagMessage: %v", err)
	}
	if !strings.Contains(msg, "- second commit") {
		t.Fatalf("expected bulletpoint for second commit, got %q", msg)
	}
}
