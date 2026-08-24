Now let me read the supporting files: cleanup_uncertain.go, smoke_types.go (for smokeError), smoke_author.go, and the locks.
# Failure-11 Investigation Report — Smoke-harness timeout, process-tree cleanup, partial evidence, and review/smoke state updates

## Scope inspected

Implementation repo `/home/antonioborgerees/coding/ultraplan/gauntlet-run/ultraplan-go` @ `eeaa034` (read-only):

- `internal/platform/process/process.go`, `process_unix.go`, `process_other.go`, `process_test.go` — direct harness execution, timeout, group kill, capture
- `internal/sprint/smoke.go`, `smoke_types.go`, `smoke_author.go`, `smoke_protocol.go`, `smoke_test.go` — run lifecycle, failure classification, flow-state writes
- `internal/sprint/verify.go` (`attemptExpired`, `reconcileExpiredAttempts`, `VerificationStatus`), `locks.go`, `verification_lock.go`, `verification_lock_test.go`, `locks_test.go`, `cleanup_uncertain.go`
- `internal/sprint/session_state.go`, `review.go` (heartbeat contrast), `service.go` (`runtimeRequest`, mutation lease)
- App/web surfaces: `internal/app/operation_runner.go`, `sprint_commands.go`, `durable_operations.go`, `operations.go`, `usecases.go`, `web_usecases.go`; `internal/web/operations.go`, `operation_handlers.go`; `internal/tui/model.go`
- Runtime boundary: `internal/platform/runtime/runtime.go` (timeout mapping, cancellation)
- Authoritative docs: workspace `system/protocols/deep-smoke-sprint-protocol.md`; repo `docs/planning-smoke.md`, `docs/recovery.md`, `docs/configuration.md`, `docs/local-web.md`, `docs/plans/server-shutdown-run-cancellation-contract.md`
- History: `git log`/`git show` for `a221683`, `b9733ce`, `f142a73` (drift provenance)

## Architecture assessment

The core ownership design for scenario 11 is sound and unusually deliberate. Truth ownership is cleanly separated: the external harness owns raw run/issue evidence; the sprint owns only `smoke.md` + smoke flow state; `pprocess.DirectRunner` owns process-tree containment. On timeout/cancel, the group is TERM→KILL escalated within grace, partial stdout is still captured (`TestDirectRunnerBoundsAndTimeout` asserts `"1234"` retained), `smoke.go:143-145` refuses all non-clean outcomes, `smoke.md` is never written on failure, and `saveSmokeAttempt` preserves `LastComplete` verdict/evidence so the previous authoritative result stays visible (`smoke.go:204-206`). Dead-owner recovery exists (`ReconcileInterruptedMutation`, PID-liveness lock takeover). The stress points found are cross-surface drift (web/TUI wiring), two unbounded waits that contradict the platform's own bounded-cleanup doctrine, and a reconciler horizon that collides with product-authorized timeouts.

## Candidate findings

### FAILURE-11-F01

- **Priority:** P1
- **Claim:** TUI and web `smoke-start` operations construct the sprint service without an agent runtime, so every non-dry-run smoke launched from those surfaces deterministically fails at the authoring gate after already mutating smoke flow state.
- **Evidence:** `internal/app/operation_runner.go:74-75` — `case OperationSmokeStart: service := sprint.NewService(root.Path).WithSmokeSettings(...)` with no `WithRuntime`/`WithStageRuntime`, unlike every sibling runtime op (`:22-23` stage, `:36` flow, `:49` execute, `:60` review, `:93` verify all use `sprintRuntimeService`, which wires `WithRuntime(controlled, req)` at `sprint_commands.go:498`). `internal/sprint/smoke_author.go:21-23` hard-fails on nil runtime (`smoke_author_runtime`). Surfaces expose it: `internal/tui/model.go:485-486` ("Run Smoke [EXTERNAL]"), `internal/web/operation_handlers.go:660`. CURRENT-CONTRACT: `docs/local-web.md:42,48` lists smoke as a supported web operation/state. Durable CLI smoke is unaffected (`sprint_commands.go:436` uses `sprintRuntimeService`).
- **Architectural reason:** drift (change-surface). HISTORY: `a221683` created this runner case before `b9733ce` made the author runtime mandatory the same day; the case was never updated. Even if a runtime were injected, missing `WithStageRuntime` would also drop the `planning.smoke_model` routing used by `runtimeRequest`.
- **Concrete consequence:** each TUI/web attempt writes flow-state running→failed (`smoke.go:30,34`) plus a failed durable run for work that can never start; users are told to "Configure the smoke model/runtime" though config is identical to the CLI path that works. `verify --to smoke` remains the only working guarded entry from those surfaces.
- **Counter-evidence searched:** dry-run path is intentionally runtime-free and returns before authoring (`smoke.go:60-65`) — consistent; no doc declares web/TUI smoke unsupported; no test covers `OperationSmokeStart` execution end-to-end (`grep OperationSmokeStart` in tests shows only inventory/contract listings).
- **Confidence:** high
- **Smallest useful action:** make the `OperationSmokeStart` case use `sprintRuntimeService(deps, root, tuiSprintRuntimeProgress(emit))` like its siblings; add one runner-level test asserting the author request carries the stage model.

### FAILURE-11-F02

- **Priority:** P2
- **Claim:** On linux/darwin, `DirectRunner.Run` contains two unbounded waits after the kill decision, so `Timeout + CleanupGrace` is not an upper bound on run latency, and the `CleanupComplete=false` outcome the Windows branch implements is unreachable there.
- **Evidence:** `internal/platform/process/process_unix.go:33` — after group SIGKILL, `return <-waited, true` blocks indefinitely (SIGKILL delivery can stall indefinitely for a leader in uninterruptible D-state). `process.go:124` — unconditional `drains.Wait()`: a descendant that escapes the process group (double-fork/`setsid` daemonization — plausible for browser/db daemons this harness explicitly probes) but inherits the stdout/stderr pipe write-ends prevents EOF, so `copyStream` (`process.go:185-197`) blocks forever and `Run` never returns. Contrast `process_other.go:17-23`, which bounds the wait by grace and reports `(nil, false)` → `!CleanupComplete` → `smoke_cleanup` guidance (`smoke.go:587-588`). Contract: `deep-smoke-sprint-protocol.md:41` ("context timeout/cancellation, **bounded** … descendant cleanup"); `docs/planning-smoke.md:93`; recovery doc expects terminal `timeout/cancellation/cleanup` states.
- **Architectural reason:** failure-semantics/lifecycle. The boundary built to bound external harness behavior fails open: the wedge propagates upward.
- **Concrete consequence:** a wedged `Run` leaves `RunSmoke` between the running and terminal `saveSmokeAttempt` calls; flow-state stays `running`, and because the owner PID is alive, `verification_lock.go:53-54` returns `ErrVerificationConflict` to every subsequent sprint mutation from any process until the stuck process is killed externally. Partial evidence captured so far is never reported.
- **Counter-evidence searched:** `TestDirectRunnerCancellationCleansOwnedDescendant` proves in-group descendants die (double `Kill(-pgid)` closes the leader-exits-first race, `process_unix.go:27-29`); Go stdlib `Wait` has the same reaping constraint, but this repo's own other-platform branch and its `cleanup_uncertain` taxonomy demonstrate the intended bounded design, so the asymmetry is internal inconsistency, not platform necessity.
- **Confidence:** high (mechanics explicit in code); trigger frequency low-medium
- **Smallest useful action:** bound the post-signal phase: give `stopAndWait`'s final wait and `drains.Wait()` a `CleanupGrace` deadline; on expiry return `CleanupComplete=false` (mirroring `process_other.go`) so existing `smoke_cleanup` classification and recovery guidance engage unchanged.

### FAILURE-11-F03

- **Priority:** P2
- **Claim:** Smoke attempts are declared timed-out/interrupted by reconciliation while the run is legitimately alive, because smoke heartbeats are written once and the implemented staleness horizon (2h) is four times shorter than both the documented horizon (24h) and the maximum authorized smoke timeout (24h).
- **Evidence:** `internal/sprint/smoke.go:192` sets `HeartbeatAt` once at attempt start; no writer refreshes it during `runSmoke` (contrast review resume: `review.go:836-838`). `verify.go:455-467`: a live-PID attempt still expires when `now-lastSeen > 2*time.Hour` (`:466`); `reconcileExpiredAttempts` (`verify.go:484-496`) then persists `AttemptTimedOut` + `SmokeFailed` with diagnostics "expired without a terminal update". Persisted by `ReconcileInterruptedMutation` (`locks.go:67-75`) at server startup (`web_usecases.go:354-367`); derived (shown as timed out) by every `VerificationStatus` read (`verify.go:154-156`). CURRENT-CONTRACT conflict: `docs/recovery.md:75` documents a **24-hour** terminal-update horizon; `docs/configuration.md:186` and `sprint_commands.go:557` authorize run timeouts up to **24h**.
- **Architectural reason:** lifecycle/drift. The liveness field exists exactly to distinguish "alive but quiet" from "dead", and the smoke module — the longest-running attempt producer — is the one that never feeds it; code and doc disagree on the fallback horizon.
- **Concrete consequence:** an operator starts a 3–24h authorized smoke; any server restart or reconcile pass flips durable status to failed/timed-out with wrong guidance ("Confirm no harness process remains") while the harness runs; a rerun then hits a live-owner lock conflict (`verification_lock.go:53-54`). State self-heals only when the real terminal save lands.
- **Counter-evidence searched:** default `run_timeout` is 30m < 2h, so default-config runs are safe; review attempts share the pattern but their runtime timeout is separately bounded; PID-liveness short-circuits expiry only until the 2h constant fires. No heartbeat refresh exists anywhere in the smoke path (`grep HeartbeatAt\s*=`).
- **Confidence:** high
- **Smallest useful action:** either refresh `ActiveAttempt.HeartbeatAt` alongside smoke progress events (one write per progress emit, matching the review precedent), or align `attemptExpired` with the documented 24h horizon; pick one source of truth for the constant.

### FAILURE-11-F04

- **Priority:** P3
- **Claim:** Scenario-11 behavior is unpinned at the sprint-service level: no test exercises a timing-out/cancelling/unclean harness through `RunSmoke`, so the timeout→state contract (`AttemptTimedOut`, failed status, `LastComplete` preservation, cleanup-uncertain classification) is protected only by untested wiring plus process-package tests.
- **Evidence:** `smokeRecordingRunner` (`smoke_test.go:31-54`) cannot inject errors or `TimedOut/Cancelled/CleanupComplete=false` results; `grep AttemptTimedOut` in tests hits only review-attempt cases (`verify_test.go:143,175`); `classifyProcessSmokeError` (`smoke.go:581-592`) and `saveSmokeAttempt`'s terminal branch (`smoke.go:193-207`) have no direct coverage. Process mechanics alone are tested (`process_test.go:32-69,84-97`).
- **Architectural reason:** change-surface/failure-semantics. The exact transitions this fault scenario depends on are the least regression-protected seam between the platform and state layers.
- **Concrete consequence:** a refactor of `classifyProcessSmokeError` ordering or the `LastComplete` restore could silently break recovery guarantees that `docs/recovery.md:77,84` promise operators.
- **Counter-evidence searched:** malformed-run preservation *is* tested (`TestSmokeRunCommitsValidatedArtifactAndPreservesItOnMalformedRun`), so the harness fixture pattern exists; extending it is cheap.
- **Confidence:** high
- **Smallest useful action:** add error fields to `smokeRecordingRunner` and assert flow-state outcomes for one timeout, one cancellation, and one `!CleanupComplete` run.

## Defended architecture / rejected hypotheses

- **"Timeout discards partial evidence."** Rejected: partial stdout/stderr remain captured in `Result` (`process.go:128`, asserted by test), and refusing to commit a summary from a killed run is the documented contract (`planning-smoke.md:93`); raw evidence stays harness-owned by design.
- **"A timeout racing completion corrupts smoke.md or flow state."** Rejected: `commitSmoke` runs only post-validation via temp+fsync+rename (`smoke.go:692-723`); failures preserve the prior artifact and restore `LastComplete` fields into stage state (`smoke.go:204-206`). Even complete-but-late protocol JSON is conservatively rejected (`smoke.go:143`).
- **"Group-kill races orphan descendants generally."** Mostly rejected: `Setpgid` (`process_unix.go:12`) plus the deliberate second `Kill(-pgid, SIGKILL)` closes the leader-exits-first race and is tested. Residual exposure requires deliberate daemonization, which I folded into FAILURE-11-F02 rather than treating as a kill-design flaw.
- **"Cancellation is misclassified as failure."** Rejected: `SmokeError.Unwrap` preserves `context.Canceled` (`smoke_types.go:263-272`); `smokeFailedResult` maps to `SmokeCancelled` and attempts to `AttemptCancelled`. Minor nuance: during the *authoring* phase, cancellation surfaces category `runtime` because the `runErr != nil` return precedes the ctx check (`smoke_author.go:107-118`) — cosmetic, not reported as a finding.
- **"Writing `running` state before the review gate pollutes state."** Rejected: it is an intentional audit trail; the deferred terminal write always records the outcome in-process, and crash-leftovers are recovered by dead-owner reconciliation.

## Open questions

- Does agentwrap treat `Timeout: 0` as unbounded? (Only relevant if F01 is "fixed" by injecting a bare runtime without `runtimeConfig`; reinforces using `sprintRuntimeService`.)
- Is the 2h constant in `attemptExpired` intentionally stricter than `recovery.md`'s 24h (e.g., a deliberate later tightening), or accidental drift? History of `verify.go:466` would settle F03's framing.
- Are there deployment norms where smoke manifests declare >2h timeouts in practice? If none ever do, F03 drops to P3.
