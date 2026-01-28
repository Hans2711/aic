package commit

import (
	"strings"
	"unicode"
)

// WrapText wraps text at the specified width without breaking words.
// It preserves existing line breaks and only adds new ones when needed.
func WrapText(text string, width int) string {
	if width <= 0 {
		return text
	}

	lines := strings.Split(text, "\n")
	var result []string

	for _, line := range lines {
		if len(line) <= width {
			result = append(result, line)
			continue
		}

		wrapped := wrapLine(line, width)
		result = append(result, wrapped...)
	}

	return strings.Join(result, "\n")
}

// wrapLine wraps a single line at the specified width without breaking words.
func wrapLine(line string, width int) []string {
	if width <= 0 {
		return []string{line}
	}

	words := strings.FieldsFunc(line, unicode.IsSpace)
	if len(words) == 0 {
		return []string{line}
	}

	var result []string
	var currentLine strings.Builder

	for i, word := range words {
		// If this is the first word or adding it won't exceed the width
		if currentLine.Len() == 0 {
			currentLine.WriteString(word)
		} else if currentLine.Len()+1+len(word) <= width {
			currentLine.WriteString(" ")
			currentLine.WriteString(word)
		} else {
			// Current line is full, start a new one
			result = append(result, currentLine.String())
			currentLine.Reset()
			currentLine.WriteString(word)
		}
	}

	// Add the last line if it has content
	if currentLine.Len() > 0 {
		result = append(result, currentLine.String())
	}

	return result
}

// WrapCommitMessage wraps a commit message at 80 characters, applying the wrapping
// intelligently to both single-line and multi-line commit messages.
func WrapCommitMessage(msg string) string {
	return WrapText(msg, 80)
}