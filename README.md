# UltraPlan Architecture Gauntlet

A deliberately small, one-off Go CLI for running a very large adversarial architecture review of **UltraPlan Go** with OpenCode agents.

It is not a general orchestration framework. It exists to exploit a short window where a strong model is cheap/free, run many independent architecture investigations, preserve their evidence, challenge weak findings, and synthesize a final review without turning UltraPlan itself into the review harness.

## Shape

The default plan contains **94 jobs**:

| Stage | Jobs | Purpose |
|---|---:|---|
| `scout` | 6 | Repository, dependency, contract, governance, state, and execution cartography |
| `generalist` | 4 | Blind whole-system reviews with different biases |
| `specialist` | 48 | 24 architecture lenses × 2 independent reviewers |
| `failure` | 12 | End-to-end crash/cancellation/restart/concurrency scenarios |
| `change` | 8 | Synthetic change-surface probes |
| `challenge` | 6 | Domain tribunals whose job is to falsify candidate findings |
| `chair` | 6 | Domain-level synthesis after challenge |
| `synth` | 3 | Independent whole-system syntheses |
| `arbiter` | 1 | Final chief-architect aggregation; may not invent findings |

Broad jobs are explicitly instructed to use bounded subagents for investigation and discovery. Narrow specialist jobs are kept independent so the review buys genuinely different attempts to understand the system rather than 48 agents echoing the same summary.

## Build

```bash
go test ./...
go build -o bin/archgauntlet ./cmd/archgauntlet
```

## Initialize a review

Run from this repository so OpenCode can discover the bundled `.opencode/agents/` definitions:

```bash
./bin/archgauntlet init \
  --target ../ultraplan-go \
  --workspace ../ultraplan-workspace \
  --model openrouter/stealth/ox-alpha
```

This creates `.archgauntlet/` containing `review.json` plus one immutable-ish prompt directory per job:

```text
.archgauntlet/
  review.json
  runs/
    scout-01/
      prompt.md
      stdout.md       # after execution
      stderr.log      # OpenCode stderr
    ...
```

The run directory is disposable and gitignored.

## Inspect the plan

```bash
./bin/archgauntlet status
./bin/archgauntlet next
./bin/archgauntlet prompt --id specialist-09a
./bin/archgauntlet run --stage scout --dry-run --project-dir .
```

## Execute

Start conservatively, then increase parallelism if the provider/runtime behaves well:

```bash
./bin/archgauntlet run --stage scout --parallel 3 --project-dir .
./bin/archgauntlet run --stage generalist --parallel 4 --project-dir .
./bin/archgauntlet run --stage specialist --parallel 8 --project-dir .
./bin/archgauntlet run --stage failure --parallel 6 --project-dir .
./bin/archgauntlet run --stage change --parallel 4 --project-dir .
./bin/archgauntlet run --stage challenge --parallel 3 --project-dir .
./bin/archgauntlet run --stage chair --parallel 3 --project-dir .
./bin/archgauntlet run --stage synth --parallel 3 --project-dir .
./bin/archgauntlet run --stage arbiter --parallel 1 --project-dir .
```

Later synthesis stages are gated until all earlier jobs have succeeded. Failed jobs are retained and are skipped by default; rerun them with `--retry`.

The OpenCode runner is effectively:

```text
opencode run --model <model> --agent <role> --format default --dir <gauntlet-repo> <prompt>
```

Use `--runner fake` to exercise orchestration without making model calls.

## Review doctrine

The bundled prompts intentionally reject generic architecture-scorecard behaviour:

- UltraPlan is **module-driven**, not global-layer-driven.
- Judge ownership, authority, lifecycle coherence, failure semantics, dependency direction, and change locality.
- A large cohesive module is not automatically a god module.
- An abstraction is only useful if it clarifies ownership or materially narrows change surface.
- Every finding begins as a hypothesis and must search for counter-evidence.
- `REALITY`, `CURRENT-CONTRACT`, `FUTURE-INTENT`, and `HISTORY` are distinct evidence classes.
- Missing future roadmap intent is not a current defect.
- Zero findings is a valid result.
- The target repositories are read-only.

## Included OpenCode agents

- `arch-orchestrator` — broad independent architecture investigator; uses `arch-worker` subagents.
- `arch-scout` — broad evidence cartographer; uses `arch-worker` subagents.
- `arch-inspector` — narrow specialist/failure/change probe; does not fan out by default.
- `arch-challenger` — adversarial falsifier; broad and subagent-enabled.
- `arch-chair` — domain synthesis; broad and subagent-enabled.
- `arch-synth` — system synthesis/final arbitration; broad and subagent-enabled.
- `arch-worker` — read-only bounded evidence subagent.

All custom agents deny edits. Broad agents can inspect the external UltraPlan repositories and spawn only the bounded `arch-worker` subagent.

## Output

`archgauntlet index` creates a Markdown index over all job results. The arbiter prompt requires the final result to distinguish:

- confirmed findings,
- disputed findings,
- defended architecture / rejected hypotheses,
- accepted debt,
- failure-scenario conclusions,
- change-surface results,
- documentation drift,
- ordered refactor waves,
- and fitness-function candidates.

The final question is not “how much can we refactor?” It is: **what should UltraPlan keep, what is now genuinely costing it, and what is the smallest coherent improvement?**
