package commit

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/diesi/aic/internal/cli"
)

type secretPattern struct {
	name string
	re   *regexp.Regexp
}

var secretPatterns = []secretPattern{
	{"OpenAI API key", regexp.MustCompile(`sk-[A-Za-z0-9]{16,}`)},
	{"AWS Access Key", regexp.MustCompile(`AKIA[0-9A-Z]{16}`)},
	{"Password assignment", regexp.MustCompile(`(?i)password\s*[:=]\s*\S`)},
	{"Secret assignment", regexp.MustCompile(`(?i)secret\s*[:=]\s*\S`)},
	{"Private key block", regexp.MustCompile(`-----BEGIN [A-Z ]+PRIVATE KEY-----`)},
}

func detectSecrets(diff string) []string {
	matches := []string{}
	for _, p := range secretPatterns {
		if p.re.MatchString(diff) {
			matches = append(matches, p.name)
		}
	}
	return matches
}

// WarnIfSecrets prints a warning to stderr if the provided diff appears
// to contain secrets. It's exported so the CLI can emit the warning before
// starting any spinners to avoid output glitches.
func WarnIfSecrets(diff string) {
    secrets := detectSecrets(diff)
    if len(secrets) > 0 {
        fmt.Fprintf(os.Stderr, "%s%s WARNING: possible secrets detected (%s)%s\n", cli.ColorRed, cli.ColorBold, strings.Join(secrets, ", "), cli.ColorReset)
    }
}
