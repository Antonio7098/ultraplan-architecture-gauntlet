I'm chair-04 for the interface tribunal synthesis. The challenge-04 tribunal produced 10 candidate findings; my job is to re-derive them against real code before promoting any. Let me start by grounding myself in the target repo and launching parallel evidence-gathering.
Subagent hit a rate limit; I'll retry it while verifying the P1 claim myself.
F01's core claim is confirmed firsthand at operation_runner.go:74-75 (smoke case bypasses `sprintRuntimeService`). Subagents are rate-limited, so I'll verify directly. Checking the runtime wiring and smoke authoring path:
F01 fully verified. Now checking whether web/TUI smoke requests could set DryRun (counter-evidence), plus F02's WriteTimeout claims:
F02's mechanism confirmed (single `SetWriteDeadline` site at operation_handlers.go:242). Now verifying F03 digests and F04 status-write claims:
F03/F04 core mechanisms verified. Checking remaining claims in one batch — F05 shutdown attribution, F06 state vocabulary, F07 retry asymmetry:
F06/F07 core mechanisms verified firsthand. Checking F04's polling path, F05's shutdown flow, and the web dedup-key plumbing:
F05/F07 mechanisms now confirmed firsthand. Final verification batch — F06 vocabulary, F09 substring mapping, F10 type assertions, F08 lock-owner argv:
All ten mechanisms verified firsthand against source. Now checking provenance of cross-references and hunting counter-evidence for the P1:
Provenance checks out. Two final verification points before synthesis — how `sprintService()` treats status writes across surfaces, and the shutdown contract doc:
All verification complete. Every mechanism was re-derived firsthand against source at eeaa034c; cross-observer provenance confirmed. Here is the chair-04 synthesis:

# Chair-04 — Interface tribunal synthesis (app composition, CLI/TUI/web, HTTP/SSE, presentation state, cross-surface parity)

### Scope inspected

**Implementation repo (`ultraplan-go` @ eeaa034c, read-only).** I re-derived every challenge-04 claim directly from source rather than trusting the tribunal:

- Composition/shared runner: `internal/app/operation_runner.go` (all 6 runtime-backed cases), `internal/app/usecases.go:121-129` (`sprintService`), `internal/app/sprint_usecases.go:90-115`, `internal/app/durable_operations.go:35-55,185-217`, `internal/app/run_control.go:140-330`, `internal/app/study_commands.go:205-212`
- Surface assembly: `internal/app/serve_commands.go:40-70`, `internal/app/tui_commands.go:30-55`, `cmd/ultraplan/main.go:19-30`
- Web: `internal/web/server.go` (timeouts, BaseContext, shutdown sequence), `operation_handlers.go` (:242 deadline, :409-424 durable projection, :610-673 kind mapping, :760-820 error mapping), `operations.go` (hub struct, :182 AcceptOperation, :477-540 drainAndWait), `security.go:438-450`, `operations_contract_test.go:81-115`
- TUI: `internal/tui/app.go:232-255,271-275` (digest, Existing handling, tick)
- Product modules on the seam: `internal/sprint/service.go:60-80,180-200` (statusWrites), `state.go:198-220` (merge guard), `smoke.go:20-70`, `smoke_author.go:19-23`, lock call-site census via grep
- runcontrol: `model.go:15-32` (11 lifecycles), `sqlite.go:440-490` (aliases; repo-wide grep found no `DELETE FROM operation_aliases`)
- Docs/contracts: workspace `system/contracts/runtime/workflows.md:101-113` (WF-IDEMPOTENCY-001), `system/contracts/surfaces/api-contracts.md` (API-IDEMP-001); repo `docs/plans/server-shutdown-run-cancellation-contract.md`, `docs/local-web.md`
- Prior outputs for provenance: challenge-04 (primary input), generalist-01, specialist-12a/b, failure-11, failure-05/06/08, challenge-01 excerpts — spot-verified for the cited finding IDs

### Architecture assessment

The surface architecture is sound and I endorse the tribunal's overall verdict. One shared use-case layer (`dashboardUseCases` + `webUseCases`) feeds all three surfaces through one shared operation runner whose doc comment (operation_runner.go:15-17) states the intent precisely: surfaces own presentation and confirmation, workflow semantics stay in product modules. The web package is transport-only. The hub's bounded design (dedup map, drain flag, slow-subscriber drop, counters) and the prepare→confirm→re-validate fingerprint protocol are correct under my own reading.

Every defect I could confirm lives at the seams, and they share a pattern: **a transport concern or identity decision that belongs to exactly one owner is re-implemented per surface** — runtime wiring (F01), write-deadline discipline (F02), idempotency digest (F03), status-write authority (F04), durable cancellation verb (F05), state vocabulary (F06), supervision retry budgets (F07). None is a layering violation; all are drift/change-locality defects with concrete failure consequences. The composition core itself needs no restructuring — fixes are one-liners or single-helper extractions.

### Candidate findings

---

**ID: CHAIR-04-F01**
**Priority: P1**

**Claim:** Non-dry-run smoke started from web/TUI deterministically fails its runtime gate: the shared runner wires the sprint runtime into five of six runtime-backed operation kinds but skips smoke, while non-dry-run smoke always authors coverage and authoring fails closed without a runtime — after persisting a running→failed attempt against the sprint.

**Evidence:** Verified firsthand. `operation_runner.go:74-75`: `case OperationSmokeStart:` builds `sprint.NewService(root.Path).WithSmokeSettings(...)` with no `.WithRuntime(...)`; siblings at :23/:36/:49/:60/:93 use `sprintRuntimeService`, which wires `.WithRuntime(controlled, req)` plus `.WithSmokeSettings(...)` (`sprint_commands.go:489-498`). `smoke_author.go:21-23` fails closed on nil runtime; `smoke.go:30-37` persists SmokeRunning before execution and the failed result after. Web maps dry-runs to the separate `OperationSmokeDryRun` kind (`operation_handlers.go:657-665`), so only real starts are broken. CLI uses `sprintRuntimeService` (`sprint_commands.go:436`).

**Architectural reason:** change-surface / composition drift — the shared runner exists so surfaces cannot diverge; one case silently bypasses its own abstraction.

**Concrete consequence:** Dashboard/TUI users get "runtime is required to author deep-smoke coverage" on every interactive smoke start while CLI succeeds; each failed attempt takes the mutation lock and writes degraded flow-state requiring repair.

**Counter-evidence searched:** Dry-run-only intent (disproved — prepare forces DryRun only transiently, start does not); alternate runtime injection (none); docs declaring web/TUI smoke unsupported (none); execution-path tests (only kind-mapping coverage, `operations_contract_test.go:46`). Independently reported by GENERALIST-01-F01 (with git history: branch added in a221683, never gained WithRuntime), FAILURE-11-F01, CHALLENGE-01-F01; falsification attempts failed in all four investigations.

**Confidence:** high.
**Smallest useful action:** Route the smoke case through `sprintRuntimeService` like its siblings; add one integration test asserting `OperationSmokeStart` reaches authoring with a non-nil runtime.

---

**ID: CHAIR-04-F02**
**Priority: P2**

**Claim:** Two of three SSE endpoints are killed by the server-wide 30 s `WriteTimeout`; only the transient-hub stream extends its write deadline, so the designed 30-minute durable-stream lifetime is unreachable and long streams terminate silently at ~30 s.

**Evidence:** Verified firsthand. `server.go:19` `WriteTimeout = 30s`; exactly one `SetWriteDeadline` in the package (`operation_handlers.go:242`, `MaxStreamLifetime+SSEHeartbeat = 30m15s`, guarding the hub path). `followRunSSE` (`run_handlers.go:473-527`) sets no write deadline and has no lifetime cap — it loops until terminal. `followDurableOperationEvents` has heartbeat+lifetime tickers (`operation_handlers.go:458-460`) but no deadline extension, making its own 30-min timer unreachable.

**Architectural reason:** drift / failure-semantics — one transport concern implemented three times with divergent deadline discipline; recorder-based tests cannot observe connection deadlines.

**Concrete consequence:** Followers of hours-long execute/study runs lose the stream at ~30 s with no error frame; browsers reconnect every 30 s (churn, cursor replay makes it recoverable), non-browser consumers stop silently while work continues.

**Counter-evidence searched:** No per-route override, middleware, proxy assumption, or documented rationale; heartbeats cannot extend an absolute deadline. SPECIALIST-12A verified the net/http mechanism empirically with a standalone program and explicitly disproved the hypothesis for the hub path (proving awareness of the hazard); SPECIALIST-12B concurred; FAILURE-05/06/08 and CHALLENGE-02-F10 consistent. Two independent empirical observers + my code read → confirmed.

**Confidence:** high.
**Smallest useful action:** Apply the same `SetWriteDeadline(now+MaxStreamLifetime+SSEHeartbeat)` in both durable paths, or extract one SSE write-loop helper; add a real-listener liveness test past 30 s.

---

**ID: CHAIR-04-F03**
**Priority: P2**

**Claim:** Each surface invents a semantically different dedup digest for `AcceptOperation`, and no policy owns the decision downstream: TUI's content digest permanently blocks identical reruns forever (aliases are never deleted and resolve regardless of lifecycle), web's session+token digest never dedupes across preparations, CLI opts out entirely — so the same operator action behaves differently per surface and cross-surface duplicate protection can never fire.

**Evidence:** Verified firsthand. `tui/app.go:236-238`: digest = `sha256(CanonicalRequest + "\x00" + InputFingerprint)` over deterministic governed inputs; `Existing=true` renders "matching durable operation already exists" and clears confirmation (`:245-250`). `web/security.go:447-450`: `confirmationDedupKey(session+"\x00"+token)`, fresh token per preparation, passed into `AcceptOperation` (`operations.go:182`). CLI passes `""` (`durable_operations.go:47`). `runcontrol/sqlite.go:445-454` inserts unique alias; `:472-484` resolves unconditionally; repo-wide grep: zero deletions. CURRENT-CONTRACT: WF-IDEMPOTENCY-001 requires the idempotency stance be explicit in workflow design/docs (`workflows.md:106`); API-IDEMP-001 requires defined replay behaviour — none of the three stances is documented.

**Architectural reason:** authority / boundary — "what makes two operations the same" is an application-level identity decision owned independently by each surface and treated as opaque by runcontrol.

**Concrete consequence:** TUI permanently refuses identical re-validation/rerun until a governed input byte changes (even after the first run finished); web permits unlimited repeats; CLI always duplicates; future surfaces inherit whatever their author improvises.

**Counter-evidence searched:** `durable_operations_test.go:65-96` pins cross-manager dedup intent but never tests same-digest accept after terminal completion; no doc states TUI blocking is intended; fingerprint inputs verified time-invariant. Extends SPECIALIST-11A-F02 / CHANGE-02-F01 (divergence) with the stronger permanent-block consequence.

**Confidence:** high on mechanism; medium on whether permanent blocking is unintended (no statement either way — itself the contract gap).
**Smallest useful action:** Move digest construction next to `AcceptOperation` as one documented policy (e.g., content digest scoped to non-terminal runs) or scope alias resolution to active lifecycles; record the stance per WF-IDEMPOTENCY-001.

---

**ID: CHAIR-04-F04**
**Priority: P2**

**Claim:** Presentation reads perform unleased durable writes: `Status()` persists derived flow-state (JSON + optionally DB row) whenever status-writes are enabled, holding no mutation lease; TUI polls dashboards at 1 Hz, making this a continuous stale-snapshot writer racing lease-holding flow/execute/review/smoke operations, while the web deliberately opted out.

**Evidence:** Verified firsthand. `service.go:191-195`: `SaveFlowState` on every `Status()` when `statusWrites`; no `acquireMutation` call site anywhere near Status (lock census: execute.go, flow.go, locks.go, review.go, smoke.go, verify.go only). `usecases.go:121-129`: `WithoutStatusWrites()` applied iff `readOnly` — set for web (`serve_commands.go:50`), not for TUI (`tui_commands.go:37-41`). `SprintSummaries` calls `Status` per sprint (`sprint_usecases.go:98-109`); TUI ticks every second (`tui/app.go:271-275`). The merge guard (`state.go:199-215`) preserves only nil Review/Smoke — non-nil stale stage progress can still overwrite a lease-holder's newer intermediate state.

**Architectural reason:** lifecycle / ownership — mutation authority for flow-state is the verification lease; a presentation-driven path bypasses it.

**Concrete consequence:** With a TUI run view open during planning flow execution, polling rewrites flow-state up to 1×/s from artifact snapshots, interleaving with and able to regress the lease-holder's writes; DB-authoritative mode inherits the race through `saveFlowStateDatabase`.

**Counter-evidence searched:** The deliberate web opt-out (`service.go:67-73`: "used by strictly read-only presentation surfaces") proves the hazard was recognized; the doc frames keeping CLI/TUI writes as legacy-behavior preservation, not as endorsing polled writes. Checked whether upstream callers guard Status (none). Corroborated ×4 (GENERALIST-03-F04, FAILURE-04-FN2, SPECIALIST-11A-F03, CHALLENGE-01-F03).

**Confidence:** high.
**Smallest useful action:** Gate `Status`'s persistence behind the existing mutation lock (or skip when contended), extending the recognized `WithoutStatusWrites` mitigation to the polling path.

---

**ID: CHAIR-04-F05**
**Priority: P2**

**Claim:** Graceful shutdown cannot durably attribute its own cancellations: `DurableOperationManager` exposes no cancellation-request verb, so the hub cancels contexts directly ("server_shutdown" lands only in the in-memory doc), the journal records generic `cancelled`/reason-less `interrupted`, and request contexts die at SIGINT via `BaseContext` before terminal SSE frames reach connected clients.

**Evidence:** Verified firsthand. `operations.go:40-44`: interface is Accept/Record/Finish only. Hub cancel path: `cancelOperation(..., "server_shutdown")` inside `drainAndWait` (`operations.go:477-540`) mutates `record.doc.Reason` — memory only. `main.go:19` signal context → `server.go:108` `BaseContext` returns it, so SIGINT cancels all request contexts immediately; shutdown sequence (`server.go:133-137`) runs `drainAndWait` *after* ctx death, so SSE loops selecting on `r.Context().Done()` exit before terminal frames flush. Contract: `docs/plans/server-shutdown-run-cancellation-contract.md` §"persist or publish `cancellation_requested` where durable operation state exists" (:70), "publish a final bounded event … close SSE responses" (:132,:143) — doc status "Proposed, normative addendum".

**Architectural reason:** failure-semantics / boundary — the durability boundary lacks the one verb shutdown needs; transport-level context cancellation substitutes for a durable command.

**Concrete consequence:** Post-interrupt, operators see bare "cancelled" with no shutdown indication (repair guidance misdirected); crash inside the 10 s drain window leaves reason-less `interrupted`; connected dashboards never display the terminal frame the contract describes.

**Counter-evidence searched:** Startup reconciliation heals aftermath (why not P1, matching CHALLENGE-02's recalibration of FAILURE-06); searched for an alternate durable-cancel path usable by the hub (none). Mechanism high-confidence; operational weight medium given reconciliation.

**Confidence:** high on mechanism; medium on weight.
**Smallest useful action:** Add `RequestCancellation(ctx, runID, reason)` to the manager interface; have `drainAndWait` call it before cancelling contexts.

---

**ID: CHAIR-04-F06**
**Priority: P3**

**Claim:** The web operations envelope documents/tests an 8-state vocabulary while `durableOperationDocument` passes raw 11-value runcontrol lifecycles through it, so reachable terminal states `timed_out`/`persistence_degraded` fall outside every web-owned classification except the run-page helper.

**Evidence:** Verified firsthand. `runcontrol/model.go:19-29`: 11 values incl. `timed_out`, `persistence_degraded`. `operation_handlers.go:417`: `State: string(snapshot.Lifecycle)` verbatim. Documented/tested set is 8 (`operations_contract_test.go:83`); `terminalOperationState` covers 5 terminals (`operations.go:618-625`); same package handles all values elsewhere (`run_handlers.go` terminal cueing, `static/app.js` run page).

**Architectural reason:** drift — one vocabulary, four partial copies in one package plus docs.

**Concrete consequence:** An API client consuming the operations fallback endpoint for a timed-out/persistence-degraded op sees an undocumented state and classifies it non-terminal (poll/retry forever); HTML paths redirect to `/runs` where rendering is correct.

**Counter-evidence searched:** `handleActiveOperations` filtering uses runcontrol-owned `IsActive()` correctly, so lists aren't polluted; impact bounded to the fallback API path. Substance matches CHANGE-08-F01; I adopt the tribunal's P3 (challenge-02 landed P2; reach is narrower than first appears).

**Confidence:** high.
**Smallest useful action:** Map durable lifecycles onto the documented operation-state vocabulary at the `durableOperationDocument` projection boundary; keep runcontrol values authoritative in `/runs`.

---

**ID: CHAIR-04-F07**
**Priority: P3**

**Claim:** Both owner supervision loops cancel the owned operation on the first unretried Snapshot/Heartbeat/Reconcile error while terminal proposals get a ~250 ms retry budget, so transient SQLite contention can kill long-running work and land completed work as `interrupted` — inverted relative to the 5 s budget the same files give event appends.

**Evidence:** Verified firsthand. `durable_operations.go:189-217`: any poll error → `owned.cancel(); return`. `run_control.go:223-258`: same shape (via `setPersistenceErr`, which cancels at :156-162). Contrast: `appendRunEventWithRetry` = 5 s budget (`run_control.go:305-316`); `proposeRunTerminalWithRetry` = 250 ms (`:318-328`). A busy_timeout exists (`sqlite.go:23`) but sustained contention still kills ownership polls.

**Architectural reason:** failure-semantics / lifecycle — supervision sensitivity is asymmetric relative to the operation's value and unexplained against sibling budgets.

**Concrete consequence:** A long execute/study loop dies because one heartbeat poll hit a busy database; the terminal proposal then races the same contention with a 250 ms budget and records `interrupted` despite successful work.

**Counter-evidence searched:** Fail-closed-on-unproven-ownership is plausible doctrine (never continue without owner proof) — but nothing documents it, and the asymmetry vs the 5 s append budget is unexplained. Behavior confirmed ×3 previously (SPECIALIST-12A-F04, FAILURE-03-F01, CHALLENGE-02-F02) + my read; intent unresolved.

**Confidence:** high on behavior; medium on intended-vs-defect.
**Smallest useful action:** Give ownership polls the append-path's short retry budget and widen the terminal-proposal budget toward the 30 s finalize windows used elsewhere — or write the fail-closed invariant down if intentional.

---

**ID: CHAIR-04-F08**
**Priority: P3**

**Claim:** Study loops started from web/TUI persist a fabricated lock-owner command `ultraplan operation` (nonexistent invocation), so lock-conflict recovery messages direct operators to something that matches no process or doc.

**Evidence:** Verified firsthand. `operation_runner.go:115`: `Command: []string{"ultraplan", "operation"}` vs truthful CLI argv `study_commands.go:209`: `["ultraplan","study",<ref>,"run-loop",...]`.

**Architectural reason:** drift — surfaces share the runner but not the identity inputs the product module persists.

**Concrete consequence:** Lock-conflict guidance names a nonexistent command; recovery time increases.

**Counter-evidence searched:** No doc defines `ultraplan operation`; no test pins the placeholder; matches SPECIALIST-11A-F04 (unchallenged).

**Confidence:** high.
**Smallest useful action:** Pass the surface-qualified argv (or a surface label) into `RunLoopRequest.Command` from shared-runner callers.

---

**ID: CHAIR-04-F09**
**Priority: P3**

**Claim:** Synchronous web error mapping classifies failures by substring-matching error text although the app layer already owns a typed taxonomy (`OperationError.Code/Category/Retryable`), so editing unrelated message prose silently changes HTTP status classes; the contract test freezes today's strings rather than semantics.

**Evidence:** Verified firsthand. `operation_handlers.go:766-791,809-816`: matches on "validation"/"incomplete"/"prerequisite"/"lock"/"in progress"/"unavailable"; `isClientOperationError` matches "required"/"invalid"/"unsupported"/"json"... Typed alternative exists (`operations.go:159-162`); terminal results already carry typed projection via hub finish — only synchronous prepare/start errors take the heuristic path.

**Architectural reason:** authority / drift — failure semantics derive from presentation heuristics instead of the producer's declared taxonomy.

**Concrete consequence:** Rewording a sprint/store error flips 422↔500 or drops `details.reason` for API consumers; `operations_contract_test.go:153-188` entrenches the prose.

**Counter-evidence searched:** Narrower than it looks (terminal results typed); confirmed by GENERALIST-01-F08 / GENERALIST-02-F04 with no counter-wiring found.

**Confidence:** medium-high.
**Smallest useful action:** Have `PrepareOperation` wrap failures in `OperationError` and map `errors.As` first in `writeOperationError`; keep substrings as last resort.

---

**ID: CHAIR-04-F10**
**Priority: P3**

**Claim:** Per-surface use-case assembly is hand-rolled in parallel places with duplicated constructor pipelines, and durable capabilities are discovered by runtime type assertion that degrades silently, so a cloned third surface compiles with durability skipped and every new option touches every assembly site.

**Evidence:** Verified firsthand. `serve_commands.go:47-51` vs `tui_commands.go:37-48` assemble overlapping `dashboardUseCases` fields independently (TUI omits `readOnly`, which is intentional per F04 analysis but undocumented as a pairing rule); capability gates `if manager, ok := h.ops.(app.DurableOperationManager); ok {...}` at `operations.go:181,234,246` degrade silently; `NewOperationalUseCases` exports values whose durable halves stay nil unless asserted.

**Architectural reason:** change-surface — composition knowledge scattered rather than owned at one root helper.

**Concrete consequence:** A third surface compiles with durability silently absent (CHANGE-02-F03); adding one option (e.g., smoke settings) fans out across assembly sites.

**Counter-evidence searched:** The app↔web inversion (`surfaces.go:18-19`) is a deliberate cycle-break and correct; residual scatter corroborated by SCOUT-01-F06/SPECIALIST-10A-F01. No invariant prevents the silent-degradation clone.

**Confidence:** high on facts; low urgency.
**Smallest useful action:** One `newInteractiveUseCases(deps, root, effective, readOnly bool)` returning non-optional durable capabilities, used by both `tui` and `serve`.

### Defended architecture / rejected hypotheses

- **Hub mutex held across `AcceptOperation` SQLite I/O** (SPECIALIST-12A-F02): admission-under-lock keeps start/status/subscribe atomic; local tool caps concurrency; FAILURE-06 withheld it and CHALLENGE-02 concurred. Rejected as a finding; watch item only.
- **Merging the two supervision engines** (`controlledRuntime.StartRun` vs `durableOperationManager`): inputs, failure envelopes, and lifecycle writers genuinely differ (SPECIALIST-10A hyp.3, SPECIALIST-11A hyp.5, CHALLENGE-02 rej.2). Concur; residual coalescing-policy divergence tracked by CHALLENGE-02-F04.
- **Web readOnly vs TUI status-write asymmetry as such**: documented split (`service.go:67-73`); the surviving defect is the *unleased* aspect (F04), not the asymmetry.
- **Transient-hub ephemerality, session-scoped operations, slow-subscriber drop, disconnect≠cancel**: contracted behavior, test-disproved as defects; my reading of `subscribe`/`appendEventLocked`/the MaxInFlight=32 semaphore found no new race or unboundedness.
- **Durable-run SSE 250 ms–1 s polling fallback**: bounded by request contexts and MaxInFlight; deliberate compatibility projection. Not a defect.
- **Per-process HMAC artifact refs invalidating bookmarks on restart**: reforging protection + allowlisted preview paths; self-healing defense-in-depth. Not raised.
- **TUI dropping local UI events on record-error while web cancels**: manager cancels the owned context itself (`durable_operations.go:168-171`), so surfaces converge on cancellation; CHANGE-02 rej.4 stands. Residual cosmetic gap (uncounted channel drops, SCOUT-06-F05) not carried as a separate finding.
- **My own falsification attempts:** dry-run escape hatch for F01 (exists but bounds, doesn't refute); time-varying inputs in TUI digest for F03 (refuted — deterministic); merge-guard sufficiency for F04 (insufficient — nil-only preservation verified at `state.go:199-215`); hidden `SetWriteDeadline` middleware for F02 (repo-wide grep: none).

### Open questions

1. **F03 intent:** Is TUI-side permanent content-idempotency an intended stance (apply everywhere) or accidental (scope/delete aliases)? Decides fix shape; either way the WF-IDEMPOTENCY-001 documentation duty is currently unmet.
2. **F07 doctrine:** Is fail-closed-on-first-supervision-error an explicit owner-proof invariant? If yes, widening retries is wrong and only the terminal-proposal budget needs review; a one-line invariant statement settles it.
3. **Shutdown contract status:** `server-shutdown-run-cancellation-contract.md` is marked "Proposed, normative addendum." If it is treated as not-yet-binding, F05 drops toward accepted debt/P3; the built drainAndWait machinery suggests the repo treats it as applicable.
4. **Session lifetime vs long operations:** signed session cookie expires at 3600 s (`security.go:113`); a hub operation outliving its cookie falls back to durable status lookups for that browser. Possibly acceptable; affects perceived reliability of F02's reconnect path.
