package version

import "testing"

func TestGetReturnsVersion(t *testing.T) {
    if Get() != Version {
        t.Fatalf("Get() should return Version (%q), got %q", Version, Get())
    }
}

