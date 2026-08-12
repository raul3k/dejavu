package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	maxFileBytes      = 2 << 20
	maxMatchesPerRule = 20
)

type Finding struct {
	Path     string `json:"path"`
	Line     int    `json:"line"`
	RuleID   string `json:"rule_id"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Excerpt  string `json:"excerpt,omitempty"`
}

func lineOf(content []byte, offset int) int {
	return bytes.Count(content[:offset], []byte("\n")) + 1
}

func excerptAt(content []byte, offset int) string {
	start := bytes.LastIndexByte(content[:offset], '\n') + 1
	end := bytes.IndexByte(content[offset:], '\n')
	if end < 0 {
		end = len(content)
	} else {
		end += offset
	}
	return strings.TrimSpace(string(content[start:end]))
}

func checkContent(rules []*Rule, target, path, repo string, content []byte) []Finding {
	var out []Finding
	for _, r := range rules {
		if r.Target != target {
			continue
		}
		if target == TargetFile && (!r.appliesToRepo(repo) || !r.appliesToPath(path)) {
			continue
		}
		switch r.Mode {
		case ModeFlagIfMatch:
			for _, loc := range r.re.FindAllIndex(content, maxMatchesPerRule) {
				out = append(out, Finding{
					Path:     path,
					Line:     lineOf(content, loc[0]),
					RuleID:   r.ID,
					Severity: r.Severity,
					Message:  r.Message,
					Excerpt:  excerptAt(content, loc[0]),
				})
			}
		case ModeFlagIfNoMatch:
			if r.re.Match(content) {
				continue
			}
			out = append(out, Finding{
				Path:     path,
				Line:     1,
				RuleID:   r.ID,
				Severity: r.Severity,
				Message:  r.Message,
			})
		}
	}
	return out
}

func checkFiles(rules []*Rule, paths []string) ([]Finding, error) {
	var out []Finding
	for _, p := range paths {
		st, err := os.Stat(p)
		if err != nil || st.IsDir() || st.Size() > maxFileBytes {
			continue
		}
		content, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		out = append(out, checkContent(rules, TargetFile, p, repoOf(p), content)...)
	}
	return out, nil
}

func splitCommitMessage(raw []byte) (subject, body string) {
	var kept []string
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		kept = append(kept, line)
	}
	text := strings.TrimLeft(strings.Join(kept, "\n"), "\n")
	parts := strings.SplitN(text, "\n", 2)
	subject = strings.TrimRight(parts[0], " \t\r")
	if len(parts) > 1 {
		body = strings.TrimSpace(parts[1])
	}
	return subject, body
}

func checkCommitMessage(rules []*Rule, path string) ([]Finding, error) {
	var raw []byte
	var err error
	if path == "-" {
		raw, err = io.ReadAll(os.Stdin)
	} else {
		raw, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, err
	}
	subject, body := splitCommitMessage(raw)
	label := path
	if label == "-" {
		label = "commit-msg"
	}

	var out []Finding
	out = append(out, checkContent(rules, TargetCommitSubject, label, "", []byte(subject))...)
	out = append(out, checkContent(rules, TargetCommitBody, label, "", []byte(body))...)
	out = append(out, checkContent(rules, TargetCommitMessage, label, "", raw)...)
	return out, nil
}

type hookPayload struct {
	ToolInput struct {
		FilePath string `json:"file_path"`
	} `json:"tool_input"`
}

func pathFromHookPayload(r io.Reader) (string, error) {
	raw, err := io.ReadAll(io.LimitReader(r, 1<<20))
	if err != nil {
		return "", err
	}
	var p hookPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", err
	}
	return p.ToolInput.FilePath, nil
}

func renderFindings(w io.Writer, findings []Finding) {
	for _, f := range findings {
		fmt.Fprintf(w, "%s:%d  [%s] %s\n    %s\n", f.Path, f.Line, f.Severity, f.RuleID, f.Message)
		if f.Excerpt != "" {
			fmt.Fprintf(w, "    > %s\n", f.Excerpt)
		}
	}
}
