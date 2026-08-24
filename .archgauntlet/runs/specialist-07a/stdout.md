### Scope inspected

Implementation repo (`ultraplan-go` @ eeaa034): `internal/runcontrol` in full (`doc.go`, `model.go`, `id.go`, `interfaces.go`, `errors.go`, `sqlite.go`, `lifecycle.go`, `retention.go`, `migration.go`, `sanitize.go`, `local_log.go`, `metrics.go`, `process*.go`); app-side owners `internal/app/run_control.go`, `internal/app/durable_operations.go`, `internal/app/run_usecases.go`, `run_commands.go`, `operations.go`, `serve_commands.go`; web consumers `internal/web/operations.go`, `run_handlers.go` (replay/SSE/cancel paths); tests (`lifecycle_test.go`, `sqlite_test.go`, `fault_test.go`, `process_integration_test.go`, `migration_test.go`, `retention_test.go`, `run_control_test.go`, `durable_operations_test.go`, `import_boundary_test.go`). Authoritative planning workspace: `projects/ultraplan-go/sprints/35-durable-run-observability/requirements.md`, `docs/architecture.md` ("Durable run control" section), `system/contracts/runtime/persistence-and-migrations.md`. Ran `go test ./internal/runcontrol/` (pass).

### Architecture assessment

The core design is sound and unusually disciplined for its size. `runcontrol` is a genuinely narrow durable authority: opaque 128-bit IDs (`id.go`), repository-allocated fencing generations with `verifyFence` on every fenced mutation (`sqlite.go:985`) plus belt-and-braces CAS guards (`WHERE ... last_sequence = ? AND current_attempt_id = ?`), immediate-tx serialization making sequence allocation monotonic under concurrent multi-process writers (tested, `sqlite_test.go:287`), one immutable arbitration path for all seven terminal outcomes (`ProposeTerminal`, `sqlite.go:722`, first-writer-wins via `terminal_outcome IS NULL` CAS; loser gets idempotent no-op), conservative reconciliation that never infers success and distinguishes exact process-birth identity from PID presence (`lifecycle.go:481`, platform probes with birth tokens), and durability pragmas verified at open (`synchronous=FULL`, WAL, foreign keys). Failure injection tests prove no stale success on full/read-only/closed stores. Lease expiry deliberately uses SQLite's connection-local clock in production (`julianday('now')`, `sqlite.go:1136`) so competing processes share one time authority — a subtle and correct choice. Module-driven doctrine is respected: product packages keep workflow authority; runcontrol projects only safe correlations.

Stress shows where durable ownership meets the app layer: the owner-side control plane (poll → cancel-ack → heartbeat → reconcile) exists twice with divergent failure semantics, control-plane writes lack the retry tolerance the data plane has, and one advertised cancellation state is unreachable.

### Candidate findings

#### SPECIALIST-07A-F01
- **Priority:** P2
- **Claim:** Two parallel owner control-loop implementations duplicate the durable-lifecycle duties and classify identical faults into different terminal outcomes.
- **Evidence:** `controlledRuntime.StartRun` loop `internal/app/run_control.go:211-261` vs `durableOperationManager.controlOperation` `internal/app/durable_operations.go:178-219`. Both poll snapshots, acknowledge cancellation, heartbeat, and run global `Reconcile` every 10s. On heartbeat/snapshot failure the runtime path classifies and proposes `persistence_degraded` ("durable event persistence failed", `run_control.go:240-247, 282-292`); the operations path silently calls `owned.cancel()` (`durable_operations.go:190-216`), after which `FinishOperation`/`failedOperation` map the resulting `context.Canceled` to `cancelled` ("operation cancelled", `durable_operations.go:244-252`; `web/operations.go:231-241`). Outcome-mapping authorities also differ (`terminalOutcome()` `run_control.go:625-637` vs the state switch in `FinishOperation`). Coalescing state machines are separately re-implemented (`run_control.go:151-208` vs `durable_operations.go:124-176`) with different keys.
- **Architectural reason:** change-surface + failure-semantics (ownership of the owner-role logic leaked into two app entry points).
- **Concrete consequence:** the same storage-contention incident durably records `persistence_degraded` for a study/sprint runtime run but `cancelled` for a web/TUI/CLI operation — misleading triage taxonomy in the single source of truth this package exists to provide. Any new control-plane duty (backoff, jittered reconcile, lease escalation) must be written twice and will drift further; per-run global reconcile loops already multiply scan load linearly with active runs.
- **Counter-evidence searched:** looked for a shared loop helper (only append/terminal retry helpers are shared, `run_control.go:305-332`); looked for tests pinning either failure mapping (`run_control_test.go` and `durable_operations_test.go` cover happy paths and fail-closed start only); checked whether the surfaces are mutually exclusive (they are not — both are live entry points per `serve_commands.go:63-65`, `study_commands.go`, `sprint_commands.go`). Both variants are fail-closed, so safety holds; the defect is taxonomy and duplication, not unsafe behavior.
- **Confidence:** high
- **Smallest useful action:** extract one `ownerLoop` (snapshot-poll / cancel-ack / heartbeat / reconcile with a typed control-error result) used by both entry points, or minimally route manager failures through the same `PersistenceLost` classification as `setPersistenceErr`.

#### SPECIALIST-07A-F02
- **Priority:** P2
- **Claim:** Control-plane writes tolerate zero retry while data-plane writes retry the very errors the codebase itself labels retryable; a single transient busy/IO failure kills live work and cements a wrong terminal outcome.
- **Evidence:** `retryableRunControlError` treats `ErrBusy`/`ErrUnavailable` as retryable and `Append` retries them for ~5s (`run_control.go:305-316, 330-332`); `errors.go` marks these codes `Retryable=true` (`sqlite.go:1090-1091`). Yet in `StartRun`'s loop one failed `Heartbeat`/`Snapshot` immediately calls `setPersistenceErr` → `cancel()` (`run_control.go:228, 240-247`), and the subsequent `ProposeTerminal(persistence_degraded)` *does* retry until contention clears (`run_control.go:295-301`), guaranteeing the misclassification commits once the blip passes.
- **Architectural reason:** failure-semantics (inconsistent fault model between journal writes and lease/liveness writes sharing the same store and error vocabulary).
- **Concrete consequence:** a >5s write-lock stall (another process's migration checkpoint, fsync burst under `synchronous=FULL`, several concurrent accepts triggering inline compaction at `sqlite.go:384-396`) destroys hours-long agent runs whose fences were still valid, recorded as storage loss that never happened.
- **Counter-evidence searched:** the fail-closed doctrine ("a failed required write fails closed", `docs/architecture.md`) justifies stopping work on `StaleFence`/`Permission`/`Corrupt`, but `CodeBusy`/`CodeUnavailable` are explicitly classified transient by the same package and retried elsewhere; continuing to poll-and-retry for a bounded window does not extend the lease risk (fence verification would catch genuine loss). No test or comment asserts single-shot intent.
- **Confidence:** medium-high
- **Smallest useful action:** apply the existing `retryableRunControlError` deadline pattern to heartbeat/snapshot calls in the control loops; reserve immediate abort for `ErrStaleFence`, `ErrPermission`, `ErrCorrupt`.

#### SPECIALIST-07A-F03
- **Priority:** P2
- **Claim:** `cancellation_state='uncertain'` is unreachable: schema, model, health counter, CLI output, and TUI rendering all support it, but no code path ever writes it, so a required health distinction is vacuous.
- **Evidence:** schema CHECK allows `'uncertain'` (`sqlite.go:260`); `CancellationUncertain` constant (`model.go:82`); `Health.CancellationUncertain` counts rows that cannot exist (`sqlite.go:877`); printed by `run diagnostics` (`run_commands.go:271`); rendered by TUI (`tui/run_view_test.go:82`). Grep confirms zero writers. CURRENT-CONTRACT: sprint-35 requirements demand operator-visible health distinguish "cancellation uncertainty" (`requirements.md:27`).
- **Architectural reason:** lifecycle + drift (state-machine branch with no incoming transition; compliance-shaped vestige).
- **Concrete consequence:** a real uncertainty case exists today — owner dies between observing `requested` and committing `acknowledged`, or reconciliation finds a dead owner with pending cancellation — and it is indistinguishable in health output from ordinary states (`interrupted` with `cancellation_state=requested`), defeating the contract's stated operator signal.
- **Counter-evidence searched:** checked migration code, web shutdown cleanup path (`RecordOperationCleanupUncertain` writes product state, not run-control), and all `ProposeTerminal` callers for a producer — none; checked whether it is FUTURE-INTENT (no later-sprint reference found in the workspace; the deliverable row belongs to the implemented sprint).
- **Confidence:** medium-high
- **Smallest useful action:** give the reconciler a writer (expired owner + `cancellation_state='requested'` → mark `uncertain` before proposing terminal), or remove the counter/state until a producer exists and record the deferral in reasoning.

#### SPECIALIST-07A-F04
- **Priority:** P3
- **Claim:** Unearned seams: `LifecycleQueued` has no producer anywhere, `Notifier` is injected-but-never-wired, and `Control` is an empty alias for `Repository`.
- **Evidence:** `LifecycleQueued` appears only in definitions and the transition table (`model.go:20`, `sqlite.go:1034-1035`); `Accept` always writes `accepted`, `Claim` writes `running`/`cancelling`; no caller passes `Queued`. `Notifier` (`interfaces.go:41-45`, option at `sqlite.go:36`) has no implementation in any composition root (`runControlState.repository` passes only `Retention`; `notify()` at `sqlite.go:1143-1147` therefore never fires in production — CLI follow and web SSE both poll). `Control` (`interfaces.go:70-71`) has zero references.
- **Architectural reason:** drift/change-surface (speculative generality the review doctrine says must be earned).
- **Concrete consequence:** maintainers must reverse-engineer whether `queued` and push-notifications are roadmap or residue; query filters over `queued` silently return nothing; the doc sentence "in-process notifications are only an optimization" describes machinery that cannot run.
- **Counter-evidence searched:** searched workspace planning docs and sprints for planned queued-state or SSE-push producers — none found; confirmed tests do not exercise these paths as contracts.
- **Confidence:** high (facts), low (stakes)
- **Smallest useful action:** delete `Control`; implement or remove `Notifier`; either drop `LifecycleQueued` or annotate it as reserved with a pointer to the deciding reasoning doc.

#### SPECIALIST-07A-F05
- **Priority:** P3
- **Claim:** Keyset pagination sorts by mutable `updated_at`, so heartbeats/stall-marking churn can skip active runs across pages.
- **Evidence:** cursor predicate `(updated_at < ? OR (= AND run_id < ?))` over `ORDER BY updated_at DESC` (`lifecycle.go:225-233`); `Heartbeat` and `markStalled` bump `updated_at` every pass (`lifecycle.go:48, 499`), so a not-yet-paged active run can repeatedly jump above the cursor.
- **Architectural reason:** boundary (pagination contract over a liveness-mutated column).
- **Concrete consequence:** `run list --after` deep pagination during concurrent activity can omit recently-active runs entirely; `recordReconciliation` also inserts one row per stalled candidate per pass with no pruning except run-deletion cascade (`retention.go:142-150`), so a wedged-but-live owner accumulates log rows indefinitely.
- **Counter-evidence searched:** primary consumers use single bounded pages (CLI default 50; support export capped at 50/1 MiB, `run_commands.go:306-320`), and stale-cursor effects self-heal when churn stops; hence P3, not P2.
- **Confidence:** high
- **Smallest useful action:** paginate on immutable `accepted_at DESC, run_id DESC` (or add a stall-idempotency guard so repeated passes don't rewrite `updated_at`).

### Defended architecture / rejected hypotheses

- **Reconciler borrowing the dead owner's fence to call `ProposeTerminal` is sound, not an authority violation.** Verified: generations are UNIQUE per run (`sqlite.go:289`), `verifyFence` requires the attempt to still be current (`sqlite.go:985-1003`), winner selection is a single CAS, losers get idempotent `won=false`, and `terminal_proposed_by='reconciler'` records the true actor. Terminal/cancellation races are explicitly tested (`lifecycle_test.go:119-174`).
- **Accept-then-claim as two transactions is not a lost-acceptance hole.** A claim failure leaves an unclaimed run that `Reconcile` interrupt-proposes after grace with no fabricated evidence (`lifecycle.go:427-479`, tested at `lifecycle_test.go:261-311`); alias conflicts resolve idempotently (`durable_operations.go:91-97`).
- **Hard-quota behavior is coherent.** `reservedEventType` (`sqlite.go:713-720`) keeps lifecycle/warning/cancellation/recovery/omission/terminal writable at hard quota, and `ProposeTerminal` is quota-free, so an owner can always reach a named terminal; heartbeats correctly refuse under hard quota to force stop. Full-disk and closed-store fault tests prove no uncommitted success (`fault_test.go`).
- **PID reuse and clock jumps are handled.** Birth-token mismatch decisions (`process_linux.go`, `reconcileProcessDecision`) and the `julianday('now')` single-clock design are tested (`lifecycle_test.go:176-259`); my hypothesized "reconciler terminalizes a revived owner" race is closed because terminal proposals require probe evidence of death/mismatch, and a live-match only marks stalled, from which the owner's own heartbeat restores `live`.
- **Web replay-gap handling satisfies the contract** (`run_handlers.go:448-459` typed `cursor_ahead`/`replay_gap`); the CLI `follow` lacking the same gap detection is cosmetic, since compaction only removes progress/message classes and the snapshot carries `oldest_retained_sequence`.

### Open questions

- Is a `cancellation_state='uncertain'` producer planned in an unfinished slice of sprint-35 work (would downgrade SPECIALIST-07A-F03 to partial delivery rather than drift)?
- Was `Notifier` intended for near-term SSE push (affects whether F04's seam is roadmap or removable residue)?
- Is per-owner-process global `Reconcile` cadence expected to scale to many concurrent runs per workspace, or is a designated reconciler/leader anticipated? Current cost is linear in active runs and safe, but it compounds F01/F02 contention exposure.
