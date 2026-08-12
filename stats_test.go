package main

import (
	"os"
	"path/filepath"
	"testing"
)

func isolatedState(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
}

func TestHitsAreRecordedAndReadBack(t *testing.T) {
	isolatedState(t)
	logHits([]Finding{
		{Path: "a.ts", Line: 3, RuleID: "regra-a"},
		{Path: "b.ts", Line: 9, RuleID: "regra-b"},
		{Path: "c.ts", Line: 1, RuleID: "regra-a"},
	}, "check")

	hits, err := readHits()
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 3 {
		t.Fatalf("esperava 3 disparos, veio %d", len(hits))
	}
	count := map[string]int{}
	for _, h := range hits {
		count[h.RuleID]++
		if h.At == "" {
			t.Error("disparo sem timestamp nao serve para medir precisao no tempo")
		}
		if h.Kind != "check" {
			t.Errorf("kind errado: %q", h.Kind)
		}
	}
	if count["regra-a"] != 2 || count["regra-b"] != 1 {
		t.Errorf("contagem por regra errada: %v", count)
	}
}

func TestHitsAccumulateAcrossRuns(t *testing.T) {
	isolatedState(t)
	logHits([]Finding{{Path: "a.ts", RuleID: "regra-a"}}, "hook")
	logHits([]Finding{{Path: "a.ts", RuleID: "regra-a"}}, "hook")
	hits, err := readHits()
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 {
		t.Errorf("o log e append-only, esperava 2, veio %d", len(hits))
	}
}

func TestCleanRunWritesNoHitsFile(t *testing.T) {
	isolatedState(t)
	logHits(nil, "check")
	if _, err := os.Stat(hitsPath()); !os.IsNotExist(err) {
		t.Error("execucao limpa nao pode criar arquivo de disparos")
	}
}

func TestReadHitsOnFreshMachineIsEmptyNotAnError(t *testing.T) {
	isolatedState(t)
	hits, err := readHits()
	if err != nil {
		t.Fatalf("ausencia do arquivo nao e erro: %v", err)
	}
	if len(hits) != 0 {
		t.Errorf("esperava vazio, veio %d", len(hits))
	}
}

func writeFeedback(t *testing.T, lines string) {
	t.Helper()
	if err := os.MkdirAll(stateDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir(), "feedback.jsonl"), []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFeedbackIsAggregatedByRuleAndVerdict(t *testing.T) {
	isolatedState(t)
	writeFeedback(t, `{"at":"2026-01-01T00:00:00Z","rule_id":"regra-a","verdict":"fp"}
{"at":"2026-01-02T00:00:00Z","rule_id":"regra-a","verdict":"fp"}
{"at":"2026-01-03T00:00:00Z","rule_id":"regra-a","verdict":"tp"}
{"at":"2026-01-04T00:00:00Z","rule_id":"regra-b","verdict":"tp"}
`)
	fb := readFeedback()
	if fb["regra-a"]["fp"] != 2 || fb["regra-a"]["tp"] != 1 {
		t.Errorf("agregacao errada para regra-a: %v", fb["regra-a"])
	}
	if fb["regra-b"]["tp"] != 1 {
		t.Errorf("agregacao errada para regra-b: %v", fb["regra-b"])
	}
}

func TestDoctorRecommendsDemotingALowPrecisionActiveRule(t *testing.T) {
	rules := []*Rule{{ID: "ruidosa", Status: StatusActive}}
	fires := map[string]int{"ruidosa": 40}
	fb := map[string]map[string]int{"ruidosa": {"fp": 8, "tp": 2}}

	got := diagnose(rules, fires, fb)
	if len(got) != 1 || got[0].RuleID != "ruidosa" {
		t.Fatalf("esperava recomendacao para a regra ruidosa, veio %+v", got)
	}
}

func TestDoctorRecommendsPromotingAProvenCandidate(t *testing.T) {
	rules := []*Rule{{ID: "promissora", Status: StatusCandidate}}
	fb := map[string]map[string]int{"promissora": {"tp": 9, "fp": 1}}

	got := diagnose(rules, map[string]int{}, fb)
	if len(got) != 1 {
		t.Fatalf("candidate com boa precisao deve ser sugerida para promocao, veio %+v", got)
	}
}

func TestDoctorFlagsAFiringRuleNobodyEverJudged(t *testing.T) {
	rules := []*Rule{{ID: "nunca-julgada", Status: StatusActive}}
	fires := map[string]int{"nunca-julgada": doctorMinFires}

	got := diagnose(rules, fires, map[string]map[string]int{})
	if len(got) != 1 {
		t.Fatalf("regra que dispara muito e nunca foi julgada deve aparecer, veio %+v", got)
	}
}

func TestDoctorStaysQuietOnAHealthyRuleSet(t *testing.T) {
	rules := []*Rule{
		{ID: "boa", Status: StatusActive},
		{ID: "nova", Status: StatusActive},
	}
	fires := map[string]int{"boa": 5, "nova": 1}
	fb := map[string]map[string]int{"boa": {"tp": 5}}

	if got := diagnose(rules, fires, fb); len(got) != 0 {
		t.Errorf("nao deve recomendar nada, veio %+v", got)
	}
}

func TestEnvFileDiscoveryPicksTheEnvironmentsAndIgnoresTheChart(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"stage.yaml", "prod.yaml", "dev.yaml", "Chart.yaml", "README.md"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("A: 1\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	found, skipped, err := discoverEnvFiles(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 2 {
		t.Fatalf("por padrao compara so os ambientes implantados, veio %v", found)
	}
	for _, f := range found {
		base := filepath.Base(f)
		if base == "Chart.yaml" || base == "README.md" {
			t.Errorf("%s nao e arquivo de ambiente", base)
		}
		if base == "dev.yaml" {
			t.Error("dev tem forma propria e nao entra por padrao")
		}
	}
	if len(skipped) != 1 || skipped[0] != "dev.yaml" {
		t.Errorf("dev.yaml deve aparecer como ignorado, veio %v", skipped)
	}

	all, _, err := discoverEnvFiles(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Errorf("--all-envs deve incluir dev, veio %v", all)
	}
}

func TestEnvStemHandlesValuesPrefixedCharts(t *testing.T) {
	if got := envStemOf("values-prod.yaml"); got != "prod" {
		t.Errorf("values-prod.yaml deve virar prod, veio %q", got)
	}
	if got := envStemOf("Chart.yaml"); got == "prod" {
		t.Error("Chart.yaml nao pode virar ambiente")
	}
}
