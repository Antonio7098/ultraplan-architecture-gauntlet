I'll execute the scout-06 assignment. Let me start by verifying the targets and orienting myself in the repository structure.
Targets verified — commits match, both clean. Let me map the package structure quickly before dispatching subagents.
Package layout confirmed: `internal/app` (command layer), `internal/tui`, `internal/web`, `internal/platform/{runtime,process}`, `internal/runcontrol`, `internal/productstate`, plus external `github.com/Antonio7098/agentwrap`. Now I'll dispatch five parallel subagents for bounded discovery while I read core files myself.
All five subagents returned. Now I'll re-derive the key claims myself before reporting. Verifying the highest-stakes candidates directly against source.
The headline candidate is confirmed at source level: `runtimeEventDraft` promotes `title/detail/text/delta/content/native_type/line` (run_control.go:435) while runcontrol's storage allowlist omits them (sanitize.go:10-17). Now checking counter-evidence: git history of both observability commits, the web JS expectations, and tests asserting these keys.
The c455510 commit message explicitly targets durable run events "carrying tool/action/title" — yet `sanitize.go` isn't in its file list. Verifying the Append path actually invokes the gate, and how the web timeline consumes it.
Chain confirmed: `sqlite.go:617` gates every Append; `run_handlers.go:288` reads exactly the keys the gate drops. Checking gate history and test counter-evidence, then the remaining candidates.
TUI verified myself: no `ctx.Done` watcher exists (`Update` has no such case), quit-refusal at app.go:142-147 is explicit and messaged as intentional ("q keeps the run active"), and the 128-buffer drop at :296-299 counts nothing. Now the study lock PID-reuse candidate and the factory-bypass candidates.
Study lock finding verified firsthand: `LockInfo` carries only PID+study+command+timestamp (locks.go:47-53), liveness is bare `kill(pid,0)` (:17-23), and `CancelRunLoop` SIGINTs that PID (:155). No birth-token check, unlike runcontrol's `ProcessIdentity`. Now the factory bypasses and remaining candidates.
Adapter confirmed: on ctx.Done it Cancels then waits up to 5s for the reaper (runtime.go:308-321) — that mitigates the "100ms give-up" concern. Final verifications: shutdown-contract doc classification, notifier wiring, and decorator internals.
Contract doc classified: "Proposed, normative addendum" requiring a *configured* grace period (:93) vs the implementation's fixed 10s constant; durable workers explicitly deferred to a later contract (:40) — so the daemon study is FUTURE-INTENT. One last firsthand check of the controlledRuntime decorator that several findings lean on.
All verification complete. I have firsthand confirmation for every candidate finding, plus several rejected hypotheses. Writing the scout report.

---

# SCOUT-06 — CLI/TUI/web through app/product/runtime/process/agentwrap boundaries

### Scope inspected

- **Implementation repo** `/home/antonioborgerees/coding/ultraplan/gauntlet-run/ultraplan-go` @ eeaa034 (clean). Packages: `cmd/ultraplan`, `internal/app` (~50 files incl. `run_control.go`, `durable_operations.go`, `serve_commands.go`, `tui_commands.go`, `health_commands.go`, `study_commands.go`, `sprint_commands.go`, `operation_runner.go`, `surfaces.go`), `internal/tui` (all prod files), `internal/web` (`server.go`, `operations.go`, `operation_handlers.go`, `run_handlers.go`, templates, static JS), `internal/platform/runtime` (`runtime.go`, `opencode.go`, `events.go`, `agentwrap.go`, `health.go`), `internal/platform/process` (all files), `internal/runcontrol` (`sqlite.go`, `sanitize.go`, `lifecycle.go`, `model.go`, `interfaces.go`, process files), `internal/study/locks.go` + run-loop/state files, `internal/sprint/smoke.go` + session/lock files, `external github.com/Antonio7098/agentwrap` (module cache).
- **Planning workspace** `/home/antonioborgerees/coding/ultraplan/gauntlet-run/ultraplan-workspace` @ 368a789: `studies/ultraplan-daemon-events-study/`, `system/reasoning/` inventory.
- **Docs**: `docs/plans/server-shutdown-run-cancellation-contract.md`.
- **Commands**: `git log/show --stat`, targeted greps for imports, `signal.Notify`, `OpenSQLite`, `sanitizeEventDraft`, `DetailText`; full reads of every file cited below.
- **Five parallel read-only subagents** produced bounded evidence (import graph, process lifecycle, cancellation/shutdown, observation paths, test-coverage survey). Every load-bearing claim below was re-derived by me directly from source before inclusion.

### Architecture assessment

The boundary system is fundamentally sound and unusually well-guarded:

- **Composition root owns all three surfaces.** `cmd/ultraplan/main.go:13-42` injects `TUIRunner`/`WebRunner` closures specifically to break app↔tui/web cycles (rationale stated at `internal/app/surfaces.go:18-19`). Neither `internal/tui` nor `internal/web` imports `platform/runtime`, `platform/process`, or `productstate` at all; web's import set is *machine-enforced* (`web/import_boundary_test.go:12-36`), as is runcontrol's leaf status (`runcontrol/import_boundary_test.go:12-38`). Surfaces consume narrow app-owned interfaces (`app.OperationalUseCases`, `app.RunUseCases` via DTO alias wall at `run_usecases.go:9-25`) plus opt-in capabilities discovered by type assertion.
- **Lifecycle ownership is coherent.** One owner per child process; unix termination is process-group TERM→grace→KILL with mandatory Wait (`platform/process/process_unix.go:14-35`, `process.go:104-124`); opencode runs add Linux `PDEATHSIG` (`opencode/process_linux.go:13`); the adapter backstops cancellation with a 5 s reap wait (`runtime/runtime.go:308-321`).
- **Durable control plane is strong.** Fenced claims with host/boot/birth-token process identity (`runcontrol/process_linux.go:16-45`), CAS-guarded append-only event journal with trigger-enforced immutability (`sqlite.go:292-331`), reconciliation that never infers success (`lifecycle_test.go:176`), and cross-surface durable cancellation proven by real multi-process tests (`process_integration_test.go:83-122`, `web/run_handlers_test.go:327`).
- **Observation splits cleanly into lossy-transient vs ordered-durable**, with drops counted everywhere except one spot (F04/F05 below), explicit replay-gap detection, and quota exhaustion converted to typed failures instead of silent loss.
- **The disconnect-never-cancels invariant holds identically on all three surfaces** (CLI `run_commands.go:189-191`; SSE `operation_handlers.go:275-277`; TUI quit-refusal `tui/app.go:142-149`), matching the shutdown contract §11.

Stress concentrates in one place: **the runtime-event payload contract is owned piecemeal by three layers** — agentwrap's clone limits, app's draft builder/promoter/redactor (`run_control.go:400-498`), and runcontrol's storage allowlist (`sanitize.go:10-17`). The last two commits both modified the middle layer; neither touched the outer gate. That seams' interaction is where the main finding lives.

### Candidate findings

---

**ID: SCOUT-06-F01**
**Priority: P1**

**Claim:** The durable storage gate deletes most of the payload facts the last two observability commits (c455510, eeaa034) promoted specifically so durable consumers could render them. Commit c455510's stated goal — durable run events carrying tool/action/title with `DetailText` rendered in the run timeline — is only partially realized: `tool` and `action` survive the gate; `title/detail/text/delta/content/output/native_type/line` and all namespaced keys (e.g. `part_state`) do not.

**Evidence:**
- Promote + ensure keys: `internal/app/run_control.go:435` (`promote := {"tool","title","detail","text","delta","content","message","action","state","status","native_type","line"}`) and `:479-486` (forces `tool/action/title/detail/text/delta` presence); comment `:409-410`: *"top-level payload keys that the run timeline JS expects (payload.tool, payload.action, payload.title, etc.)"*.
- Storage gate: `internal/runcontrol/sqlite.go:617` calls `sanitizeEventDraft(draft)` inside `Append`; allowlist `internal/runcontrol/sanitize.go:10-17` contains `tool/action/state/status/message…` but **not** `title/detail/text/delta/content/output/native_type/line` or any `*_` namespaced keys — dropped keys increment the `"unsafe event detail omitted"` omission counter (`sanitize.go:30-44`).
- Consumers read exactly the dropped keys: `internal/web/run_handlers.go:288` `firstNonEmptyPayload(event.Payload, "text","delta","detail","message","content","title","output")` → rendered at `templates/run.html:18` (`data-run-text`, `.run-event-detail`); the TUI run view reads the same durable journal (`tui/app.go:357-389`).
- Commit intent: `git show c455510` — *"preserve safe payload fields into runcontrol with nested flattening… now carries tool/action/title"*; `git log -- internal/runcontrol/sanitize.go` shows the gate untouched since e09d394 (predates both commits).
- No test pins either behavior: zero test references to `promoteObservablePayloadFields`, `DetailText`, or durable assertions on `title/text/delta` (coverage survey + my grep).

**Architectural reason:** drift (plus change-surface). The payload vocabulary is jointly owned by three packages with no shared definition; the middle layer evolved ahead of the outer gate.

**Concrete consequence:** Live transient streams (web hub SSE, TUI op channel) show rich agent detail, but every durable consumer — web run page, TUI run view, `run follow` — renders sparse rows whose text falls back to `message`-only, alongside inflated omission counts that make healthy runs look redacted. A contributor fixing this will predictably edit `runtimeEventDraft` again (as c455510 did) and be re-bitten by the gate, because nothing connects the two lists.

**Counter-evidence searched:** Gate history (not updated in either commit); tests asserting presence *or* absence of these keys durably (none); possibility the strict allowlist is deliberate post-c455510 hardening (gate comment predates the commits; commit messages claim durable carriage, contradicting intent); whether consumers source data elsewhere (verified `run.html`/`DetailText`/TUI all read durable rows). Only remaining explanation for current behavior being intended would be an undocumented policy decision.

**Confidence:** high (mechanism), medium-high (unintentionality).

**Smallest useful action:** Add the promoted observable keys to `allowedEventPayloadFields` (and/or define the single allowlist beside `runtimeEventDraft` and import/reference it from both sides), plus one test asserting a nested `{part:{state:{text,title}}}` event lands `text/title/tool` in a durable row.

---

**ID: SCOUT-06-F02**
**Priority: P2**

**Claim:** Study run-loop locking and cross-process cancellation identify processes by bare PID, although this codebase already solved the identical problem (PID reuse defeating liveness checks) with birth-token identity in runcontrol. `CancelRunLoop` can SIGINT an unrelated recycled process.

**Evidence:**
- `internal/study/locks.go:47-53` — `LockInfo{PID, Command, AcquiredAt}`; no start-time/boot/birth token.
- `locks.go:17-23` — `processAlive` = `kill(pid,0)==nil || EPERM`; used by stale-lock replacement (`:66-69`), `RunLoopActive` (`:136`), and `CancelRunLoop` which then does `syscall.Kill(info.PID, syscall.SIGINT)` (`:155`).
- Precedent showing known better pattern: `internal/runcontrol/process_linux.go:16-45` (host digest + boot_id + `/proc/<pid>/stat` starttime), asserted by `lifecycle_test.go:176` (*"PID reuse detected via BirthToken mismatch → Interrupted"*).

**Architectural reason:** drift / failure-semantics — two subsystems own the concept "is this PID still the process that wrote this record" with divergent rigor.

**Concrete consequence:** Run-loop owner dies (SIGKILL/crash) → PID recycled by an unrelated process → user runs `study cancel` (CLI/TUI `OperationStudyCancel`, `app/operation_runner.go:133-143`) → SIGINT delivered to the innocent process; symmetric failure makes a stale lock look live, blocking new runs until the unrelated process exits. Low probability per event, but the action is *signaling an arbitrary process*, and the fix pattern already exists in-repo.

**Counter-evidence searched:** Self-signal guard `locks.go:152-154`; study-name match `:149-151`; command string recorded but never identity-checked; tests `study/locks_test.go:45,41` cover dead-PID replacement but not reuse; no other caller of `CancelRunLoop` performs additional verification. Nothing mitigates PID reuse itself.

**Confidence:** medium-high.

**Smallest useful action:** Record the owner's process-start identity (same `/proc` starttime source runcontrol uses) in `LockInfo` at acquire; require a match before stale-replacement or `Kill`, falling back to PID-only on platforms lacking it.

---

**ID: SCOUT-06-F03**
**Priority: P2**

**Claim:** Runtime construction authority leaks around the composition root: two production paths build the OpenCode runtime outside any injectable factory, while sprint's equivalent is `Config`-injected — so the wiring seam advertised by `Config.SprintRuntimeFactory` covers only one of three runtime-backed surfaces.

**Evidence:**
- Sprint (the modeled pattern): `Config.SprintRuntimeFactory` (`internal/app/app.go:36-38`), default `defaultSprintRuntimeFactory` → `NewOpenCode` (`sprint_commands.go:21-25`), honored at `sprint_commands.go:486`.
- Study: mutable package-level `var studyRuntimeFactory = func(c) { return runtimepkg.NewOpenCode(c) }` (`study_commands.go:22-24`), consumed at `:323,664,833`; not reachable from `main.go`.
- Health: constructs the adapter inline, ignoring the configured factory entirely — `health_commands.go:113` `runtimepkg.NewOpenCode(c)` (only the in-package test var `runtimeHealthChecks`, `health_commands.go:28`, can intercept).

**Architectural reason:** authority / change-surface — `main.go` is the declared composition root for runtimes, but cannot substitute runtimes for study or health.

**Concrete consequence:** A harness (or future embedding binary) that swaps in a recording/fake runtime via `Config.SprintRuntimeFactory` silently leaves study runs and `health` hitting the real opencode binary; behavior drifts between surfaces reading the same config, and study runtime behavior is testable only from inside package `app`. This is exactly the divergence class the runner-injection design (`surfaces.go:18-19`) was built to prevent.

**Counter-evidence searched:** Workspace docs/plans for an intentional distinction (none found); whether study tests rely on the var (they do — but that confirms the asymmetry rather than justifying it); whether health's direct construction differs semantically from the factory default (it does not — same constructor, same config).

**Confidence:** high (facts), medium (materiality).

**Smallest useful action:** Add `StudyRuntimeFactory` to `app.Config` (default = current var body) and route health through a factory too; keep the vars as test shims only.

---

**ID: SCOUT-06-F04**
**Priority: P3**

**Claim:** The TUI never observes its own context: there is no `ctx.Done()` watcher or cancellation message anywhere in `internal/tui`, so an external SIGINT/SIGTERM cancels the in-flight durable operation but leaves the Bubble Tea program alive in a refused-to-quit state, with no path to exit until the user acts again.

**Evidence:**
- Root context is signal-bound once, in main: `cmd/ultraplan/main.go:19` (`signal.NotifyContext(os.Interrupt, SIGTERM)`); only signal wiring in the repo.
- Op contexts derive from it (`tui/app.go:233` `context.WithCancel(m.ctx)`; replaced by `accepted.Context` `:252-254`), so signal → operation aborts and persists terminal state (`:302-309`) — but `Update` (`:56-230`) handles only key/tick/event messages; nothing converts ctx cancellation into `tea.Quit`.
- Quit with active work is deliberately refused (`:142-147`), so after a signal the UI shows the cancelled-op error and waits; contrast the web surface, where the same signal drives drain→terminal-persist→HTTP-shutdown→exit (`server.go:131-153`).

**Architectural reason:** lifecycle — asymmetric shutdown semantics between two surfaces sharing one signal context.

**Concrete consequence:** `kill -INT` on a busy TUI yields a live fullscreen program displaying a finished/cancelled operation; scripted/remote sessions hang until manual input. Terminal state is safe (durable), so impact is operational, not correctness.

**Counter-evidence searched:** Quit-refusal is intentional and self-documenting (`:145`, and `:265` *"q keeps the run active"*) — not disputed; searched for any goroutine/msg bridging ctx→tea (none); considered whether Bubble Tea handles signals natively (it does not when the process installs `NotifyContext`, as main does).

**Confidence:** high.

**Smallest useful action:** In `Run`, spawn `go func() { <-ctx.Done(); p.Send(ctxDoneMsg{}) }()` and handle it as `tea.Quit`.

---

**ID: SCOUT-06-F05**
**Priority: P3**

**Claim:** The TUI operation-event channel drop is the only lossy handoff in the pipeline that discards events without counting them, and it also swallows live events whenever durable recording fails — breaking the repo-wide convention that every bounded buffer reports its losses.

**Evidence:**
- `tui/app.go:296-299` — `select { case stream <- event: default: }` on a cap-128 channel: silent drop, no counter.
- Same callback, `:287-295`: if `RecordOperationEvent` fails with a non-`ErrWebUnavailable` error, the function `return`s before the send — the live stream loses the event with no surfaced error (contrast the web path, where record failure cancels the operation, `operations.go:245-255`).
- Convention elsewhere: `process.Result.DroppedEvents` consumed at `sprint/smoke.go:129-131`; runtime ring emits drop warnings (`runtime/runtime.go:336-338`); web hub tracks `slowSubscribers` and evicts visibly (`operations.go:455-464`).

**Architectural reason:** drift / failure-semantics (observability consistency across the event pipeline).

**Concrete consequence:** Under bursty agent output a TUI user silently sees fewer progress lines than were emitted, indistinguishable from a quiet run; a transient DB hiccup freezes the visible stream while the run continues. Debugging relies on knowing an unwritten invariant.

**Counter-evidence searched:** Durability-first ordering means dropped live events remain in the journal (real mitigation for post-hoc inspection — noted); no test asserts TUI event completeness under saturation (coverage survey confirms); no existing counter field on `Model` that was overlooked.

**Confidence:** high (mechanism), medium (user impact).

**Smallest useful action:** Count drops on the model (one int, surfaced in the op footer) and forward-or-count on the record-error branch instead of bare `return`.

---

**ID: SCOUT-06-F06**
**Priority: P3**

**Claim:** Server shutdown grace is a fixed 10 s constant shared sequentially by all drained operations, while the governing contract document — which the implementation otherwise follows and is tested against — requires a *configured* grace period with optional second-signal shortening.

**Evidence:**
- Implementation: `ShutdownTimeout = 10 * time.Second` (`web/server.go:21`); `drainAndWait` iterates records under one budget (`operations.go:491-499`).
- Contract: `docs/plans/server-shutdown-run-cancellation-contract.md:93` (*"must wait for a configured, bounded shutdown grace period"*), `:211-215` (`shutdown_grace_period` config example), `:167` (second signal may shorten grace). Document status header `:3`: *"Proposed, normative addendum."*
- Everything else in the same doc's sequence is implemented and pinned by tests (`operations_test.go:162-207`, `server_test.go:80-84`).

**Architectural reason:** drift (CURRENT-CONTRACT, qualified by the doc's "Proposed" status).

**Concrete consequence:** One long-running op can exhaust the entire budget and push sibling ops into `cleanup_uncertain`, deterministically; operators cannot tune the bound per deployment. Bounded harm because uncertainty is durably recorded and reconciled.

**Counter-evidence searched:** Doc status is "Proposed," so the configurable clause could be FUTURE-INTENT; however the surrounding mandatory clauses are already enforced by tests, indicating the doc functions as the current contract for shipped shutdown behavior. Reported at P3 accordingly.

**Confidence:** medium.

**Smallest useful action:** Hoist the constant into `config`/`ServeRunOptions` with the 10 s default; second-signal shortening can wait.

---

**ID: SCOUT-06-F07**
**Priority: P3**

**Claim:** `jsonMarshalTruncated` never marshals JSON nor truncates: it unconditionally returns `fmt.Sprintf("%v", v), nil`, so structured runtime payload values reach durable event rows as Go-syntax strings (`map[foo:bar] …`) despite the helper's name, comment, and error-handling call site promising compact JSON.

**Evidence:** `internal/app/run_control.go:570-575` (body is a bare `fmt.Sprintf`; comments claim *"marshal then truncate to safe limit"*); sole caller `payloadValueString` `:558-564` (its `err != nil` fallback branch at `:561-563` is dead code); feeds `boundedPayloadValue` → `EventDraft.Payload` → durable rows and the SSE/web timeline rendering of F01's surviving fields.

**Architectural reason:** drift — a helper whose contract is fictitious inside the durability path introduced by c455510.

**Concrete consequence:** Structured values (arrays of tool results, nested maps that pass the allowlist under generic keys) persist and render in non-JSON Go syntax; anyone extending the draft builder inherits a function whose signature promises marshal-and-truncate semantics that don't exist.

**Counter-evidence searched:** Checked whether `%v` output is re-parsed as JSON downstream (it is not — values travel as opaque strings; `encoding/json` escapes them safely, so no corruption, only fidelity loss); checked whether encoding/json import is genuinely forbidden in package `app` (it is not — `run_control.go` already imports it).

**Confidence:** high.

**Smallest useful action:** Implement the function as actual `json.Marshal` + length truncation (import already present), or rename to reflect `%v` formatting.

### Defended architecture / rejected hypotheses

1. **"The adapter returns before the child process is reaped" (100 ms Cancel give-up in opencode).** Rejected as exposed behavior: `Adapter.StartRun` blocks up to 5 s for the waiter goroutine after `Cancel` (`runtime/runtime.go:308-321`), Linux children carry `PDEATHSIG=SIGTERM` (`opencode/process_linux.go:13`), and smoke-path termination always joins `cmd.Wait` (`process_unix.go:14-35`, `process.go:124`). Real-child cleanup is additionally proven by test (`process_test.go:32` verifies grandchildren die).
2. **"Synchronous SQLite append inside the event hot path is a performance defect."** Defended as intentional: durability-before-broadcast is a tested invariant (`app/run_control_test.go:32` — commit-before-delivery visibility), coalescing bounds identical-progress writes (250 ms window, `run_control.go:177-196`), and overload converts to typed `persistence_degraded` semantics that cancel the run rather than corrupt it (`run_control.go:156-163,282-301`). Watch item only: the per-append retention scan (`sqlite.go:687` → `retention.go:58-95`) makes every write O(events-in-run); fine at current scale.
3. **"TUI quit-refusal traps users with running work."** Intentional, messaged design (`tui/app.go:145`, `:265`) with a first-class `c` cancel path; the genuine residual gap is only the external-signal case (reported narrowly as F04).
4. **"Runs should survive server/control-plane restart — the daemon study proves the current model wrong."** Rejected: the shutdown contract explicitly defers durable-worker ownership to a later contract (`docs/plans/…contract.md:40`); current runs-die-with-owner + reconciliation model is the specified behavior. The workspace daemon/events study is FUTURE-INTENT and was not scored against the implementation.
5. **"`Notifier` is defined but never wired — dead abstraction or missing feature."** Legitimate optional seam: documented as best-effort latency optimization over a correctness-guaranteed polling replay API (`runcontrol/interfaces.go:41-45`); all 20+ `OpenSQLite` production/test call sites intentionally omit it; late observers recover via `OldestRetainedSequence`/replay-gap detection.
6. **"agentwrap types leaking into sprint/study (`ValidationSpec`) violate the platform boundary."** Investigated and left unreported: the leak is confined to validation-spec construction (`sprint/runtime_validation.go`, `study/runtime_validation.go`, `runtime.Request.Validation` at `runtime.go:38`), the module shares the repo owner, and mandatory translation would add a mapping layer with no behavioral difference today — indirection without earned value under review doctrine. Revisit if agentwrap is ever swapped.
7. **"Claim→spawn ghost-attempt window lets runs vanish."** By design: claim persists fenced owner identity before spawn, and reconciliation terminalizes ghosts after grace without inferring success (`run_control.go:125-138`; `process_integration_test.go:177`; idempotence asserted).

### Open questions

1. **Was the `sanitize.go` allowlist strictness an intentional post-hoc policy** (making F01 an undocumented product decision rather than drift)? Author confirmation flips F01's remedy from "extend list" to "document the gate as authoritative and fix the commit-message/consumer expectations."
2. **Is Windows a supported surface?** If yes, the non-unix paths (no group kill, no `Pdeathsig`, `CleanupComplete=false` after grace in `process_other.go:13-24`) graduate from footnote to real orphan exposure and deserve their own finding.
3. **Expected concurrent durable-run observers per workspace?** Polling-only observation (250 ms–1 s adaptive) against a 4-connection SQLite pool is comfortable at CLI/TUI/few-browser scale; the contention threshold where the unwired `Notifier` becomes necessary is unstated anywhere in docs or benchmarks.
