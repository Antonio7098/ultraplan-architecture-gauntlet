This is valuable HISTORY evidence — the repo contains the actual plan used when `code-context` was last added. Now let me trace the real current edit surface. Reading core stage-consuming files in parallel:
Two notable drift signals in the app layer. Let me verify them and look at prompt_bundle.go plus the remaining surfaces:
Final verification greps before writing the report:
# CHANGE-01 — Add a governed sprint stage: edit-surface trace

## Scope inspected

**Implementation repo** (`ultraplan-go` @ eeaa034, clean): `internal/sprint/{domain,state,artifacts,flow,service,prompts,prompt_bundle,prompt_context,input_contract,store_fs,validation,verify,review,smoke,smoke_author,direct_inputs}.go`; `internal/app/{sprint_commands,sprint_usecases,operations,web_usecases,usecases,skills_commands,config_commands}.go`; `internal/platform/config/{config,redaction}.go`; `internal/workspace/{skills,defaults,init}.go` + `scaffold/prompts|templates`; `internal/web/templates/sprint.html`, `static/js/app.js`, web operation tests; `internal/runcontrol`, `internal/productstate` (stage-name checks); tests (`sprint_test.go`, `sprint_usecases.go` history via `git log -L`, commit `2d10aba "Add code context planning stage"` diff-stat); docs: `docs/architecture.md`, `docs/stage-skills.md`, `docs/plans/sprint-code-context-stage.md`.

**Planning workspace** (@ 368a789): `system/contracts/**` stage references; persisted `projects/ultraplan-go/sprints/35-durable-run-observability/flow-state.json` (schemaVersion 2, 7 stages: requirements → code-context → sprint-index → technical-handbook → area-reasoning → reasoning → plan).

## Architecture assessment

**Sound.** The stage state machine has one clear owner: `internal/sprint`. A canonical ordered list (`PlanningStages()`, domain.go:264-274) derives several behaviors automatically: validity (domain.go:284), flow-state shape enforcement (state.go:307-313 fails closed on length/order mismatch), empty/failed stage matrices (flow.go:304-323), and status derivation (service.go:1352). Execute/review/smoke are correctly modeled as governed non-planning stages outside that list (domain.go:135-143 separate FlowState records). Several surfaces are genuinely generic and need **zero** edits: session state (keyed by `PlanningStage`), mutation locks, run-control (opaque `sprint.*` operation strings — no stage enumeration in `internal/runcontrol` or `internal/productstate`), web JS (`app.js` contains no stage names; stages arrive server-rendered), web operations mapping (`"sprint-stage"` + string, operations_contract_test.go:65-78), skills materialisation (single registry entry + renderer, workspace/skills.go:38-229).

**Stressed.** Beyond the owning module, the stage sequence is re-enumerated by hand in ~12 Go-side lists plus template/config/help/doc surfaces. The repo's own HISTORY proves both the cost and the pattern: adding `code-context` (commit `2d10aba`) touched 46 files (+2001/−186) across five packages, guided by a thorough plan doc (`docs/plans/sprint-code-context-stage.md:259-445`) that itself enumerates the required surface. Drift from these parallel lists has already occurred for an existing stage (finding F2).

**Ambiguous.** Whether a new stage's artifact joins the cacheable shared prompt prefix (requirements + code-context frames, prompts.go:27-33, prompt_context.go:164-226) is a design decision that changes the blast radius: joining it touches the documented fixed prefix order (docs/architecture.md:143) and cache-key identity (prompt_bundle.go:87 `"ultraplan-sprint-v1-"`); not joining keeps locality high.

## Candidate findings

### CHANGE-01-F1
- **Priority:** P2
- **Claim:** Cumulative flow ordering is encoded as hand-maintained positional indices that must be renumbered whenever a stage is inserted, duplicating order information already owned by `PlanningStages()`.
- **Evidence:** flow.go:176-201 — `ordered := append(PlanningStages(), StageExecute)` then a literal `end` switch (1..8) per target; sibling enumerations `validateFlowTarget` (flow.go:348-353, including an error string listing all ten stages), `validatePlanningStageTarget` (flow.go:167-174), `runFlowStage` dispatch (flow.go:145-165).
- **Architectural reason:** change-surface / lifecycle.
- **Concrete consequence:** Inserting a stage mid-chain without incrementing every later literal makes `flow --to execute/review/smoke` silently run the wrong prefix (skipping or mis-ordering stages) instead of failing — durable state advances on a truncated chain. No table-driven test asserts cumulative dispatch order per target (checked `sprint_commands_test.go`, `reasoning_test.go`: only per-stage coverage).
- **Counter-evidence searched:** The constants sit adjacent to the list edit and `TestDomainStagesStatusesAndArtifactPaths` pins `PlanningStages()` exactly (sprint_test.go:16-20), forcing a domain-level test update; per-stage flows are individually tested. This bounds but does not eliminate the risk, since the failure mode is silent mis-scheduling of *other* targets.
- **Confidence:** medium-high
- **Smallest useful action:** Replace the literal `end` switch with index lookup against `PlanningStages()` (e.g., `slices.Index`+offset) so insertion shifts boundaries mechanically; add one table test asserting the cumulative stage list per target.

### CHANGE-01-F2
- **Priority:** P2
- **Claim:** App-layer presentation/fingerprint lists re-enumerate stages independently of `internal/sprint` and have already drifted: `area-reasoning` is absent from four of them, and nothing forces a new-stage author to revisit them.
- **Evidence:** `SprintSummaries` artifact lists omit area-reasoning twice (sprint_usecases.go:119-127, 193-205); validation sweep omits `StageAreaReasoning` while its own dispatcher supports it (sprint_usecases.go:206 vs :264-265); sort map lacks it (sprint_usecases.go:240); governed-operation fingerprint inputs omit `reasoning/` outputs entirely (operations.go:504-519). Sprint status JSON includes all seven stages, so summaries show the stage yet never its findings/artifacts. History confirms this predates code-context and survived three stage additions unnoticed (`git log -L` on sprint_usecases.go: f142a73 → 2d10aba added code-context to the same lists without noticing area-reasoning's absence).
- **Architectural reason:** drift / change-surface.
- **Concrete consequence:** A new governed stage added today will appear in `sprint status --json` but silently miss TUI/web artifact links, summary findings, and operation input fingerprints unless the author independently discovers each list; the same class of gap already exists for area-reasoning, where invalid artifacts produce no user-visible findings in dashboards.
- **Counter-evidence searched:** Deliberate-exclusion evidence exists partially: project page surfaces area-reasoning docs separately (project_usecases.go:61-62); multi-file `reasoning/` output does not fit the single-file `DisplayArtifact` model; `operations.go:765` keeps the area-reasoning validate case reachable via explicit web Validate. None of this explains the sweep omission, and nothing in code or docs marks the exclusions intentional.
- **Confidence:** high (drift is factual; deliberateness unresolved)
- **Smallest useful action:** Derive Go-side lists (artifact labels, validation sweep, fingerprint inputs) from `sprint.PlanningStages()`/`ArtifactRelPath` plus a small per-stage capability record, or at minimum add a test asserting every planning stage appears in `SprintSummaries` artifacts and the fingerprint inventory.

### CHANGE-01-F3
- **Priority:** P3
- **Claim:** Persisted-flow-state compatibility for stage insertion relies on a bespoke, hard-coded shim keyed to the exact pre-`code-context` shape; each future insertion stacks another named interpretation whose composition with earlier shims is untested.
- **Evidence:** state.go:74-76 triggers `interpretPreCodeContextStages` only when `len(stages) == len(PlanningStages())-1` and the exact legacy sequence matches (state.go:83-113); service.go:150 carries a parallel guard (`preCodeContextFlowState`). After inserting stage N+1, old states must pass through both interpretations to reach the new canonical shape.
- **Architectural reason:** lifecycle / boundary.
- **Concrete consequence:** `ValidateFlowState` fails closed on any length/order mismatch (state.go:307-313), so skipping the shim bricks status refresh for every existing sprint (the authoritative workspace's persisted states are verifiably 7-stage). Composed shims risk subtle status mis-derivation (e.g., double-inserted `skipped` inference) with no fixture covering a doubly-legacy state.
- **Counter-evidence searched:** The established pattern makes the next insertion cheap (copy shim, adjust detection) and deliberately avoided a schema bump to keep v2 stable — a defensible trade for one insertion; migration-on-load with fail-closed validation is otherwise coherent (state.go:53-77).
- **Confidence:** medium
- **Smallest useful action:** Generalize detection to "canonical list minus one stage" (insert as missing/skipped by predecessor status) or add a regression fixture exercising two sequential insertions before the second one ships.

## Defended architecture / rejected hypotheses

- **"The fan-out to ~20 files means ownership is broken."** Rejected. Non-Go surfaces (CLI help text sprint_commands.go:1131-1310, template select options sprint.html:40, config keys config.go:40-59/:303-341, redaction redaction.go:51-64, docs) are genuine boundary translation across different languages/contracts; a cross-cutting "stage registry" generating all of them would add indirection without removing the edits. The Go-side duplicates (F1/F2) are the real, fixable subset.
- **"Review/smoke should be folded into `PlanningStages()`."** Rejected. They have distinct durable record types, freshness/fingerprint semantics, and gates (domain.go:135-143, verify.go:49-110); folding them into the planning list would corrupt flow-state shape validation for no gain.
- **"Skills/defaults materialisation needs structural change."** Rejected — single registry entries plus auto-sync tests (workspace_test.go:136-165) make this the best-localized part of the surface; `2d10aba` confirms (init.go +10, scaffold files additive).
- **"Commit `2d10aba` bundling study runtime-summary changes proves module coupling."** Rejected as architecture: the +48 lines in study_commands.go are orthogonal progress summarisation riding along in one commit — process hygiene, not a dependency between modules.

## Open questions

- Is area-reasoning's exclusion from sprint-summary artifacts/findings intentional (project-page dedup, multi-file model)? If yes, a one-line comment or contract note would prevent a new-stage author from copying the ambiguity; if no, F2 is stronger than stated.
- Should a new stage's artifact join the shared cacheable prompt prefix? This single design decision determines whether prompts/prompt_context/prompt_bundle and the architecture doc's fixed-prefix contract (docs/architecture.md:143) are in scope; the probe cannot answer intent.
