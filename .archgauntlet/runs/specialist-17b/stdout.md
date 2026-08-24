Large repo with a clear error/retry surface area. Let me read the core files for this assignment in parallel.
# SPECIALIST-17B — Errors / retry / idempotency / recovery

## Scope inspected

- `internal/platform/runtime/` — runtime.go, opencode.go, events.go (+ tests); agentwrap@v0.0.0-20260821190033 `errors.go`, `policy.go` (pinned module) for the authoritative category taxonomy
- `internal/study/` — run_loop.go, run.go, execution_domain.go, run_state.go, cleanup_uncertain.go, locks.go, runtime_metadata.go, run_all.go, state.go, run_history.go (+ run_loop_test.go, cleanup_uncertain_test.go)
- `internal/sprint/` — execute.go, execute_state.go, locks.go, service.go (Status), domain.go, session/resume surfaces
- `internal/app/` — durable_operations.go, operation_runner.go, run_control.go, run_commands.go, study_commands.go, web_usecases.go, usecases.go, app.go
- `internal/runcontrol/` — errors.go, lifecycle.go, model.go, process_*.go (+ lifecycle_test.go)
- `internal/web/` — operations.go, operation_handlers.go, server.go (+ operations_test.go)
- Authoritative docs: `ultraplan-workspace/projects/ultraplan-go/docs/TRD.md` §14 (retry/fallback), §18.5 (run-state recovery), §18C/§21.2 (liveness, locks); roadmap Sprints 11–12
- Verified: SIGINT/SIGTERM → context via `signal.NotifyContext` (cmd/ultraplan/main.go:19)

## Architecture assessment

The error/retry architecture is unusually disciplined. Three coherent tiers own distinct concerns: agentwrap owns in-run retry/fallback classification (16 categories, `RetryAfter`, redaction; TRD §14.1 compliance verified in opencode.go:32-52); product modules own task-level retry policy (`executionShouldRetry`, run_loop.go:759-766) with session-checkpoint resume compatibility (run.go:112-153, fingerprint-gated); runcontrol owns durable ownership with fencing, CAS transitions, process-birth identity, reconciliation, and explicit `cleanup_uncertain` outcomes (lifecycle.go:301-495). Web operations add confirmation digests, dedup keys, idempotent cancel (`cancelOnce` + terminal check, operations.go:345-372), and a tested shutdown drain that persists cleanup uncertainty before terminal projection (operations.go:477-555). CLI exit codes 0–8 with stable codes give scripts actionable recovery signals (app.go:15-25). Cleanup uncertainty is handled at three layers with consistent `server_shutdown` reason discipline.

The stress points are at the seams between the older Phase-1 filesystem mechanisms (study lock, sprint run-state) and the Sprint-35 identity machinery, and in how benign cancellation errors are allowed to contaminate the first-error channel of the study run loop.

## Candidate findings

### SPECIALIST-17B-F01
- **Priority:** P2
- **Claim:** Graceful cancellation (SIGINT drain) of the study run loop usually fails to persist terminal task states: debounced persists return `ctx.Err()` during the 250 ms coalescing window, that error is recorded as a hard failure, and the first-error early return skips the forced final save, history sync, and result population.
- **Evidence:** internal/study/run_loop.go:143-175 — non-forced `persist` waits on a debounce timer and returns `ctx.Err()` when ctx fires first (:156-158); every task transition goes through `update`→`persist(id,false)` (:177-189); on cancel, `markTaskCancelled`/`applyExecutionResult` errors feed `recordErr` (:237-244, :309-310); :381-383 returns before the forced `save()`/`SyncRunHistory`/`result.State` population at :385-396. Also :267-307 — `runTask` proceeds to execute the runtime call even when the Running-transition persist already failed.
- **Architectural reason:** failure-semantics — a liveness artifact of the persist coalescer is conflated with execution failure in the single `firstErr` channel.
- **Concrete consequence:** After Ctrl-C of a multi-minute run, the state file commonly still shows tasks Running/Retrying while memory held Cancelled/Completed; completed-but-unpersisted analyses are re-executed from scratch on next resume (duplicate provider cost); run-history misses its final sync; callers receive empty `result.Status`/`Counts`. Recovery remains safe (ResumeValidateRunState maps active→pending, run_state.go:293-312), so this is evidence/cost loss, not corruption.
- **Counter-evidence searched:** Tests cover pre-cancelled contexts (run_loop_test.go:307-335) and runtimes that *report* cancellation with a live context (:44-90), but never real ctx cancellation racing the debounce window; the package comment ("intentionally resumable rather than exactly-once", run_loop.go:16-22) covers correctness, not the skipped final save. No deferred save exists.
- **Confidence:** high (mechanism), medium (frequency — depends on last-persist timing)
- **Smallest useful action:** Treat `ctx.Err()` from a debounced persist as non-recording (or force-save once on drain), and let the post-loop `save()`+`SyncRunHistory` run whenever the only error is cancellation.

### SPECIALIST-17B-F02
- **Priority:** P2
- **Claim:** `sprint execute --resume` silently discards an unloadable existing `.run-state.json` (malformed, unsupported schema version, or invalid content) and proceeds as a fresh execution that overwrites it, while sibling paths treat the identical condition as a hard, diagnostic-bearing error.
- **Evidence:** internal/sprint/execute.go:175-177 — `if existing, loadErr := LoadExecuteRunState(...); loadErr == nil && req.Resume { state = reconcileExecuteState(...) }`; any load error falls through to the fresh `NewExecuteRunState` created at :174, which `SaveExecuteRunState` then persists over the old file (:178, execute_state.go:105-118). Contrast: ReconcileInterruptedMutation returns the error (locks.go:64-66), Status returns it (service.go:202-208), and Load itself produces precise classified errors including `ErrExecuteRunStateUnsupported` (execute_state.go:188-189).
- **Architectural reason:** drift — one recovery policy inside a module whose other entrypoints enforce the opposite policy.
- **Concrete consequence:** A state file written by a newer binary (schema bump) or externally damaged file + `--resume` resets all task progress without a finding or diagnostic; completed work is re-executed and the stale-running/deferral evidence chain is destroyed. TRD §18.5 requires loading to "reject malformed JSON and unsupported schema versions with clear diagnostics" — the rejection exists but the resume path swallows it.
- **Counter-evidence searched:** prepareExecute's `validateResolvedResumeTasks` catches the condition only when plan.md checkboxes are checked/deferred (execute.go:350-352, :366-399); DB-authoritative mode bypasses JSON but JSON is the default path; no test exercises corrupt-state resume; no doc declares best-effort resume intentional.
- **Confidence:** high (mechanism), medium (reachability)
- **Smallest useful action:** On `req.Resume` with a load error other than `ErrExecuteRunStateMissing`, return a validation finding (or fail) instead of silently substituting fresh state.

### SPECIALIST-17B-F03
- **Priority:** P2
- **Claim:** Study run-loop ownership liveness and cross-process cancel rely on bare-PID checks, so a recycled PID makes a dead owner look live: `CancelRunLoop` then delivers SIGINT to an unrelated process, and reconciliation silently no-ops.
- **Evidence:** internal/study/locks.go:17-23 (`processAlive` = `kill(pid,0)`, EPERM counted alive); :66-83 (stale-lock takeover keyed only on liveness); :141-159 (`CancelRunLoop` sends `SIGINT` to `info.PID` after only a self-PID and study-name check). The same blind spot gates `ReconcileInterruptedRun`, which returns `(false,nil)` when the lock looks held (cleanup_uncertain.go:74-79). Meanwhile runcontrol models exactly this hazard with `ProcessIdentity{HostDigest, BootID, PID, BirthToken}` (model.go:193-204, process_linux.go:43) and even has a test named "pid reuse is interrupted" (lifecycle_test.go:186).
- **Architectural reason:** boundary/lifecycle — Phase-1 lock contract (TRD §21.2 permits PID+command+timestamp) was never upgraded to reuse the birth-identity machinery the same repository already ships, though §21.2 also requires conservative stale detection.
- **Concrete consequence:** On a busy machine where PIDs wrap, a user clicking study-cancel in the dashboard can signal an unrelated local process; interrupted-task reconciliation is skipped indefinitely until manual `--force-unlock`.
- **Counter-evidence searched:** Lock records command and timestamp but neither is verified against the live process; no boot-time/start-time cross-check anywhere in study; TRD line 2208's conservative-liveness clause is written for Sprint-35 runs, which comply — the gap is specific to the study lock surface.
- **Confidence:** high (mechanism), medium (practical probability)
- **Smallest useful action:** Record the owner's process birth identity (reuse `runcontrol.ProcessIdentity` probing or `/proc/<pid>` starttime) in the lock and verify it in `processAlive`/`CancelRunLoop`.

### SPECIALIST-17B-F04
- **Priority:** P3
- **Claim:** Only `run-loop` takes the per-study lock; `study run-all`, `study run`, and `study synthesize` execute agents that write the same report/summary artifacts with no mutual-exclusion or detection against an active run-loop.
- **Evidence:** internal/study/run_all.go:13-47 (no `AcquireRunLoopLock`); app/study_commands.go:535-565 (run-all), :790/:815 (single run/synthesize) — none check `RunLoopActive`; report paths are shared via `RunAnalysis`/`Synthesize` (run.go:38, :51).
- **Architectural reason:** boundary — the lock's authority is scoped to one orchestrator rather than to the study's mutable state, so duplicate concurrent execution of identical work is undetectable.
- **Concrete consequence:** A run-all started beside an active run-loop duplicates provider spend and interleaves writes to the same output files; subsequent resume revalidation flaps while agents mid-write reports.
- **Counter-evidence searched:** TRD §21.2 mandates the lock only "for run-loop", and single-shot commands are documented user-invoked primitives; web/TUI expose only RunLoop (operation_runner.go:108-128). So this is contract-compliant but leaves the mutation-safety goal ("prevent accidental concurrent mutation", §21.2 preamble) enforced by convention only.
- **Confidence:** high (mechanics), low-med (practice)
- **Smallest useful action:** Have single-shot execution paths take the study lock non-forced (or refuse when `RunLoopActive`) instead of extending lock scope everywhere.

## Defended architecture / rejected hypotheses

- **"`model_unavailable` missing from `executionShouldRetry` is a retry gap"** — rejected. It is deliberate product policy layered over agentwrap's in-run PolicyRunner (opencode.go:32-36); `studyContinuationNeedsFreshFallback` treats it consistently as non-transient (run.go:157-171), and Failed tasks are re-runnable on any later invocation anyway (run_loop.go:608-619), so nothing is stranded.
- **"Failed tasks being runnable causes unbounded automatic retries"** — rejected: bounded per invocation by the `attempted` map and scheduling loop; resumption-retries are the documented design (run_loop.go:16-22; TRD §18.5 "recovered to a retryable or failed state").
- **"Silent degradation to non-durable web operations on `ErrWebUnavailable` hides persistence failures"** — rejected: that sentinel is produced only when the durable capability is absent (read-only mode, usecases.go:102-107); genuine repository failures propagate and cancel the operation (operations.go:183-187, durable_operations.go:106-114 proposes `TerminalPersistenceLost`).
- **"Two stacked durable wrappers (operation manager + controlledRuntime) duplicate ownership"** — rejected: they implement the Sprint-35 operation-vs-runtime identity hierarchy; each layer has distinct fences, terminal arbitration, and event streams, with retry helpers shared (run_control.go:305-332).
- **"`retryAfterFromAgent` restarts the Retry-After clock at study level, inflating waits"** — minor and conservative-direction only (runtime_metadata.go:171-201 takes the minimum across decisions/attempts); not reported as a defect.
- **"Adapter.StartRun discards events if Wait doesn't finish within 5 s of cancel"** — bounded diagnostics loss only; sessions are checkpointed through the event stream before teardown, preserving resumability (runtime.go:277-325).

## Open questions

- Is there documented intent that CLI Ctrl-C need not land terminal states durably because reconcile-on-resume suffices? Nothing found in tests, help text, or workspace docs; the answer changes F01's priority.
- Does the project support running an older binary against a newer-schema sprint run-state (a migration matrix exists only for the runcontrol store)? If yes, F02 becomes P1.
- Were study locks deliberately excluded from the Sprint-35 process-birth identity work as frozen Phase-1 surface, or simply never revisited? This determines whether F03 is debt or a conscious compatibility decision.
