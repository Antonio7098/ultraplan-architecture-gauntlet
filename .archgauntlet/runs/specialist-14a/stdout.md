Now let me find the consumers of this process package — harness selection and evidence ownership.
There are two process-boundary implementations (`platform/process` and `runcontrol/process*`). Let me read the second one and the smoke harness files.
The smoke protocol is substantial. Now let me read the authoring path, settings, and the raw-exec call sites that bypass `platform/process`.
Let me verify the config validation for `smoke.environment` and check the docs for the smoke protocol contract.
Let me empirically verify the StdoutPipe/Wait truncation race hypothesis before reporting it.
# SPECIALIST-14A — External process boundary, harness selection, cancellation & cleanup

## Scope inspected

**Implementation repo** (`ultraplan-go` @ eeaa034, clean):
- `internal/platform/process/`: `process.go`, `process_unix.go`, `process_other.go`, `process_test.go`
- `internal/sprint/`: `smoke.go`, `smoke_protocol.go`, `smoke_author.go`, `smoke_types.go`, `verify.go`, `verification_lock.go`, `cleanup_uncertain.go`, `service.go`, `smoke_test.go`
- `internal/app/`: `operation_runner.go`, `operations.go`, `sprint_commands.go`, `usecases.go`, `durable_operations.go`
- `internal/web/`: `operations.go`, `operation_handlers.go`
- `internal/study/`: `init_clone.go`, `init.go` (call site), `locks.go`
- `internal/runcontrol/`: `process.go`, `process_linux.go`, `lifecycle.go` (probe usage), `import_boundary_test.go`
- `internal/platform/runtime/`: `runtime.go`, `agentwrap.go`; `internal/platform/config/config.go`

**Authoritative workspace**: `system/protocols/deep-smoke-sprint-protocol.md`, `system/contracts/core/architecture.md`, `system/contracts/core/security.md`.

**Verification commands**: grep-based call-site census of `os/exec`; empirical StdoutPipe/Wait race reproduction in `/tmp/opencode/piperace` (target repos untouched).

## Architecture assessment

The external-process surface is deliberately split by lifetime and trust: `platform/process.DirectRunner` owns untrusted, bounded, killable harness execution (setpgid group ownership, TERM→grace→group KILL, bounded capture, drop-counted progress); `platform/runtime.Adapter` delegates agent processes to `agentwrap` with cancellation fencing; run-control adds exact cross-restart process identity (`boot_id`+birth token) for durable runs; narrow git identity probes use raw `exec.Command` inline. This is coherent module-driven ownership, not duplication. The stress points are at the seams: composition wiring for one operation kind, a stdlib pipe-lifetime hazard inside DirectRunner, an env allowlist that silently inverts under an empty resolution, and three places where liveness/timeout semantics drift from the owned boundary.

## Candidate findings

### SPECIALIST-14A-F01
- **Priority:** P1
- **Claim:** TUI/web `OperationSmokeStart` builds its sprint service without a runtime, so every non-dry-run smoke launched from those surfaces deterministically fails at authoring — despite the confirmation contract declaring it a runtime-backed external mutation.
- **Evidence:** `internal/app/operation_runner.go:74-75` constructs `sprint.NewService(root.Path).WithSmokeSettings(...)` only; compare sibling runtime kinds using `sprintRuntimeService` (`WithRuntime(controlled,...)` + `WithStageRuntime`) at `operation_runner.go:23,36,49,60,93` and the equivalent CLI smoke path `internal/app/sprint_commands.go:430-441`. `authorSmokeSuite` hard-fails without a runtime: `internal/sprint/smoke_author.go:21-23`. Contract side declares runtime + external harness: `internal/app/operations.go:215-218` ("EXTERNAL HARNESS + SMOKE ARTIFACT WRITE") and `operations.go:498-499` ("configured smoke author and harness"). Reachable from TUI nav `internal/tui/model.go:485-486` → `tui/app.go:286` → `dashboardUseCases.RunOperation` default → `u.runner` (`operations.go:399-403`) and from web serve wiring `serve_commands.go:63`. The durable manager persists/lifecycle-wraps but does not execute (`durable_operations.go`), so it cannot compensate.
- **Architectural reason:** drift / change-surface (composition authority split between per-kind cases; one case missed when runtime plumbing moved behind `sprintRuntimeService`).
- **Concrete consequence:** operator confirms a mutating external operation; flow-state is flipped to `running` (`smoke.go:30`), authoring fails with "runtime is required to author deep-smoke coverage", attempt is recorded failed. Smoke is unreachable from exactly the surfaces whose job is one-click verification; `planning.smoke_model` overrides are also silently dropped on this path.
- **Counter-evidence searched:** searched for alternate execution paths (durable runs page, web_usecases), tests exercising `OperationSmokeStart` through `sharedOperationRunner` (none; only a kind inventory in `run_control_inventory_test.go:26`), docs marking TUI/web smoke as dry-run-only (none found).
- **Confidence:** high
- **Smallest useful action:** route `OperationSmokeStart` through `sprintRuntimeService(deps, root, ...)` like `OperationVerifyStart`, or explicitly reject non-dry-run smoke-start at confirmation time until wired.

### SPECIALIST-14A-F02
- **Priority:** P2
- **Claim:** `DirectRunner` reads stdout/stderr via `StdoutPipe` while `cmd.Wait()` runs concurrently, so Wait's post-exit pipe close can discard buffered tail bytes — valid harness JSON can be truncated and misclassified as a protocol violation.
- **Evidence:** `internal/platform/process/process.go:88-94` (pipes), `:108-109` (Wait goroutine), `:124` (drains waited after select). The os/exec contract states Wait closes pipes after exit and callers must not Wait before reads complete. Empirical mirror of this exact pattern lost the tail in 3/4000 runs of a single-write fixture (`/tmp/opencode/piperace`). Downstream misclassification: `smoke.go:79-84` (`smoke_discovery_truncated`/`smoke_discovery_malformed`) and `:132-141` (`smoke_run_truncated`/`smoke_run_malformed`).
- **Architectural reason:** failure-semantics at the sole production choke point for harness evidence (only consumers of `pprocess` are `sprint/smoke.go` and `service.go` defaults).
- **Concrete consequence:** intermittent false "harness returned malformed protocol" verdicts that direct operators to fix a healthy harness; retry succeeds, eroding trust in diagnostics and wasting a full harness run (cost class can be expensive). Fails closed, never accepts invalid evidence — hence P2, not P1.
- **Counter-evidence searched:** no comment or test acknowledges the Wait/read ordering; existing tests use tiny outputs where the reader usually wins (`process_test.go`); timeout/cancel paths are unaffected (kill makes loss moot).
- **Confidence:** high (mechanism verified empirically; field frequency low)
- **Smallest useful action:** assign `cmd.Stdout`/`cmd.Stderr` to the capture writers instead of `StdoutPipe` (exec-owned copiers are awaited by Wait), keeping limits/dispatch unchanged; or drain both pipes to EOF before treating `waited` as terminal on the normal-exit path.

### SPECIALIST-14A-F03
- **Priority:** P2
- **Claim:** An empty resolved environment allowlist silently degrades to full parent-environment inheritance for the external smoke harness, inverting containment; the pinned test invariant, the authoritative protocol, and the rendered artifact all say otherwise.
- **Evidence chain:** `smokeEnvironment` returns nil when no allowlisted name has a value (`smoke_protocol.go:623-642`); `Request.Env=nil` → `append([]string(nil))` stays nil → Go exec inherits the whole parent env (`process.go:86`); clearing the allowlist is a supported config state (`config.go:200`, `:239-242`, `clearListField` at `:516-517`; validation checks names only, `:458-462`). Intent is pinned by `smoke_test.go:108-128` (non-allowlisted values must not pass); CURRENT-CONTRACT requires "allowlisted environment" (`deep-smoke-sprint-protocol.md:41`); yet `RenderSmoke` unconditionally asserts "Environment: bounded allowlist" into the durable artifact (`smoke.go:489`).
- **Architectural reason:** boundary (containment semantics of the product→harness handoff) + drift (artifact claim vs actual behavior).
- **Concrete consequence:** with a cleared/unresolvable allowlist, every secret in the ultraplan process environment (provider keys, tokens) is exported into the untrusted harness process, while smoke.md records the opposite. The `Request.Env` API cannot even express "empty env", so callers cannot defend themselves.
- **Counter-evidence searched:** no doc/test defines empty-allowlist as inherit-by-design; security contract warns about subprocess exposure (`security.md:14-26`); default settings usually yield PATH/HOME so the common path is bounded — the defect is the empty-resolution corner plus the API gap.
- **Confidence:** high
- **Smallest useful action:** make `DirectRunner` distinguish unset vs empty `Env` (e.g., explicit sentinel or always emit a non-nil slice) and have `smokeEnvironment` fail closed with guidance ("allowlist resolved empty") instead of returning nil.

### SPECIALIST-14A-F04
- **Priority:** P3
- **Claim:** `study init`'s git clone is the only external process in the repo with no timeout, no cancellation path, and no descendant-cleanup semantics.
- **Evidence:** `internal/study/init_clone.go:44-50` (`exec.Command("git","clone","--depth","1",...)` + `CombinedOutput`); `CloneRunner.Clone(url, dest)` and `runCloneActions` carry no context (`init_clone.go:38-40,72-93`); `Init(req)` itself is context-free (`init.go:55,84`). Every other boundary carries explicit timeouts (`process.go:71-73` requires positive timeout; runtime requests carry `Timeout`).
- **Architectural reason:** failure-semantics / lifecycle inconsistency at one seam of the same concept.
- **Concrete consequence:** a network-stalled clone hangs study-init indefinitely in non-interactive contexts with no timeout category in `CloneFailure`; future embedding (web/TUI operations) inherits an uncancellable primitive because the seam itself omits context.
- **Counter-evidence searched:** interactive Ctrl-C does reach the child (same foreground process group), mitigating CLI harm; redaction and partial-failure taxonomy are present and good; architecture contract warns against ceremony abstractions (`architecture.md:156`) — but a ctx parameter is not ceremony here given every sibling process owns one.
- **Confidence:** high (mechanism), medium (practical impact)
- **Smallest useful action:** add `context.Context` to `CloneRunner.Clone` and use `exec.CommandContext` with a bounded timeout; classify deadline as a retryable clone-failure category.

### SPECIALIST-14A-F05
- **Priority:** P3
- **Claim:** Sprint/study file locks answer "is the owner alive?" with bare-PID `kill(pid,0)`, while the repo already owns exact process-birth identity (runcontrol) used elsewhere — leaving PID-recreuse hazards in lock reconciliation and cross-process cancel.
- **Evidence:** `internal/sprint/verification_lock.go:95-100`; `internal/study/locks.go:17-23` and SIGINT-by-PID at `locks.go:155`; contrast `runcontrol/process.go:20-45` + `lifecycle.go:481-491` (PID reuse ⇒ interrupted, correctly) consumed by durable runs (`app/durable_operations.go:211`, `app/run_control.go:64,92`). The verification file lock has no time-based fallback (unlike attempt expiry's 2h heartbeat window, `verify.go:455-467`).
- **Architectural reason:** lifecycle (ownership truth for locks) / change-surface (two answers to one concept question).
- **Concrete consequence:** a recycled PID makes a dead owner's lock look live, blocking sprint mutations indefinitely until manual removal; `CancelRunLoop` can SIGINT an unrelated recycled process. Both require crash-plus-reuse coincidence.
- **Counter-evidence searched:** current behavior fails safe (conflict, never corruption); integrating boot-id/birth-token into JSON locks adds complexity the short-lived lock may not warrant; heartbeat expiry bounds attempt-level staleness. Kept at P3 accordingly.
- **Confidence:** high (mechanism), low-medium (real-world frequency)
- **Smallest useful action:** record birth-token alongside PID in both lock formats and consult `NativeProcessProbe` before declaring a lock alive; at minimum, add a max-age fallback to `acquireVerificationFileLock` mirroring attempt expiry.

### SPECIALIST-14A-F06
- **Priority:** P3
- **Claim:** Hashing of harness-produced evidence files is unbounded, unlike every other read of external content in the repo.
- **Evidence:** `hashFile` = `os.ReadFile` with no size guard (`smoke.go:685-691`), applied to harness-controlled evidence and issue files during `validateSmokeRun` (`smoke.go:337-351,369-373`) and again on every fingerprint refresh/status re-validation (`verify.go:330-347`, `:226-240` under strict freshness). Contrast the pervasive 64MiB bounded-read discipline: `smoke_author.go:363-365`, `verify.go:421-423`.
- **Architectural reason:** boundary (untrusted-input bounding) / failure-semantics.
- **Concrete consequence:** a multi-GB evidence file turns smoke validation — and each subsequent status call that re-hashes inputs — into an OOM risk on the orchestrator, converting one bad harness write into repeated orchestrator failures.
- **Counter-evidence searched:** evidence roots are containment-checked and hash-pinned, so the file is authenticated *after* reading — the bound must precede the read; no size cap exists anywhere on this path.
- **Confidence:** high
- **Smallest useful action:** `os.Stat` before hashing and reject evidence above the established 64MiB bound with a dedicated diagnostic code.

## Defended architecture / rejected hypotheses

- **"Two process boundaries is duplication."** Rejected as framed. `DirectRunner` (owned group kill, bounded capture, timeouts) governs long-lived untrusted harnesses; raw `exec.Command` in `verify.go:357-370` / `smoke_author.go:185` covers fast, local, read-only git identity probes with explicit "diagnostic only" comments. Forcing them through the runner would be ceremony (`architecture.md:156`). Only the clone (F04) crosses from probe-kind into long-running-kind and therefore genuinely belongs behind the owned boundary.
- **Group-kill cleanup design is unsound.** Rejected: `stopAndWait` (`process_unix.go:14-35`) TERM→grace→group KILL, including the post-wait defensive KILL for descendants that miss TERM, is pinned by a real descendant-survival test (`process_test.go:32-69`). Non-unix degradation reports `CleanupComplete=false` which smoke classifies as uncertain cleanup (`smoke.go:587-588`) rather than hiding it.
- **Evidence staying in harness roots is an ownership gap.** Rejected: protocol line 41 mandates "Harness `runs/` is the detailed source of truth; do not copy raw streams into the sprint". Implementation matches: containment-checked paths, SHA256 pinning, input fingerprints, digest revalidation — custody stays external, integrity stays internal.
- **Timeout precedence is ambiguous.** Rejected: request > workspace/env-configured > manifest > default (`smokeTimeout`, `smoke_protocol.go:644-657`) is deliberate layering with manifest capped at 24h and validated at config load (`config.go:422-437`).
- **Progress dispatcher can deadlock or block drains.** Rejected: bounded queue with counted drops (`process.go:221-232`), closed only after drains finish, surfaced as diagnostics (`smoke.go:129-131`), pinned by `TestDirectRunnerSlowProgressDoesNotBlockDrain`.
- **Shutdown cancellation leaks state.** Rejected for the hub path: `drainAndWait` cancels all non-terminal ops, waits on done channels, and on deadline persists product-owned `.cleanup-uncertain.json` markers consumed by reconciliation (`operations.go:477-555`, `cleanup_uncertain.go:28-58`).
- **runcontrol NativeProcessProbe is redundant with lock liveness.** Rejected: it solves a different lifetime problem (cross-restart durable run reconciliation with fencing), and its existence is what exposes F05 rather than excusing it.

## Open questions

- Does `agentwrap` guarantee descendant termination when `Adapter.StartRun` abandons waiting after the 5-second post-cancel window (`runtime.go:308-320`)? External dependency; determines whether the same group-ownership guarantee exists for agent processes as for harness processes.
- Is TUI/web non-dry-run smoke-start intentionally deferred to the CLI anywhere (docs, roadmap)? Nothing found; if intended, F01 becomes a contract-vs-implementation documentation defect rather than a wiring bug.
- Operational frequency of F02 under production schedulators/load (my repro: ~0.075% on trivial output); larger multi-chunk protocol responses likely widen the window.
