package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

//go:embed data/rules.json
var embeddedRules []byte

//go:embed data/classes.json
var embeddedClasses []byte

const version = "0.2.0"

const usage = `dejavu - checagens deterministicas derivadas de erros ja cometidos

uso:
  dejavu check <arquivo>...          roda as regras de arquivo nos caminhos dados
  dejavu commit-msg <arquivo|->      roda as regras de mensagem de commit
  dejavu env-parity <arq> <arq>...   compara as CHAVES entre arquivos de ambiente
  dejavu env-parity <diretorio>      descobre os arquivos de ambiente e compara
                                     (dev/local ficam de fora; --all-envs inclui)
  dejavu hook                        le o payload do hook do Claude Code no stdin
  dejavu rules list                  lista as regras carregadas
  dejavu rules stats                 disparos e precisao por regra
  dejavu rules doctor                recomenda rebaixar ou promover (nao aplica)
  dejavu rules fp|tp <id> [nota]     registra veredito sobre um disparo
  dejavu classes list                lista o vocabulario de classes
  dejavu classes validate            aponta slugs do ledger fora do vocabulario
  dejavu classes alias <slug> <can>  absorve um slug derivado em uma classe canonica
  dejavu classes add <slug> --desc   registra uma classe nova
  dejavu version

flags:
  --json        saida em JSON (check, commit-msg)
  --no-fail     sai com 0 mesmo havendo achados
  --candidates  inclui regras em calibracao (status candidate), fora do hook

saida: 0 limpo, 2 achados, 1 erro
`

func homeDir() string {
	if home := os.Getenv("HOME"); home != "" {
		return home
	}
	home, _ := os.UserHomeDir()
	return home
}

func stateDir() string {
	return filepath.Join(homeDir(), ".claude", "dejavu")
}

func hasFlag(args []string, name string) bool {
	for _, a := range args {
		if a == name {
			return true
		}
	}
	return false
}

func stripFlags(args []string) []string {
	var out []string
	for _, a := range args {
		if !strings.HasPrefix(a, "--") {
			out = append(out, a)
		}
	}
	return out
}

func emit(findings []Finding, kind string, asJSON, noFail bool) int {
	logHits(findings, kind)
	if asJSON {
		raw, _ := json.MarshalIndent(findings, "", "  ")
		fmt.Println(string(raw))
	} else {
		renderFindings(os.Stdout, findings)
	}
	if len(findings) > 0 && !noFail {
		return 2
	}
	return 0
}

func mustLoad(includeCandidates bool) []*Rule {
	rules, err := loadRules()
	if err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		os.Exit(1)
	}
	return selectRules(rules, includeCandidates)
}

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Print(usage)
		os.Exit(2)
	}

	asJSON := hasFlag(args, "--json")
	noFail := hasFlag(args, "--no-fail")
	withCandidates := hasFlag(args, "--candidates")
	cmd := args[0]
	rest := stripFlags(args[1:])

	switch cmd {
	case "check":
		if len(rest) == 0 {
			fmt.Fprintln(os.Stderr, "uso: dejavu check <arquivo>...")
			os.Exit(2)
		}
		findings, err := checkFiles(mustLoad(withCandidates), rest)
		if err != nil {
			fmt.Fprintln(os.Stderr, "erro:", err)
			os.Exit(1)
		}
		os.Exit(emit(findings, "check", asJSON, noFail))

	case "commit-msg":
		if len(rest) == 0 {
			fmt.Fprintln(os.Stderr, "uso: dejavu commit-msg <arquivo|->")
			os.Exit(2)
		}
		findings, err := checkCommitMessage(mustLoad(withCandidates), rest[0])
		if err != nil {
			fmt.Fprintln(os.Stderr, "erro:", err)
			os.Exit(1)
		}
		os.Exit(emit(findings, "commit-msg", asJSON, noFail))

	case "env-parity":
		os.Exit(cmdEnvParity(rest, hasFlag(args, "--all-envs")))

	case "hook":
		path, err := pathFromHookPayload(os.Stdin)
		if err != nil || path == "" {
			os.Exit(0)
		}
		rules, err := loadRules()
		if err != nil {
			os.Exit(0)
		}
		findings, err := checkFiles(selectRules(rules, false), []string{path})
		if err != nil || len(findings) == 0 {
			os.Exit(0)
		}
		logHits(findings, "hook")
		fmt.Fprintf(os.Stderr, "dejavu: %d achado(s) em %s\n", len(findings), path)
		renderFindings(os.Stderr, findings)
		os.Exit(2)

	case "rules":
		sub := "list"
		if len(rest) > 0 {
			sub = rest[0]
		}
		switch sub {
		case "list":
			for _, r := range mustLoad(true) {
				fmt.Printf("%-34s %-16s %-8s %-10s %s\n", r.ID, r.Target, r.Severity, r.Status, r.Source)
			}
		case "stats":
			os.Exit(cmdRulesStats())
		case "doctor":
			os.Exit(cmdRulesDoctor())
		case "fp", "tp":
			os.Exit(cmdRulesFeedback(rest[1:], sub))
		default:
			fmt.Fprintf(os.Stderr, "subcomando desconhecido: rules %s\n", sub)
			os.Exit(2)
		}

	case "classes":
		sub := "list"
		if len(rest) > 0 {
			sub = rest[0]
		}
		switch sub {
		case "list":
			os.Exit(cmdClassesList())
		case "validate":
			os.Exit(cmdClassesValidate())
		case "alias":
			os.Exit(cmdClassesAlias(rest[1:]))
		case "add":
			os.Exit(cmdClassesAdd(args[2:]))
		default:
			fmt.Fprintf(os.Stderr, "subcomando desconhecido: classes %s\n", sub)
			os.Exit(2)
		}

	case "version":
		fmt.Println("dejavu", version)

	default:
		fmt.Print(usage)
		os.Exit(2)
	}
}
