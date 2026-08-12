package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	TargetFile          = "file"
	TargetCommitSubject = "commit-subject"
	TargetCommitBody    = "commit-body"
	TargetCommitMessage = "commit-message"

	ModeFlagIfMatch   = "flag_if_match"
	ModeFlagIfNoMatch = "flag_if_no_match"

	StatusActive    = "active"
	StatusDemoted   = "demoted"
	StatusCandidate = "candidate"
)

type Rule struct {
	ID       string   `json:"id"`
	Target   string   `json:"target"`
	Repos    []string `json:"repos,omitempty"`
	Paths    []string `json:"paths,omitempty"`
	Pattern  string   `json:"pattern"`
	Mode     string   `json:"mode"`
	Severity string   `json:"severity"`
	Message  string   `json:"message"`
	Source   string   `json:"source"`
	Status   string   `json:"status"`

	re *regexp.Regexp
}

func (r *Rule) compile() error {
	re, err := regexp.Compile(r.Pattern)
	if err != nil {
		return fmt.Errorf("rule %s: invalid pattern: %w", r.ID, err)
	}
	r.re = re
	return nil
}

func (r *Rule) active() bool {
	return r.Status == StatusActive
}

func selectRules(rules []*Rule, includeCandidates bool) []*Rule {
	out := make([]*Rule, 0, len(rules))
	for _, r := range rules {
		if r.active() || (includeCandidates && r.Status == StatusCandidate) {
			out = append(out, r)
		}
	}
	return out
}

func (r *Rule) appliesToRepo(repo string) bool {
	if len(r.Repos) == 0 {
		return true
	}
	for _, want := range r.Repos {
		if want == "*" || want == repo {
			return true
		}
	}
	return false
}

func (r *Rule) appliesToPath(path string) bool {
	if len(r.Paths) == 0 {
		return true
	}
	base := filepath.Base(path)
	for _, pat := range r.Paths {
		clean := strings.TrimPrefix(pat, "**/")
		if ok, _ := filepath.Match(clean, base); ok {
			return true
		}
		if ok, _ := filepath.Match(pat, path); ok {
			return true
		}
	}
	return false
}

func loadRules() ([]*Rule, error) {
	byID := map[string]*Rule{}

	var embedded []*Rule
	if err := json.Unmarshal(embeddedRules, &embedded); err != nil {
		return nil, fmt.Errorf("embedded rules: %w", err)
	}
	for _, r := range embedded {
		byID[r.ID] = r
	}

	overridePath := filepath.Join(stateDir(), "rules.json")
	if raw, err := os.ReadFile(overridePath); err == nil {
		var user []*Rule
		if err := json.Unmarshal(raw, &user); err != nil {
			return nil, fmt.Errorf("%s: %w", overridePath, err)
		}
		for _, r := range user {
			byID[r.ID] = r
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	out := make([]*Rule, 0, len(byID))
	for _, r := range byID {
		if r.Status == "" {
			r.Status = StatusActive
		}
		if r.Mode == "" {
			r.Mode = ModeFlagIfMatch
		}
		if err := r.compile(); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func repoOf(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	dir := abs
	if st, err := os.Stat(abs); err == nil && !st.IsDir() {
		dir = filepath.Dir(abs)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return filepath.Base(dir)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
