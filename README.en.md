# dejavu

Cheap safeguard: deterministic checks derived from mistakes that have already been made.

Portuguese version: [README.md](README.md)

## The problem

Every team accumulates a list of mistakes that already cost something. The commit subject that
fails commitlint and blocks the whole pipeline. The test with `.only` that makes the gate pass
without running anything. They are known, documented, discussed in review, and they come back.

They come back because that knowledge lives in the wrong place: in the head of whoever got burned,
in a `CONTRIBUTING.md` nobody rereads, or in the attention of the reviewer. All three are slow and
non-deterministic.

dejavu starts from one question: **if this mistake has been made before, why does it still need
human attention to be caught?** A mistake that becomes a regex becomes a detector, and a detector
runs for free, on every file, every time.

## What it does

- Turns recurring review findings into executable rules (`data/rules.json`, RE2 regexes).
- Runs as a Claude Code `PostToolUse` hook on `Edit`/`Write`, so AI-generated code gets feedback
  before it becomes a diff.
- Validates commit messages locally against `COMMIT_EDITMSG`, before CI does.
- Compares config keys across environment files (`env-parity`).
- Keeps a finding vocabulary and detects drift between slugs (`classes validate`, `classes alias`).
- Tracks rule precision (`hits.jsonl` plus manual `fp`/`tp` verdicts), and `rules doctor` recommends
  demoting noisy rules or promoting proven candidates. It recommends, it never applies.

It is not a linter, not a reviewer, and not a mandatory gate.

## Requirements

Go 1.22 or newer. No external dependencies: rules and vocabulary are embedded in the binary.

## Install

```bash
make install
```

Runs `gofmt`, `go vet`, the tests, builds, and installs to `~/.local/bin` (override with `PREFIX=`).

## Usage

```bash
dejavu check <file>...            # file rules on the given paths
dejavu commit-msg <file|->        # commit message rules ('-' reads stdin)
dejavu env-parity <file> <file>... # compare KEYS across environment files
dejavu env-parity <dir>           # discover environments in a directory and compare
dejavu hook                       # read a Claude Code hook payload from stdin
dejavu rules list|stats|doctor
dejavu rules fp|tp <id> [note]    # record a verdict on a rule hit
dejavu classes list|validate
dejavu version
```

Flags: `--json`, `--no-fail`, `--candidates`, `--all-envs`.

Exit codes: `0` clean (or findings with `--no-fail`), `2` findings or usage error, `1` runtime error.
Exit `2` is what makes the Claude Code hook send the finding back to the model.

## Rules

A rule is data, not code. Example entry in `data/rules.json`:

```json
{
  "id": "spec-focused-test",
  "target": "file",
  "paths": ["**/*.spec.ts", "**/*.test.ts"],
  "pattern": "\\b(describe|it|test)\\.only\\(",
  "mode": "flag_if_match",
  "severity": "high",
  "message": "Focused test with .only: the rest of the file stops running and the gate passes without executing anything.",
  "source": "ledger:test-not-run-by-any-gate",
  "status": "active"
}
```

`status` is `active`, `candidate` (runs only with `--candidates`, never in the hook) or `demoted`.
Repository-specific rules go in `~/.claude/dejavu/rules.json` and override built-ins by `id`.

## Claude Code hook

In `~/.claude/settings.json`:

```json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Edit|Write",
        "hooks": [{ "type": "command", "command": "dejavu hook", "statusMessage": "dejavu" }]
      }
    ]
  }
}
```

## License

MIT.
