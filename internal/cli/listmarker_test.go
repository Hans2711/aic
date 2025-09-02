package cli

import "testing"

func TestStripLeadingListMarker(t *testing.T) {
	cases := []struct{ in, want string }{
		{"1. do thing", "do thing"},
		{"  2) do thing", "do thing"},
		{"- bullet item", "bullet item"},
		{"* bullet item", "bullet item"},
		{"+ bullet item", "bullet item"},
		{"(3) wrapped number", "wrapped number"},
		{"i) roman not number", "i) roman not number"},
		{"no marker here", "no marker here"},
		{"", ""},
		{"   -   spaced bullet  ", "spaced bullet"},
	}
	for _, c := range cases {
		got := StripLeadingListMarker(c.in)
		if got != c.want {
			t.Fatalf("input %q: want %q, got %q", c.in, c.want, got)
		}
	}
}
