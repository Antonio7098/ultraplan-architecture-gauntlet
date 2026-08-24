### Scope inspected

- **Implementation repo** (`ultraplan-go` @ `eeaa034`, clean): `internal/sprint/{domain,flow,state,state_database,artifacts,service,prompts}.go`; `internal/runcontrol/{model,interfaces,migration}.go`; `internal/platform/runtime/{runtime,opencode}.go`; `internal/platform/config/{config,redaction}.go`; `internal/workspace/{defaults,skills,init}.go`; `internal/app/{operations,durable_operations,run_control,sprint_commands,sprint_usecases,web_usecases,surfaces,health_commands}.go`; `internal/web/{operations,import_boundary_test}.go`, `templates/sprint.html`; `internal/tui/model.go`; import graph greps across all non-test files.
- **History probes**: `git show 2d10aba` (real "add a governed stage" change, 46 files), ancestry of `7a94a11` (TUI) vs `2d10aba` (code-context).
- **Workspace** (@ `368a789`): `projects/ultraplan-go/docs/{ARCHITECTURE,PRD}.md`, `roadmap.md`, `sprints/33-code-context-stage/requirements.md`, `sprints/34-shared-context/requirements.md` + review, `ultraplan.yml`.
- Verification: `go build ./...`, `go vet ./...`, targeted `go test ./internal/{sprint,app,tui,web}` — all green.

### Architecture assessment

The core evolution story is sound and repeatedly proven. Schema versions absorb change well (`FlowStateSchemaVersion=2` with v1 migration, legacy map-format detection, and a mid-version stage-insertion interpreter at `state.go:53-113`; `runcontrol/migration.go:25-82` adds locked, backed-up, integrity-checked forward migration). Dependency direction is machine-enforced where it matters (`runcontrol/import_boundary_test.go`, `web/import_boundary_test.go`); `sprint` does not import `runcontrol` or `study`. Runtime adapter boundaries are earned abstractions: products consume `runtime.Runtime` (`runtime.go:245-249`) and OpenCode specifics sit in one constructor (`opencode.go:14`). Prompt override resolution cleanly splits mechanical policy (`workspace.DefaultOverrideFile`, `prompts.go:253`) from module semantics. `DeriveStages` (`service.go:1340`) is data-driven over `PlanningStages()`.

The stress point is **stage enumeration outside `internal/sprint`**: the ordered stage list is re-declared by hand in each surface (TUI nav array, web template `<option>` list, three app dispatch switches, two app lists, a config struct/map pair), with no parity contract tying them back to `PlanningStages()`. The one real "add a governed stage" probe (code-context) executed through 46 files across 8 packages — and missed one of the three interface surfaces entirely.

### Candidate findings

#### SPECIALIST-24A-F01
- **Priority:** P1
- **Claim:** Adding a governed sprint stage requires parallel hand-edits in every surface, and the architecture has no enforcement or even a test tying surfaces to the canonical stage order; this already failed once — the TUI never received the code-context stage added by Sprint 33.
- **Evidence:**
  - `internal/tui/model.go:453`: `stages := []string{"requirements", "sprint-index", "technical-handbook", "area-reasoning", "reasoning", "plan", "execute", "review"}` — no `code-context`; drives Validate / Preview-Prompt / Flow-to nav items (lines 455–466). Artifact nav labels at `model.go:441-451` also lack code-context.
  - `internal/tui/model_test.go:81` asserts the stale 8-item list, cementing the drift.
  - Commit `2d10aba` ("Add code context planning stage", 2026-08-20, after TUI commit `7a94a11` 2026-07-09) touched 46 files including web templates/CSS/JS, but zero files under `internal/tui/`.
  - Web replicates the order by hand too: `internal/web/templates/sprint.html:40` (hardcoded `<option>` chain), plus code-context special cases at `:82,:98`; no view model supplies stage targets.
  - App-side re-declarations: `operations.go:676-699` (`promptSprintStage` switch), `web_usecases.go:511-539` (`PromptBundle` switch), `sprint_usecases.go:254-279` (`validateSprintStage` switch), `sprint_commands.go:606-650` (`planningStageRuntime` map), config keys in `config.go:42,123,144,307` + `redaction.go:52` + `workspace/init.go:150`.
  - CURRENT-CONTRACT: `ARCHITECTURE.md:23` — Phase 5 adds code-context "surfaced uniformly through all three interfaces"; PRD line 57 and `sprints/34-shared-context/requirements.md:52` ("CLI, TUI, and browser continue to agree on code-context readiness…").
- **Architectural reason:** change-surface / drift — interface parity rests on developer memory instead of a shared authority (`sprint.PlanningStages()` is consumed only inside `internal/sprint`).
- **Concrete consequence:** TUI operators cannot validate, preview, or flow to code-context while CLI and browser can; every future stage insertion repeats the same multi-surface lottery, and the surface missed will be silent (no error anywhere — the nav item simply doesn't exist).
- **Counter-evidence searched:** No doc records TUI code-context support as deferred (Sprint 33 Non-Goals defer only the skill and docs; Sprint 34 asserts tri-surface agreement as delivered). TUI commit `d1e8b45` ("expose all sprint operations in tui") shows intent of completeness, so the short list is not a curated subset. Sprint 34 review verified ordering tests in sprint/web packages but contains no TUI-parity check.
- **Confidence:** high
- **Smallest useful action:** Add `code-context` to `tui/model.go:453` (and artifact label list), then replace the literal arrays with iteration over one exported ordered stage list surfaced through `app` (e.g., from `SprintSummary.Stages`/a `sprint.PlanningStages()` accessor), plus one parity test asserting every canonical stage appears in TUI nav and web flow-target options.

#### SPECIALIST-24A-F02
- **Priority:** P2
- **Claim:** The dashboard/web validation-findings sweep omits exactly one supported stage — area-reasoning — from its hand-maintained list, so its findings never reach shared summaries.
- **Evidence:** `internal/app/sprint_usecases.go:206` iterates `{Requirements, CodeContext, SprintIndex, TechnicalHandbook, Reasoning, Plan, Execute, Review, Smoke}`; `validateSprintStage` (`:264-265`) supports `StageAreaReasoning` via `service.ValidateAreaReasoning` (`service.go:323-346`), which degrades gracefully when no templates are selected. The list has been edited repeatedly (+CodeContext, +Review/Smoke) without ever including AreaReasoning; the list predates code-context (`git show 7a94a11` had `{Requirements…Execute}`).
- **Architectural reason:** drift — the same enumeration-without-authority pattern as F01, already divergent inside a single package.
- **Concrete consequence:** invalid or missing area-reasoning artifacts are invisible in `SprintSummaries` → dashboard AttentionFindings and the web sprint page until someone runs `sprint validate area-reasoning` manually, undermining the "shared use cases expose findings" contract used by all three surfaces.
- **Counter-evidence searched:** No comment, test, or review note justifies exclusion; cost/noise arguments don't hold (validator handles the skipped/no-templates case, and strictly more expensive stages like smoke are swept).
- **Confidence:** medium (omission is fact; intent unproven)
- **Smallest useful action:** Add `sprint.StageAreaReasoning` to the sweep at `sprint_usecases.go:206`, or drive the sweep from the same shared stage list introduced in F01.

#### SPECIALIST-24A-F03
- **Priority:** P3
- **Claim:** Durable progress-coalescing and owner-control loop policy is implemented twice inside `app`, once per entry path, with already-divergent mechanics.
- **Evidence:** `run_control.go:151-209` (content-hash coalescing keyed on payload hash) vs `durable_operations.go:124-176` (field-concatenation key); heartbeat/cancellation/reconcile poll loops at `run_control.go:211-261` vs `durable_operations.go:178-219`; identical "persistence failure ⇒ cancel + propose `persistence_degraded`" epilogue at `run_control.go:282-292` vs `durable_operations.go:106-114`; both flush omissions on finish (`run_control.go:263-275`, `durable_operations.go:229-237`).
- **Architectural reason:** change-surface — lifecycle policy (coalescing window semantics, liveness reactions, terminal proposals) has two owners inside the composition layer.
- **Concrete consequence:** any policy change (e.g., coalescing window behavior, reaction to snapshot errors) must be replicated in both paths or CLI-started runtime runs and web/TUI operations silently acquire different durability semantics.
- **Counter-evidence searched:** The two layers translate different event vocabularies (`runtime.Event` vs `OperationEvent`), so full unification would add indirection; only the policy skeleton, not the mapping, is genuinely duplicated, hence P3.
- **Confidence:** medium
- **Smallest useful action:** Extract the coalescing bookkeeping (key check, omission accumulation, finish-flush) into one small helper in `app` used by both call sites; leave the mappings separate.

#### SPECIALIST-24A-F04
- **Priority:** P3
- **Claim:** `governedOperationInputs` re-enumerates sprint artifact filenames instead of deriving them from the owning module's path authority.
- **Evidence:** `internal/app/operations.go:504-518` hardcodes `requirements.md`, `code-context.md`, … duplicating `sprint.ArtifactRelPath` (`artifacts.go:11-37`). Line 512 shows code-context required a remembered manual addition during Sprint 33.
- **Architectural reason:** drift / change-surface — the stale-operation fingerprint's input coverage depends on a second, unlinked copy of the artifact map.
- **Concrete consequence:** a future planning-stage artifact forgotten in this list will mutate without invalidating prepared operations, weakening the `ErrStaleOperation` guard for exactly the newest stages.
- **Counter-evidence searched:** Excluding execute/review/smoke outputs is defensible (fingerprint covers governed upstream inputs, not stage outputs), and the canonical request JSON catches request-shape changes — so this is coverage erosion risk, not a live bug; P3.
- **Confidence:** medium
- **Smallest useful action:** Build the planning-input list from `sprint.ArtifactRelPath(sp, stage)` over the shared stage list rather than string literals.

### Defended architecture / rejected hypotheses

- **"Ordinal-based `flowStages` (`flow.go:182-199`) and the success-chain builders (`flow.go:243-302`) are dangerous duplication."** Rejected as a defect: all sites are local to `flow.go`, the ordering is fail-fast enforced by `ValidateFlowState` (`state.go:307-314` exact length/order checks), and the noTemplates conditionals make table-driven replacement partly imaginary. Folded into F01's shared-list remedy only where it crosses package lines.
- **"`runtime.default: opencode` pinning plus four `NewOpenCode` sites blocks a second runtime adapter."** Rejected as current defect: config validation (`config.go:409-410`) makes unsupported values fail loudly, the `Runtime` interface (`runtime.go:245`) is the sole product-facing seam, and per ARCHITECTURE.md the adapter exists "if needed". Second-adapter work is additive FUTURE-INTENT, though health's direct construction (`health_commands.go:113`) should join whatever dispatch point emerges.
- **"Sprint `LatestOutcome`/`AttemptStatus` vocabularies duplicating `runcontrol` terminal outcomes violate single authority."** Rejected: `sprint` deliberately does not import `runcontrol` (verified imports of `service.go`); the mirrored value sets are necessary boundary translation between independent product state and the operational boundary, matching ARCHITECTURE.md's "records execution facts; does not replace sprint flow state".
- **"Event-type additions ripple everywhere."** Rejected: durable `EventType` consumers are permissive (web passes type/payload through as strings, `run_handlers.go:282-293`; SSE names fall back to `"progress"`, `web/operations.go:470-473`), so probe 4's edit surface is `model.go` + the single translation switch `runtimeEventDraft` (`run_control.go:377-399`). Good locality.

### Open questions

- Was TUI code-context support consciously deferred to an unbudgeted later sprint anywhere outside the documents inspected (plans under `docs/plans/` were referenced but not exhaustively read)? Documentation of that decision would reclassify F01 from realized drift toward accepted debt — the missing parity mechanism remains either way.
- Is the area-reasoning sweep exclusion (F02) a deliberate presentation choice recorded in reasoning notes for sprints 25/33? Nothing in requirements, plan, or review states it.
