# SPECIALIST-23B — Simplicity and earned complexity (independent b)

## Scope inspected

- Repo: `ultraplan-go` @ eeaa034 (clean). Packages: `internal/app` (all files incl. `operations.go`, `operation_runner.go`, `usecases.go`, `run_usecases.go`, `web_usecases.go`, `run_control.go`, `serve_commands.go`, `tui_commands.go`, `sprint_commands.go`, `study_commands.go`, `durable_operations.go`), `internal/sprint` (`service.go`, `review.go`, `smoke.go`, `smoke_author.go`, `state.go`, `execute_state.go`, `flow.go`, `index.go`, `runtime_metrics.go`, `cleanup_uncertain.go`), `internal/study` (`service.go`, `run_loop.go`, `state.go`, `summary.go`, `cleanup_uncertain.go`), `internal/runcontrol` (`interfaces.go` + symbol usage), `internal/platform/{runtime,config,filesystem,process}`, `internal/web` (`operations.go`, `operation_handlers.go`), `internal/tui` (`model.go`, `app.go`), `cmd/ultraplan/main.go`.
- Workspace docs: `system/contracts/core/architecture.md`, `projects/ultraplan-go/docs/ARCHITECTURE.md`, `docs/TRD.md` §18.2, `roadmap.md`, sprint plans 01/08/12.
- Commands: `go build ./...`, `go test ./...` (all green); history traces (`git log -S`, `git show`) for smoke wiring; repo-wide grep for interface usage.
- Interface census of all Go `interface{...}` declarations and their references.

## Architecture assessment

The module shape is largely sound by the repo's own doctrine. `internal/sprint` (~18k lines) is not a god module: it is exactly the bounded context ARCHITECTURE.md assigns it ("sprint planning, execute, review, and smoke behavior should stay with the sprint module"), fronted by a deep value-type `Service`; `review.go`'s 1768 lines are one stage's manifest/coverage/resume/render machinery behind four public entry points. `internal/platform/runtime` is a justified translation layer over agentwrap; `controlledRuntime` is a deep decorator adding durable Accept/Claim/fenced events without leaking SQL upward. Interfaces are overwhelmingly consumer-side single-method ports (`sprint.Runtime`, `study.Runtime`, `Clock`, `ProcessProbe`), which ARCH-LAYER-002 explicitly endorses.

The stress point is **service-construction knowledge duplicated per surface** inside `app`: the recipe "loadEffectiveConfig → RequestFromConfig → runtime factory → controlledRuntimeFor → NewService(+stage runtime/review concurrency/smoke settings)" exists in at least five variants (`sprint_commands.go:81`, `:498`, `operation_runner.go:75`, `study_commands.go:307/:648/:824`, `web_usecases.go:336/:359`), and one variant has already drifted into a functional defect (F001). Secondary ceremony exists around dead/aliased interfaces and a triplicated dispatch table.

## Candidate findings

---

### SPECIALIST-23B-F001
**Priority:** P1

**Claim:** `sharedOperationRunner`'s smoke-start case builds the sprint service without a runtime, so "Run Smoke [EXTERNAL]" from the TUI and web always fails at smoke authoring; the CLI path was fixed for this exact requirement but the shared runner was missed.

**Evidence:**
- `internal/app/operation_runner.go:74-75`: `case OperationSmokeStart: service := sprint.NewService(root.Path).WithSmokeSettings(...)` — no `WithRuntime`, unlike every sibling case (`:23`, `:36`, `:49`, `:60`, `:93` use `sprintRuntimeService`).
- `internal/sprint/smoke_author.go:21-23`: `if s.runtime == nil { return smokeError("smoke_author_runtime", ...) }`.
- `internal/sprint/smoke.go:59-64`: authoring is unconditional for non-dry-run smoke.
- Reachability: TUI `internal/tui/model.go:485-486` → `internal/tui/app.go:286 RunOperation` → `internal/app/operations.go:399-403` default→runner; web `internal/web/operation_handlers.go:657-665` → same runner wired at `internal/app/serve_commands.go:63`. Production chain starts at `cmd/ultraplan/main.go:27-30`.
- History: `f142a73` introduced the bare construction; later `b9733ce` ("Add agent-owned stage execution and smoke authoring") added the runtime requirement and patched only the CLI path (`git show b9733ce -- internal/app/sprint_commands.go` swaps in `sprintRuntimeService`).

**Architectural reason:** drift / change-surface — one product operation's service-wiring knowledge lives in two independently edited places.

**Concrete consequence:** every TUI/web smoke start fails after durable acceptance (`AcceptOperation` has already recorded a run), returning "runtime is required to author deep-smoke coverage" despite a configured runtime; each attempt leaves a failed durable run record. Untested: no test references `OperationSmokeStart` through the runner; web/TUI fakes implement `RunOperation` directly (`internal/tui/test_fakes_test.go:28`).

**Counter-evidence searched:** default runtime in `NewService` (none, `service.go:63-65`); alternate injection via `SmokeSettings` (none, `smoke_types.go:44-50`); skip-authoring branch when a harness already exists (none); a different web Runner (serve uses `sharedOperationRunner`); intentional disablement docs (none found).

**Confidence:** high

**Smallest useful action:** in `operation_runner.go:74`, construct via `sprintRuntimeService(deps, root)` (as sibling cases do) plus `.WithSmokeSettings(...)`, and add one test driving `OperationSmokeStart` through `sharedOperationRunner` with the fake runtime factory already used in `app_test.go:285`.

---

### SPECIALIST-23B-F002
**Priority:** P2

**Claim:** The crash-safe atomic-write protocol (same-dir temp → write → sync → close → optional BeforeRename hook → rename → dir-sync) is implemented six times across two modules, with the hook type and `syncDir` also duplicated — mechanical filesystem knowledge the authoritative TRD says must be extracted once two modules share it.

**Evidence:**
- Copies: `internal/sprint/state.go:239` (`saveFlowStateWithHooks`), `internal/sprint/execute_state.go:132`, `internal/sprint/review.go:1683-1716` (`atomicWriteReviewWithHooks`), `internal/sprint/smoke.go:692`, `internal/study/state.go:73`, `internal/study/summary.go:94`.
- Duplicated support: `atomicWriteHooks` (`sprint/state.go:16`, `study/state.go:23`, identical), `syncDir` (`sprint/state.go:374`, `study/state.go:177`, identical).
- Contract: `TRD.md` §18.2 lists "filesystem helpers that are genuinely cross-module, such as atomic writes" as sanctioned reuse and states "If sprint and study both need the same mechanical file operation, extract the file operation"; `ARCHITECTURE.md:655` concurs; `roadmap.md:198` reserves `internal/platform/filesystem`, whose `doc.go` documents exactly this deferred boundary. Dependency direction `sprint/study -> platform/filesystem` is pre-approved (ARCHITECTURE.md:539-548).

**Architectural reason:** drift / change-surface on a durability-critical primitive.

**Concrete consequence:** any durability fix (rename semantics, permission handling, error wrapping, fsync policy) must be replicated in six sites; copies already diverge cosmetically (temp patterns, error text, hook presence), and a future edit to one copy silently leaves five stale — precisely the class of latent bug seen in F001.

**Counter-evidence searched:** semantic differences justifying separate implementations (none — all six perform identical steps); contract language permitting local copies ("implement locally first… extract after second concrete use") — that threshold was crossed when sprint gained flow-state persistence (sprint plans 08/12); tests pinning per-copy behavior exist but pin the *current* copies, not the duplication.

**Confidence:** high

**Smallest useful action:** move one `WriteFileAtomic(path string, data []byte, hooks)` (+`SyncDir`) into the already-reserved `internal/platform/filesystem` and delegate the six sites, keeping marshal/validate logic local.

---

### SPECIALIST-23B-F003
**Priority:** P3

**Claim:** `runcontrol.Control` is a dead alias interface — defined as exactly `Repository` and referenced nowhere.

**Evidence:** `internal/runcontrol/interfaces.go:67-72`; repo-wide grep for `Control`/`runcontrol.Control` finds zero uses outside the declaration (source and tests).

**Architectural reason:** speculative interface (ceremony) — violates contract principle 7 ("do not introduce ports… speculatively").

**Concrete consequence:** readers must reason about a supposed adapter-neutral seam that does not exist; if decoration is later needed, the alias would also force identical method sets rather than narrow capabilities.

**Counter-evidence searched:** test doubles implementing it (none); planned decorator mentioned in its comment ("permits app composition to decorate") — no such decoration exists; code generation or docs referencing it (none).

**Confidence:** high

**Smallest useful action:** delete the `Control` type (or use it where `Repository` is currently injected in `app/run_control.go:35` if the distinction is intended).

---

### SPECIALIST-23B-F004
**Priority:** P3

**Claim:** The same concept — "build the agent runtime from config" — is wired through two different mechanisms: injectable `dependencies.sprintRuntimeFactory` for sprint, and a mutable package-level `var studyRuntimeFactory` for study.

**Evidence:** `internal/app/app.go:38,119-123` + `sprint_commands.go:21-25` vs `internal/app/study_commands.go:22-25`; both wrap `runtimepkg.NewOpenCode`. Tests swap the global with save/restore (`study_run_commands_test.go:194-198`, `study_status_commands_test.go:89-94`, `study_validate_commands_test.go:80+`) while sprint tests inject (`app_test.go:285`).

**Architectural reason:** duplicated knowledge / lifecycle — two ownership stories for one seam in one package.

**Concrete consequence:** parallel tests mutating the package global race on it; future runtime-wiring changes (e.g., the F001 fix) must be discovered and applied in both mechanisms; new collaborators can't tell which seam is authoritative.

**Counter-evidence searched:** a reason study needs process-global substitution (none found); ordering guarantees preventing races in current tests (tests do restore via defer, but nothing prevents `t.Parallel` collisions).

**Confidence:** medium-high

**Smallest useful action:** add `StudyRuntimeFactory` to `dependencies` mirroring `SprintRuntimeFactory`, defaulting to `NewOpenCode`, and update the three test files.

---

### SPECIALIST-23B-F005
**Priority:** P3

**Claim:** The web layer re-implements parsing of the governed `requirements.md` format ("## Sprint Goal" heading scan) that the sprint module owns and validates.

**Evidence:** `internal/app/web_usecases.go:559-581` (`sprintOverview` hand-scans markdown headings, swallows errors) vs `internal/sprint/index.go:35-50,138-146` (`ValidateRequirementsContent`/`markdownHeadingPresent` define the section grammar).

**Architectural reason:** boundary / duplicated knowledge of an artifact schema across modules.

**Concrete consequence:** if the requirements template evolves (heading renamed/nested), sprint validation and the web overview drift silently — the overview degrades to an empty string while validation passes, and no test fails.

**Counter-evidence searched:** TRD §18.2 explicitly permits local-first implementations until a second concrete use proves stability — that tolerance caps this at P3; app already imports `sprint`, so no dependency obstacle exists; the goal text is not currently carried in `SprintSummary`.

**Confidence:** medium

**Smallest useful action:** export a tiny accessor (e.g., `sprint.SprintGoal(content) string`) or populate the goal once where sprint reads requirements, and delete the app-side parser.

---

### SPECIALIST-23B-F006
**Priority:** P3

**Claim:** The planning-stage → prompt-method dispatch table exists in three copies within `app`, already drifted (smoke handled differently per copy).

**Evidence:** `internal/app/operations.go:674-697` (`promptSprintStage`), `internal/app/sprint_commands.go:176-197` (CLI `prompt`, inline switch), `internal/app/web_usecases.go:511-539` (`PromptBundle`, adds smoke-specific unavailable-reason). Adding a stage requires editing all three; the CLI and `promptSprintStage` reject `smoke` as unsupported while web explains why it is unavailable.

**Architectural reason:** change-surface duplication of one mapping owned by one layer.

**Concrete consequence:** next stage addition (or stage rename) updates some surfaces and not others; inconsistent user-facing explanations for the same gap (usage error vs rationale).

**Counter-evidence searched:** legitimate divergence — web needs per-stage scope annotations and CLI needs exit-code mapping — but those wrap the shared switch rather than requiring a third copy; `promptSprintStage` is already exported within the package and unused by the other two sites.

**Confidence:** medium

**Smallest useful action:** have the CLI and `PromptBundle` call `promptSprintStage` and attach their surface-specific scope/error mapping around its result.

---

## Defended architecture / rejected hypotheses

- **`repositoryRunUseCases` pure forwarding (`run_usecases.go:37-57`) is not ceremony.** It deliberately narrows the 15-method `Repository` to the five observation/command methods transports may call, hiding `Accept/Claim/Append/Heartbeat` command-side authority. Removing it would widen the transport-facing surface against the file's own stated contract.
- **Nil-guard forwarders in `dashboardUseCases`/`webUseCases` (~20 delegation methods) — investigated, defended.** The same struct serves minimal compositions (`NewReadOnlyUseCases`, `NewOperationalUseCases`) and full serve/TUI compositions; the guards convert absent optional capabilities into typed `ErrWebUnavailable` instead of nil-interface panics. Collapsing them moves the same checks into handlers.
- **`internal/sprint` size is earned depth, not a god module.** One bounded context per ARCHITECTURE.md:21,278; consumers touch a small facade; intra-package files split by stage. The duplicate consumer-side `Runtime` ports (`sprint/flow.go:13`, `study/service.go:25`) match ARCH-LAYER-002's "ports near the consumer".
- **study vs sprint run-loop similarity is contract-defended.** ARCHITECTURE.md:655 forbids a global scheduler/workflow package and instructs keeping similar product workflows in owning modules; mechanics differ (sequential session queue vs bounded parallel workers with retry scheduling). I did not flag a generic engine here.
- **Product schema inside `platform/config` — investigated, not flagged.** ARCH-SHARED-001 prefers domain-neutral platform code, but the authoritative docs assign "config loading, precedence, redaction" to `platform/config` and allow all modules to depend on it (ARCHITECTURE.md:539-548, 634); splitting parser from schema now would add indirection against documented intent.
- **`platform/runtime` Adapter over agentwrap** is mandated boundary translation (ARCHITECTURE.md:453), not a pass-through: it adds cancellation waits, event retention/coalescing, and error mapping.

## Open questions

1. Is TUI/web smoke-start intentionally disabled pending a browser-safe authoring runtime? No doc, changelog, or test evidence found; if intentional, F001 downgrades to a missing guard with clearer UX than a runtime error mid-operation.
2. Is `internal/platform/filesystem` scheduled to receive the atomic-write helper in an upcoming sprint (roadmap reserves the package)? If yes, F002 becomes an execution-timing item rather than a design gap; the extraction target and dependency direction are already agreed.
3. Does any external tooling reference `runcontrol.Control` (docs/codegen) that repo grep wouldn't reveal? If not, deletion is trivially safe today.
