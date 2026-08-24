runcontrol has fenced CAS on both cancel (`RequestCancellation`) and terminalization (`ProposeTerminal`). Now I need the application layer where these race: who calls each, and how product stages get terminalized.
This is the core seam: `controlledRuntime.StartRun` (internal/app/run_control.go:122) owns accept→claim→watch→terminalize. Now I need the callers that terminalize *product* state (flow state / execute tasks / study loop) and the web-side cancel path.
Study loop cancels via context only. Now the execute stage (task-level terminalization) and the web hub's cancel path.
Execute marks tasks via ordered switch — note `ctx.Err()` outranks a successful run's evidence (execute.go:286-303). Now the web hub cancel path and app runner wiring to see who owns cancellation authority at runtime.
Two parallel owner implementations exist (`controlledRuntime.StartRun`, `durableOperationManager`). Now the platform runtime: does ctx cancellation reliably map process kills to `context.Canceled`, and how does `Result.Status` get set?
The runtime adapter maps kills to `context.Canceled` reliably (runtime.go:308-325, 341-349). Now I've spotted a suspect in `controlledRuntime.StartRun`: the final omission flush and the ack path interact badly with cancellation. Let me verify counter-evidence: tests covering this race, the study runtime wiring, and productstate.
Tests cover happy-path terminalization but not the cancellation race windows. Let me verify study/run.go's cancellation mapping and the web handler fallback, then check productstate.
Key counter-evidence: runcontrol tests explicitly defend completion-wins-over-cancellation. Let me read them.
Before writing findings, I must check the authoritative planning workspace for contracts on cancellation/terminal semantics.
Contracts confirmed: Sprint 35 requires truthful terminal outcomes and health that distinguishes storage failure from cancellation facts. Let me re-verify exact line numbers for citations before writing the report.
# FAILURE-07 — Cancellation racing runtime/process completion and product-stage terminalization

## Scope inspected

Implementation repo `/home/antonioborgerees/coding/ultraplan/gauntlet-run/ultraplan-go` @ eeaa034 (read-only):

- `internal/runcontrol/`: `model.go`, `sqlite.go`, `lifecycle.go` (full); tests `lifecycle_test.go`, `fault_test.go`, `process_integration_test.go` (targeted)
- `internal/app/`: `run_control.go` (full), `durable_operations.go` (full) + both test files, `operations.go`, `web_usecases.go`
- `internal/web/`: `operations.go` (hub, full), `operation_handlers.go` (cancel paths)
- `internal/sprint/`: `service.go` (flow functions, `DeriveStages`, `flowFailedStages`), `execute.go` (full), `runtime_metrics.go`, `domain.go` (status vocabulary)
- `internal/study/`: `run_loop.go` (full), `run.go` (execution/classification)
- `internal/platform/runtime/runtime.go` (adapter cancel mapping); `internal/productstate/store.go` (ownership check)

Authoritative workspace docs: `projects/ultraplan-go/docs/TRD.md` (lifecycle/cancellation/shutdown contracts), `docs/ARCHITECTURE.md` (shutdown ownership), `sprints/35-durable-run-observability/requirements.md`.

## Architecture assessment

The terminalization spine is sound. Every runtime-backed execution is wrapped by one of two fenced owners — `controlledRuntime.StartRun` for sprint/stage/study runtime runs (internal/app/run_control.go:122) and `durableOperationManager` for web/CLI operations (internal/app/durable_operations.go:82) — over a single arbitration substrate: `ProposeTerminal` CASes on `current_attempt_id + terminal_outcome IS NULL` (internal/runcontrol/sqlite.go:769-786), so exactly one outcome wins regardless of proposer (owner, reconciler, shutdown). Completion-wins-over-cancellation is explicit contract ("a failure/completion that was already authoritative before cancellation won", TRD.md:2132) and is tested (internal/runcontrol/lifecycle_test.go:80-100,119-174). Cancellation reaches the owner via a 1s control-loop poll that acknowledges then cancels the derived context; the runtime adapter deterministically maps kills/timeouts to `context.Canceled` / category `"cancellation"` (internal/platform/runtime/runtime.go:308-349). Product-stage truth stays package-owned (flow-state, execute-run-state, study run-state) and is revalidated before trust, so no corruption path was found — the stress points below are about *which truthful label* survives the race, not about lost authority.

One structural observation, not reported as a defect: the two owners duplicate the watch loop (heartbeat + cancellation poll + reconcile) twice with different error policies (see F1). The duplication is currently where the failure-semantics divergence lives.

## Candidate findings

### FAILURE-07-F1

- **Priority:** P1
- **Claim:** In `controlledRuntime.StartRun`, cancellation-induced write failures are classified as persistence failures and terminalize user-cancelled runs as `persistence_degraded` instead of `cancelled`/`succeeded`, corrupting the truthful-outcome contract in exactly the completion/cancellation race Sprint 35 requires tests for.
- **Evidence:**
  - Window A: after `base.StartRun` returns (run_control.go:262), the coalesced-progress omission flush is issued on `runCtx` (run_control.go:265) — already dead if the watch loop acknowledged a durable cancellation and called `cancel()` (run_control.go:237), or if the parent operation context was cancelled concurrently. `Append` fails with `context.Canceled`, which `classifyStoreError` deliberately passes through unclassified (internal/runcontrol/sqlite.go:1100-1102) and which `retryableRunControlError` does not retry (run_control.go:330-332). That sets `persistenceErr` (run_control.go:272-274).
  - Window B: the `AcknowledgeCancellation` error path has no `runCtx.Err()` guard, unlike its three sibling paths in the same select (`Snapshot` :225-227, `Heartbeat` :241-243, `Reconcile` :251-253 all check before calling `setPersistenceErr`) — run_control.go:232-235.
  - Either window routes to the dedicated branch proposing `TerminalPersistenceLost` / "durable event persistence failed" on a detached context (run_control.go:282-292); `terminalOutcome` (which would have produced `cancelled` or `succeeded`, run_control.go:625-637) is never reached.
- **Architectural reason:** failure-semantics / lifecycle — the app-layer owner mislabels the cause of a journal-write abort, and the durable record is the system of record other surfaces reconcile against.
- **Concrete consequence:** A user cancellation that races completion produces `lifecycle=persistence_degraded`, `terminal_reason="durable event persistence failed"` plus a dangling `cancellation_state=requested|acknowledged`. Operators responding to degraded storage signals chase phantom disk/SQLite incidents (Sprint 35 explicitly requires health to "distinguish storage failure … and cancellation uncertainty", requirements.md "Operational telemetry" row; TRD.md:2132 requires "one truthful terminal outcome"). On the pure-durable-cancel path (parent ctx untouched) the product layer additionally diverges: `Execute` sees `runErr != nil` with clean ctx and marks the task `failed` (execute.go:289-291), planning flows mark the stage `failed` — while the real event was cancellation. Retention keeps a misleading tombstone class for 30 days.
- **Counter-evidence searched:** No test covers either window — `run_control_test.go` covers accept-failure, success, and plain failure only; repository-level race tests (lifecycle_test.go) sit below the misclassification. A journal-completeness rationale ("the omission metadata really couldn't be written") was considered and rejected: the omitted entries are intentionally coalesced diagnostics, not product truth, and the same function's guarded siblings show the author's invariant that cancelled-context errors must not become persistence failures. The twin implementation proves the correct shape exists: `controlOperation`'s ack failure just cancels (durable_operations.go:196-201) and `FinishOperation` reclassifies from state/error into `TerminalCancelled` (durable_operations.go:241-253).
- **Confidence:** high (mechanism deterministic given the interleaving; trigger frequency depends on coalescing backlog at completion, which is common for chatty progress streams).
- **Smallest useful action:** Before `setPersistenceErr`, exclude `errors.Is(err, context.Canceled)` (and add the missing `runCtx.Err()` guard on the ack path); flush the final omission with a detached context like the terminal proposal already does. Add a regression test: spy base runtime returning success while the fence sees `CancellationRequested` and `progressOmitted > 0`; assert lifecycle `cancelled`/`succeeded`, never `persistence_degraded`.

### FAILURE-07-F2

- **Priority:** P3
- **Claim:** Late cancellation (arriving after the runtime process finished but before bookkeeping commits) is resolved by opposite conventions in sibling modules: `Execute` flips genuinely completed work to `cancelled` (discarding captured evidence and forcing duplicate agent work on resume), while planning flows ignore late cancellation entirely and commit the stage as complete — so the same race yields different verdicts depending on which stage pipeline ran.
- **Evidence:** execute.go:276-303 — the switch orders `deferReason` and `ctx.Err()` (:286-288) above successful-evidence completion (:292-296); evidence/artifacts are only recorded in the complete branch, so a cancelled-marked task loses artifact linkage even though artifacts exist, and `executeQueueFromState` re-enqueues cancelled tasks for full re-execution (execute.go:200, 527-541; session continuation via `reusableExecuteSession` :543-554 only softens cost). Contrast: `FlowPlan`/`FlowRequirements` et al. perform validation and `SaveFlowState(...complete...)` after a successful stage run with no ctx re-check (service.go:673-711), and `RunOperation`'s tail never re-checks ctx either (operations.go:405-406) — late cancel silently becomes success end-to-end. Planning-stage status vocabulary has no cancelled value at all (domain.go:45-49); cancellations mid-flight are persisted as `failed` via `flowFailedStages` (service.go:1077-1092).
- **Architectural reason:** lifecycle coherence / change-surface — "did cancellation beat completion?" has one durable answer but two incompatible product-layer readings; new stage flows must guess the convention.
- **Concrete consequence:** For execute, an agent task that finished seconds before a cancel is redone from scratch on resume (duplicate cost/token spend) and its evidence provenance is dropped from run-state; for planning, an identical timing race records success. Cross-surface history is inconsistent for the same event class, complicating recovery guidance ("was my plan written or not?").
- **Counter-evidence searched:** Sprint 35 requires only that UIs not use planning-stage status to represent running execution (the run projection carries liveness), so the missing planning `cancelled` status is not a contract breach on its own; conservative redo-on-cancel is safe (no corruption; validation-before-trust exists), and the windows are narrow (post-process bookkeeping, milliseconds to low seconds). This is why it is P3, not higher.
- **Confidence:** medium.
- **Smallest useful action:** Pick one rule and encode it once: either evaluate completion evidence before `ctx.Err()` in `Execute`'s switch (record complete-with-evidence, note the late cancel in diagnostics), or document the redo-on-cancel convention as the single product-wide rule for stage terminalization.

## Defended architecture / rejected hypotheses

- **Completion winning over a pending cancellation request leaves `cancellation_state=requested` forever on a succeeded run** — rejected as defect: this is the documented arbitration (`TestCompletionMayWinAfterCancellation...`, lifecycle_test.go:80-100; TRD.md:2206 "arbitrated … so only one terminal outcome wins"), the snapshot honestly carries both facts, and post-terminal `RequestCancellation` is an idempotent no-op.
- **`Adapter.StartRun` discarding a successful result when `ctx.Done()` wins the ready-select (~50% of ties, runtime.go:304-325)** — rejected: forcing `waitErr = ctx.Err()` on a clean child exit is deliberate conservatism (cancel means stop trusting results) and is applied consistently at every layer above it.
- **Two fenced owner implementations are harmful duplication** — rejected for now: they own different lifecycles (streaming runtime run vs. request-scoped operation wrapper) over the same CAS substrate; merging would couple runtime event streaming to operation events. The real cost of the duplication is the inconsistent error policy exposed in F1, which is fixable in place.
- **Study tasks lacking per-task durable run-control records** — rejected: product owns per-task truth in run-state.json with resume reconciliation (run_loop.go:16-22, `ReconcileRunState`); the study loop is durably tracked at operation level, matching the module-driven split of authority.
- **Hub cancel not durably recording the request (crash between `cancel()` and `FinishOperation`)** — rejected: owner death is covered by lease expiry + reconciler probe (never inferring success from process absence, lifecycle_test.go:176+), which is the designed recovery for that gap.

## Open questions

- `proposeRunTerminalWithRetry` budgets only ~250ms of outer retry (run_control.go:318-328) inside a 30s finish context; whether transient `SQLITE_BUSY` beyond one attempt can leave a genuinely completed operation unterminalized until the reconciler marks it `interrupted` could materially change how often completion actually lands. Worth a targeted fault test.
- Actual frequency of `progressOmitted > 0` at process completion (governs F1 trigger rate) is measurable from retained omission events in existing journals; a quick empirical count would calibrate F1's priority.
