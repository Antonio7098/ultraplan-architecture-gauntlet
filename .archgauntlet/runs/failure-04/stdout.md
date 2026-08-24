Docs read. Now let me examine the locking mechanisms — the heart of this scenario.
### Scope inspected

- **Docs**: `docs/architecture.md`, `docs/local-web.md`, `docs/user-guide.md`, `docs/recovery.md`, `docs/cli-reference.md`, `docs/plans/server-shutdown-run-cancellation-contract.md`
- **Locks/leases**: `internal/sprint/locks.go`, `internal/sprint/verification_lock.go`, `internal/sprint/service.go`, `internal/study/locks.go`, `internal/study/cleanup_uncertain.go`
- **Workflow owners**: `internal/sprint/{flow,execute,review,smoke,verify}.go`, `internal/study/{run_loop,run,run_all,run_history}.go`, `internal/study/state_database` callers
- **Surfaces**: `internal/app/{operations,operation_runner,serve_commands,tui_commands,usecases,web_usecases,durable_operations,run_control,sprint_commands,study_commands,study_runs_commands,project_commands}.go`, `internal/web/{operations,operation_handlers,server}.go`
- **Tests**: `internal/sprint/locks_test.go`, `verification_lock_test.go`, `internal/study/locks_test.go`, `internal/runcontrol/{lifecycle,sqlite,process_integration}_test.go`, `internal/app/run_control_inventory_test.go`, `internal/web/operations_contract_test.go`

### Architecture assessment

The sprint side of this scenario is sound. Every mutating sprint entry point (`Flow` flow.go:71, `FlowStage` flow.go:129, `Execute` execute.go:129, `DeferExecuteTask` execute.go:36, `Review` review.go:407, `RunSmoke` smoke.go:24, `Verify` verify.go:41, `ReconcileInterruptedMutation` locks.go:26) funnels through one product-owned per-sprint lease: an in-process `sync.Map` plus an `O_EXCL` PID-liveness file lock (`service.go:77-98`, `verification_lock.go:26-61`), with a context marker for legitimate composite nesting (`locks.go:90-104`). CLI, TUI, and web all converge on the same service methods (`sharedOperationRunner` operation_runner.go:18-149; CLI inventory pinned by `run_control_inventory_test.go:22-35`). Durable run control accepts before execution and carries fencing/cancellation across surfaces (`durable_operations.go:82-122`, `run_control.go:122-303`); a loser of the lease race records an honest `workflow.locked` failure (`operations.go:642-643`) and maps to HTTP 409 `operation_conflict` (`operation_handlers.go:768-769`). Startup reconciliation skips live leases (`locks.go:28-30`, `web_usecases.go:354-382`).

The study side is stressed: the per-study lock exists but guards only one of several first-class mutation entry points, which breaks the cross-surface mutual-exclusion story precisely where the web surface is active.

### Candidate findings

#### FAILURE-04-FN1

- **Priority**: P1
- **Claim**: Study-scope mutation authority is split. `RunLoopLock` is acquired only by `RunLoop`; the equally supported CLI mutations `study run`, `study synthesize`, `study run-all`, and `study summary` mutate the same study scope (agent-written reports, `summary.csv`, run history) without acquiring or even probing the lock, so a terminal command races a web/TUI-held run-loop with zero arbitration.
- **Evidence**:
  - Sole workflow acquisition points: `internal/study/run_loop.go:31` and recovery `internal/study/cleanup_uncertain.go:74`; `AcquireRunLoopLock`/`RunLoopActive` are called nowhere else outside tests/display (`internal/app/study_usecases.go:126` is display-only).
  - Unguarded mutators: `RunAll` (`internal/study/run_all.go:16-51` — no lock, calls `RunAnalysis`:74, `Synthesize`:109, `WriteSummary`:39); CLI `study run` → `service.RunAnalysis` directly (`internal/app/study_commands.go:782-796`), `synthesize` (:807-819), `run-all` (:544-558), `runs --refresh` rewrites history unlocked (`study_runs_commands.go:28`).
  - Agents execute with `WorkDir = study.Path` and write the report at `SourceReportPath` (`internal/study/run.go:49-51,95-102`), i.e., the exact artifact a concurrent loop task writes/validates.
  - Web/TUI expose only lock-taking `study-run-loop`/`study-resume`/`study-cancel` kinds (`internal/web/operation_handlers.go:674-683`), so the unguarded party is always the other surface.
  - Run control does not compensate: `Accept` enforces no target-scope uniqueness (`internal/runcontrol/sqlite.go:288-289,378`), so overlapping runs are both durably accepted and claimed.
  - CURRENT-CONTRACT: docs present these as first-class today (`docs/user-guide.md:184-193`, `docs/recovery.md:38,41` recommend `run-all`/`run` for repair), while `docs/architecture.md:112` asserts "Study run-loop keeps its independent product lock" — accurate literally, insufficient for the concurrency contract.
- **Architectural reason**: ownership / boundary / failure-semantics — the lock owner (run-loop) and the artifact truth can disagree with no authority resolving it.
- **Concrete consequence**: Browser starts `study-run-loop` (server PID owns lock). Operator reruns one failing task in a terminal: `ultraplan study demo run 03-consistency spec-a`. A second agent starts in the same study dir and writes the same source report the loop is generating/validating; last writer wins. The loop has already marked the task completed with validation metadata; the subsequent silent overwrite diverges artifact from durable run-state (no watcher; snapshot invalidation disabled, `docs/local-web.md:263-268`), and downstream synthesis consumes whichever bytes exist. Neither surface emits a conflict; both durable runs record "success".
- **Counter-evidence searched**: session fingerprinting guards continuation only within one owner (`run.go:106-116`); `unexpectedEditWarnings` yields warnings only (`run.go:54-58`); `ReconcileRunState`/`ResumeValidateRunState` revalidate only at loop start (`run_loop.go:62-63`); edit-attribution hard failures apply to smoke authoring, not study analysis; no test covers cross-entry study concurrency. The sprint module demonstrates the repo's own intended fix shape (nested-safe shared lease via context marker, `internal/sprint/locks.go:16-21,90-104`), so this is not a deliberate global no-locking stance.
- **Confidence**: high
- **Smallest useful action**: route `RunAnalysis`/`Synthesize`/`RunAll`/`WriteSummary` through the same `RunLoopLock` using the sprint-style context marker (loop-nested calls reuse the held lock), or minimally have the direct-execution CLI paths fail fast when `RunLoopActive(study)` is true.

#### FAILURE-04-FN2

- **Priority**: P2
- **Claim**: CLI and TUI `sprint status` persist `flow-state.json` outside the per-sprint mutation lease, creating an uncoordinated second writer against lease-holding flow/execute/review/smoke operations.
- **Evidence**: `Status()` writes via `SaveFlowState` when `statusWrites` (`internal/sprint/service.go:191-195`); the CLI builds its service without `WithoutStatusWrites` (`internal/app/sprint_commands.go:81-88`) and never touches `acquireMutation*`; the TUI likewise leaves `readOnly` false (`tui_commands.go:37-48`, help text admits recompute at :83). All other writers of this file hold the lease. Writes are individually atomic (temp+rename, `state.go:201-284`) but there is no cross-writer ordering.
- **Architectural reason**: authority / lifecycle — a presentation command performs a durable product write that the lease regime does not own.
- **Concrete consequence**: While a web-started execute holds the sprint lease for hours, an operator runs `ultraplan sprint p s status` in another terminal. Status derives stages from a mid-flight artifact snapshot and renames it over the state the operation just committed, reverting recorded stage errors/timestamps or the freshly written `Review`/`Smoke` block until the next mutating write. Bounded: stage states are re-derived from artifacts (`DeriveStages`), and no scheduling decision trusts flow-state alone (`flowStageAlreadyValid`, flow.go:203-235), so impact is projection drift and misleading verdicts, not corruption.
- **Counter-evidence searched**: This is partly documented intentional debt: `WithoutStatusWrites` exists precisely for the web ("existing CLI/TUI status behavior remains unchanged", service.go:67-73; `usecases.go:126-128`; architecture.md:69-70), and the web confirmation even overstates `OperationSprintStatus` as mutating (`operations.go:183-186`) — the safe direction. What remains undocumented is why an unleased write racing a lease holder is acceptable; nothing reconciles or version-checks the lost update.
- **Confidence**: high (gap exists), medium (materiality in single-user practice)
- **Smallest useful action**: Either make CLI/TUI status use `WithoutStatusWrites()` like the web projection, or wrap the `SaveFlowState` inside `Status` with a brief `acquireMutationContext` so the persist step honors the same authority.

### Defended architecture / rejected hypotheses

- **"Durable acceptance before lease acquisition causes double execution."** Rejected. Both surfaces converge on the same product services; the second contender fails closed at `ErrVerificationConflict`/`ErrStudyLocked` and records an honest terminal `workflow.locked` run (`operations.go:642-647`, `study_commands.go:343-346`). Same-process double-submits are deduped per confirmation/session (`durable_operations.go:91-97`, `operations.go:154-159`).
- **"Two servers or server+CLI can corrupt sprint state."** Rejected for the lease-guarded paths: `O_EXCL` + PID liveness serializes across processes (`verification_lock.go:32-59`); stale locks after SIGKILL are taken over cleanly; startup reconcile refuses live leases (`locks.go:28-30`) and fails closed on unresolved cleanup markers (`cleanup_uncertain.go:141-148`, `server.go:76-81`).
- **"Web sprint-status mutates without the lease."** Rejected: web's `sprintService()` is `WithoutStatusWrites` (`usecases.go:126-128`), so the browser path genuinely does not write; only CLI/TUI retain the persisting legacy behavior (covered by FN2).
- **"Cross-surface cancellation is racy."** Rejected: cancellation travels through fenced SQLite commands polled by the owner's control loop regardless of originating surface (`durable_operations.go:178-219`, `run_control.go:211-261`), and the cancel-vs-terminal race preserves one winner (`lifecycle_test.go:119-163`).

### Open questions

1. Does the team consider `study run` / `study run-all` compatibility shims scheduled for deprecation behind `run-loop`? If yes, FN1 downgrades to a contract/doc enforcement issue rather than a locking change.
2. Is there an invariant making lost `flow-state.json` updates from status refreshes harmless that survives `DeriveStages`' merge of prior `Review`/`Smoke` blocks? If so, documenting it would close FN2 as intentional debt.
