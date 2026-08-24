The durable layer looks tightly fenced. Now I need to find who can actually trigger a takeover (new attempt owning a run) and how product-side loops react to stale-fence errors.
The durable layer fences correctly. Now I need to see how the product layers (TUI/web/study loops) react to stale-fence failures, and whether product-side locking can produce a live dual-writer despite run-control fencing.
Timing model confirmed (1s tick, 5s heartbeat, 15s lease, 45s grace). Now let me examine the product-level locks — the actual takeover surface where a stale owner could keep writing state files.
The study lock has a notable asymmetry: the automatic path checks `processAlive`, but `ForceUnlockRunLoop` removes the lock unconditionally. Let me trace whether the run-loop detects lock loss mid-run, and how state writes are serialized.
The run-loop never re-validates its lock mid-run — it notices loss only at release. Now let me check the cross-process fencing integration test, the contract documents, and how the runtime honors context cancellation.
# FAILURE-03 — Stale fenced owner writes after takeover

### Scope inspected

- `internal/runcontrol`: `sqlite.go` (`Claim`, `Append`, `ProposeTerminal`, `Heartbeat`, `verifyFence`, schema/triggers), `lifecycle.go` (`Heartbeat`, `AcknowledgeCancellation`, `Reconcile`, `markStalled`), `errors.go`, `model.go` (fence/attempt/snapshot validation, timing constants), `doc.go`
- Owner loops: `internal/app/run_control.go` (`controlledRuntime.StartRun`, `appendRunEventWithRetry`, `proposeRunTerminalWithRetry`), `internal/app/durable_operations.go` (manager, `controlOperation`, `RecordOperationEvent`, `FinishOperation`)
- Surfaces: `internal/web/operations.go`, `operation_handlers.go`; `internal/tui/app.go` (`beginOperation`, `operationCmd`); CLI durable command wrapper (`durableCLICommand`)
- Product locking/lifecycle: `internal/study/locks.go`, `run_loop.go`, `state.go` (atomic rename); `internal/sprint/locks.go`, `verification_lock.go`, `execute.go`, `service.go` (`acquireMutation`)
- Runtime cancellation: `internal/platform/runtime/runtime.go:308-344`, `runtime_test.go:472`
- Tests: `runcontrol/lifecycle_test.go`, `fault_test.go`, `process_integration_test.go`, `app/durable_operations_test.go`
- Workspace contracts: `system/contracts/runtime/persistence-and-migrations.md`, `workflows.md` (no run-control fencing clauses found — implementation comments/README are the operative contract)
- Verified: `go test ./internal/runcontrol -run 'TestHeartbeat|TestReconcile|TestEveryTerminal|TestCancellation'` passes at commit eeaa034

### Architecture assessment

The fencing core is sound and unusually disciplined. Authority transfer is *closure-based*, not steal-based: `Claim` requires `current_attempt_id IS NULL` (sqlite.go:561-571) and refuses terminal runs (sqlite.go:533-535); no code path clears `current_attempt_id` except terminalization. Therefore two live attempts can never own one run through the API. The only takeover is the reconciler terminalizing an expired-lease owner, and only with probe evidence (death, birth-token mismatch, or identity uncertainty); a provably-live stalled owner keeps ownership (`markStalled`, lifecycle.go:497-509).

For the assigned scenario — stale owner attempting heartbeat/event/terminal writes after loss of authority — every durable write path rejects correctly:

- **Heartbeat**: attempt-row update is guarded by `outcome IS NULL`, run update by `terminal_outcome IS NULL`; both inside one immediate tx → `CodeStaleFence` (lifecycle.go:38-55). After reconciler terminalization the attempt row carries `outcome`, so even `verifyFence`'s successor check trips first.
- **Event append**: `verifyFence` plus CAS on `(last_sequence, current_attempt_id)` plus the immutable-events trigger → rejected (`CodeTerminal`/`CodeStaleFence`) with zero partial commit (sqlite.go:647-686; fault_test.go proves full-disk and read-only failures never yield stale success).
- **Terminal proposal**: idempotent loser — returns `(winner, won=false, nil)` once terminal (sqlite.go:753-758).
- **Owner reaction**: any heartbeat/append failure cancels the operation context (durable_operations.go:168-171, 192-207; run_control.go:156-163, 224-258), and the runtime cancels underlying agent sessions on ctx done (runtime.go:308-320, tested). The exposure window is bounded by the 1s tick.

What is stressed is not the fence itself but **failure classification around the fence**, and the fact that **product-level takeover primitives do not participate in run-control fencing** (finding F02).

### Candidate findings

#### FAILURE-03-F01
- **Priority**: P2
- **Claim**: Heartbeat treats every error — including explicitly retryable store errors — as irreversible ownership loss, unlike event appends which get a bounded retry; a transient SQLite busy spell kills live operations and records misleading `persistence_degraded` terminal evidence.
- **Evidence**: `app/run_control.go:239-247` and `app/durable_operations.go:204-207` cancel on the first heartbeat error with no retry; contrast `appendRunEventWithRetry` (run_control.go:305-316, 5s deadline) built precisely because `ErrBusy`/`ErrUnavailable` are marked `Retryable: true` (runcontrol/errors.go:31-32; sqlite.go:1090-1091 classifies SQLITE_BUSY as retryable). After cancellation, run_control.go:283-291 proposes `TerminalPersistenceLost` ("durable event persistence failed").
- **Architectural reason**: failure-semantics — the error type system says "retry", the heartbeat consumer says "fence lost"; conflating *cannot confirm durability* with *lost authority*.
- **Concrete consequence**: CLI + server + TUI share one workspace SQLite file; a >5s write burst (busy_timeout exhausted) during one surface's heartbeat cancels a possibly long-running agent session mid-task, then stamps the run `persistence_degraded` — corrupting operational evidence and destroying work for a transient contention blip. Lease math makes a bounded retry safe: heartbeat every 5s against a 15s lease leaves room for the same 5s retry budget append already uses.
- **Counter-evidence searched**: WAL + immediate short txns + 5s busy timeout reduce (not eliminate) busy windows; multi-process concurrency is a designed scenario (`SQLiteRepository` doc comment, sqlite.go:41-42; cross-process integration tests). No comment or contract documents fail-fast-on-busy as intentional. Fail-closed direction prevents corruption either way — this is an availability/evidence defect, not a safety one.
- **Confidence**: medium
- **Smallest useful action**: route heartbeat through the existing bounded-retry helper (or a shared one) so only non-retryable failures cancel ownership; keep stale-fence errors immediate.

#### FAILURE-03-F02
- **Priority**: P2
- **Claim**: The study `--force-unlock` takeover does not fence the displaced owner: it removes the lock without any liveness check, and a running loop never re-validates possession, so a forced unlock against a live holder produces two concurrent mutators of `run-state.json`/`tasks.jsonl` — each holding its own healthy fenced run-control record that reconciliation can never arbitrate.
- **Evidence**: `ForceUnlockRunLoop` deletes the lock unconditionally (study/locks.go:161-167), whereas the automatic acquisition path liveness-checks the holder before stealing (locks.go:66-83); `RunLoop` acquires once and re-checks ownership only at release (run_loop.go:31-39; locks.go:105-123 refuses foreign release but only at exit). Both loops schedule from overlapping task views (run_loop.go:214-312), so duplicated LLM analysis and interleaved checkpoint overwrites follow. Run-control sees two distinct live runs over the same study scope; both owners are alive, so `Reconcile`'s probe-based arbitration structurally cannot resolve the conflict.
- **Architectural reason**: lifecycle/boundary — run-control owns *run* fencing, but the *product-scope* mutation authority (the lock file) can be revoked behind a live writer's back with no feed-back into the writer's context; the two authorities disagree and nobody resolves it.
- **Concrete consequence**: operator runs `study <x> run-loop --force-unlock` believing the lock is stale while the original process is merely hung (e.g., provider stall — exactly the case where heartbeats keep flowing and run-control looks healthy). Old loop resumes, both write task transitions and spawn duplicate agents; progress history and `summary.md` interleave; cost doubles silently until both exit.
- **Counter-evidence searched**: docs deliberately scope the flag ("Use `--force-unlock` only for operator-confirmed stale locks", cli-reference.md:210, user-guide.md:201) — but nothing *checks* staleness on the force path, and `CancelRunLoop` (locks.go:141-158) already implements the correct primitive for live holders (SIGINT), showing the asymmetry is not a considered design. Release-time refusal limits damage only at exit, not during. Sprint's counterpart lock gets this right: liveness-checked stale removal, no force flag (sprint/verification_lock.go:53-58), making the study variant look accidental rather than doctrinal.
- **Confidence**: high (mechanism verified; intent ambiguous)
- **Smallest useful action**: in `AcquireRunLoopLock` when `force` is set, consult `RunLoopActive` and refuse (or require `--yes`) when the recorded PID is alive; optionally have `RunLoop` re-validate lock possession on each scheduling pass and self-cancel via ctx.

#### FAILURE-03-F03
- **Priority**: P3
- **Claim**: `Heartbeat` checks the hard-quota gate before verifying the fence, so a stale owner heartbeating against a full workspace receives `CodeQuota` (retryable) instead of `CodeStaleFence`.
- **Evidence**: lifecycle.go:24-28 (quota check) precedes lifecycle.go:35 (`verifyFence`). Every other fenced entry verifies the fence first.
- **Architectural reason**: failure-semantics/diagnostics — error classification, not safety.
- **Concrete consequence**: run logs/reconciliation evidence attribute a takeover to quota pressure; consumers currently treat all heartbeat errors as fatal so no behavioral harm today, but any future consumer keying on `ErrQuota.Retryable` would keep working after losing authority.
- **Counter-evidence searched**: no caller distinguishes quota from stale-fence heartbeat failures; ordering appears incidental rather than load-bearing.
- **Confidence**: high
- **Smallest useful action**: move `verifyFence` above the quota pre-check in `Heartbeat`.

### Defended architecture / rejected hypotheses

1. **"Append after takeover corrupts history."** Rejected. `verifyFence` + CAS on `(last_sequence, current_attempt_id)` + `trg_events_immutable` trigger (sqlite.go:327-331, 647-686); fault tests assert no stale success under full/read-only storage (fault_test.go).
2. **"The reconciler steals ownership from live owners."** Rejected. Live-but-stalled holders are only marked stalled (lifecycle.go:388-397, 497-509); terminalization requires probe death/mismatch/uncertainty; `TestReconcileClockJumpNeverExpiresAnOwnerEarly` guards clock skew (lifecycle_test.go:233-259); repeated reconciliation is idempotent (process_integration_test.go:229-232).
3. **"A new attempt can coexist with the old owner."** Impossible by construction: `Claim` requires `current_attempt_id IS NULL` (sqlite.go:536-538, 561-571) and nothing clears it outside acceptance; terminal runs are unclaimable (sqlite.go:533-535). Authority transfer = close the old run; new work gets a new run ID.
4. **"Alias-based `Existing` resume bypasses fencing."** Rejected. On digest conflict, `AcceptOperation` resolves the alias and returns without claiming or writing (durable_operations.go:91-97); web (operations.go:193-197) and TUI (tui/app.go:245-250) render status only.
5. **"Web degraded mode runs duplicate unfenced operations."** Rejected for production: `ErrWebUnavailable` is produced only when the durable manager is nil (web_usecases.go:263-267), and production wiring always installs it (serve_commands.go:65; tui_commands.go:47). The fall-through at operations.go:184-199 is a test seam; any real repository failure fails closed.
6. **"Sprint product state admits dual writers."** Rejected. All execute-state writes sit inside `acquireMutationContext` scopes (sprint/execute.go:36-40, 129-133); the verification lock liveness-checks before removing a stale lock and refuses foreign release (verification_lock.go:53-58, 89-92).
7. **"The notifier is a second authority."** Rejected — dormant best-effort optimization, unwired outside tests; observers recover from `Repository.Events` per the interface contract (interfaces.go:41-45).

### Open questions

1. Is fail-fast-on-any-heartbeat-error deliberate fail-closed doctrine ("never continue external work whose durability is unconfirmable")? If documented somewhere authoritative, F01 downgrades to accepted debt rather than defect; the retry budget (≤5s, well inside the 15s lease) suggests the tradeoff was simply not considered for heartbeat.
2. Should `--force-unlock` remain a raw override for live holders as pure operator escape hatch? If yes, F02 becomes accepted debt — but the sprint lock's stricter design and the existence of `CancelRunLoop` indicate the safer behavior is already the codebase norm.
