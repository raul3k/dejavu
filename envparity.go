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

var envStems = map[string]bool{
	"dev": true, "develop": true, "development": true, "local": true,
	"stage": true, "staging": true, "hml": true, "homolog": true,
	"prod": true, "production": true, "qa": true, "sandbox": true,
}

// dev/local usually carry a different SHAPE, not just different values (own probes,
// nodeSelector, no external secrets). Comparing them against the deployed environments
// floods the output and buries the one asymmetry that matters.
var shapeDivergentStems = map[string]bool{
	"dev": true, "develop": true, "development": true, "local": true,
}

func envStemOf(name string) string {
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	stem = strings.TrimPrefix(stem, "values-")
	stem = strings.TrimPrefix(stem, ".env.")
	if name == ".env" {
		return "default"
	}
	return strings.ToLower(stem)
}

func discoverEnvFiles(dir string, includeAll bool) (found, skipped []string, err error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		stem := envStemOf(e.Name())
		if !envStems[stem] {
			continue
		}
		if !includeAll && shapeDivergentStems[stem] {
			skipped = append(skipped, e.Name())
			continue
		}
		found = append(found, filepath.Join(dir, e.Name()))
	}
	sort.Strings(found)
	sort.Strings(skipped)
	return found, skipped, nil
}

func cmdEnvParity(paths []string, includeAll bool) int {
	if len(paths) == 1 {
		if st, err := os.Stat(paths[0]); err == nil && st.IsDir() {
			found, skipped, err := discoverEnvFiles(paths[0], includeAll)
			if err != nil {
				fmt.Fprintln(os.Stderr, "erro:", err)
				return 1
			}
			if len(found) < 2 {
				fmt.Fprintf(os.Stderr, "menos de dois arquivos de ambiente comparaveis em %s\n", paths[0])
				return 2
			}
			var names []string
			for _, f := range found {
				names = append(names, filepath.Base(f))
			}
			fmt.Printf("comparando: %s\n", strings.Join(names, ", "))
			if len(skipped) > 0 {
				fmt.Printf("ignorado por ter forma propria: %s (use --all-envs para incluir)\n", strings.Join(skipped, ", "))
			}
			fmt.Println()
			paths = found
		}
	}
	if len(paths) < 2 {
		fmt.Fprintln(os.Stderr, "uso: dejavu env-parity <arquivo> <arquivo> [...]  |  dejavu env-parity <diretorio>")
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
		var present, absent []string
		for _, p := range paths {
			if perFile[p][k] {
				present = append(present, filepath.Base(p))
			}
		}
		for _, p := range missing {
			absent = append(absent, filepath.Base(p))
		}
		fmt.Printf("%-44s presente em %s | AUSENTE em %s\n", k, strings.Join(present, ", "), strings.Join(absent, ", "))
	}

	if found == 0 {
		fmt.Printf("paridade ok: %d chaves iguais em %d arquivos\n", len(all), len(paths))
		return 0
	}
	fmt.Printf("\n%d chave(s) assimetrica(s) entre %d arquivos.\n", found, len(paths))
	fmt.Println("Config presente em um ambiente e ausente no outro e o escape mais comum deste codebase; justifique cada assimetria.")
	return 2
}
