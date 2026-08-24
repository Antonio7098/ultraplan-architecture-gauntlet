# FAILURE-02 — Owner death after external work begins

## Scope inspected

Implementation repo (`ultraplan-go` @ eeaa034):

- `internal/runcontrol`: `doc.go`, `model.go`, `lifecycle.go` (Heartbeat, RequestCancellation/Acknowledge, Reconcile, reconcileUnclaimed, reconcileProcessDecision), `sqlite.go` (Claim fencing, ProposeTerminal arbitration), `process.go`/`process_linux.go`/`process_darwin.go`, `fault_test.go`, `process_integration_test.go`
- `internal/app`: `run_control.go` (controlledRuntime owner loop, startup reconcile), `durable_operations.go` (owner tick/heartbeat/reconcile goroutine), `operation_runner.go`, `run_commands.go`, `serve_commands.go`, `tui_commands.go`
- `internal/sprint`: `service.go` (acquireMutation), `verification_lock.go`, `locks.go` (ReconcileInterruptedMutation), `cleanup_uncertain.go`, `verify.go` (attemptExpired/reconcileExpiredAttempts/VerificationStatus), `execute.go`/`execute_state.go`/`state_database.go`, `runtime_metrics.go`, `review.go` (attempt heartbeat), `smoke.go` (harness invocation)
- `internal/study`: `locks.go` (AcquireRunLoopLock, RunLoopActive, CancelRunLoop, ForceUnlockRunLoop), `cleanup_uncertain.go`, `run_loop.go` (resume of active tasks)
- `internal/platform/process`: `process.go`, `process_unix.go`; `cmd/ultraplan/main.go`; `internal/web/server.go`; `internal/app/web_usecases.go`
- External dependency: `github.com/Antonio7098/agentwrap@v0.0.0-20260821190033-e79a38a58da3/opencode/process*.go` (module cache)
- Docs: `docs/architecture.md`, `docs/recovery.md`

Planning workspace (@ 368a789): `projects/ultraplan-go/docs/TRD.md` (:2208, :2225, :2498, :2645, :2667), `docs/PRD.md` (:1212), `sprints/15-documentation-and-packaging/plan.md` (:180–181), `sprints/27-deep-smoke/reasoning.md` (:158, :224), `sprints/31…32` plans.

## Architecture assessment

The core ownership chain for this scenario is sound and unusually well-specified:

- **Accept-before-work**: every runtime call is wrapped by `controlledRuntime.StartRun` (internal/app/run_control.go:122) and every long CLI operation by `durableCLICommand` — accept + claim commit to SQLite before any goroutine or child starts; required-write failure fails closed.
- **Death detection**: heartbeats (5s/15s lease) stop at owner death; any later repository open reconciles at startup (run_control.go:64), each live owner loop reconciles every 10s, and reconciliation waits grace beyond lease expiry, probes exact birth identity (host digest + boot id + birth token, linux and darwin both implemented), never adopts, never signals on PID alone, and proposes one immutable terminal (`ProposeTerminal` CAS, sqlite.go:722–807). Unclaimed accepts converge to `interrupted` with no fabricated attempt (lifecycle.go:427–479; tested in process_integration_test.go:127–233).
- **Product truth stays product-owned**: run control projects status only (runcontrol/doc.go). Execute persists task `running` before starting the runtime turn (execute.go:204–210) and resume converts stale-running→failed with diagnostics (execute.go:600–618); `ReconcileInterruptedMutation` does the same under the product lease (sprint/locks.go:25–88); `VerificationStatus` derives expired review/smoke attempts without mutating (verify.go:154–156); study run-loop resumes active tasks by default (run_loop.go:214–218, :880). Authorities can transiently disagree (run says `interrupted`, execute task says `running`) and resolution is assigned deterministically to the next explicit product operation.

The stress points are at the **edges of the boundary**: the two places that must reason about *whether a foreign process is the same process* (product lease staleness, cross-process study cancel) still use bare-PID liveness, exactly the mechanism the authoritative TRD forbids relying on, and the two external-child spawn sites have asymmetric parent-death semantics.

## Candidate findings

### FAILURE-02-FN1
- **Priority**: P2
- **Claim**: Cross-process study run-loop cancellation signals `SIGINT` to a recorded PID after only a `kill(pid,0)` liveness check, with no boot-id/birth-token verification. After owner death followed by PID reuse — precisely the window this scenario traces — the web/TUI “study cancel” operation delivers SIGINT to an unrelated local process.
- **Evidence**: `internal/study/locks.go:141-159` (`CancelRunLoop`: `RunLoopActive` → `syscall.Kill(info.PID, syscall.SIGINT)`; liveness is the bare-PID probe at :17-23); reachable via `OperationStudyCancel` (`internal/app/operation_runner.go:133-143`). Contrast: `internal/runcontrol/process_linux.go:42-44` / `process_darwin.go:34-38` prove full birth identity is cheaply available in-process; `architecture.md:174-175` and workspace `TRD.md:2208` (“A PID alone is insufficient because of PID reuse”) state the invariant; TRD:2667 asks specifically what prevents “falsely killing” on PID reuse.
- **Architectural reason**: authority/failure-semantics — two cancellation authorities gate the same study run: the fenced, identity-safe `runcontrol.RequestCancellation` path, and this weaker PID-signalling path that bypasses fencing entirely.
- **Concrete consequence**: owner is killed mid-run-loop; OS reuses its PID for an unrelated daemon; user clicks cancel in the dashboard; unrelated process receives SIGINT. Meanwhile the real stale lock (if the reused PID is long-lived) reads as “active”, blocking resume until `--force-unlock`.
- **Counter-evidence searched**: lock content does carry study name/command/acquire-time and `info.Study != study.Name` is checked — but none of it is verified against the process being signalled; `EPERM` is treated as alive (conservative but irrelevant to signalling); no test covers PID reuse for this path (locks_test.go covers conflict/force/release only). The `pid == os.Getpid()` guard only excludes self-signalling.
- **Confidence**: high (mechanism certain; trigger probability moderate-low)
- **Smallest useful action**: store the owner’s runcontrol `ProcessIdentity` (or at minimum proc birth-time) in the run-loop lock and verify it before signalling — or route `OperationStudyCancel` through `RequestCancellation` when a durable run correlates with the lock.

### FAILURE-02-FN2
- **Priority**: P2
- **Claim**: Product mutation-lease staleness (sprint flow/execute/review/smoke; study run-loop) is decided solely by bare-PID aliveness with no acquisition-age bound and no birth identity, so after owner death plus PID reuse the sprint lease is treated as live indefinitely, blocking all mutation of that sprint with no sprint-equivalent of `--force-unlock`.
- **Evidence**: `internal/sprint/verification_lock.go:53-55` (conflict if `verificationProcessAlive(existing.PID)`), `:95-101` (`syscall.Kill(pid,0)`, EPERM=alive; no `AcquiredAt` age check anywhere in `acquireVerificationFileLock` :26-61); same pattern in `internal/study/locks.go:66-83`. Study has a documented escape hatch (`--force-unlock`, recovery.md:120-134); sprint has none — recovery.md:115-116 only says assess, never delete casually. Contract: `TRD.md:2208` makes PID-alone insufficiency CURRENT-CONTRACT for exactly the stale-lease case; runcontrol implements it, product modules predate/drift from it.
- **Architectural reason**: drift/boundary — two authorities serialize the same sprint mutation with different identity strength; the module that owns product truth kept a weaker liveness oracle than the module that observes it.
- **Concrete consequence**: crash mid-execute → PID reused by an editor helper/daemon → every subsequent `flow|execute|review|smoke|verify` on that sprint returns `ErrVerificationConflict` citing a PID belonging to an unrelated process; recovery is blocked until that unrelated process exits or the operator hand-deletes a lock file the runbook tells them not to delete casually.
- **Counter-evidence searched**: failure direction is fail-closed (no corruption, no concurrent writers); graceful owners release the lock via deferred release, so only hard death hits this; modern `pid_max` makes reuse rare on short sessions; `ReconcileInterruptedMutation` correctly skips live leases so web startup is not poisoned. None of these bound the blocking duration.
- **Confidence**: high on mechanism, medium on operational frequency.
- **Smallest useful action**: reuse runcontrol’s birth-token probe (already in-tree) in `verificationProcessAlive`/`processAlive`, or add an `AcquiredAt` staleness bound to takeover; document the sprint-lock stale-takeover/remedy next to the study `--force-unlock` section.

### FAILURE-02-FN3
- **Priority**: P3
- **Claim**: Smoke-harness child processes have no parent-death cleanup on any platform (`configureOwnedProcess` sets only `Setpgid`), while the agentwrap opencode child gets `Pdeathsig=SIGTERM` on Linux and nothing on darwin — asymmetric owner-death semantics between the two external-execution boundaries of the same binary, leaving an orphaned harness writing evidence/targets while run-control already records `interrupted`.
- **Evidence**: `internal/platform/process/process_unix.go:12` (`//go:build linux || darwin`, Setpgid only; used by `smoke.go:75,128`); agentwrap `opencode/process_linux.go` (`Pdeathsig: syscall.SIGTERM`) vs `opencode/process_unix.go` (darwin: Setpgid only); darwin is a supported target (`internal/runcontrol/process_darwin.go`). Reconciliation deliberately never adopts or reaps (architecture.md:174-176), so nothing in UltraPlan stops the orphan.
- **Architectural reason**: failure-semantics/lifecycle — operational truth converges to `interrupted` within ~60s while the actual external writer may still be live; ambiguity resolution is silently delegated to an undocumented manual step (“confirm no harness process remains”, recovery.md:77).
- **Concrete consequence**: SIGKILL mid-smoke → reconciler records `interrupted` → operator resumes/repairs while the orphan harness still mutates the target tree; a resumed attempt races an unsupervised writer that no fence protects (fences cover the run-control DB, not the target repo).
- **Counter-evidence searched**: sprint-27 reasoning (:158, :224) shows descendant cleanup and cleanup-uncertainty were explicitly designed — but only for supervised cancellation/timeout paths; sprint-15 plan (:180-181) HISTORY records the darwin Pdeathsig removal as a build-compat patch, showing awareness but never recording the surviving owner-death gap as accepted debt; stale/non-passing evidence semantics contain the *verdict* damage but not the *concurrent-writer* damage. On Linux the fix is a one-line build-tagged attribute, so “platform limitation” does not explain the Linux half.
- **Confidence**: medium-high.
- **Smallest useful action**: add `Pdeathsig` to `configureOwnedProcess` under the linux tag (mirroring agentwrap), and record the darwin deviation in recovery.md next to the existing “confirm no harness process remains” step.

### FAILURE-02-FN4
- **Priority**: P3
- **Claim**: Recovery documentation drift in the exact procedure operators follow after owner death: recovery.md promises status derives timed-out attempts “after more than 24 hours”, but code expires attempts immediately on dead OwnerPID or after 2 hours without heartbeat.
- **Evidence**: `docs/recovery.md:75` vs `internal/sprint/verify.go:455-467` (`now.Sub(lastSeen) > 2*time.Hour`; `!verificationProcessAlive(attempt.OwnerPID)` → expired immediately).
- **Architectural reason**: drift (contract↔reality) with no behavioral harm — code is stricter than documented.
- **Concrete consequence**: an operator following the runbook waits up to 24h for a derived verdict that appears immediately (or at 2h), eroding trust in the recovery doc during incident response.
- **Counter-evidence searched**: no second code path implements a 24h constant; grep found no other expiry window; likely a stale doc from an earlier iteration (HISTORY).
- **Confidence**: high.
- **Smallest useful action**: update recovery.md:75 to match the implemented dead-PID/2h rule.

## Defended architecture / rejected hypotheses

- **“Run-control terminals can corrupt product truth.”** Rejected: runcontrol is projection-only by charter (runcontrol/doc.go), product reconciliation runs under the product lease and skips live leases (sprint/locks.go:26-33), `VerificationStatus` derives expiry without mutating (verify.go:154-156), and terminal arbitration is a single fenced CAS winner (sqlite.go:753-785). Tests prove idempotent post-death reconciliation (process_integration_test.go:177-233).
- **“A dead owner’s unclaimed run leaves fabricated or adoptable evidence.”** Rejected: `reconcileUnclaimed` records `interrupted`/`owner_never_claimed_after_grace` with no invented attempt or process identity (lifecycle.go:427-479, tested :127-175), matching recovery.md:206-209.
- **“Startup reconciliation rewrites live cross-process work.”** Rejected: `ReconcileOperations` → `ReconcileInterruptedMutation` acquires each sprint lease first and returns neutral on conflict (web_usecases.go:354-368, sprint/locks.go:26-31); matches architecture.md:72-75.
- **“Persistence failures during a run fake success.”** Rejected: event-append or heartbeat failure cancels the owned context and proposes `persistence_degraded` through a fresh 30s background context (run_control.go:156-163, :282-292; durable_operations.go:110-114); typed quota/read-only/closed-repository fault tests confirm no uncommitted success (fault_test.go).
- **“Darwin’s missing Pdeathsig is an unnoticed bug.”** Partly rejected as *accidental*: HISTORY (workspace sprint-15 plan.md:180-181) records the deliberate Linux/Unix split of agentwrap process setup to unblock darwin builds; the residual gap is reported as FN3’s documentation/asymmetry issue, not as an unknown.

## Open questions

- `ReconcileInterruptedMutation` returns `ErrCleanupUncertain` when a `.cleanup-uncertain.json` marker exists but nothing changed (sprint/locks.go:84-86), which fails `serve` startup (server.go:76-81) until the marker changes state or is manually removed. Intentional fail-closed attention-forcing, or a permanent-startup trap after an actually-clean shutdown? Could not determine intent from docs/tests.
- `OperationSmokeStart` builds its service without `WithRuntime` (operation_runner.go:75) unlike every other operation kind — irrelevant to owner death, but it may mean browser/TUI smoke cannot reach agent-backed authoring; flagged only as a possible adjacent wiring inconsistency worth a specialist look.
