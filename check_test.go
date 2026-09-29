package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func findingIDs(findings []Finding) map[string]bool {
	ids := map[string]bool{}
	for _, f := range findings {
		ids[f.RuleID] = true
	}
	return ids
}

func TestCheckCommitMessageReportsSubjectBodyAndTrailer(t *testing.T) {
	rules := selectRules(embeddedRuleSet(t), false)
	path := writeTemp(t, "COMMIT_EDITMSG", "feat: ABC-12 - expoe o motivo\n\nexplicacao longa\n\nCo-Authored-By: Someone <a@b.c>\n")

	findings, err := checkCommitMessage(rules, path)
	if err != nil {
		t.Fatal(err)
	}

	ids := findingIDs(findings)
	for _, want := range []string{
		"commit-subject-uppercase-after-prefix",
		"commit-body-not-empty",
		"commit-coauthored-by",
	} {
		if !ids[want] {
			t.Errorf("esperava %s, veio %v", want, ids)
		}
	}
}

func TestCheckCommitMessageAcceptsAValidSingleLineSubject(t *testing.T) {
	rules := selectRules(embeddedRuleSet(t), false)
	path := writeTemp(t, "COMMIT_EDITMSG", "feat(api-example): expoe o motivo da falha (ABC-12)\n# Please enter the commit message\n")

	findings, err := checkCommitMessage(rules, path)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Errorf("esperava limpo, veio %v", findingIDs(findings))
	}
}

func TestCheckCommitMessageFailsOnMissingFile(t *testing.T) {
	rules := selectRules(embeddedRuleSet(t), false)
	if _, err := checkCommitMessage(rules, filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Error("esperava erro para arquivo inexistente")
	}
}

func TestCheckFilesFlagsFocusedTestsAndSkipsUnreadableInputs(t *testing.T) {
	rules := selectRules(embeddedRuleSet(t), false)
	spec := writeTemp(t, "thing.spec.ts", "describe('x', () => {\n  it.only('y', () => {})\n})\n")
	clean := writeTemp(t, "clean.spec.ts", "describe('x', () => {\n  it('y', () => {})\n})\n")
	missing := filepath.Join(t.TempDir(), "missing.spec.ts")

	findings, err := checkFiles(rules, []string{spec, clean, missing, t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}

	if len(findings) != 1 || findings[0].RuleID != "spec-focused-test" {
		t.Fatalf("esperava so spec-focused-test, veio %+v", findings)
	}
	if findings[0].Line != 2 {
		t.Errorf("linha errada: %d", findings[0].Line)
	}
	if findings[0].Path != spec {
		t.Errorf("path errado: %q", findings[0].Path)
	}
}

func TestCheckFilesSkipsFilesOverTheSizeLimit(t *testing.T) {
	rules := selectRules(embeddedRuleSet(t), false)
	big := writeTemp(t, "big.spec.ts", strings.Repeat(" ", maxFileBytes)+"it.only('x', () => {})\n")

	findings, err := checkFiles(rules, []string{big})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Errorf("arquivo acima do limite deve ser ignorado, veio %v", findingIDs(findings))
	}
}

func TestRenderFindingsIncludesLocationRuleAndExcerpt(t *testing.T) {
	var out strings.Builder
	renderFindings(&out, []Finding{{
		Path: "a.ts", Line: 3, RuleID: "some-rule", Severity: "high", Message: "consequencia", Excerpt: "it.only(",
	}})

	got := out.String()
	for _, want := range []string{"a.ts:3", "[high] some-rule", "consequencia", "> it.only("} {
		if !strings.Contains(got, want) {
			t.Errorf("saida sem %q:\n%s", want, got)
		}
	}
}

func TestHomeDirFallsBackWhenHomeIsUnset(t *testing.T) {
	t.Setenv("HOME", "")
	if homeDir() == "" && os.Getenv("USERPROFILE") != "" {
		t.Error("homeDir deveria cair para os.UserHomeDir quando HOME esta vazio")
	}

	t.Setenv("HOME", "/custom/home")
	if got := homeDir(); got != "/custom/home" {
		t.Errorf("HOME explicito deve ter prioridade, veio %q", got)
	}
}
