# dejavu

Salvaguarda barato: checagens determinísticas derivadas de erros que já foram cometidos.

Não é um revisor e não substitui o `revisador`. É a rede mecânica que pega, de graça e sempre,
a classe de erro que já custou caro antes. Se uma regra não vira detector executável, ela não
entra aqui - continua no `repo-rules.local.md` como conhecimento para a review.

> `revisador`, `pr-review` e `repo-rules.local.md` são artefatos do setup local de quem escreveu
> isto (skills e notas em `~/.claude/`), não fazem parte deste repositório e não são necessários
> para usar o `dejavu`. Aparecem no campo `source` das regras só para dizer de onde cada regra
> veio. As regras embutidas aqui são as genéricas; regra específica de um repositório é para ser
> declarada no override local, descrito em "Como as regras funcionam".

## Instalação

```bash
make install
```

Roda `gofmt`, `go vet`, os testes, compila e instala em `~/.local/bin`. Use `PREFIX=` para
mudar o destino.

## Uso

```bash
dejavu check src/app.ts src/page.html   # regras de arquivo
dejavu commit-msg .git/COMMIT_EDITMSG   # regras de mensagem de commit
dejavu env-parity stage.yaml prod.yaml  # compara as CHAVES entre arquivos de ambiente
dejavu env-parity apps/x/manifests      # descobre os ambientes do diretório e compara
dejavu hook                             # lê o payload do hook do Claude Code no stdin
dejavu rules list                       # regras carregadas
dejavu rules stats                      # disparos e precisão por regra
dejavu rules doctor                     # recomenda rebaixar ou promover (não aplica)
dejavu rules fp <id> [nota]             # marca um disparo como falso positivo
dejavu classes list                     # vocabulário de classes de achado
dejavu classes validate                 # slugs do ledger fora do vocabulário
```

Saída: `0` limpo, `2` achados, `1` erro. O `2` é o que faz o hook devolver o achado ao modelo.

## Como as regras funcionam

Detector é dado, não código. Uma regra é uma entrada em `data/rules.json`:

| Campo | Papel |
| --- | --- |
| `target` | `file`, `commit-subject`, `commit-body`, `commit-message` |
| `mode` | `flag_if_match` (o padrão) ou `flag_if_no_match` |
| `repos` / `paths` | escopo; vazio significa todos |
| `pattern` | regex RE2 (sem lookahead) |
| `source` | de onde a regra veio, para poder auditar |
| `status` | `active`, `candidate`, `demoted` |

`candidate` é a regra em calibração: só roda com `--candidates` e nunca no hook. É onde fica
a regra que ainda produz falso positivo demais, em vez de ser deletada ou de poluir o hook.

Regras adicionais podem ser postas em `~/.claude/dejavu/rules.json`, que sobrepõe as embutidas
por `id`. Para desligar uma regra embutida, redeclare o mesmo `id` com `"status": "demoted"`.

## Vocabulário de classes

`data/classes.json` é a lista fechada de classes de achado usada pelo ledger de review
(`~/.claude/review-metrics/ledger/`). Cada classe carrega `desc` (o que faz o modelo escolher
certo) e `aliases` (os slugs derivados que já foram usados para ela).

O `aliases` é o que impede a deriva de acumular: quando uma review grava um slug novo para algo
que já existe, `classes validate` aponta, e `classes alias <slug> <canônica>` absorve. O
histórico inteiro se reconcilia sem reescrever nada.

Regra: fundir classe é decisão humana. A ferramenta detecta e sugere, nunca funde sozinha -
duas classes parecidas podem ser dois modos de falha diferentes.

## Precisão

Cada disparo é registrado em `~/.claude/dejavu/hits.jsonl`. `dejavu rules stats` cruza isso
com os vereditos de `dejavu rules fp|tp` e mostra a precisão por regra.

Regra com muitos disparos e precisão baixa deve virar `candidate`, não ser tolerada: uma
checagem em que ninguém confia treina a pessoa a ignorar todas as outras.

## Estado local

| Arquivo | Papel |
| --- | --- |
| `~/.claude/dejavu/rules.json` | regras adicionais ou override das embutidas |
| `~/.claude/dejavu/classes.json` | override do vocabulário |
| `~/.claude/dejavu/hits.jsonl` | todo disparo, para medir precisão |
| `~/.claude/dejavu/feedback.jsonl` | vereditos manuais (fp / tp) |

## Hooks

Instalado: `~/.claude/settings.json`, um `PostToolUse` em `Edit|Write` chamando `dejavu hook`.

Nenhum hook é instalado em repositório de trabalho. As regras de commit rodam sob demanda:

```bash
dejavu commit-msg .git/COMMIT_EDITMSG
```
