Now let me explore the target repository structure.
The core store is clear. Now let me look at lifecycle, fault tests, and the filesystem platform layer.
The sprint package has artifacts, state database, and cleanup_uncertain files. Let me read them.
`productstate` opens the same `.ultraplan/run-control.db` file with its own pool. Now let me see the filesystem store side of sprint state.
Now let me read cleanup_uncertain.go and the flow transition logic.
The code-context promotion has a compensating-restore pattern. Let me now read review.go's persistence paths and the smoke writer.
`internal/app/durable_operations.go` is the key integration point. Let me read it fully.
Let me examine execute_state.go for its dual-write ordering.
Let me check the authoritative planning workspace for contracts about state persistence.
I now have the complete picture. Let me compile the failure-stage report.

### Scope inspected

**Implementation repo** (`ultraplan-go` @ eeaa034):
- `internal/runcontrol`: `sqlite.go` (Accept/Claim/Append/ProposeTerminal/Snapshot/classifyStoreError), `lifecycle.go` (Heartbeat/cancellation/Reconcile/reconcileUnclaimed), `retention.go` (Compact/storageBytes/compactRunJournal), `migration.go` (migrateSchema/migration lock/backups/RestoreBackup), `fault_test.go`, `doc.go`
- `internal/productstate/store.go` (entire package — no test files exist)
- `internal/sprint`: `state.go` (SaveFlowState dual-write), `execute_state.go`, `state_database.go`, `store_fs.go`, `artifacts.go`, `locks.go`, `verification_lock.go`, `cleanup_uncertain.go`, `session_state.go`, `code_context.go` (promoteCodeContext compensation), `review.go` (saveReviewState/resume checkpoints/atomicWriteReview), `smoke.go` (commit ordering), `execute.go` (WriteExecuteSummary/DeferExecuteTask), `service.go` (acquireMutation/Status/FlowRequirements)
- `internal/app`: `durable_operations.go`, `run_control.go` (controlledRuntime, retry helpers), `run_usecases.go`
- `internal/web/operations.go` (drainAndWait/publishAppEvent), `internal/platform/filesystem/doc.go`

**Docs**: `docs/recovery.md`, `docs/architecture.md` (§Durable run control), `docs/migration-product-state.md`; authoritative workspace contract `system/contracts/runtime/persistence-and-migrations.md`.

**Commands**: `go test ./internal/runcontrol/ ./internal/productstate/ ./internal/sprint/` (all pass/cached).

### Architecture assessment

The mid-transition failure story is **layered by design, and mostly sound**. Truth ownership is explicit: `runcontrol` owns the durable operational journal (single immediate transactions, CAS via `RowsAffected`, WAL + `synchronous=FULL`, pragma verification at open, typed error taxonomy, fault tests asserting "never returns uncommitted success"); product modules own their own state (`product_states` tables + artifact files) with declared authority ("After import, SQLite is authoritative… JSON files stay as compatibility checkpoints", migration-product-state.md:24-26). Cross-layer ordering follows artifact-first-then-state, with three distinct, deliberate treatments of the same partial-failure window: protective compensation (code-context restore), typed reconciliation-required outcomes (smoke), and artifact-truth regeneration (planning stages). Recovery is conservative at every layer (interrupted/cleanup_uncertain/persistence_degraded terminals; stale digests; lease-based reconciliation).

Stress points: the two SQLite tenants of the single `.ultraplan/run-control.db` file have **diverged durability governance**, one dual-write path can return failure after its authoritative commit landed, and two parallel app-layer implementations of the accept/append/terminal protocol classify the identical storage failure differently.

### Candidate findings

---

**ID: FAILURE-09-F01**
- **Priority:** P2
- **Claim:** `productstate` shares the physical `run-control.db` file with `runcontrol` but lacks every durability mechanism its co-tenant applies: no driver-error classification (busy/quota/corrupt/permission), no pragma verification, no symlink/mode hardening on open, no fault tests. Sprint transitions therefore fail opaquely mid-transition under exactly the SQLite stress conditions where the run layer retries or degrades with a typed outcome.
- **Evidence:** `internal/productstate/store.go:150-206` (`Save` returns raw driver errors from Exec/Commit), `store.go:41-51` (`Existing` returns raw stat errors), `store.go:63-79` (`open`: no `_defensive`, no `verifyPragmas`, no symlink rejection — contrast `runcontrol/sqlite.go:73-79, 154-205, 207-227`); error taxonomy exists only in `runcontrol/sqlite.go:1085-1108` (`classifyStoreError` maps SQLITE_BUSY/LOCKED→retryable, FULL→quota, CORRUPT→corrupt). App-layer retry helpers wrap only runcontrol calls (`app/run_control.go:305-332` `retryableRunControlError`). `productstate` has zero test files; `runcontrol` has a dedicated fault suite (`fault_test.go:12-129`: quota-exhaustion never reports uncommitted success, closed-repo, read-only loss).
- **Architectural reason:** failure-semantics drift between two owners of one physical store (boundary).
- **Concrete consequence:** under write contention (e.g., a long compaction batch inside an `Append` transaction, `retention.go:58-95`) or quota pressure, a sprint stage save fails with an untyped error after runtime work already completed; callers emit guidance like "Repair flow-state persistence before retrying" (`smoke.go:31,35`) or mark the stage failed (`code_context.go:443-445`), while the same condition against runcontrol would be retried for 5s or surfaced as typed `persistence_degraded`. Diagnostics/support export built on runcontrol codes cannot see or classify the productstate side.
- **Counter-evidence searched:** both sides set identical `busy_timeout=5000`, immediate transactions serialize writers, and the per-sprint mutation lease (`locks.go:77-98`) serializes same-sprint mutations — so corruption and frequent contention are unlikely; module doctrine legitimately gives each module its own tables. The asymmetry remains observable because consumers demonstrably need the distinction (the app layer built `retryableRunControlError` for exactly this).
- **Confidence:** high (asymmetry is factual), medium (materiality).
- **Smallest useful action:** add minimal driver-error classification to `productstate.Save/open` (busy→retryable, full→quota, corrupt→corrupt, mirroring `classifyStoreError`'s subset) plus table-driven fault tests mirroring `fault_test.go`. No new abstraction.

---

**ID: FAILURE-09-F02**
- **Priority:** P3
- **Claim:** In the DB-authoritative branch, `SaveFlowState`/`SaveExecuteRunState` commit the transition to SQLite first, then attempt the JSON checkpoint file, and return the file error — reporting failure for a transition whose authoritative effect durably landed.
- **Evidence:** `internal/sprint/state.go:216-227` (DB save → `if !flowStateCheckpoint(state) { return nil }` → `return saveFlowStateWithHooks(...)`), `execute_state.go:105-118`; caller treats it as stage failure (`service.go:537-539`). The hook-based failure tests bypass this branch entirely because `SaveFlowState` always passes `atomicWriteHooks{}` (`sprint_test.go:135`, `execute_state_test.go:76`, `review_test.go:584` all call the `*WithHooks` functions directly).
- **Architectural reason:** lifecycle/failure-semantics — uncertain-outcome reporting at a dual-write boundary.
- **Concrete consequence:** disk pressure at exactly checkpoint states yields "stage failed" CLI output while reload shows the stage complete (loads prefer the DB, `state.go:21-32`). An operator following recovery.md ("rerun `flow --to <stage>`") regenerates an already-complete artifact, changing content/fingerprints and cascading staleness into downstream review/smoke evidence.
- **Counter-evidence searched:** authority rules bound the damage — DB wins reads (`LoadFlowState`), and `sprint status` re-derives and re-persists checkpoints (`service.go:164-195`), healing divergence; PERSIST-DERIVED-001 declares the file a derived copy, so no corruption occurs. Not disproven: nothing marks the outcome as "applied but checkpoint lagging," and the branch is untested.
- **Confidence:** medium.
- **Smallest useful action:** treat the post-commit checkpoint write as best-effort (log-and-succeed) or return a distinct typed outcome, and add one test driving `SaveFlowState` through the DB branch with a failing checkpoint.

---

**ID: FAILURE-09-F03**
- **Priority:** P3
- **Claim:** The web `durableOperationManager` converts mid-run journal-write loss into a plain cancellation, while the two sibling implementations of the same protocol record `persistence_degraded` for the identical failure.
- **Evidence:** `RecordOperationEvent` cancels the owned operation on append failure (`app/durable_operations.go:165-171`; triggered from `web/operations.go:247-250`); `FinishOperation` then matches `errors.Is(runErr, context.Canceled)` first and proposes `TerminalCancelled` (`durable_operations.go:243-252`). Contrast: `controlledRuntime` captures `persistenceErr` and proposes `TerminalPersistenceLost` with reason "durable event persistence failed" (`app/run_control.go:156-163, 276-292`), and the start path does the same (`durable_operations.go:109-114`).
- **Architectural reason:** failure-semantics drift within one boundary (three implementations of accept/append/terminal).
- **Concrete consequence:** after SQLite failure during a web operation, the journal says `cancelled`, hiding the storage cause; recovery.md routes `cancelled` to retry and `persistence_degraded` to disk/quota inspection (`docs/recovery.md:193-198`), so the operator gets the wrong playbook. If the terminal proposal also fails, the run falls to reconciler classification instead.
- **Counter-evidence searched:** both outcomes are conservative, non-corrupting terminals; busy/unavailable are already retried before giving up (`appendRunEventWithRetry`). No counter-evidence found for the classification inconsistency itself; no test pins either behavior in this path (`durable_operations_test.go` covers only happy paths and explicit cancellation).
- **Confidence:** high on behavior, medium on impact.
- **Smallest useful action:** propagate the append error into `FinishOperation`'s classification (propose `TerminalPersistenceLost` when the finishing cause was journal-write loss), matching `controlledRuntime`.

---

**ID: FAILURE-09-F04**
- **Priority:** P3
- **Claim:** `WriteExecuteSummary` is the only durable-file writer without temp+rename atomicity, and `DeferExecuteTask` returns an error after the deferral already committed if the summary write fails.
- **Evidence:** `internal/sprint/execute.go:583-586` (direct `os.WriteFile` overwrite; contrast `atomicWriteFile` `smoke.go:692-723`, `atomicWriteReviewWithHooks` `review.go:1686-1716`, `saveFlowStateWithHooks` `state.go:239-289`); ordering `execute.go:70-75` (state committed, then summary error fails the command).
- **Architectural reason:** failure-semantics consistency of derived-artifact writers (PERSIST-ATOMIC-001 "temporary files plus same-directory rename… where practical").
- **Concrete consequence:** crash/disk-full mid-write leaves truncated `execute.md`; a failed summary after committed deferral misreports a durable state change as failed. Impact bounded: `execute.md` is derived/regenerable and not validated by `verify`/`status` (no `ValidateExecuteContent` exists; truth is `.run-state.json`/DB).
- **Counter-evidence searched:** verified no reader treats `execute.md` as authoritative; recovery.md's atomic-write-failure runbook (`docs/recovery.md:172-179`) covers operator response. Residual is the non-atomic window and the uncertain-outcome return.
- **Confidence:** high on facts, low on impact.
- **Smallest useful action:** route `WriteExecuteSummary` through `atomicWriteFile` and make the summary refresh best-effort after a committed state change.

### Defended architecture / rejected hypotheses

- **Run-control transaction discipline under mid-write failure.** Every mutation is one immediate transaction with deferred rollback, fence verification, and CAS row-count checks; sequence allocation re-reads `last_sequence` inside the transaction, so an ambiguous commit (error after fsync) cannot produce duplicate or lost events — next append continues from DB truth. Fault tests prove quota exhaustion and permission loss never yield uncommitted success (`fault_test.go:12-129`). No finding.
- **Crash between artifact promotion and state save (code-context).** Compensating restore of the prior artifact is implemented and tested (`code_context.go:464-509`, `code_context_test.go:488-508`), matching recovery.md:46. Defended.
- **`smoke.md` committed but flow-state write failed.** Deliberately NOT auto-reconciled: typed `smoke_reconciliation` outcome with `Reconciliation: true` (`smoke.go:429-432, 455-457`) and documented manual recovery ("automatic recovery is deferred", recovery.md:86). Documented intentional rationale.
- **Cleanup-uncertain marker write failure during shutdown drain.** Not silent: persist errors are joined into the server shutdown error (`web/server.go:133-147`, `operations.go:495-497, 524-526`); deadline-free marker writes use a 1s bounded context. The marker deliberately bypasses the mutation lease (comment at `cleanup_uncertain.go:28-30`). Defended.
- **Two pools opening one database concurrently.** `CREATE TABLE IF NOT EXISTS` schema creation on both sides, O_EXCL database creation, migration lock with PID+birth-token stale detection (`migration.go:92-165`), and immediate transactions serialize; no interleaving produces partial schemas. Fresh-workspace concurrent startup can transiently return `CodeBusy` to the second opener — fail-closed, acceptable.
- **Quota pre-check/Compact outside the Accept transaction.** Heuristic TOCTOU is benign: compaction is CAS-guarded on `terminal_outcome IS NOT NULL` (`retention.go:194-202`) and reserved headroom protects active runs.
- **RestoreBackup losing product state.** Migration backups copy the entire file including `product_states` rows; restore is offline-only, integrity-checked, documented as a stop-condition procedure (recovery.md:217-224). Stale-backup edge cases are accepted disaster-recovery posture, not a current defect.

### Open questions

- Does the modernc driver surface `SQLITE_BUSY`/`SQLITE_FULL` on `tx.Commit()` (post-`busy_timeout`) in all cases `classifyStoreError` and a future `productstate` classifier would need? Affects F01 completeness.
- Is the absence of any test (or debt marker) for the DB-authoritative checkpoint-failure branch tracked in the planning workspace plans? If deliberate, F02 downgrades further.
- For migrated workspaces, which recovery timing dominates in practice — review's 24h attempt-expiry derivation versus `ReconcileInterruptedMutation`'s immediate stale-running rewrite? Affects how far an F02-triggered rerun cascade propagates.
