package releaseinfo

import (
	"bufio"
	_ "embed"
	"fmt"
	"strings"
)

const versionKey = "VERSION"

//go:embed release.env
var rawRelease string

var metadata = parseReleaseEnv(rawRelease)

func parseReleaseEnv(data string) map[string]string {
	values := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.IndexRune(line, '=')
		if idx <= 0 {
			panic(fmt.Sprintf("releaseinfo: invalid line %q", line))
		}
		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])
		if key == "" || value == "" {
			panic(fmt.Sprintf("releaseinfo: invalid line %q", line))
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		panic(fmt.Sprintf("releaseinfo: scan error: %v", err))
	}
	return values
}

func valueFor(key string) string {
	value, ok := metadata[key]
	if !ok || value == "" {
		panic(fmt.Sprintf("releaseinfo: %s not set", key))
	}
	return value
}

// Version returns the release version string.
func Version() string {
	return valueFor(versionKey)
}

// Raw returns the contents of release.env for tooling that needs to parse it directly.
func Raw() string {
	return rawRelease
}
