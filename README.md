# dejavu

Salvaguarda barata: checagens determinísticas derivadas de erros que já foram cometidos.

English version: [README.en.md](README.en.md)

## O problema

Todo time acumula uma lista de erros que já custaram caro. O subject do commit que reprova no
commitlint e trava a pipeline inteira. O teste com `.only` que faz o gate passar sem rodar nada.
O cast cego do body HTTP que transforma todo campo em `undefined` em silêncio. São erros
conhecidos, documentados, já discutidos em review. E voltam.

Voltam porque o lugar onde esse conhecimento mora é o lugar errado. Ele mora na cabeça de quem
já se queimou, em um `CONTRIBUTING.md` que ninguém relê, ou na atenção de quem está revisando a
PR. Os três são caros, lentos e não determinísticos: dependem de a pessoa certa estar olhando no
momento certo, e a chance de escapar cresce com o tamanho do diff e com o cansaço de quem revisa.

O `dejavu` parte de uma pergunta só: **se esse erro já foi cometido antes, por que ele ainda
precisa de atenção humana para ser pego?**

Erro que vira regex vira detector. Detector roda de graça, sempre, em todo arquivo, sem cansar e
sem depender de quem está de plantão. O que sobra de atenção humana passa a ser gasto no que
realmente exige julgamento, em vez de ser gasto relendo pela décima vez se o subject do commit
começa com maiúscula.

## O que ele resolve, concretamente

1. **Regressão de erro conhecido.** A classe de erro que já apareceu em review vira uma checagem
   mecânica e não volta a consumir tempo de ninguém.
2. **Código gerado por IA que entra sem passar por review.** O `dejavu` roda como hook do Claude
   Code em cada `Edit`/`Write`. Se o modelo escreve um comentário em pt-BR ou um em dash, ele
   recebe o achado de volta na hora, antes do arquivo virar diff. É o caso de uso que originou
   a ferramenta.
3. **Convenção que quebra CI antes de você descobrir.** As regras de commit rodam localmente
   contra o `COMMIT_EDITMSG`, então o subject inválido reprova na sua máquina em vez de reprovar
   na pipeline dez minutos depois.
4. **Deriva de vocabulário.** Quando reviews sucessivas classificam o mesmo defeito com slugs
   diferentes, o histórico deixa de ser agregável. O `classes validate` aponta a deriva e o
   `classes alias` absorve, sem reescrever nada do que já foi gravado.
5. **Assimetria entre ambientes.** O `env-parity` compara as chaves de configuração entre stage e
   prod e aponta a que existe em um e falta no outro.

## O que ele não é

**Não é um linter.** Linter checa a ideia que a linguagem ou o framework tem de correto, com
regras que o ecossistema publica. O `dejavu` checa o seu histórico de erros, incluindo coisas que
nenhum linter modela: uma convenção de mensagem de commit específica da sua pipeline, um par de
classes CSS que reprova em contraste no seu design system, um slug de taxonomia fora do
vocabulário. Os dois convivem, e o `dejavu` não substitui nenhum.

**Não é um revisor.** Não lê intenção, não entende o domínio e não avalia se a mudança está
certa. É a rede mecânica embaixo da review, não a review. Se uma regra não vira detector
executável, ela não entra aqui: continua como conhecimento em prosa para quem revisa.

**Não é um gate obrigatório.** Roda como hook e sob demanda. A decisão de bloquear ou não é de
quem integra, via código de saída e `--no-fail`.

## Requisitos

Go 1.22 ou superior. Nenhuma dependência externa: o binário embute as regras e o vocabulário.

## Instalação

```bash
make install
```

Roda `gofmt`, `go vet`, os testes, compila e instala em `~/.local/bin`. Use `PREFIX=` para mudar
o destino:

```bash
make install PREFIX=/usr/local
```

## Uso

```bash
dejavu check <arquivo>...            # regras de arquivo nos caminhos dados
dejavu commit-msg <arquivo|->        # regras de mensagem de commit ('-' lê do stdin)
dejavu env-parity <arq> <arq>...     # compara as CHAVES entre arquivos de ambiente
dejavu env-parity <diretorio>        # descobre os ambientes do diretório e compara
dejavu hook                          # lê o payload do hook do Claude Code no stdin
dejavu rules list                    # regras carregadas
dejavu rules stats                   # disparos e precisão por regra
dejavu rules doctor                  # recomenda rebaixar ou promover (não aplica)
dejavu rules fp|tp <id> [nota]       # registra veredito sobre um disparo
dejavu classes list                  # vocabulário de classes de achado
dejavu classes validate              # slugs do ledger fora do vocabulário
dejavu classes alias <slug> <canon>  # absorve um slug derivado em uma classe canônica
dejavu classes add <slug> --desc     # registra uma classe nova
dejavu version
```

### Flags

| Flag | Efeito |
| --- | --- |
| `--json` | saída em JSON (`check`, `commit-msg`) |
| `--no-fail` | sai com `0` mesmo havendo achados |
| `--candidates` | inclui as regras em calibração (`status: candidate`), que ficam fora do hook |
| `--all-envs` | no `env-parity`, inclui `dev` e `local`, que por padrão ficam de fora |

## Saída

```
$ dejavu check exemplo.ts
exemplo.ts:1  [medium] accented-comment-in-source
    Comentario com acentuacao em codigo. Codigo-fonte e English-only, e comentario em pt-BR
    marca o diff como gerado por maquina na hora da review.
    > const u = 1; // configuração inválida
```

Códigos de saída:

| Código | Significado |
| --- | --- |
| `0` | limpo, ou houve achados com `--no-fail` |
| `2` | achados, ou erro de uso (comando desconhecido, operando faltando) |
| `1` | erro de execução (regra inválida, arquivo ilegível) |

O `2` é o que faz o hook devolver o achado ao modelo.

## Como as regras funcionam

Detector é dado, não código. Uma regra é uma entrada em `data/rules.json`:

| Campo | Papel |
| --- | --- |
| `id` | identificador único; é por ele que o override substitui |
| `target` | `file`, `commit-subject`, `commit-body` ou `commit-message` |
| `mode` | `flag_if_match` (o padrão) ou `flag_if_no_match` |
| `repos` / `paths` | escopo; vazio significa todos |
| `pattern` | regex RE2, a sintaxe do pacote `regexp` do Go (sem lookahead nem backreference) |
| `severity` | string livre, não validada pelo código; a convenção em uso é `blocker`, `high`, `medium`, `low` |
| `message` | o texto que o humano ou o modelo recebe; deve dizer a consequência, não só o nome do erro |
| `source` | de onde a regra veio, para poder auditar |
| `status` | `active`, `candidate` ou `demoted` |

Exemplo:

```json
{
  "id": "spec-focused-test",
  "target": "file",
  "paths": ["**/*.spec.ts", "**/*.test.ts"],
  "pattern": "\\b(describe|it|test)\\.only\\(",
  "mode": "flag_if_match",
  "severity": "high",
  "message": "Teste focado com .only: o resto do arquivo deixa de rodar e o gate passa sem executar nada.",
  "source": "ledger:test-not-run-by-any-gate",
  "status": "active"
}
```

O campo `source` é rastro de procedência, não um caminho resolvível. Ele pode apontar para
qualquer coisa que justifique a regra: uma seção de um documento de convenções, uma skill de
review, ou uma entrada do ledger com a contagem de ocorrências que motivou a promoção
(`ledger:test-assertion-too-weak (9x, 80% aceite)`). Vários apontam para artefatos que vivem
fora deste repositório, no setup de quem escreveu a regra. Serve para você poder perguntar
"de onde veio isso?" antes de mexer.

`candidate` é a regra em calibração: só roda com `--candidates` e nunca no hook. É onde fica a
regra que ainda produz falso positivo demais, em vez de ser deletada ou de poluir o hook.

### Override local

As regras embutidas neste repositório são as **genéricas**, as que valem para qualquer
codebase. Regra específica de um repositório seu não pertence aqui: declare em
`~/.claude/dejavu/rules.json`, que é um array no mesmo formato e sobrepõe as embutidas por `id`.

Para desligar uma regra embutida, redeclare o mesmo `id` com `"status": "demoted"`.

O mesmo vale para o vocabulário, via `~/.claude/dejavu/classes.json`.

## Vocabulário de classes

`data/classes.json` é a lista fechada de classes de achado usada por um ledger de review externo
à ferramenta. Cada classe carrega `desc`, que é o que faz o modelo escolher a classe certa, e
`aliases`, os slugs derivados que já foram usados para ela.

O `aliases` é o que impede a deriva de acumular: quando uma review grava um slug novo para algo
que já existe, `classes validate` aponta, e `classes alias <slug> <canônica>` absorve. O
histórico inteiro se reconcilia sem reescrever nada.

Regra: fundir classe é decisão humana. A ferramenta detecta e sugere, nunca funde sozinha, porque
duas classes parecidas podem ser dois modos de falha diferentes.

## Precisão

Esta é a parte que decide se a ferramenta sobrevive ou vira ruído.

Ruleset pessoal costuma apodrecer porque ninguém mede se as regras estão certas. A regra entra,
começa a disparar em caso legítimo, a pessoa aprende a ignorar, e o custo não fica só nela: uma
checagem em que ninguém confia treina a pessoa a ignorar **todas** as outras.

Então todo disparo é registrado em `~/.claude/dejavu/hits.jsonl`. Você marca o veredito com
`dejavu rules fp <id>` (falso positivo) ou `dejavu rules tp <id>` (verdadeiro), e o
`dejavu rules stats` cruza as duas coisas e mostra a precisão por regra.

`dejavu rules doctor` lê isso e recomenda rebaixar a regra barulhenta para `candidate` ou
promover a regra em calibração que se provou. Ele recomenda e não aplica: mexer no conjunto que
roda no hook é decisão de quem usa.

O ciclo completo: a regra nasce `candidate`, roda só com `--candidates`, acumula disparos e
vereditos, e só entra no hook quando a precisão justifica.

## Integração com o Claude Code

`dejavu hook` lê o payload do hook no stdin, extrai `tool_input.file_path` e roda as regras de
arquivo naquele caminho. Achado sai no stderr com código `2`, que é como o Claude Code devolve o
conteúdo ao modelo. Sem achado, sai `0` e em silêncio.

Em `~/.claude/settings.json`:

```json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Edit|Write",
        "hooks": [
          { "type": "command", "command": "dejavu hook", "statusMessage": "dejavu" }
        ]
      }
    ]
  }
}
```

O hook nunca inclui regras `candidate`.

### Mensagem de commit

Nenhum hook de git é instalado automaticamente. As regras de commit rodam sob demanda:

```bash
dejavu commit-msg .git/COMMIT_EDITMSG
```

Para tornar obrigatório em um repositório, o conteúdo de `.git/hooks/commit-msg`:

```bash
#!/bin/sh
exec dejavu commit-msg "$1"
```

## Estado local

| Arquivo | Papel |
| --- | --- |
| `~/.claude/dejavu/rules.json` | regras adicionais ou override das embutidas |
| `~/.claude/dejavu/classes.json` | override do vocabulário |
| `~/.claude/dejavu/hits.jsonl` | todo disparo, para medir precisão |
| `~/.claude/dejavu/feedback.jsonl` | vereditos manuais (`fp` / `tp`) |

Nada disso é criado até existir o primeiro disparo ou o primeiro override.

## Licença

MIT.
