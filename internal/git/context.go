package git

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"strconv"
	"strings"
)

// RepoContext gathers branch name, PR title, and linked issue descriptions.
// It attempts to use the `gh` CLI when available. Failures are ignored.
func RepoContext() string {
	branch := currentBranch()
	prTitle, issues := prInfo()
	return formatContext(branch, prTitle, issues)
}

// formatContext builds a human-readable context string from parts.
func formatContext(branch, prTitle string, issues []string) string {
	parts := []string{}
	if strings.TrimSpace(branch) != "" {
		parts = append(parts, "Branch: "+strings.TrimSpace(branch))
	}
	if strings.TrimSpace(prTitle) != "" {
		parts = append(parts, "PR Title: "+strings.TrimSpace(prTitle))
	}
	for _, is := range issues {
		is = strings.TrimSpace(is)
		if is != "" {
			parts = append(parts, "Issue: "+is)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n")
}

func currentBranch() string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return ""
	}
	return strings.TrimSpace(out.String())
}

func prInfo() (string, []string) {
	cmd := exec.Command("gh", "pr", "view", "--json", "title,closingIssuesReferences")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", nil
	}
	var data struct {
		Title                   string `json:"title"`
		ClosingIssuesReferences []struct {
			Number int `json:"number"`
		} `json:"closingIssuesReferences"`
	}
	if err := json.Unmarshal(out.Bytes(), &data); err != nil {
		return strings.TrimSpace(out.String()), nil
	}
	issues := []string{}
	for _, ref := range data.ClosingIssuesReferences {
		if ref.Number <= 0 {
			continue
		}
		if txt := issueText(ref.Number); txt != "" {
			issues = append(issues, txt)
		}
	}
	return strings.TrimSpace(data.Title), issues
}

func issueText(num int) string {
	cmd := exec.Command("gh", "issue", "view", strconv.Itoa(num), "--json", "title,body")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return ""
	}
	var data struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	if err := json.Unmarshal(out.Bytes(), &data); err != nil {
		return ""
	}
	body := strings.TrimSpace(data.Body)
	r := []rune(body)
	if len(r) > 200 {
		body = string(r[:200])
	}
	return strings.TrimSpace(data.Title + ": " + body)
}
