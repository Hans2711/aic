package cli

import "strings"

// StripLeadingListMarker removes leading numbering or bullet from a line.
func StripLeadingListMarker(s string) string {
	orig := s
	s = strings.TrimLeft(s, " ")
	for i := 0; i < 2; i++ {
		if len(s) == 0 { break }
		if idx := strings.IndexAny(s, ".:)])> \t-"); idx != -1 {
			p := s[:idx+1]
			if isListMarker(p) { s = strings.TrimSpace(s[idx+1:]); continue }
		}
		if strings.HasPrefix(s, "-") || strings.HasPrefix(s, "*") || strings.HasPrefix(s, "+") {
			s = strings.TrimSpace(s[1:])
			continue
		}
		break
	}
	if s == "" { return orig }
	return s
}

func isListMarker(p string) bool {
	p = strings.TrimSpace(p)
	if p == "" { return false }
	if len(p) <= 4 {
		hasDigit := false
		for _, r := range p { if r >= '0' && r <= '9' { hasDigit = true } }
		if hasDigit && strings.ContainsAny(p, ".:)])") { return true }
		if hasDigit && (strings.HasSuffix(p, ":") || strings.HasSuffix(p, "-")) { return true }
	}
	return false
}
