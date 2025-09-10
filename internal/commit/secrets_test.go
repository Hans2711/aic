package commit

import "testing"

func TestDetectSecrets(t *testing.T) {
	diff := "+ api_key=sk-1234567890abcdef1234567890abcdef\n"
	matches := detectSecrets(diff)
	if len(matches) == 0 {
		t.Fatalf("expected secret detection, got none")
	}
	safe := "+ fmt.Println(\"hello\")\n"
	if m := detectSecrets(safe); len(m) != 0 {
		t.Fatalf("unexpected secret detection: %v", m)
	}
}
