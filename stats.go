package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type hitRecord struct {
	At     string `json:"at"`
	RuleID string `json:"rule_id"`
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Kind   string `json:"kind"`
}

func hitsPath() string {
	return filepath.Join(stateDir(), "hits.jsonl")
}

func logHits(findings []Finding, kind string) {
	if len(findings) == 0 {
		return
	}
	if err := os.MkdirAll(stateDir(), 0o755); err != nil {
		return
	}
	fh, err := os.OpenFile(hitsPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer fh.Close()
	now := time.Now().UTC().Format(time.RFC3339)
	w := bufio.NewWriter(fh)
	for _, f := range findings {
		raw, err := json.Marshal(hitRecord{At: now, RuleID: f.RuleID, Path: f.Path, Line: f.Line, Kind: kind})
		if err != nil {
			continue
		}
		w.Write(raw)
		w.WriteByte('\n')
	}
	w.Flush()
}

func readHits() ([]hitRecord, error) {
	fh, err := os.Open(hitsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer fh.Close()
	var out []hitRecord
	sc := bufio.NewScanner(fh)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var r hitRecord
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, sc.Err()
}

func feedbackPath() string {
	return filepath.Join(stateDir(), "feedback.jsonl")
}

type feedbackRecord struct {
	At      string `json:"at"`
	RuleID  string `json:"rule_id"`
	Verdict string `json:"verdict"`
	Note    string `json:"note,omitempty"`
}

func cmdRulesFeedback(args []string, verdict string) int {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "uso: dejavu rules %s <rule_id> [nota]\n", verdict)
		return 2
	}
	rules, err := loadRules()
	if err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		return 1
	}
	known := false
	for _, r := range rules {
		if r.ID == args[0] {
			known = true
		}
	}
	if !known {
		fmt.Fprintf(os.Stderr, "regra desconhecida: %s\n", args[0])
		return 1
	}
	if err := os.MkdirAll(stateDir(), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		return 1
	}
	fh, err := os.OpenFile(feedbackPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		return 1
	}
	defer fh.Close()
	rec := feedbackRecord{
		At:      time.Now().UTC().Format(time.RFC3339),
		RuleID:  args[0],
		Verdict: verdict,
		Note:    strings.Join(args[1:], " "),
	}
	raw, _ := json.Marshal(rec)
	fh.Write(append(raw, '\n'))
	fmt.Printf("%s registrado para %s\n", verdict, args[0])
	return 0
}

func readFeedback() map[string]map[string]int {
	out := map[string]map[string]int{}
	fh, err := os.Open(feedbackPath())
	if err != nil {
		return out
	}
	defer fh.Close()
	sc := bufio.NewScanner(fh)
	for sc.Scan() {
		var r feedbackRecord
		if err := json.Unmarshal([]byte(sc.Text()), &r); err != nil {
			continue
		}
		if out[r.RuleID] == nil {
			out[r.RuleID] = map[string]int{}
		}
		out[r.RuleID][r.Verdict]++
	}
	return out
}

const (
	doctorMinFires     = 10
	doctorMinPrecision = 0.5
)

type doctorVerdict struct {
	RuleID string
	Reason string
}

func diagnose(rules []*Rule, fires map[string]int, fb map[string]map[string]int) []doctorVerdict {
	var out []doctorVerdict
	for _, r := range rules {
		tp, fp := fb[r.ID]["tp"], fb[r.ID]["fp"]
		judged := tp + fp
		switch {
		case r.Status == StatusActive && judged > 0 && float64(tp)/float64(judged) < doctorMinPrecision:
			out = append(out, doctorVerdict{r.ID, fmt.Sprintf(
				"precisao %.0f%% (%d/%d julgados) abaixo de %.0f%%: rebaixe para candidate",
				100*float64(tp)/float64(judged), tp, judged, 100*doctorMinPrecision)})
		case r.Status == StatusActive && judged == 0 && fires[r.ID] >= doctorMinFires:
			out = append(out, doctorVerdict{r.ID, fmt.Sprintf(
				"%d disparos e nenhum veredito: julgue com rules fp|tp antes que vire ruido tolerado", fires[r.ID])})
		case r.Status == StatusCandidate && judged > 0 && float64(tp)/float64(judged) >= doctorMinPrecision:
			out = append(out, doctorVerdict{r.ID, fmt.Sprintf(
				"precisao %.0f%% (%d/%d julgados): promova para active",
				100*float64(tp)/float64(judged), tp, judged)})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RuleID < out[j].RuleID })
	return out
}

func cmdRulesDoctor() int {
	rules, err := loadRules()
	if err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		return 1
	}
	hits, err := readHits()
	if err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		return 1
	}
	fires := map[string]int{}
	for _, h := range hits {
		fires[h.RuleID]++
	}

	verdicts := diagnose(rules, fires, readFeedback())
	if len(verdicts) == 0 {
		fmt.Println("nenhuma recomendacao: nenhuma regra cruzou os limiares.")
		return 0
	}
	fmt.Printf("%d recomendacao(oes):\n\n", len(verdicts))
	for _, v := range verdicts {
		fmt.Printf("  %-34s %s\n", v.RuleID, v.Reason)
	}
	fmt.Println("\nNada foi alterado. Rebaixar ou promover regra e decisao sua:")
	fmt.Println("edite o campo status em data/rules.json ou ~/.claude/dejavu/rules.json.")
	return 0
}

func cmdRulesStats() int {
	rules, err := loadRules()
	if err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		return 1
	}
	hits, err := readHits()
	if err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		return 1
	}
	fires := map[string]int{}
	for _, h := range hits {
		fires[h.RuleID]++
	}
	fb := readFeedback()

	fmt.Printf("%-34s %-10s %6s %5s %5s %8s\n", "REGRA", "STATUS", "DISPAR", "TP", "FP", "PRECISAO")
	sort.Slice(rules, func(i, j int) bool { return fires[rules[i].ID] > fires[rules[j].ID] })
	for _, r := range rules {
		tp, fp := fb[r.ID]["tp"], fb[r.ID]["fp"]
		prec := "-"
		if tp+fp > 0 {
			prec = fmt.Sprintf("%.0f%%", 100*float64(tp)/float64(tp+fp))
		}
		fmt.Printf("%-34s %-10s %6d %5d %5d %8s\n", r.ID, r.Status, fires[r.ID], tp, fp, prec)
	}
	fmt.Printf("\n%d disparos registrados em %s\n", len(hits), hitsPath())
	fmt.Println("marque veredito com: dejavu rules fp <id> [nota]  |  dejavu rules tp <id> [nota]")
	fmt.Println("regra com precisao baixa e muitos disparos deve virar candidate em data/rules.json")
	return 0
}
