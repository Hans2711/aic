//go:build linux

package commit

import (
	"errors"
	"testing"
)

func contains(s []string, v string) bool {
	for _, a := range s {
		if a == v {
			return true
		}
	}
	return false
}

func TestCopyToClipboardAttemptsLinuxTools(t *testing.T) {
	var attempted []string
	look := func(name string) (string, error) {
		attempted = append(attempted, name)
		return "", errors.New("not found")
	}
	run := func(name string, args ...string) error { return nil }
	tryClipboard("msg", look, run)
	if !contains(attempted, "xclip") && !contains(attempted, "wl-copy") {
		t.Fatalf("expected xclip or wl-copy to be attempted, got %v", attempted)
	}
}
