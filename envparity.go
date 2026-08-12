package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	dotenvKey = regexp.MustCompile(`(?m)^\s*(?:export\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*=`)
	yamlKey   = regexp.MustCompile(`(?m)^\s*-?\s*(?:name:\s*)?["']?([A-Za-z_][A-Za-z0-9_.-]*)["']?\s*:`)
)

func extractKeys(path string, content []byte) map[string]bool {
	re := dotenvKey
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		re = yamlKey
	}
	keys := map[string]bool{}
	for _, m := range re.FindAllSubmatch(content, -1) {
		keys[string(m[1])] = true
	}
	return keys
}

func cmdEnvParity(paths []string) int {
	if len(paths) < 2 {
		fmt.Fprintln(os.Stderr, "uso: dejavu env-parity <arquivo-env-1> <arquivo-env-2> [...]")
		return 2
	}

	perFile := map[string]map[string]bool{}
	union := map[string]bool{}
	for _, p := range paths {
		content, err := os.ReadFile(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "erro: %s: %v\n", p, err)
			return 1
		}
		keys := extractKeys(p, content)
		perFile[p] = keys
		for k := range keys {
			union[k] = true
		}
	}

	all := make([]string, 0, len(union))
	for k := range union {
		all = append(all, k)
	}
	sort.Strings(all)

	found := 0
	for _, k := range all {
		var missing []string
		for _, p := range paths {
			if !perFile[p][k] {
				missing = append(missing, p)
			}
		}
		if len(missing) == 0 {
			continue
		}
		found++
		var present []string
		for _, p := range paths {
			if perFile[p][k] {
				present = append(present, filepath.Base(p))
			}
		}
		fmt.Printf("%-44s presente em %s | AUSENTE em %s\n", k, strings.Join(present, ", "), strings.Join(missing, ", "))
	}

	if found == 0 {
		fmt.Printf("paridade ok: %d chaves iguais em %d arquivos\n", len(all), len(paths))
		return 0
	}
	fmt.Printf("\n%d chave(s) assimetrica(s) entre %d arquivos.\n", found, len(paths))
	fmt.Println("Config presente em um ambiente e ausente no outro e o escape mais comum deste codebase; justifique cada assimetria.")
	return 2
}
