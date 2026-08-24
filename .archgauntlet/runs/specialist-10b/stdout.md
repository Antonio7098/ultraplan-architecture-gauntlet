### Scope inspected

- **Composition root & wiring**: `cmd/ultraplan/main.go`, `internal/app/app.go` (`Run`, `dependencies`, exit classes), `surfaces.go` (runner injection to break `app↔web` cycle), `serve_commands.go`, `tui_commands.go`
- **Shared use cases**: `operations.go` (`OperationalUseCases`, `WebOperations`, `DurableOperationManager`, `PrepareOperation`/`RunOperation`), `usecases.go` (`dashboardUseCases`, preview allowlist), `web_usecases.go`, `sprint_usecases.go`, `study_usecases.go`, `project_usecases.go`, `run_usecases.go`
- **Durability plumbing**: `run_control.go` (`controlledRuntime.StartRun`, `runControlState`), `durable_operations.go` (`durableOperationManager`, `durableCLICommand`)
- **Transport adapters**: `internal/tui/app.go` (operation lifecycle, capability type assertions), `internal/web/server.go`, `operations.go` (hub), `operation_handlers.go` (`mapOperationRequest`), `import_boundary_test.go`
- **Command layer**: `run_commands.go`, `sprint_commands.go`, `study_commands.go`, `storage_commands.go`, `health_commands.go`, `code_commands.go`, plus `planningStageRuntime`/`smokeSettings` construction helpers
- **Product seams**: `internal/sprint/service.go` (value-type builder), `internal/sprint/index.go`, `internal/study/run_state.go`, `internal/runcontrol/sqlite.go` (`Accept`)
- **Authoritative docs**: `system/contracts/core/architecture.md` (ARCH-CORE-002, ARCH-ENTRY-001, ARCH-COMP-001), sprints 12/31/35 requirements, implementation `docs/architecture.md`
- Verified: `go build ./...`, `go test ./internal/app/` (pass)

### Architecture assessment

The composition design is sound and unusually disciplined for its size. `main.go` injects TUI/Web runners as functions to break the `app→web→app` cycle (documented in surfaces.go:18-19 and enforced by web/import_boundary_test.go:12-36, which permits only stdlib+`app`). One `dashboardUseCases` value type serves CLI, TUI, and web; runtime-backed work funnels through a single `sharedOperationRunner`; durability acceptance is triplicated deliberately across surfaces (CLI `durableCLICommand`, TUI `beginOperation`, web hub `startConfirmed`) with an inventory test (run_control_inventory_test.go) guarding against bypass. Typed boundaries are real: web decoders construct `OperationRequest` from explicit fields so `ExpectedFingerprint` cannot be injected (operation_handlers.go:594-601), honoring the invariant at operations.go:120-124. Lifecycle ownership is coherent: accept→claim→heartbeat→terminal arbitration with fencing, and cancellation contexts flow correctly from manager-owned cancels.

Stress points are in the *seams between duplicated machinery* rather than in direction or ownership: three surfaces implement divergent failure policy for the same journal-failure class, and two parallel implementations of coalescing/control-loop mechanics exist inside `app`.

### Candidate findings

---

**ID**: SPECIALIST-10B-F01
**Priority**: P2
**Claim**: Durable event-persistence failure semantics diverge across the three surfaces sharing one operation pipeline: TUI silently drops events and continues, web cancels the whole operation, CLI runtime commands escalate to a `persistence_lost` terminal outcome.
**Evidence**:
- TUI: internal/tui/app.go:288-291 — `RecordOperationEvent` error → bare `return` (event lost from journal *and* live stream; operation keeps running)
- Web: internal/web/operations.go:246-250 — same error → `record.cancel()` (whole operation cancelled)
- Controlled runtime: internal/app/run_control.go:156-163,197-201 sets `persistenceErr` and cancels; :282-292 proposes `TerminalPersistenceLost`
**Architectural reason**: failure-semantics (+ drift): `DurableOperationManager` (operations.go:40-44) exposes the capability but delegates the failure policy to each adapter, contradicting the stated split that "workflow semantics remain here" (operation_runner.go:15-17) and the doc claim that failed required writes "fail closed" (docs/architecture.md:163-164).
**Concrete consequence**: a TUI-started execute that survives a >5s SQLite outage (retry budget in appendRunEventWithRetry) finishes with a truthful `succeeded` terminal from `FinishOperation` but a silent journal gap and no omission record; `run show`/`run follow` present an incomplete history as complete. Identical failure on CLI yields `persistence_lost`, on web yields `cancelled`. Any future retry-policy change must be re-derived three times.
**Counter-evidence searched**: 5-second busy/unavailable retry (run_control.go:305-316) mitigates transient faults; no test pins the TUI drop behavior; no doc declares per-surface divergence intentional; web/TUI pass `context.Background()` to `RecordOperationEvent` (so retries aren't even cancellation-bounded).
**Confidence**: high (divergence is factual); medium (real-world frequency)
**Smallest useful action**: centralize the policy in app — e.g., have `RunOperation`'s emit wrapper treat `RecordOperationEvent` failure as operation cancellation (matching web), or document the TUI divergence explicitly in `docs/architecture.md` as accepted debt.

---

**ID**: SPECIALIST-10B-F02
**Priority**: P3
**Claim**: Progress-coalescing and owner control-loop mechanics exist twice inside `app` with already-drifting details.
**Evidence**: run_control.go:164-208 + control goroutine :211-261 vs durable_operations.go:131-176 + `controlOperation` :178-219. Coalescing keys differ (content hash of payload, run_control.go:177-178, vs field concatenation, durable_operations.go:137); the omission literal `"equivalent progress coalesced"` is duplicated 4× (run_control.go:190,268; durable_operations.go:159,231). Shared helpers exist only for retry/terminal (`appendRunEventWithRetry`, `proposeRunTerminalWithRetry`).
**Architectural reason**: change-surface/drift.
**Concrete consequence**: tuning `ProgressCoalesceWindow` semantics or omission vocabulary requires synchronized edits; parent-operation and child-session omission accounting can disagree for the same wall-clock work.
**Counter-evidence searched**: input shapes genuinely differ (`runtimepkg.Event` payload map vs `OperationEvent` struct), so full unification may only add indirection; retry/terminal helpers already shared, showing the team consolidates where shapes match.
**Confidence**: medium
**Smallest useful action**: extract the coalescing window (key, counters, first/last timestamps, flush-to-omission) into one small type used by both call sites; leave the control loops separate.

---

**ID**: SPECIALIST-10B-F03
**Priority**: P3
**Claim**: `webUseCases.sprintOverview` re-implements requirements.md structure knowledge that the sprint module owns.
**Evidence**: internal/app/web_usecases.go:559-581 hand-parses the `## Sprint Goal` heading (including its own `` `* `` trimming rules); authority lives in internal/sprint/index.go:42-47 (`ValidateRequirementsContent` required-headings list) and the scaffold template internal/workspace/scaffold/templates/requirements.md:7. No unit test exercises `sprintOverview` parsing (grep of `internal/app/*_test.go`: no hits).
**Architectural reason**: ownership/drift.
**Concrete consequence**: renaming the heading or restructuring the goal section updates sprint validation and the scaffold together but silently empties the web overview (returns `""`) while validation still passes — a two-place artifact-contract change that looks like one.
**Counter-evidence searched**: extraction is display-only with empty-string fallback, so blast radius is cosmetic; moving it into `sprint` adds public API surface for one consumer.
**Confidence**: high (fact), low (severity)
**Smallest useful action**: export a narrow accessor (e.g., `sprint.RequirementsGoal(content string) string`) beside `ValidateRequirementsContent` and call it from the web projection.

---

**ID**: SPECIALIST-10B-F04
**Priority**: P3
**Claim**: `storage migrate` opens its own SQLite repository directly, bypassing the process-scoped handle cache, startup reconciliation, and configured retention applied everywhere else.
**Evidence**: internal/app/storage_commands.go:57 `runcontrol.OpenSQLite(deps.ctx, root.Path, runcontrol.SQLiteOptions{})`; contrast the documented single-handle invariant (run_control.go:18-27: "prevents accidental duplicate connection pools") and the cached+retention-aware path (`runRepository`, run_commands.go:42-59).
**Architectural reason**: boundary/composition consistency.
**Concrete consequence**: harmless today in one-command-per-process CLI, but it is precisely the "accidental duplicate connection pool" the invariant exists to prevent, and migration writes run under different quota/retention options than run-control writes; invoking migration from any long-lived surface would break the invariant.
**Counter-evidence searched**: CLI never overlaps handles within a process; independence from startup reconciliation is arguably desirable for a repair command; WAL tolerates multiple connections.
**Confidence**: medium
**Smallest useful action**: route through `deps.runControl.repository(...)` (or add a comment in storage_commands.go recording why direct open is intentional).

---

**ID**: SPECIALIST-10B-F05
**Priority**: P3
**Claim**: The study runtime factory is a mutable package-level var while the sprint runtime factory flows through `app.Config` — two mechanisms for the same kind of seam, and the var contradicts the repo's own "no package-global mutable registry" statement.
**Evidence**: internal/app/study_commands.go:22-24 (`var studyRuntimeFactory = ...`), swapped by tests (study_run_commands_test.go:195, study_status_commands_test.go:90, study_validate_commands_test.go:81); contrast `SprintRuntimeFactory` injection (app.go:38, sprint_commands.go:20-23); docs/architecture.md:22-23.
**Architectural reason**: composition consistency/change-surface.
**Concrete consequence**: test-seam asymmetry means the sprint seam is overridable per-`Run` call while the study seam is process-global; adding `t.Parallel` to `internal/app` tests or embedding the binary elsewhere makes swaps racy.
**Counter-evidence searched**: no `t.Parallel` in the package today; every swap restores the prior value; confined to one internal package.
**Confidence**: high (fact), low (impact)
**Smallest useful action**: add `StudyRuntimeFactory` to `app.Config` alongside `SprintRuntimeFactory` and default both in `Run`.

---

**ID**: SPECIALIST-10B-F06
**Priority**: P3
**Claim**: Runtime-service construction is triplicated, and each copy independently re-loads effective config, so a single `sprint flow` command parses configuration three times.
**Evidence**: `sprintRuntimeService` (sprint_commands.go:477-499, reloads config despite callers holding `effective`), `runLoopService` (study_commands.go:307-337), `runAllService` (study_commands.go:652-676) are near-identical (load config → resolve parallelism → build runtime request → factory → `controlledRuntimeFor`); `runSprint` loads config at :66-69, again inside `beginDurableCLICommand`→`runRepository` (run_commands.go:50), again in `sprintRuntimeService`.
**Architectural reason**: change-surface.
**Concrete consequence**: adding a collaborator (new `With*` option, new config-derived setting) requires touching three helpers; triple validation-error surfaces for the same command.
**Counter-evidence searched**: outputs legitimately differ (`ConfigSummary`, parallelism resolution); env is static within a process so no behavioral drift; cost negligible.
**Confidence**: high (fact), low (severity)
**Smallest useful action**: pass the already-loaded `config.Effective` into the constructors instead of reloading; optionally fold the two study variants together.

### Defended architecture / rejected hypotheses

- **"Two durable runs per interactive operation is a duplicate workflow engine."** Rejected. Parent (operation-kind target, `durableOperationManager.AcceptOperation`, durable_operations.go:82-122) and child (per agent session, `controlledRuntime.StartRun` → `repository.Accept`, which always allocates a fresh run id, sqlite.go:378-440) are distinct lifecycle layers by design; docs/architecture.md:162-165 states "Direct CLI commands, TUI actions, web operations, and individual runtime children share that boundary." Explicit parent/child identifiers are an acknowledged open question in sprint 35 requirements (line 56) — FUTURE-INTENT, not a current defect.
- **"`webUseCases` wrapping `dashboardUseCases` duplicates use-case state."** Rejected. Both instances built in `runServe` carry identical fields including `readOnly: true` (serve_commands.go:47-51; web_usecases.go:245-252); `dashboardUseCases` holds only immutable config, so the standalone instance captured by the runner closure (used solely for `PreviewArtifact`, operation_runner.go:85) cannot diverge.
- **"TUI type assertions on `OperationalUseCases` are untyped boundary leakage."** Rejected. They are capability probes enabling narrower test fakes and nil-safe degradation (`ErrWebUnavailable` guards, usecases.go:72-119); production wiring always installs both capabilities (tui_commands.go:42-48).
- **"`internal/web` importing `internal/app` DTOs couples transport to business logic."** Rejected. It is the intended inward direction (ARCH-CORE-002), mechanically enforced (import_boundary_test.go forbids everything except stdlib+app), and the app result types are display projections (`DisplayFinding`, redaction via `displaySafe`), not domain entities.
- **"`RunUseCases` type aliases leak `runcontrol` through the boundary."** Rejected as harmful: the aliasing is documented (run_usecases.go:26-28, "DTOs contain only sanitized facts owned by run control") and avoids a second DTO translation layer with zero sanitization gain.
- **"Read-only dashboards mutate state via `study.ReconcileRunState`."** Checked and disproved: it mutates only the passed in-memory struct (study/run_state.go:67-100); no file writes.
- **"TUI `readOnly=false` vs web `readOnly=true` is an inconsistency."** Rejected: intentional — TUI help documents that refresh may recompute flow-state.json (tui_commands.go:78-84) while the web is a read-only dashboard; `RefreshMayWrite` plumbing reflects it (sprint_usecases.go:115-117).

### Open questions

- Should parent operation runs (accepted via `DurableOperationManager`) ever propose `TerminalPersistenceLost` like controlled child runs do? An affirmative answer would upgrade F01's fix from "align adapters" to "extend the manager."
- Whether sprint 35 follow-up work intends to unify the coalescing/omission vocabulary across the two `app` implementations — this would set F02's priority.
