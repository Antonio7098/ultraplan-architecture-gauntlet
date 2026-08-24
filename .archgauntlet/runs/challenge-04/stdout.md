I'll start by reading the composition root and entry points myself, then delegate bounded discovery tasks to subagents.
Now I'll delegate three bounded discovery tasks in parallel while I read the core web/TUI code myself.
Subagents are rate-limited; I'll do the investigation directly. Reading the SSE/operations hub first.
Now let me retry the subagent digest of prior outputs while I read run_handlers.go.
The digest gives me strong leads. Now I'll re-derive the top claims myself. Starting with the SSE WriteTimeout claim and run handlers.
SSE claim verified firsthand. Now verifying the smoke-start runtime wiring claim.
Let me verify the repo builds/tests green in my areas before finalizing claims.
All verification complete. Here is my challenge-stage report.

# Challenge-04 — App composition, CLI/TUI/web, HTTP/SSE, presentation state, cross-surface parity

### Scope inspected

**Implementation repo (`ultraplan-go` @ eeaa034c, read-only):**
- Composition/root: `cmd/ultraplan/main.go`, `internal/app/app.go`, `surfaces.go`, `usecases.go`, `operations.go`, `web_usecases.go`, `operation_runner.go`, `durable_operations.go`, `run_usecases.go`, `run_control.go`, `tui_commands.go`, `serve_commands.go`
- Web surface: `internal/web/{server,routes,handlers,run_handlers,operation_handlers,operations,security}.go`, templates (`operation.html`), static clients (`static/app.js`, `static/js/*`)
- TUI surface: `internal/tui/{app,model}.go`
- Shared modules touched by surfaces: `internal/sprint/{service,smoke,smoke_author,state}.go`, `internal/runcontrol/{model,interfaces,sqlite}.go`
- Tests executed: `go build ./...`; `go test ./internal/web/... ./internal/app/... ./internal/tui/...` — all green
- Docs: `docs/local-web.md`, `docs/web-compatibility-baseline.md`, `docs/cli-reference.md`

**Authoritative planning workspace:** `system/contracts/runtime/workflows.md` (WF-IDEMPOTENCY-001), `system/contracts/surfaces/api-contracts.md` (API-IDEMP-001).

**Prior outputs digested (provenance preserved below):** scouts 01–06, generalists 01–04, specialists 10a/b, 11a, 12a/b, failures 01–12, changes 01–08, challenges 01–02.

### Architecture assessment

The surface architecture is fundamentally sound. All three surfaces consume one shared use-case layer (`dashboardUseCases` + `webUseCases` + `RunUseCases`) with typed DTOs; the web package is transport-only (DTO mapping, security middleware, session/CSRF, bounded hub); workflow semantics live in app/product modules (`sharedOperationRunner` doc comment, operation_runner.go:15–17). Claims that web leaks workflow logic or TUI reimplements product logic were previously rejected and I concur after reading handlers end-to-end. The operation hub (bounded events, slow-subscriber drop, drain-with-persisted-uncertainty) and the prepare→confirm→re-validate fingerprint protocol are well built and well tested.

The stress points are at the seams between surfaces, not inside them: (1) one runtime-backed operation case skipped the shared wiring every sibling case uses; (2) three different SSE endpoints have three different deadline disciplines; (3) each surface invented its own idempotency-digest formula; (4) a read-only presentation path performs unleased durable writes. Each is a change-locality/failure-semantics defect, not a layering violation.

### Candidate findings

---

**ID: CHALLENGE-04-F01**
**Priority: P1**

**Claim:** Non-dry-run smoke started from the web or TUI can never succeed: the shared runner wires the sprint runtime into five of six runtime-backed operation kinds but not smoke, so authoring always fails its runtime gate — after recording a running→failed smoke attempt against the sprint.

**Evidence:**
- `internal/app/operation_runner.go:74-82` — `case OperationSmokeStart:` constructs `sprint.NewService(root.Path).WithSmokeSettings(...)` with no `.WithRuntime(...)`; every sibling case (Stage :23, Flow :36, Execute :49, Review :60, Verify :93) goes through `sprintRuntimeService(...)`, which wires `.WithRuntime(controlled, req)` (`sprint_commands.go:498`).
- `internal/sprint/smoke_author.go:21-23` — `if s.runtime == nil { return smokeError("smoke_author_runtime", ...) }`.
- `internal/sprint/smoke.go:60-65` — every non-dry-run `runSmoke` calls `authorSmokeSuite` unconditionally.
- `internal/sprint/smoke.go:30-37` — a "running" attempt is persisted before execution and a failed attempt persisted on the authoring error.
- CLI counterpart is correct: `sprint_commands.go:436-441` uses `sprintRuntimeService`. Web/TUI reach the broken case via `RunOperation` default branch (`app/operations.go:399-404`) → `u.runner` (`serve_commands.go:63`, `tui_commands.go:48`).

**Architectural reason:** change-surface / composition drift — the shared-runner abstraction exists precisely so surfaces cannot diverge; one case silently bypasses it.

**Concrete consequence:** Every interactive smoke attempt from the dashboard or TUI deterministically fails ("runtime is required to author deep-smoke coverage"), while the same command succeeds from CLI; each failed attempt acquires the sprint mutation lock and writes running→failed verification state, degrading flow-state evidence users then have to repair.

**Counter-evidence searched:** Searched for alternate runtime wiring, compensating factories, and end-to-end smoke tests in `web`/`tui`/`app` test files — only kind-mapping contract coverage exists (`web/operations_contract_test.go:46`), no execution-path test. No doc declares web/TUI smoke unsupported.

**Confidence:** high (independently confirmed by GENERALIST-01-F01, FAILURE-11-F01, CHALLENGE-01-F01; re-derived here end-to-end).

**Smallest useful action:** Route `OperationSmokeStart` through `sprintRuntimeService` like its siblings; add one integration test asserting web/TUI smoke-start reaches harness discovery with a fake runtime.

---

**ID: CHALLENGE-04-F02**
**Priority: P2**

**Claim:** Two of the three SSE endpoints are killed by the server-wide 30 s `WriteTimeout` ~30 seconds after connect; only the transient-hub operation stream extends its write deadline, so the designed 30-minute stream lifetimes for durable runs are unreachable and streams terminate silently.

**Evidence:**
- `internal/web/server.go:19` — `WriteTimeout = 30s` applied absolutely per request by `net/http`.
- `internal/web/operation_handlers.go:242` — the only `SetWriteDeadline` call (`MaxStreamLifetime+SSEHeartbeat = 30m15s`) guards `handleOperationEvents`'s hub path.
- `internal/web/run_handlers.go:473-527` — `followRunSSE` sets no write deadline and has no lifetime cap; designed to stream until terminal.
- `internal/web/operation_handlers.go:432-499` — `followDurableOperationEvents` has a 30-min lifetime timer (`:460`) but no write deadline, so the timer is unreachable.
- In-repo proof of intent: the hub endpoint's deadline extension exists precisely because authors knew `WriteTimeout` kills long streams.

**Architectural reason:** drift / failure-semantics — three implementations of one transport concern with divergent deadline discipline; recorder-based tests (`run_handlers_test.go:362`) cannot observe real connection deadlines.

**Concrete consequence:** A client following a hours-long execute/study run over `/api/v1/runs/{id}/events` loses the stream at ~30 s with no error frame; browsers self-heal via EventSource reconnect (churn every 30 s per tab), non-browser consumers stop receiving silently while work continues.

**Counter-evidence searched:** No per-route timeout override, no middleware resetting deadlines, no proxy assumption documented; heartbeats do not extend an absolute deadline. Confirmed mechanism matches six prior runs (SPECIALIST-12A-F01/-12B-F01, FAILURE-05/06/08, CHALLENGE-02-F10); prior tribunals landed P2 given EventSource auto-reconnect and cursor replay make loss recoverable — I adopt P2.

**Confidence:** high.

**Smallest useful action:** Apply the same one-line `SetWriteDeadline(h.now().Add(MaxStreamLifetime+SSEHeartbeat))` in both durable stream paths, or extract one SSE write-loop helper; add a real-listener test asserting >30 s liveness.

---

**ID: CHALLENGE-04-F03**
**Priority: P2**

**Claim:** Each surface supplies a semantically different dedup digest to `AcceptOperation`, and the TUI's content-derived digest permanently maps an identical request to the first run ever accepted: rerunning the same operation from the TUI is blocked forever (even after the first run finished), while the web permits unlimited repeats and the CLI opts out entirely; aliases are never deleted.

**Evidence:**
- `internal/tui/app.go:236-238` — TUI digest = `sha256(CanonicalRequest + "\x00" + InputFingerprint)`; deterministic for unchanged governed inputs (`app/operations.go:262-278`).
- `internal/web/security.go:447-450` — web digest = `sha256(session + "\x00" + token)`; fresh token per preparation, so cross-preparation repeats never dedupe.
- `internal/app/durable_operations.go:47` — CLI passes `""` (no alias).
- `internal/runcontrol/sqlite.go:445-454, 472-484` — unique `operation_aliases`, resolved on conflict regardless of lifecycle; repo-wide search finds no `DELETE FROM operation_aliases` anywhere (retention/migration included).
- `internal/tui/app.go:245-250` — `Existing=true` renders "matching durable operation already exists", clears the confirmation; no TUI test covers this path.
- CURRENT-CONTRACT: workspace `system/contracts/runtime/workflows.md:106` requires the idempotency stance be explicit in design/docs — none documents any of these three stances.

**Architectural reason:** authority / boundary — "what makes two operations the same" is an application-level identity decision, but it is owned independently by each surface and treated as opaque downstream.

**Concrete consequence:** Same operator action behaves differently per surface (TUI permanently refuses identical re-validation/re-runs until a governed input byte changes; web allows repeats; CLI always duplicates), and cross-surface duplicate protection can never fire; future surface additions inherit whatever digest their author invents.

**Counter-evidence searched:** `durable_operations_test.go:65-96` pins dedup-across-managers intent but never tests same-digest accept after terminal completion; searched docs for TUI dedup rationale (none); considered whether fingerprint includes time-varying inputs (it does not). Extends SPECIALIST-11A-F02 / CHANGE-02-F01 (which flagged divergence) with the stronger permanent-rerun-block consequence, re-derived here.

**Confidence:** high on mechanism; medium on whether blocking reruns is unintended (no statement either way — itself the contract gap).

**Smallest useful action:** Move digest construction next to `AcceptOperation` (one documented policy, e.g. content-digest scoped to non-terminal runs), or scope alias resolution to active lifecycles; document the chosen stance per WF-IDEMPOTENCY-001.

---

**ID: CHALLENGE-04-F04**
**Priority: P2**

**Claim:** Presentation reads persist durable state outside the mutation lease: `Status()` writes derived flow-state (JSON + DB row) whenever status-writes are enabled, with no verification-lock held; the TUI polls dashboards at 1 Hz, making this a continuous unleased writer racing lease-holding flow/execute/review/smoke operations.

**Evidence:**
- `internal/sprint/service.go:191-195` — `SaveFlowState` on every `Status()` when `statusWrites`; no `acquireMutation` in the path (contrast `smoke.go:24`).
- `internal/sprint/state.go:201-215` — merge guard preserves only nil Review/Smoke; stage progress from Status's artifact snapshot can overwrite a lease-holder's newer intermediate states.
- `internal/app/sprint_usecases.go:98-109` — `SprintSummaries` calls `Status` per sprint; TUI tick loop `internal/tui/app.go:222-226` + `runViewTickCmd` (1 s) triggers it repeatedly; `tui_commands.go:37-41` composes without `readOnly`, unlike web (`serve_commands.go:50`).
- Web deliberately opted out: `service.go:67-73` `WithoutStatusWrites` — "used by strictly read-only presentation surfaces".

**Architectural reason:** lifecycle / ownership — mutation authority for flow-state is the verification lease, but a presentation-driven path bypasses it.

**Concrete consequence:** With a TUI run-view open during a planning flow, status polling rewrites flow-state up to once per second from stale snapshots, interleaving with (and able to regress) the lease-holder's writes; DB-authoritative mode inherits the same race through `saveFlowStateDatabase`.

**Counter-evidence searched:** `WithoutStatusWrites` comment shows web-only mitigation was deliberate, proving the hazard is recognized but left armed on CLI/TUI; checked whether Status writes are lease-guarded upstream (they are not); prior confirmation ×4 (GENERALIST-03-F04, FAILURE-04-FN2, SPECIALIST-11A-F03, CHALLENGE-01-F03), re-derived here.

**Confidence:** high.

**Smallest useful action:** Gate `Status`'s persist behind the existing mutation lock (or skip persistence when the lock is contended), mirroring the web `WithoutStatusWrites` choice for the polling path.

---

**ID: CHALLENGE-04-F05**
**Priority: P2**

**Claim:** Graceful shutdown never persists its own attribution: `DurableOperationManager` exposes no cancellation-request method, so the hub cancels contexts directly and the durable journal records a generic `cancelled` (reason `server_shutdown` lives only in the transient doc); if the drain deadline passes, the run lands as reason-less `interrupted`; meanwhile request contexts die at SIGINT via `BaseContext` before terminal SSE frames can be delivered.

**Evidence:**
- `internal/app/operations.go:40-44` — interface is Accept/Record/Finish only; no request-cancel.
- `internal/web/operations.go:83-84, 131-153` — `cancelOperations` context cancel + `hub.drainAndWait`; `:345-371` writes the reason only into the in-memory doc.
- `cmd/ultraplan/main.go:19` → `server.go:106` `BaseContext` — SIGINT cancels all request contexts immediately; SSE loops exit on `r.Context().Done()` before `drainAndWait` projects terminal/cancelled frames.
- Contract: `docs/plans/server-shutdown-run-cancellation-contract.md` (shutdown cancellation recorded/idempotent; publish terminal SSE then stop HTTP).

**Architectural reason:** failure-semantics / boundary — the durability boundary lacks the one verb shutdown needs, so transport-level cancellation substitutes for a durable command.

**Concrete consequence:** After an interrupt mid-operation, operators inspecting `/runs` see "cancelled" with no indication the server shut down (repair guidance misdirected); a crash during the 10 s drain window leaves `interrupted` with empty reason; connected dashboards never display the terminal frame they are documented to receive.

**Counter-evidence searched:** Startup reconciliation heals aftermath (hence not P1, matching CHALLENGE-02's recalibration of FAILURE-06-F01/F04); searched for an alternate durable cancel path usable by web (none).

**Confidence:** high on mechanism; medium on operational weight.

**Smallest useful action:** Add `RequestCancellation(ctx, runID, reason)` to the manager interface and have `drainAndWait` call it before cancelling contexts.

---

**ID: CHALLENGE-04-F06**
**Priority: P3**

**Claim:** The web operations envelope documents/tests an 8-state vocabulary while `durableOperationDocument` passes the raw 11-value runcontrol lifecycle through it, so reachable terminal states `timed_out`/`persistence_degraded` fall outside every web-owned classification except the run-page view helper.

**Evidence:**
- `internal/runcontrol/model.go:19-29` — 11 lifecycle values incl. `timed_out`, `persistence_degraded`.
- `internal/web/operation_handlers.go:409-420` — `State: string(snapshot.Lifecycle)` verbatim.
- `internal/web/operations_contract_test.go:83,110` + `operations.go:614-621` — documented set and `terminalOperationState` cover only 8 states; `templates/operation.html:13` active-set is 3 states; `docs/local-web.md` envelope lists 8.
- Same package knows better elsewhere: `run_handlers.go:312-323` cues all 11 values; `static/app.js:1392` handles all terminal values on the run page.

**Architectural reason:** drift — one vocabulary, four partial copies in one package plus docs.

**Concrete consequence:** An API client consuming `GET /api/v1/operations/{id}` fallback for a timed-out/persistence-degraded op sees an undocumented state and classifies it as non-terminal (retry/poll forever); HTML paths redirect to `/runs` where rendering is correct, bounding impact.

**Counter-evidence searched:** `IsActive()` filtering in `handleActiveOperations` is correct (runcontrol-owned), so lists aren't polluted; run page fully correct. Confirms CHANGE-08-F01 substance at reduced severity (challenge-02 landed P2; I assess P3 given bounded reach).

**Confidence:** high.

**Smallest useful action:** Map durable lifecycles onto the documented operation-state vocabulary at the projection boundary (`durableOperationDocument`), keeping runcontrol values authoritative in `/runs`.

---

**ID: CHALLENGE-04-F07**
**Priority: P3**

**Claim:** Both owner control loops cancel the owned operation on the first unretried Snapshot/Heartbeat/Reconcile error — even errors already classified retryable — while terminal proposals get only a ~250 ms retry budget, so transient SQLite contention can cancel long-running work and land completed work as `interrupted`.

**Evidence:**
- `internal/app/durable_operations.go:189-217` — any Snapshot/Acknowledge/Heartbeat/Reconcile error → `owned.cancel(); return`.
- `internal/app/run_control.go:223-258` — same shape for runtime runs.
- Contrast: event appends get a 5 s retry budget (`appendRunEventWithRetry`, `run_control.go:305-316`); terminal proposals get `time.Now().Add(250*time.Millisecond)` (`:318-328`).

**Architectural reason:** failure-semantics / lifecycle — supervision sensitivity is inverted relative to the operation's value (a 5 s busy_timeout exists, `sqlite.go:23`, but sustained contention or a quota/compaction hiccup still kills the run).

**Concrete consequence:** A long execute/study loop dies because one heartbeat poll hit a busy database; the terminal write then races the same contention with a 250 ms budget and records `interrupted` despite success.

**Counter-evidence searched:** Fail-closed is arguably intentional (never continue without proof of ownership) — plausible doctrine, but the asymmetry versus the 5 s append budget and the thin terminal budget are unexplained; confirmed 3× previously (SPECIALIST-12A-F04, FAILURE-03-F01, CHALLENGE-02-F02), re-derived here.

**Confidence:** high on behavior; medium on intended-vs-defect.

**Smallest useful action:** Give heartbeat/snapshot polls the same short retry budget as event appends, and widen the terminal-proposal budget to match the 30 s finalize windows used everywhere else.

---

**ID: CHALLENGE-04-F08**
**Priority: P3**

**Claim:** Study loops started from web/TUI record a fabricated lock-owner command `ultraplan operation` (which does not exist), so lock-conflict recovery messages direct operators to a nonexistent invocation.

**Evidence:** `internal/app/operation_runner.go:115` (`Command: []string{"ultraplan", "operation"}`) vs truthful CLI argv `internal/app/study_commands.go:209`.

**Architectural reason:** drift — presentation surfaces share the runner but not the identity inputs the product module persists.

**Concrete consequence:** A user hitting the study lock while a dashboard-started loop runs is told to inspect `ultraplan operation`, which matches no process or doc; recovery time increases.

**Counter-evidence searched:** No doc defines `ultraplan operation`; no test asserts the placeholder. Matches SPECIALIST-11A-F04 (unchallenged); verified both sites here.

**Confidence:** high.

**Smallest useful action:** Pass the real surface-qualified argv (or surface label) into `RunLoopRequest.Command` from the shared runner's callers.

---

**ID: CHALLENGE-04-F09**
**Priority: P3**

**Claim:** Synchronous operation-error mapping on the web classifies failures by substring-matching error text while the app layer already owns typed taxonomies (`OperationError.Code/Category`, CLI `classedError` codes), so editing an unrelated message text silently changes HTTP status classes.

**Evidence:** `internal/web/operation_handlers.go:766-771, 782-791, 814-817` (matches on "validation"/"lock"/"unavailable"/"required"/"invalid"...); typed alternative exists at `app/operations.go:159-162, 619-648`; current strings pinned by `web/operations_contract_test.go:153-188`.

**Architectural reason:** authority / drift — failure semantics derive from presentation heuristics instead of the producer's declared taxonomy.

**Concrete consequence:** Rewording a sprint/store error flips 422→500 or drops `details.reason` guidance for API consumers; the contract test freezes today's prose rather than the semantics.

**Counter-evidence searched:** Terminal results DO carry the typed projection (hub `finish` → `Result.Error`), so only synchronous prepare/start errors take the heuristic path — narrower than it first appears, hence P3. Confirmed by GENERALIST-01-F08/GENERALIST-02-F04; verified here.

**Confidence:** medium-high.

**Smallest useful action:** Have `PrepareOperation` wrap failures in the existing `OperationError` taxonomy and map `errors.As` first in `writeOperationError`, keeping substrings as last resort.

---

**ID: CHALLENGE-04-F10**
**Priority: P3**

**Claim:** Per-surface use-case assembly is hand-rolled in three places with duplicated constructor pipelines and capability wiring that type assertions silently degrade, so a new surface must rediscover the full recipe.

**Evidence:** `serve_commands.go:47-66` vs `tui_commands.go:37-48` assemble overlapping `dashboardUseCases` fields independently (TUI omits `readOnly`, intentionally); `usecases.go:121-130` rebuilds `sprint.Service` per call; `web/operations.go:181,234,246` + `handlers.go:440,466` discover capabilities by runtime type assertion with silent degradation; `NewOperationalUseCases` (`operations.go:724`) exports a value whose durability halves stay nil unless asserted.

**Architectural reason:** change-surface — composition knowledge is scattered rather than owned at one root helper.

**Concrete consequence:** A cloned third surface compiles with durability silently skipped (CHANGE-02-F02); adding one option (e.g., smoke settings) touches every assembly site.

**Counter-evidence searched:** Deliberate cycle-break explains app↔web inversion (`surfaces.go:18-19`) and is correct; the residual is intra-app assembly scatter, confirmed by SCOUT-01-F06/SPECIALIST-10A-F01/CHANGE-02-F03; no invariant prevents the silent-degradation clone.

**Confidence:** high on facts, low urgency.

**Smallest useful action:** One `newInteractiveUseCases(deps, root, effective, readOnly bool)` helper returning a struct whose durable capabilities are non-optional, used by both `tui` and `serve`.

### Defended architecture / rejected hypotheses

- **Hub mutex held across `AcceptOperation` SQLite I/O** (SPECIALIST-12A-F02): I re-checked `operations.go:150-199`. Admission-under-lock keeps start/status/subscribe atomic and the local tool caps concurrency at 8 ops; FAILURE-06 withheld it and CHALLENGE-02 rej.10 concurred. **Rejected as a finding**; noted as watch item only.
- **Merging the two supervision engines** (`controlledRuntime.StartRun` vs `durableOperationManager`): rejected by SPECIALIST-10A hyp.3, SPECIALIST-11A hyp.5, CHALLENGE-02 rej.2 — inputs, failure envelopes, and lifecycle writers genuinely differ. I concur; the residual coalescing-policy divergence is tracked by CHALLENGE-02-F04, not re-raised here.
- **Web/TUI readOnly asymmetry and TUI status-write help text**: `WithoutStatusWrites` doc (service.go:67-69) documents the split; CHALLENGE-01 rej.6 treats the stale "read-only" label as HISTORY. Defended; only the *unleased* aspect survives (F04).
- **Transient-hub ephemerality, session-scoped operations, slow-subscriber drop, disconnect≠cancel**: all contracted and test-disproved as defects by earlier twins; my reading of `subscribe`/`appendEventLocked`/security semaphore (MaxInFlight=32 bounds even durable SSE) found no race or unboundedness to add.
- **Durable-run SSE polling fallback (250 ms–1 s)**: bounded by request contexts and MaxInFlight; deliberate compatibility projection (`followDurableOperationEvents` doc comment). Not a defect.
- **Per-process HMAC artifact refs invalidating bookmarks on restart**: reforging protection + allowlisted preview paths make this defense-in-depth, self-healing via navigation. Not raised.
- **TUI dropping events locally on record-error while web cancels**: the manager cancels the owned context itself (`durable_operations.go:168-171`), so both surfaces converge on cancellation; CHANGE-02 rej.4 is correct and the "keeps running" framing is rejected. Residual: the dropped UI event and uncounted channel drops (`tui/app.go:296-299`, SCOUT-06-F05) remain a P3 cosmetic gap I choose not to carry as a separate finding.

### Open questions

1. **F03 intent:** Is TUI-side permanent content-idempotency ("identical inputs ⇒ reuse first run") an intended stance that should also apply to web, or accidental? The answer decides whether the fix is scoping, deletion-on-terminal, or documentation only.
2. **F07 doctrine:** Is fail-closed-on-first-supervision-error an explicit owner-proof invariant (making widened retries wrong), or elasticity debt? A one-line invariant statement would settle it.
3. **Session lifetime vs long operations:** the signed session cookie expires after 3600 s (`security.go:113`); a hub operation outliving the cookie becomes invisible to its own browser session (status falls back to durable). Possibly acceptable; could materially affect perceived reliability of F02's reconnect path if confirmed.
