package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Class struct {
	Desc    string   `json:"desc"`
	Aliases []string `json:"aliases"`
}

type Vocabulary map[string]Class

func loadVocabulary() (Vocabulary, error) {
	vocab := Vocabulary{}
	if err := json.Unmarshal(embeddedClasses, &vocab); err != nil {
		return nil, fmt.Errorf("embedded classes: %w", err)
	}
	overridePath := filepath.Join(stateDir(), "classes.json")
	raw, err := os.ReadFile(overridePath)
	if err != nil {
		if os.IsNotExist(err) {
			return vocab, nil
		}
		return nil, err
	}
	user := Vocabulary{}
	if err := json.Unmarshal(raw, &user); err != nil {
		return nil, fmt.Errorf("%s: %w", overridePath, err)
	}
	for name, c := range user {
		vocab[name] = c
	}
	return vocab, nil
}

func saveUserVocabulary(vocab Vocabulary) error {
	if err := os.MkdirAll(stateDir(), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(vocab, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(stateDir(), "classes.json"), append(raw, '\n'), 0o644)
}

func (v Vocabulary) resolve(slug string) string {
	if _, ok := v[slug]; ok {
		return slug
	}
	for name, c := range v {
		for _, a := range c.Aliases {
			if a == slug {
				return name
			}
		}
	}
	return ""
}

func (v Vocabulary) nearest(slug string, n int) []string {
	parts := map[string]bool{}
	for _, p := range strings.Split(slug, "-") {
		parts[p] = true
	}
	type scored struct {
		name  string
		score int
	}
	var all []scored
	for name := range v {
		score := 0
		for _, p := range strings.Split(name, "-") {
			if parts[p] {
				score++
			}
		}
		if score > 0 {
			all = append(all, scored{name, score})
		}
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].score != all[j].score {
			return all[i].score > all[j].score
		}
		return all[i].name < all[j].name
	})
	var out []string
	for i, s := range all {
		if i >= n {
			break
		}
		out = append(out, s.name)
	}
	return out
}

type ledgerEntry struct {
	PR       int `json:"pr"`
	Findings []struct {
		Class string `json:"class"`
	} `json:"findings"`
}

func ledgerDir() string {
	return filepath.Join(homeDir(), ".claude", "review-metrics", "ledger")
}

func scanLedgerClasses() (map[string]int, error) {
	counts := map[string]int{}
	files, err := filepath.Glob(filepath.Join(ledgerDir(), "*.jsonl"))
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		fh, err := os.Open(f)
		if err != nil {
			return nil, err
		}
		sc := bufio.NewScanner(fh)
		sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" {
				continue
			}
			var e ledgerEntry
			if err := json.Unmarshal([]byte(line), &e); err != nil {
				continue
			}
			for _, fd := range e.Findings {
				if fd.Class != "" {
					counts[fd.Class]++
				}
			}
		}
		fh.Close()
	}
	return counts, nil
}

func cmdClassesValidate() int {
	vocab, err := loadVocabulary()
	if err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		return 1
	}
	counts, err := scanLedgerClasses()
	if err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		return 1
	}
	var unknown []string
	total, resolved := 0, 0
	for slug, n := range counts {
		total += n
		if vocab.resolve(slug) != "" {
			resolved += n
			continue
		}
		unknown = append(unknown, slug)
	}
	sort.Slice(unknown, func(i, j int) bool { return counts[unknown[i]] > counts[unknown[j]] })

	fmt.Printf("ledger: %d achados, %d classes distintas\n", total, len(counts))
	fmt.Printf("resolvidos pelo vocabulario: %d/%d\n", resolved, total)
	if len(unknown) == 0 {
		fmt.Println("nenhum slug desconhecido")
		return 0
	}
	fmt.Printf("\n%d slugs fora do vocabulario:\n", len(unknown))
	for _, slug := range unknown {
		near := vocab.nearest(slug, 3)
		if len(near) > 0 {
			fmt.Printf("  %-52s (%dx)  proximos: %s\n", slug, counts[slug], strings.Join(near, ", "))
			continue
		}
		fmt.Printf("  %-52s (%dx)\n", slug, counts[slug])
	}
	fmt.Println("\nresolva com: dejavu classes alias <slug> <canonica>  |  dejavu classes add <slug> --desc \"...\"")
	return 1
}

func cmdClassesAlias(args []string) int {
	if len(args) != 2 {
		fmt.Fprintln(os.Stderr, "uso: dejavu classes alias <slug> <canonica>")
		return 2
	}
	slug, canonical := args[0], args[1]
	vocab, err := loadVocabulary()
	if err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		return 1
	}
	c, ok := vocab[canonical]
	if !ok {
		fmt.Fprintf(os.Stderr, "classe canonica %q nao existe; crie com: dejavu classes add %s --desc \"...\"\n", canonical, canonical)
		return 1
	}
	if existing := vocab.resolve(slug); existing != "" {
		fmt.Fprintf(os.Stderr, "%q ja resolve para %q\n", slug, existing)
		return 1
	}
	c.Aliases = append(c.Aliases, slug)
	sort.Strings(c.Aliases)
	vocab[canonical] = c
	if err := saveUserVocabulary(vocab); err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		return 1
	}
	fmt.Printf("%s -> %s\n", slug, canonical)
	return 0
}

func cmdClassesAdd(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "uso: dejavu classes add <slug> --desc \"...\"")
		return 2
	}
	slug := args[0]
	desc := ""
	for i := 1; i < len(args)-1; i++ {
		if args[i] == "--desc" {
			desc = args[i+1]
		}
	}
	if desc == "" {
		fmt.Fprintln(os.Stderr, "--desc e obrigatorio: e o que faz o modelo escolher a classe certa")
		return 2
	}
	vocab, err := loadVocabulary()
	if err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		return 1
	}
	if existing := vocab.resolve(slug); existing != "" {
		fmt.Fprintf(os.Stderr, "%q ja resolve para %q\n", slug, existing)
		return 1
	}
	vocab[slug] = Class{Desc: desc, Aliases: []string{}}
	if err := saveUserVocabulary(vocab); err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		return 1
	}
	fmt.Printf("classe adicionada: %s\n", slug)
	return 0
}

func cmdClassesList() int {
	vocab, err := loadVocabulary()
	if err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		return 1
	}
	names := make([]string, 0, len(vocab))
	for n := range vocab {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Printf("%-40s %s\n", n, vocab[n].Desc)
		if len(vocab[n].Aliases) > 0 {
			fmt.Printf("%-40s aliases: %s\n", "", strings.Join(vocab[n].Aliases, ", "))
		}
	}
	fmt.Printf("\n%d classes\n", len(names))
	return 0
}
