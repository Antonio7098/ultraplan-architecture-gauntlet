package gauntlet

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func MaterializePrompts(out string, r *Review) error {
	for i := range r.Tasks {
		t := &r.Tasks[i]
		dir := filepath.Join(out, "runs", t.ID)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "prompt.md"), []byte(RenderPrompt(r, t)), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func RenderPrompt(r *Review, t *Task) string {
	broad := "Do not delegate this narrow assignment unless a tightly scoped lookup would materially improve evidence quality."
	if t.Broad {
		broad = `This is a BROAD assignment. Use subagents aggressively for independent discovery and evidence gathering when the host supports them. Delegate bounded questions (package maps, lifecycle traces, counter-evidence searches, specific subsystems) rather than trying to hold the entire repository in one context. Re-derive conclusions yourself from returned evidence. Do not ask subagents to rubber-stamp a draft.`
	}
	prior := "Do not read other review-agent outputs. Independence is part of the experiment."
	if t.Stage == "challenge" || t.Stage == "chair" || t.Stage == "synth" || t.Stage == "arbiter" {
		prior = fmt.Sprintf("Read prior review outputs under %s/runs as required for this synthesis stage. Preserve provenance and never promote an unsupported claim.", r.OutputPath)
	}
	return fmt.Sprintf(`# UltraPlan Architecture Gauntlet — %s

ROLE: %s
STAGE: %s
TASK ID: %s

## Target

- implementation repository: %s
- implementation commit: %s (dirty=%t)
- authoritative planning workspace: %s
- planning workspace commit: %s (dirty=%t)

## Assignment

%s

%s

## Review doctrine

UltraPlan is module-driven, not global-layer-driven. Judge architecture by ownership, authority, lifecycle coherence, dependency direction, failure behaviour, and change locality. Do not penalize the repository for not resembling Clean Architecture folder diagrams. Deep cohesive modules are valid. Abstractions must be earned.

Every finding is a hypothesis. Before reporting it, actively search for counter-evidence: tests, runtime wiring, docs, reasoning, local invariants, intentional debt, necessary boundary translation, or evidence that your proposed alternative merely adds indirection.

Distinguish evidence:
- REALITY: current source/tests/runtime wiring
- CURRENT-CONTRACT: requirements applicable to implemented behaviour
- FUTURE-INTENT: roadmap/future sprint requirements
- HISTORY: superseded decisions or migration context

Never report missing FUTURE-INTENT as a current defect.

%s

## Mandatory investigation rules

1. Read real code, tests, package imports and relevant authoritative docs before judging.
2. For architectural claims, cite concrete file:line or symbol evidence wherever possible.
3. Trace state and lifecycle ownership across boundaries instead of stopping at one file.
4. Describe a concrete architectural consequence or failure/change-cost scenario.
5. Prefer the smallest useful action; do not invent a second architecture.
6. Returning zero findings is valid. Do not manufacture criticism to satisfy the task.
7. Read-only review: do not modify target repositories.

## Output contract

Write Markdown with:

### Scope inspected
Concrete files/packages/docs/commands inspected.

### Architecture assessment
What is sound, ambiguous, or stressed in this assignment.

### Candidate findings
For each:
- ID: %s-FNN
- Priority: P0 | P1 | P2 | P3
- Claim
- Evidence (file:line/symbol)
- Architectural reason (ownership | authority | boundary | lifecycle | drift | change-surface | failure-semantics)
- Concrete consequence
- Counter-evidence searched
- Confidence: high | medium | low
- Smallest useful action

### Defended architecture / rejected hypotheses
Important concerns you investigated but disproved, with evidence.

### Open questions
Only uncertainties that could materially change the assessment.

Do not include style nits or generic praise.
`, t.Title, t.Role, t.Stage, t.ID, r.TargetPath, emptyAsNone(r.TargetCommit), r.TargetDirty, emptyAsNone(r.WorkspacePath), emptyAsNone(r.WorkspaceCommit), r.WorkspaceDirty, t.Focus, broad, prior, strings.ToUpper(t.ID))
}

func emptyAsNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}
