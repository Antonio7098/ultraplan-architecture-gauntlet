---
name: ultraplan-architecture-gauntlet
description: Run or inspect the one-off, multi-agent adversarial architecture review of UltraPlan Go. Use only for this repository's architecture-gauntlet workflow.
---

# UltraPlan Architecture Gauntlet

Use the `archgauntlet` CLI as the durable coordinator. Do not manually collapse the swarm into one giant prompt.

## Operating sequence

1. Build/test the CLI.
2. Initialize with both the implementation repo and authoritative planning workspace.
3. Inspect `status`, `next`, and representative prompts.
4. Run investigative stages independently: `scout`, `generalist`, `specialist`, `failure`, `change`.
5. Run adversarial aggregation stages in order: `challenge`, `chair`, `synth`, `arbiter`.
6. Build the result index.

Broad roles must use subagents for bounded discovery when useful. They should delegate questions, not conclusions. Narrow specialist twins must remain independent and should not read sibling outputs.

## Architectural doctrine

UltraPlan is module-driven. Review concept ownership, authority, lifecycle, state truth, dependency direction, failure semantics, and change locality. Do not prescribe global Clean Architecture layers or package splitting merely because files are large.

Every candidate finding must be challenged. Search for counter-evidence in code, tests, runtime wiring, authoritative documents, reasoning, migration history, and explicit local invariants. Apparent duplication may be necessary boundary translation. A proposed abstraction that only adds indirection is not a fix.

Classify evidence as REALITY, CURRENT-CONTRACT, FUTURE-INTENT, or HISTORY. Never turn future roadmap intent into a present defect.

Target repositories are read-only.

## Useful commands

```bash
go test ./...
go build -o bin/archgauntlet ./cmd/archgauntlet
./bin/archgauntlet status
./bin/archgauntlet next
./bin/archgauntlet run --stage <stage> --parallel <n> --project-dir .
./bin/archgauntlet index
```

Read the references in this skill when you need the review catalogue, scenario catalogue, finding contract, or synthesis rules.
