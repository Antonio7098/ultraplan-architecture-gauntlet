Commits verified. Now let me inspect the run state and index to find prior findings relevant to my assignment scope.
I'm challenge-02 (Execution tribunal). Let me map the target repo structure and identify which prior stage outputs fall in my scope.
Several relevant prior outputs exist (some empty). Let me read the core ones for my scope plus the finding contract.
Now let me read the runcontrol specialists and key failure-scenario outputs in my scope.
Let me skim generalist-03 (whole-system failure review) to complete the prior-findings inventory, then dispatch verification subagents.
I now have the prior-findings inventory for my scope. Dispatching parallel verification subagents while I independently re-derive the highest-stakes claims myself.
Subagents are rate-limited; I'll verify directly myself. Starting with the central file for the cancellation/failure-semantics claims.
`StartRun` fully read — windows A/B and the retry asymmetry are visible firsthand. Now the repository layer to confirm error classification, arbitration, and the second terminal-writer claim.
Now the concurrency-critical pieces: the runtime adapter's event-delivery semantics and the sprint execute callback path (GENERALIST-03-F01's claimed data race).
The execute main loop indeed mutates/saves without `sessionMu`. The pivotal question is whether the adapter guarantees no OnEvent callbacks after `StartRun` returns. Reading the adapter now.
`Adapter.StartRun` structurally confirmed to have no post-return quiescence guarantee (two abandonment exits: :314-320 and :339). Now verifying the pinned agentwrap's event-channel semantics — the last link of the claimed data race.
agentwrap confirmed: `close(done)` precedes `close(events)` (LIFO defers at :289-290), so events keep flowing after Wait returns. Checking whether any serialization exists in the sprint runtime wiring that could mitigate.
Race chain fully confirmed firsthand. Now the shutdown/SSE cluster and lock primitives.
All key claims verified firsthand. Running quick REALITY checks (build + core test packages) before writing the report.
Build/vet/race-clean on existing suites (expected — no test drives the abandonment interleaving). Final contract-doc checks, then the report.
All verification complete. Writing the tribunal report.

---

# CHALLENGE-02 — Execution tribunal (runcontrol, runtime, process, concurrency, leases, cancellation, arbitration, failure semantics)

### Scope inspected

**Implementation repo** `/home/antonioborgerees/coding/ultraplan/gauntlet-run/ultraplan-go` @ eeaa034 (clean before and after; read-only):

- `internal/app/run_control.go` (read in full — controlledRuntime accept→claim→watch→flush→terminal, retry helpers, draft builder, `terminalOutcome`), `internal/app/durable_operations.go` (full — manager, `controlOperation`, `FinishOperation`, CLI wrapper)
- `internal/runcontrol/lifecycle.go` (full — Heartbeat, RequestCancellation/AcknowledgeCancellation, List/cursor, Reconcile, `reconcileUnclaimed`, `markStalled`, `recordReconciliation`), `sqlite.go` (Append/Claim/ProposeTerminal/verifyFence region :560–820, schema/CHECK regions, `classifyStoreError` :1085–1108, `isConstraint`, `expiredLeasePredicate`, `reservedEventType`, health query), `retention.go` (full), `sanitize.go` (full), `model.go` constants (:427–481)
- `internal/platform/runtime/runtime.go` (`Adapter.StartRun` :264–355), agentwrap module cache `opencode/runtime.go` (Wait/Cancel/run goroutine close ordering :230–340)
- `internal/sprint/execute.go` (:195–324 OnEvent closure + main loop), `runtime_metrics.go:115–121`, `service.go` (mutex inventory), `verification_lock.go:26–105`
- `internal/study/locks.go` (full), `run_loop.go` (lock acquire/release sites)
- `internal/web/server.go` (full), `operations.go` (drain/cancel/`terminalOperationState` regions), `operation_handlers.go` (SSE deadline sites, `durableOperationDocument`), `run_handlers.go:282–295`, `templates/operation.html:14`, `operations_contract_test.go:81–114`
- `internal/platform/process/process_unix.go` (full); `internal/tui/app.go:283–299`; `docs/recovery.md:75`; `docs/local-web.md:146–149`
- Tests: `runcontrol/lifecycle_test.go` (test inventory), `app/run_control_test.go`, `app/durable_operations_test.go`
- **Commands**: `go build ./...`, `go vet` (clean), `go test -race ./internal/runcontrol ./internal/app` (pass)

**Planning workspace** @ 368a789: `projects/ultraplan-go/docs/TRD.md` (:2132, :2206, :2208, :2225, :2498, :2635, :2645, :2667).

**Prior outputs read**: scout-05, scout-06, specialist-07a/b, generalist-03 (F-sections), failure-02/03/06/07, change-04, change-08, specialist-13b/14b/19b (truncated logs only).

Every finding below was **re-derived firsthand from source** before inclusion; none is accepted on another stage's word alone.

### Architecture assessment

The durable core survives adversarial re-derivation. Terminal arbitration is genuinely single-winner: `ProposeTerminal` CASes on `current_attempt_id = ? AND terminal_outcome IS NULL` (sqlite.go:769–786) with an idempotent loser path (:753–757); `reconcileProcessDecision` never infers success and requires probe evidence (lifecycle.go:481–495); leases use SQLite's own clock in production (`julianday('now')`, sqlite.go:1136–1141). Acceptance fails closed before child start; events are immutable by trigger and fence-guarded on every mutation. Cross-process tests are real subprocess tests. This is sound and unusually disciplined.

Stress concentrates where I was assigned: **the app-layer owner loops translate store errors into terminal truth, and they do it inconsistently and without retry tolerance**; the **runtime adapter exports an implicit no-callback-after-return contract its implementation does not honor**, and exactly one consumer (sprint execute) trusts it without synchronization; **product-level locks still identify processes by bare PID** against TRD:2208; and the **web shutdown/projection layer loses cancellation attribution and leaks lifecycle values outside its own documented envelope**. Several prior-stage priorities needed recalibration downward (observability drift overstated relative to truth-corrupting defects); two needed confirmation upward into precise mechanisms (cancellation misclassification; execute data race).

Minor prior findings re-verified and standing as reported by their original stages (provenance preserved, not restated in full): pagination churn on mutable `updated_at` (specialist-07a-F05, lifecycle.go:230–233 vs :48/:498 confirmed), `jsonMarshalTruncated` returning `fmt.Sprintf` (scout-06-F07, run_control.go:570–575 confirmed), recovery.md 24h-vs-code-2h drift (failure-02-FN4, verify.go:466 confirmed), serveErr branch skipping drain (generalist-03-F07, server.go:126–130 confirmed), SSE request contexts dying at signal via `BaseContext` before terminal frames (failure-06-F04, server.go:106 confirmed), fixed 10s grace vs configured-grace clause of the proposed contract (scout-06-F06, server.go:21 confirmed), `queued`/`Notifier`/`Control` vestiges (specialist-07a-F04 confirmed zero-writer).

### Candidate findings

---

**ID: CHALLENGE-02-F01**
**Priority: P1**

**Claim:** In `controlledRuntime.StartRun`, a user/durable cancellation that races completion or teardown is durably terminalized as `persistence_degraded` ("durable event persistence failed") instead of `cancelled`/`succeeded`, corrupting the truthful-terminal property the whole arbitration design exists to provide.

**Evidence (all re-derived):**
- Window A (deterministic given coalesced backlog): after `base.StartRun` returns, the omission flush runs on `runCtx` (`internal/app/run_control.go:263–276`). If the watch loop acknowledged a durable cancellation it has already called `cancel()` (:236), so `Append` fails via the explicit ctx check (`runcontrol/sqlite.go:639–643`) or driver ctx error; `context.Canceled` passes through unclassified (`sqlite.go:1100–1102`) and is not retryable (`retryableRunControlError`, run_control.go:330–332), so `persistenceErr` is set (:272–274) and the dedicated branch proposes `TerminalPersistenceLost` on a detached context (:282–291). `terminalOutcome` (:294, :625–637) — which would have produced `cancelled` — is never reached whenever `persistErr != nil`.
- Wider than previously reported: every OnEvent-driven append during teardown uses the same dead `runCtx` (:197), so straggler callbacks between durable-cancel-ack and base return set `persistenceErr` identically (:198–199).
- Window B: the `AcknowledgeCancellation` error branch lacks the `runCtx.Err()` guard its three siblings have (compare :232–235 with :225–227, :241–243, :251–253).
- No test covers any window (`run_control_test.go`: 3 tests, happy-path/fail-closed-start/generic-failure only; race coverage lives below the misclassification in `lifecycle_test.go`).

**Architectural reason:** failure-semantics / lifecycle — the app-layer owner mislabels the *cause* of a journal-write abort; the immutable terminal field then lies to every downstream consumer.

**Concrete consequence:** cancelling a chatty run (coalesced progress backlog is common) produces `lifecycle=persistence_degraded`, reason "durable event persistence failed", plus dangling `cancellation_state=requested|acknowledged`. Operators chase phantom storage incidents; retention preserves the false class for 30 days; Sprint-35's requirement to distinguish storage failure from cancellation facts (workspace requirements; TRD:2132 "one truthful terminal outcome") is violated deterministically, not marginally.

**Counter-evidence searched:** flush context is not detached (verified — unlike both terminal proposals which use detached contexts, the flush uses `runCtx`); no `ctx.Err()` pre-check exists anywhere before `setPersistenceErr`; the twin operation supervisor handles ack failure by plain cancel (`durable_operations.go:196–201`), proving the correct shape exists in-tree; considered "journal really couldn't be written" justification — rejected, the omitted entries are coalesced diagnostics, not product truth.

**Confidence:** high (mechanism deterministic; trigger frequency tied to coalescing backlog at completion).

**Provenance:** confirms and extends FAILURE-07-F1 (adds teardown-append window; verifies windows A/B line-by-line).

**Smallest useful action:** exclude `errors.Is(err, context.Canceled)` before `setPersistenceErr`, add the missing guard on the ack branch, and issue the final flush on a detached context (as both terminal paths already do); regression-test a spy runtime succeeding while the fence sees `CancellationRequested` and `progressOmitted > 0`.

---

**ID: CHALLENGE-02-F02**
**Priority: P2**

**Claim:** Both owner supervision loops treat one unretried `Snapshot`/`Heartbeat`/`Reconcile` error — including errors the repository itself classifies retryable, and including workspace-wide `Reconcile` maintenance that says nothing about this run — as instant, irreversible loss of authority, while data-plane writes get a 5 s retry budget; and normal-path terminal proposals get only ~250 ms, so genuinely completed work can be left unterminalized until the reconciler records `interrupted`.

**Evidence:**
- Runtime loop: first error from Snapshot/Ack/Heartbeat/Reconcile → `setPersistenceErr` → `cancel()` (run_control.go:223–258). Operation loop: same calls → bare `owned.cancel(); return` (durable_operations.go:190–216). No retry on either side.
- Retry asymmetry is internal to the same files: `appendRunEventWithRetry` retries `ErrBusy|ErrUnavailable` for 5 s (run_control.go:305–316); `errors.go` marks those codes `Retryable=true`; `classifyStoreError` maps SQLITE_BUSY/LOCKED to retryable busy (sqlite.go:1090–1091).
- Terminal budget: `proposeRunTerminalWithRetry` deadlines at `time.Now().Add(250ms)` per call site (run_control.go:324) inside a 30 s finish context; exhaustion on the success path returns a joined error (:297–300) leaving the run `running` despite a live owner able to write — later reconciled to `interrupted` after 45 s grace (lifecycle.go:360–409). This confirms FAILURE-07's open question as real behavior.
- Contention is designed-in: every owner reconciles the whole workspace every 10 s (:249–257), every repository open reconciles at startup (run_control.go:64), and long writer txs exist (`compactRunJournal`, ≤32 batches in one tx, retention.go:59–93).

**Architectural reason:** failure-semantics — fail-closed doctrine correctly covers required writes (accept/claim/start) but is applied to observation/maintenance calls that share only the store, coupling global store health to every active run's continuation.

**Concrete consequence:** one >5 s writer stall (WAL checkpoint pressure, fsync burst under `synchronous=FULL`, concurrent compaction) kills all healthy operations in every attached process mid-flight; the runtime path then commits a false `persistence_degraded` (compounding F01), and a genuinely successful run whose terminal proposal hits >250 ms busy is recorded `interrupted` instead of `succeeded`. Lease math makes bounded retry safe: heartbeat every 5 s against a 15 s lease leaves room for the same 5 s budget appends already use.

**Counter-evidence searched:** `busy_timeout=5000` makes single-shot busy rare (why this is P2, not P1); no doc/comment declares fail-fast-on-busy intentional for heartbeats (TRD:2223 asks for reasoned safe behavior on heartbeat persistence failure — unanswered by implementation either way); fence safety is preserved under bounded retry because `verifyFence` catches genuine authority loss; heartbeat-failure-stops-work direction prevents corruption — this is availability/evidence damage only.

**Confidence:** high (asymmetry code-certain), medium (operational frequency).

**Provenance:** confirms SPECIALIST-07A-F02, SPECIALIST-07B-F02, FAILURE-03-F01; adds the 250 ms terminal-budget consequence.

**Smallest useful action:** route loop calls through the existing bounded-busy retry helper (immediate abort only for `ErrStaleFence`/`ErrPermission`/`ErrCorrupt`), stop treating global `Reconcile` failure as fatal for the owning run, and give terminal proposals the same 5 s budget as appends.

---

**ID: CHALLENGE-02-F03**
**Priority: P1**

**Claim:** `platform/runtime.Adapter.StartRun` provides no quiescence guarantee for `OnEvent` — it returns through two exits that abandon a live event-delivery goroutine — and sprint execute mutates/persists shared execute-state from that callback without mutual exclusion against its main loop: a genuine Go data race with lost-update potential on state that feeds review fingerprints and verify gates.

**Evidence (full chain re-derived firsthand):**
- Adapter abandonment exit 1: on `ctx.Done()` with unreaped wait after 5 s, StartRun returns while the events goroutine ranges on (`runtime/runtime.go:308–321` vs :278–290). Abandonment exit 2: success path waits only 1 s for the drained collection (:328–340, `<-time.After(time.Second)`).
- Late deliveries are real: agentwrap `run()` defers run LIFO — `r.cancel()`, `close(r.done)`, then `close(r.events)` last (module-cache `opencode/runtime.go:288–290`), so `Wait` returns before the events channel closes, and producer finalization (proc wait, stderr drain, DB reconcile) outlives cancellation by seconds; `Cancel` waits only 100 ms (:255–266).
- Consumer races: execute's OnEvent closure mutates `task.Runtime`/`task.UpdatedAt`/`state.UpdatedAt` and saves the file under `sessionMu` (`internal/sprint/execute.go:244–259`); the main loop mutates the same fields and saves **without** `sessionMu` (:266–306 — it takes the mutex only to read `checkpointErr`, :261–263). No Service-level mutex covers `SaveExecuteRunState` (service.go:32–64 — `mutations` is the lease map, `metricsMu` guards metrics only; `startSprintRuntime` adds none, runtime_metrics.go:115–121).
- Test blindness: all sprint fakes fire OnEvent synchronously inside StartRun; `go test -race ./internal/app ./internal/runcontrol` passes because nothing exercises post-return delivery (verified by running it).

**Architectural reason:** boundary / lifecycle — a volatile-boundary seam exports an implicit concurrency postcondition ("no callbacks after return") that its implementation does not provide; the one consumer that keeps working after return does so unlocked. Review (collector-goroutine pattern, review.go:504–551) and study (`saveMu` versioned persist) defend the same hazard; only execute is exposed.

**Concrete consequence:** operator cancels an execute whose opencode finalization exceeds 5 s (realistic), or a chatty stream drains >1 s past completion: abandoned callbacks interleave whole-file atomic renames and pointer-field writes with the main loop marking the task cancelled/completed. Last-writer-wins loses the terminal diagnostic or the session checkpoint; concurrent field writes are UB under `-race`. Corrupted `.run-state.json` then feeds review input fingerprints and `requireCompleteExecute` gates.

**Counter-evidence searched:** agentwrap source confirms the gap rather than refuting it; searched for any serializer around `SaveExecuteRunState` (none); considered whether `sessionMu` covered main-loop writes (it does not — read the loop twice); considered whether channels close before Wait returns (LIFO order proves the opposite); race-detector silence explained by synchronous fakes, not by absence of the race.

**Confidence:** high (structure), medium-high (trigger frequency).

**Provenance:** independently confirms GENERALIST-03-F01 end-to-end, including the library half it could only sketch.

**Smallest useful action:** extend `sessionMu` to cover all main-loop task mutations and saves in `execute.go`, and document/state the OnEvent postcondition in `Adapter.StartRun` (or bound-drain `eventsCh` unconditionally since the channel closes LIFO).

---

**ID: CHALLENGE-02-F04**
**Priority: P2**

**Claim:** The owner-supervision protocol exists twice in the same package family with already-divergent terminal taxonomy: identical persistence faults produce `persistence_degraded` for runtime-backed runs but `cancelled` for web/TUI/CLI operations — and a failed final omission-flush even demotes a *completed* operation to `failed`.

**Evidence:**
- Runtime path: persistence error → `TerminalPersistenceLost`/"durable event persistence failed" (run_control.go:156–163, :282–291).
- Operation path: `controlOperation` cancels silently with no cause captured (durable_operations.go:190–216); `FinishOperation`'s switch has no persistence case, so `Context.Err()!=nil` collapses to `TerminalCancelled`/"operation cancelled" (:246–247); `durableCLICommand.Finish` derives cancelled purely from `accepted.Context.Err()` (:56–62), which cannot distinguish user Ctrl-C from loop-initiated cancel (heartbeat/quota faults included).
- Demotion: if `owned.omitted > 0` and the final flush append fails, `runErr` becomes the append error (:230–236) and the switch lands on `TerminalFailed`/"operation failed" (:248–249) — for work that actually finished.
- Both loops are live in production wiring simultaneously (serve_commands.go:63–66: `StageRuntime` + `DurableOperations` over one repository).
- No test pins either mapping (test inventories verified).

**Architectural reason:** change-surface / drift — one lifecycle protocol (tick cadence, ack semantics, fail-closed policy, outcome vocabulary — the field recovery.md tells operators to trust) maintained as two hand-copies; every fix to one (including F01/F02 remedies) will predictably miss the other.

**Concrete consequence:** the same quota/busy incident durably records different truths depending on which entry point owned the work, breaking cross-surface triage and the parity intent of the shutdown contract §9.5; future protocol changes must be re-derived twice.

**Counter-evidence searched:** considered whether the products warrant different outcomes — both share the repository, fence machinery, `TerminalOutcome` enum, and recovery docs; considered whether ctx-based detection is intentional operator-cancel semantics — it conflates system-initiated cancel by construction. FAILURE-07 rejected merging the loops wholesale; I agree the duplication is currently structural, which is why the action is classification parity, not unification.

**Confidence:** high (divergence certain), medium (impact).

**Provenance:** confirms SPECIALIST-07A-F01, SPECIALIST-07B-F03, FAILURE-06-F03; adds the success→failed demotion path.

**Smallest useful action:** capture the loop's failure cause in closure state and map persistence-class causes to `TerminalPersistenceLost` in both finish paths (mirroring run_control.go:282–291); keep the loops separate.

---

**ID: CHALLENGE-02-F05**
**Priority: P2**

**Claim:** The durable-event payload contract is jointly owned by three layers with no shared definition or round-trip test, and it has already drifted: the producer promotes `title/detail/text/delta/content/native_type/line` for durable carriage (commit c455510's stated goal), the storage allowlist silently drops all of them except `tool/action/message/state/status`, and consumers read exactly the dropped keys.

**Evidence (all three ends re-derived):**
- Producer promote set + force-inserted display keys: run_control.go:435, :479–486; comment claims durable carriage "that the run timeline JS expects" (:409–410).
- Gate applied to the only write path: `sqlite.go:617` → `allowedEventPayloadFields` omits the promoted keys (`sanitize.go:10–17`), counting them into `"unsafe event detail omitted"` (:30–45).
- Consumer reads dropped keys: `newRunEventView` prefers `text,delta,detail,message,content,title,output` (web/run_handlers.go:288–289) — only `message` can ever match; TUI run view reads the same journal.
- Same drop hits web-operation events built in durable_operations.go:148–152 (`detail` key not allowlisted).
- No test asserts survival or intentional dropping of any promoted key.

**Architectural reason:** drift / change-surface — adding one observable field requires synchronized edits across `platform/runtime` limits, `app` promotion, and `runcontrol`'s gate, with the security-redaction failure semantics ("unsafe") misreporting benign truncation.

**Concrete consequence:** durable timelines (`run show`, `/runs/<id>`, replay, support export) render sparse rows while omission counters inflate, making healthy runs look redacted; the next contributor will edit the producer again (as c455510 did) and be re-bitten by the gate, because nothing connects the lists.

**Counter-evidence searched:** strict-allowlist-as-security-policy is plausible for the *gate itself* — but it cannot explain the consumer expectations and commit intent contradicting it; browser JS reads mostly surviving keys, so degradation is partial; no aliasing/normalization exists downstream of Append.

**Confidence:** high (facts), medium-high (unintentionality).

**Provenance:** confirms SCOUT-05-F01, SCOUT-06-F01, SPECIALIST-07B-F01. **Priority recalibrated from the prior P1s:** no lifecycle, arbitration, or correctness impact; bounded to observability fidelity and change-cost — the true P1s in this scope are F01/F03. Kept at P2 because the failure semantics actively misreport ("unsafe") and the drift is guaranteed to recur.

**Smallest useful action:** add the promoted observable keys to `allowedEventPayloadFields` (or derive both sides from one definition) plus one `draft→Append→Events→view` round-trip test.

---

**ID: CHALLENGE-02-F06**
**Priority: P2**

**Claim:** Per-run compaction can only remove `progress/message/omission`, while `EventWarning` is quota-reserved but non-removable — so a run emitting more than `MaxRetainedEventsPerRun` (4096) warnings/errors permanently wedges its journal (every subsequent append fails inside Append itself), and the owner then stamps the false `persistence_degraded` terminal from F01's machinery.

**Evidence:**
- Removable sets are inline SQL literals excluding `warning` and `finding`: retention.go:76 (`IN ('progress','message','omission')`), :185, :187; `changed == 0` → typed `CodeQuota` "required durable event history reached its bounded capacity" (:85–86).
- Compaction is invoked inside `Append` (sqlite.go:687–689), so once non-removable events dominate past 4096, *every* append — including progress and the terminal-adjacent ones — fails.
- `EventWarning` bypasses even hard quota (`reservedEventType`, sqlite.go:713–720) yet cannot be compacted.
- Producer maps runtime `warning|warn|error` events to `EventWarning` with no coalescing/dedup (run_control.go:382–383; the 250 ms coalescer applies only to `EventProgress`, :180).
- No exhaustive registration test exists (`retention_test.go` exercises progress/warning removal only in isolation — the warning-dominance case is uncovered).

**Architectural reason:** change-surface / failure-semantics — per-type retention policy is scattered across three SQL literals plus a Go switch with no single table and no guard test; fail-closed bounded-capacity doctrine turns a bookkeeping gap into run termination with a lying label.

**Concrete consequence:** a long agent session dominated by provider/tool warnings (>4096 over hours is plausible; each turn emits distinct payloads so nothing dedups) wedges mid-run; the owner cancels and records `persistence_degraded` although storage was healthy — same triage poison as F01, different trigger. A new event type added without registration wedges identically (probe-verified by change-04 on synthetic volume).

**Counter-evidence searched:** mixed streams survive because progress/message free space — the wedge requires warning/finding dominance, which bounds frequency (why P2, matching my downgrade of CHANGE-04-F02's P1); the fail-closed bound itself is defensible doctrine; no doc documents tier policy anywhere in `docs/`.

**Confidence:** high (mechanism read directly), medium (real-world frequency).

**Provenance:** confirms CHANGE-04-F02 mechanics firsthand; priority recalibrated P1→P2 on reachability.

**Smallest useful action:** centralize per-type retention class in one Go table rendered into the SQL literals and `reservedEventType`, make `warning` compactable-or-tombstoneable, and add a fill-past-capacity test per declared type.

---

**ID: CHALLENGE-02-F07**
**Priority: P2**

**Claim:** Graceful server shutdown cancels server-owned operations in memory only — it never persists a `server_shutdown` cancellation request or reason to the authoritative journal — so the durable record reads generic `cancelled`/"operation cancelled", and a crash inside the drain window loses shutdown context entirely until reconciliation writes generic `interrupted`.

**Evidence:**
- `drainAndWait` cancels via hub-only `cancelOperation("", id, "server_shutdown")` (operations.go:477–495, :345–371) which mutates the in-memory doc and fires `record.cancel()`.
- The hub's capability interface has no cancellation-request method (`DurableOperationManager` exposes Accept/Record/Finish only, app/operations.go:40–44); repo-wide grep confirms production `RequestCancellation` callers are CLI/TUI/web-run pages only (run_commands.go:216, run_usecases.go:52) — never shutdown.
- `FinishOperation` hardcodes "operation cancelled" (:246–247) and cannot see the hub's reason.
- CURRENT-CONTRACT grounding: workspace TRD.md:2132 — shutdown must "request canonical cancellation exactly once with reason `server_shutdown` … and persist one truthful terminal outcome". Only the product-marker path records the reason today (`web_operations_test.go:138`).

**Architectural reason:** authority / boundary — the journal is the designated truth owner for lifecycle, but the shutdown actor cannot reach it through the interface it holds; attribution lives only in ephemeral projection and product markers.

**Concrete consequence:** `run show`/diagnostics/support export cannot distinguish shutdown-cancelled from user-cancelled work; the exact window the 10 s budget can miss degrades to reason-less `interrupted`/`owner_process_missing_after_grace`. State remains truthful (cancelled *is* terminal); attribution is lost — hence P2, recalibrated from FAILURE-06-F01's P1.

**Counter-evidence searched:** `RecordOperationEvent` may carry a volunteered `Reason` in payloads — incidental, not authoritative lifecycle; checked whether local-web.md phrasing is SSE-only — TRD:2132 is unambiguous durable wording; crash-window recovery via reconciliation works but is attribution-blind.

**Confidence:** high (mechanism), medium (materiality).

**Provenance:** confirms FAILURE-06-F01 with corrected priority.

**Smallest useful action:** add a narrow optional `OperationCancelRequester` backed by `repository.RequestCancellation(runID, "server_shutdown")`, invoked from `drainAndWait` before `record.cancel()`; let `FinishOperation` prefer the snapshot's cancellation reason.

---

**ID: CHALLENGE-02-F08**
**Priority: P2**

**Claim:** Product-scope locking identifies processes by bare PID and its takeover protocols are unfenced, against the repo's own solved pattern and CURRENT-CONTRACT (TRD:2208: "A PID alone is insufficient because of PID reuse"): cross-process study cancel SIGINTs a recycled PID; `--force-unlock` deletes a possibly-live holder's lock producing two concurrent mutators; stale-reclaim has a remove/create TOCTOU in both implementations; and a reused PID blocks sprint mutation indefinitely with no escape hatch.

**Evidence (each re-derived):**
- Bare liveness everywhere: study `processAlive` = `kill(pid,0)` (study/locks.go:17–23); sprint `verificationProcessAlive` identical (verification_lock.go:96–103). LockInfo carries no birth token (study/locks.go:47–53).
- `CancelRunLoop` SIGINTs that PID behind only a self-signal and study-name check (locks.go:141–158); reachable from web/TUI via `OperationStudyCancel`.
- `ForceUnlockRunLoop` removes unconditionally (:161–167), also invoked by `AcquireRunLoopLock(force=true)` (:39–43) with no liveness consultation; `RunLoop` acquires once and re-checks possession only at release (run_loop.go:31–36; locks.go:116–118 refuses foreign release at exit — after concurrent mutation already happened). Sprint has no force flag at all, so a reused PID blocks flow/execute/review/smoke indefinitely (`ErrVerificationConflict`, verification_lock.go:49–52) with no takeover path.
- Reclaim TOCTOU: read-lock → `os.Remove` → O_EXCL recreate interleaves between two reclaimers in both packages (locks.go:66–72; verification_lock.go:46–58, 2-attempt loop).
- In-repo precedent proving the better pattern exists cheaply: `runcontrol/process_linux.go:16–45` birth identity, asserted by `lifecycle_test.go:176+`.
- No test covers PID reuse or concurrent reclaim outside runcontrol (`locks_test.go` conflict/force/release only).

**Architectural reason:** authority / drift / failure-semantics — run-control owns *run* fencing with birth-token rigor while the product modules that serialize actual mutation kept a weaker identity oracle; two authorities disagree and neither resolves the disagreement (both writers hold valid fenced run-control records, so reconciliation structurally cannot arbitrate the force-unlock case).

**Concrete consequence:** crash + PID reuse (the exact scenario recovery docs direct operators through) yields: SIGINT delivered to an innocent local process; or duplicate run-loops writing interleaved task transitions and spawning duplicate agents; or a sprint locked out until an unrelated process happens to exit. Low probability per event, catastrophic and silent when hit.

**Counter-evidence searched:** docs scope `--force-unlock` as operator-confirmed-stale (cli-reference/user-guide) — but nothing checks staleness on the force path, and `CancelRunLoop` shows the safer primitive exists, so the asymmetry looks accidental; fail-closed directions limit corruption but not arbitrary signalling or indefinite blocking; modern pid_max lowers frequency (P2, not P1).

**Confidence:** high (mechanism), medium (incidence).

**Provenance:** consolidates SCOUT-06-F02, FAILURE-02-FN1/FN2, FAILURE-03-F02, GENERALIST-03-F03 — verified individually; reported as one identity/takeover cluster because the remedy is one shared primitive.

**Smallest useful action:** record the owner's proc birth-time identity in both lock formats and require a match before stale-removal, SIGINT, or release-refusal fallback; consult liveness on the force path; stat-and-compare inode before remove to close the TOCTOU.

---

**ID: CHALLENGE-02-F09**
**Priority: P2**

**Claim:** The durable→web operation compatibility surface projects raw `runcontrol.Lifecycle` through an envelope that its own documentation, contract test, and classifier define as an 8-state closed set; the two reachable extra terminals `timed_out` and `persistence_degraded` escape it, and `terminalOperationState` misclassifies them as non-terminal.

**Evidence (re-derived):**
- Raw pass-through: `State: string(snapshot.Lifecycle)` (operation_handlers.go:415, :424).
- Reachability: `DeadlineExceeded → TerminalTimedOut` (durable_operations.go:244–245); start-persistence failures → `TerminalPersistenceLost` (:111) and run_control.go:285; `terminalOutcome` timeout/cancellation mappings (:628–631).
- Documented set excludes both: local-web.md:146–147; the self-described producer/consumer contract test pins exactly that stale list and asserts 5-terminal classification within it (operations_contract_test.go:83, :110); `terminalOperationState` recognizes 5 terminals (operations.go:614–621).
- Asymmetry proves drift, not design: the sibling SSE projection deliberately normalizes unknowns into the documented vocabulary (operation_handlers.go:501–512).

**Architectural reason:** boundary / drift — the package owns a compatibility envelope but projects an unowned superset through it; the guard test enshrines the stale contract.

**Concrete consequence:** a timed-out or persistence-degraded durable operation queried after restart returns states `local-web.md` declares impossible; any consumer keyed on `terminalOperationState` treats a finished run as ongoing. Blast radius bounded today because hub-created records carry only the closed set and the HTML page redirects durable operations to `/runs` — latent, not live misrouting (hence P2, matching CHANGE-08-F01's number but confirming its substance).

**Counter-evidence searched:** traced every feed into `terminalOperationState` (hub writers only, closed switch) — no current misroute; browser JS terminal array happens complete; redirect path handles all values generically. None repairs the JSON contract.

**Confidence:** high (facts), medium (impact).

**Provenance:** confirms CHANGE-08-F01.

**Smallest useful action:** decide the envelope once — extend doc/test/classifier to the full vocabulary or normalize at `durableOperationDocument` as the event projection already does.

---

**ID: CHALLENGE-02-F10**
**Priority: P2**

**Claim:** Production `WriteTimeout=30s` caps two of the three SSE streaming paths at 30 s wall clock — `followRunSSE` and `followDurableOperationEvents` — contradicting the 30-minute `MaxStreamLifetime`/heartbeat design; only `handleOperationEvents` extends the write deadline.

**Evidence:** server.go:19/:104 (deadline set once per request; activity does not renew); the known mechanism used exactly once (operation_handlers.go:242) and absent from :432–499 and run_handlers.go:473–527; browser consumer streams this exact path (static/js/app.js EventSource); no test serves SSE through production timeouts (recorder-based tests bypass net/http).

**Architectural reason:** change-surface / failure-semantics — one concern implemented three times with divergent deadline handling, invisible to recorder-based CI.

**Concrete consequence:** durable-run follow streams terminate at ~30 s regardless of activity; browsers self-heal via reconnect churn (re-entering replay/gap logic), scripted/API consumers see closure indistinguishable from a crashed server.

**Counter-evidence searched:** IdleTimeout/ReadTimeout ruled out; `MaxStreamLifetime` is not documented endpoint-only; reconnect masks impact for the bundled UI (medium real-world severity, high mechanical certainty).

**Confidence:** high.

**Provenance:** confirms FAILURE-06-F02.

**Smallest useful action:** the same one-line `SetWriteDeadline(MaxStreamLifetime+SSEHeartbeat)` in both handlers, plus one integration test through production timeouts.

---

**ID: CHALLENGE-02-F11**
**Priority: P3**

**Claim:** `cancellation_state='uncertain'` is unreachable — zero writers repo-wide (grep-verified: model constant, IsValid, CHECK constraint, Health count query sqlite.go:877, CLI print run_commands.go:271 are its only references) — while Health/CLI present it as an observable operator signal; the one real uncertainty case (dead owner found with pending cancellation) surfaces only as `interrupted` + `requested`.

**Architectural reason:** lifecycle / drift — declared state space exceeds reachable state space with an operator-facing counter promising distinctions that cannot occur; Sprint-35 asks health to distinguish cancellation uncertainty (CURRENT-CONTRACT per specialist-07a's citation).

**Concrete consequence:** the metric is structurally always 0; a genuine pending-cancel-after-death is indistinguishable in health output from ordinary interruption.

**Counter-evidence searched:** forward-compat reservation plausible but CHECK membership alone preserves it; no dynamic SQL construction of the value; no later-sprint producer in the workspace.

**Confidence:** high (zero-writer fact), medium (intent).

**Provenance:** confirms SPECIALIST-07A-F03 / SCOUT-05-F05.

**Smallest useful action:** give the reconciler a writer (expired owner + `cancellation_state='requested'` → mark `uncertain` before terminal proposal), or remove the counter/state and annotate as reserved.

---

**ID: CHALLENGE-02-F12**
**Priority: P3**

**Claim:** Two diagnostic-truth defects in the durable layer's error ordering and evidence path: `Heartbeat` checks the hard-quota gate *before* verifying the fence (lifecycle.go:24–28 precede :35), so a stale owner against a full store gets retryable `CodeQuota` instead of `CodeStaleFence`; and reconciliation evidence writes are fire-and-forget (`_ = r.recordReconciliation(...)` at :352, :411), so third-party terminal decisions can take effect with their audit row silently lost.

**Architectural reason:** failure-semantics (misattributed causality) / observability (the reconciliation log is the auditable "why" behind an immutable terminal written by the reconciler).

**Concrete consequence:** logs attribute a takeover to quota pressure; support bundles show `interrupted`/`cleanup_uncertain` terminals with missing `reconciliation_log` rows — degrading exactly the forensic path `run diagnostics --support-export` exists for. Decisions remain visible in report/snapshots; harm is evidentiary.

**Counter-evidence searched:** no caller distinguishes quota from stale-fence heartbeat failures today (ordering incidental, not load-bearing); evidence rows are support-only, not authority; failure window narrow. Both stay P3.

**Confidence:** high (both read directly).

**Provenance:** confirms FAILURE-03-F03 and SPECIALIST-07B-F04.

**Smallest useful action:** move `verifyFence` above the quota pre-check; count evidence-write failures into `ReconcileReport` and surface via Health.

---

**ID: CHALLENGE-02-F13**
**Priority: P3**

**Claim:** The TUI operation-event handoff breaks two repo-wide observability conventions at once: the cap-128 channel drops silently with no counter (`select … default:`, tui/app.go:295–298), and a failed durable `RecordOperationEvent` (non-`ErrWebUnavailable`) returns before the live send (:287–291), freezing the visible stream on a transient DB hiccup while the run continues.

**Architectural reason:** failure-semantics / drift — every other bounded buffer counts its losses (process.Result.DroppedEvents, runtime ring warnings, hub slowSubscribers); durability-first ordering mitigates post-hoc inspection but not live fidelity.

**Concrete consequence:** bursty output silently under-renders; a transient append failure blanks live progress with no surfaced error — indistinguishable from a quiet run.

**Counter-evidence searched:** dropped live events remain in the journal (mitigation noted); no counter field exists on Model; no test asserts TUI stream completeness under saturation.

**Confidence:** high (mechanism), medium (impact).

**Provenance:** confirms SCOUT-06-F05 lines firsthand.

**Smallest useful action:** count drops on the model and forward-or-count on the record-error branch instead of bare `return`.

### Defended architecture / rejected hypotheses

1. **"Reconciler impersonating the dead owner's fence is an authority violation."** Rejected: generations are UNIQUE per run, `verifyFence` binds the attempt to `current_attempt_id`, the terminal CAS enforces single-winner, losers are idempotent, `terminal_proposed_by='reconciler'` records the actor; raced in `TestCancellationAndTerminalRacePreservesOneWinnerAndIdempotentCommand` (lifecycle_test.go:119–174).
2. **"`reconcileUnclaimed`'s direct INSERT+CAS is a present-day double-arbiter bug"** (challenging SCOUT-05-F03's framing). Partially rejected as defect: `ProposeTerminal` structurally requires a bound attempt an unclaimed run cannot have; predicates are semantically equivalent today; no test exercises the unclaimed shape, so it stands only as duplication risk (P3 hygiene), not the P2 its priority implied.
3. **"Adapter discarding the result when `ctx.Done()` wins the ready-select (~50% of ties) is a bug."** Rejected: forcing `waitErr = ctx.Err()` on a clean child exit is deliberate conservatism applied consistently at every layer (runtime.go:304–325); distinct from F03, which is about callbacks *after* return.
4. **"Unwired `Notifier` is a missing feature/dead abstraction undermining push delivery."** Rejected: documented best-effort optimization over a correctness-guaranteed polling replay API (interfaces.go:41–45); late observers recover via replay-gap detection; `notify()` nil-guard confirmed (sqlite.go:1143–1147).
5. **"Single-attempt-per-run / generation-always-1 makes fencing dead weight."** Rejected: fences fence zombie writers of the *same* attempt on every mutation; adoption is documented as refused (architecture.md; recovery.md:206–209); late claim after reconciliation fails with `ErrTerminal` (tested).
6. **"`go test -race` passing disproves F03."** Rejected as counter-evidence: existing fakes deliver events synchronously inside StartRun; no test constructs the abandonment interleaving — detector silence is coverage absence, not absence of the race.
7. **"Sanitize strictness is intentional security hardening, so F05 is author-intended."** Rejected as full defense: the gate predates both observability commits, whose messages claim durable carriage, and the consumer reads the dropped keys; at most it is an undocumented policy — the drift and the misleading "unsafe" omission accounting remain.
8. **"Web degraded mode runs unfenced duplicates."** Rejected: `ErrWebUnavailable` arises only when the durable manager is absent; production serve/TUI wiring always installs it (serve_commands.go:65, tui_commands.go:47).
9. **"Shutdown drain can deadlock or double-terminalize."** Rejected: shared bounded shutdownCtx forces `server.Close()`; in-memory terminal checks plus durable CAS make cancel/completion races idempotent (tested at both layers).
10. **"Hub holding `mu` across SQLite admission serializes all hub traffic."** Held back by failure-06; I concur — WAL + busy_timeout + single loopback user + ≤8 ops makes serialized admission defensible; not a defect.

### Open questions

1. **Was fail-fast-on-first-control-error for heartbeats a considered tradeoff?** If some authoritative reasoning doc declares "never continue work whose durability is unconfirmable," F02 downgrades to accepted debt; the 5 s append budget vs 0 s heartbeat budget suggests it was simply not considered. Determines whether the fix is behavior change or documentation.
2. **Is the `sanitize.go` allowlist widening (F05) slated for review in unfinished sprint-35 slices** (requirements exist; no reasoning/plan/review artifacts in the workspace)? If the strict gate is ratified as policy, the remedy flips from "extend list" to "document gate as authoritative and fix commit/consumer expectations."
3. **Are mixed-binary workspaces (two versions sharing `run-control.db`) supported?** Bears on the CHECK-constraint/migration cluster reported by change-04/change-08 (outside my P-set but adjacent): if supported, their cross-version consequences escalate materially.
4. **Actual frequency of `progressOmitted > 0` at completion** (governs F01's trigger rate) is measurable empirically from retained journals; a targeted count would calibrate F01 vs F02 as the dominant source of false `persistence_degraded` records.
