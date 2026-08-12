package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func embeddedRuleSet(t *testing.T) []*Rule {
	t.Helper()
	var rules []*Rule
	if err := json.Unmarshal(embeddedRules, &rules); err != nil {
		t.Fatalf("data/rules.json invalido: %v", err)
	}
	for _, r := range rules {
		if err := r.compile(); err != nil {
			t.Fatalf("regra %s nao compila: %v", r.ID, err)
		}
	}
	return rules
}

func ruleByID(t *testing.T, id string) *Rule {
	t.Helper()
	for _, r := range embeddedRuleSet(t) {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf("regra ausente: %s", id)
	return nil
}

func TestEveryEmbeddedRuleIsWellFormed(t *testing.T) {
	for _, r := range embeddedRuleSet(t) {
		if r.ID == "" || r.Pattern == "" || r.Message == "" || r.Source == "" {
			t.Errorf("regra incompleta: %+v", r.ID)
		}
		switch r.Target {
		case TargetFile, TargetCommitSubject, TargetCommitBody, TargetCommitMessage:
		default:
			t.Errorf("regra %s: target invalido %q", r.ID, r.Target)
		}
		switch r.Mode {
		case ModeFlagIfMatch, ModeFlagIfNoMatch:
		default:
			t.Errorf("regra %s: mode invalido %q", r.ID, r.Mode)
		}
		switch r.Status {
		case StatusActive, StatusCandidate, StatusDemoted:
		default:
			t.Errorf("regra %s: status invalido %q", r.ID, r.Status)
		}
	}
}

func TestCommitSubjectRulesMatchTheRealCIFailure(t *testing.T) {
	rules := embeddedRuleSet(t)
	cases := []struct {
		name    string
		subject string
		wantIDs []string
	}{
		{
			name:    "codigo da tarefa abrindo o subject quebra subject-case",
			subject: "feat: ABC-12 - expoe o motivo da falha",
			wantIDs: []string{"commit-subject-uppercase-after-prefix"},
		},
		{
			name:    "subject correto com ticket no fim",
			subject: "feat(api-example): expoe o motivo da falha na timeline (ABC-12)",
			wantIDs: nil,
		},
		{
			name:    "sem prefixo conventional",
			subject: "ajusta o timeout do worker",
			wantIDs: []string{"commit-subject-missing-conventional-prefix"},
		},
		{
			name:    "revert e prefixo valido",
			subject: "revert: remove o filtro de cidade (ABC-9)",
			wantIDs: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := checkContent(rules, TargetCommitSubject, "msg", "", []byte(tc.subject))
			ids := map[string]bool{}
			for _, f := range got {
				ids[f.RuleID] = true
			}
			for _, want := range tc.wantIDs {
				if !ids[want] {
					t.Errorf("esperava %s, veio %v", want, ids)
				}
			}
			if tc.wantIDs == nil && len(got) > 0 {
				t.Errorf("esperava limpo, veio %v", ids)
			}
		})
	}
}

func TestCommitSubjectTooLongFiresExactlyAtTheLimit(t *testing.T) {
	r := ruleByID(t, "commit-subject-too-long")
	at100 := "feat: " + string(make([]byte, 0))
	for len(at100) < 100 {
		at100 += "a"
	}
	if r.re.Match([]byte(at100)) {
		t.Errorf("100 chars nao deve disparar")
	}
	if !r.re.Match([]byte(at100 + "a")) {
		t.Errorf("101 chars deve disparar")
	}
}

func TestCommitBodyIsSeparatedAndCommentsStripped(t *testing.T) {
	raw := []byte("feat: adiciona coisa\n\n# comentario do git\nCorpo real aqui\n")
	subject, body := splitCommitMessage(raw)
	if subject != "feat: adiciona coisa" {
		t.Errorf("subject errado: %q", subject)
	}
	if body != "Corpo real aqui" {
		t.Errorf("body errado: %q", body)
	}
}

func TestCommitWithOnlyGitCommentsHasEmptyBody(t *testing.T) {
	raw := []byte("fix: corrige o retry\n\n# Please enter the commit message\n# On branch abc-9\n")
	_, body := splitCommitMessage(raw)
	if body != "" {
		t.Errorf("comentario do git nao pode virar body: %q", body)
	}
}

func TestPathScopingKeepsSpecRulesOutOfProductionCode(t *testing.T) {
	r := ruleByID(t, "spec-weak-called-assertion")
	if r.appliesToPath("apps/web/src/user.service.ts") {
		t.Error("regra de spec nao pode valer para codigo de producao")
	}
	if !r.appliesToPath("apps/web/src/user.service.spec.ts") {
		t.Error("regra de spec deve valer para o spec")
	}
}

func TestRepoScopingIsolatesRepoSpecificRules(t *testing.T) {
	r := ruleByID(t, "web-refetch-on-mount-always")
	if r.appliesToRepo("api") {
		t.Error("regra com escopo nao pode valer para api")
	}
	if !r.appliesToRepo("web") {
		t.Error("regra com escopo deve valer para o repo declarado")
	}
}

func TestWeakAssertionRuleDistinguishesTimesVariant(t *testing.T) {
	r := ruleByID(t, "spec-weak-called-assertion")
	if !r.re.MatchString("expect(spy).toHaveBeenCalled();") {
		t.Error("toHaveBeenCalled() deve disparar")
	}
	if r.re.MatchString("expect(spy).toHaveBeenCalledTimes(1);") {
		t.Error("toHaveBeenCalledTimes(1) nao pode disparar")
	}
	if r.re.MatchString("expect(spy).toHaveBeenCalledWith(id);") {
		t.Error("toHaveBeenCalledWith nao pode disparar")
	}
}

func TestBlindDataCastOnlyFiresOnResponseReceiver(t *testing.T) {
	r := ruleByID(t, "web-blind-data-cast")
	if !r.re.MatchString("const u = response.data as UserProfile;") {
		t.Error("body de resposta HTTP deve disparar")
	}
	if r.re.MatchString("const c = route.snapshot.data as ProfileListConfig;") {
		t.Error("route.snapshot.data nao e body HTTP e nao pode disparar")
	}
}

func TestSelectRulesKeepsCandidatesOutOfTheHook(t *testing.T) {
	rules := embeddedRuleSet(t)
	hook := selectRules(rules, false)
	for _, r := range hook {
		if r.Status == StatusCandidate {
			t.Errorf("regra candidate %s vazou para o conjunto do hook", r.ID)
		}
	}
	if len(selectRules(rules, true)) <= len(hook) {
		t.Error("--candidates deve incluir mais regras que o hook")
	}
}

func embeddedVocabulary(t *testing.T) Vocabulary {
	t.Helper()
	v := Vocabulary{}
	if err := json.Unmarshal(embeddedClasses, &v); err != nil {
		t.Fatalf("data/classes.json invalido: %v", err)
	}
	return v
}

func TestAliasResolvesToCanonicalClass(t *testing.T) {
	v := embeddedVocabulary(t)
	if got := v.resolve("non-transactional-dual-write"); got != "non-atomic-multi-write" {
		t.Errorf("alias nao resolveu: %q", got)
	}
	if got := v.resolve("non-atomic-multi-write"); got != "non-atomic-multi-write" {
		t.Errorf("canonica deve resolver para si mesma: %q", got)
	}
	if got := v.resolve("slug-que-nunca-existiu"); got != "" {
		t.Errorf("slug desconhecido deve resolver vazio: %q", got)
	}
}

func TestNoSlugIsClaimedByTwoClasses(t *testing.T) {
	v := embeddedVocabulary(t)
	owner := map[string]string{}
	for name, c := range v {
		for _, a := range c.Aliases {
			if prev, dup := owner[a]; dup {
				t.Errorf("alias %q reivindicado por %q e %q", a, prev, name)
			}
			owner[a] = name
			if _, isCanonical := v[a]; isCanonical {
				t.Errorf("alias %q tambem e classe canonica", a)
			}
		}
	}
}

func TestEveryClassInTheRealLedgerResolves(t *testing.T) {
	dir := ledgerDir()
	if _, err := os.Stat(dir); err != nil {
		t.Skip("ledger indisponivel neste ambiente")
	}
	counts, err := scanLedgerClasses()
	if err != nil {
		t.Fatalf("erro lendo ledger: %v", err)
	}
	if len(counts) == 0 {
		t.Skip("ledger vazio")
	}
	v := embeddedVocabulary(t)
	for slug := range counts {
		if v.resolve(slug) == "" {
			t.Errorf("classe do ledger fora do vocabulario: %s", slug)
		}
	}
}

func TestEnvKeyExtractionHandlesDotenvAndYaml(t *testing.T) {
	dotenv := []byte("# comentario\nexport API_URL=https://x\nDB_HOST=localhost\n")
	got := extractKeys("stage.env", dotenv)
	for _, want := range []string{"API_URL", "DB_HOST"} {
		if !got[want] {
			t.Errorf("chave dotenv nao extraida: %s", want)
		}
	}

	yaml := []byte("env:\n  API_URL: https://x\n  DB_HOST: localhost\n")
	gotYaml := extractKeys("values-prod.yaml", yaml)
	for _, want := range []string{"API_URL", "DB_HOST"} {
		if !gotYaml[want] {
			t.Errorf("chave yaml nao extraida: %s", want)
		}
	}
}

func TestEnvParityDetectsKeyPresentInOnlyOneEnvironment(t *testing.T) {
	dir := t.TempDir()
	stage := filepath.Join(dir, "stage.env")
	prod := filepath.Join(dir, "prod.env")
	if err := os.WriteFile(stage, []byte("API_URL=a\nFEATURE_X=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(prod, []byte("API_URL=b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := cmdEnvParity([]string{stage, prod}, false); code != 2 {
		t.Errorf("assimetria deve sair com 2, veio %d", code)
	}
	if err := os.WriteFile(prod, []byte("API_URL=b\nFEATURE_X=0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := cmdEnvParity([]string{stage, prod}, false); code != 0 {
		t.Errorf("paridade ok deve sair com 0, veio %d", code)
	}
}

func TestHookPayloadYieldsTheEditedPath(t *testing.T) {
	path, err := pathFromHookPayload(strings.NewReader(`{"tool_input":{"file_path":"/tmp/a.ts"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if path != "/tmp/a.ts" {
		t.Errorf("path errado: %q", path)
	}
}

func TestLineNumberPointsAtTheOffendingLine(t *testing.T) {
	content := []byte("linha um\nlinha dois\nexpect(spy).toHaveBeenCalled();\n")
	got := checkContent(embeddedRuleSet(t), TargetFile, "a.spec.ts", "", content)
	if len(got) == 0 {
		t.Fatal("esperava achado")
	}
	if got[0].Line != 3 {
		t.Errorf("linha errada: %d", got[0].Line)
	}
}
