# SPECIALIST-07B — Durable Run Control Review

### Scope inspected

Implementation repo (`ultraplan-go` @ eeaa034, clean):

- `internal/runcontrol`: `model.go`, `sqlite.go`, `lifecycle.go`, `id.go`, `sanitize.go`, `retention.go`, `migration.go`, `metrics.go`, `errors.go`, `process*.go`, `doc.go`, full test suites (`lifecycle_test.go`, `fault_test.go`, `process_integration_test.go`, `sqlite_test.go`, `retention_test.go`, `sanitize_test.go`, `migration_test.go`, `benchmark_test.go`)
- `internal/app`: `run_control.go`, `durable_operations.go`, `run_usecases.go`, `run_commands.go`, `operation_runner.go`, `serve_commands.go`, `usecases.go`, `web_usecases.go`, `run_control_test.go`
- `internal/web`: `operations.go`, `run_handlers.go`; `internal/tui/app.go`
- `internal/sprint/locks.go`, `internal/study/locks.go` (boundary check only)
- Authoritative workspace: `projects/ultraplan-go/docs/TRD.md` §18C/§23, `sprints/35-durable-run-observability/requirements.md` + `flow-state.json`, `system/contracts/runtime/persistence-and-migrations.md`; implementation docs `docs/architecture.md` ("Durable run control"), `docs/recovery.md`
- Verification: `go build ./...`, `go test ./internal/runcontrol ./internal/app` pass; one empirical probe run via `go test -overlay` (target repo untouched, confirmed clean afterwards)

### Architecture assessment

The core is sound and unusually disciplined for this class of problem. Ownership matches the module doctrine: `internal/runcontrol` owns identity, fenced attempts, ordered sanitized events, and single-terminal arbitration; sprint/study never import it (verified: no `runcontrol` references in product packages), so operational state stays a projection, not a shadow authority.

Specific strengths traced end-to-end:

- **Acceptance fails closed before child start** (`run_control.go:125-142`, test `TestControlledRuntimeDoesNotStartWhenAcceptancePersistenceFails`).
- **Ordering**: sequence allocated inside `BEGIN IMMEDIATE` txs, `PK(run_id, sequence)` plus `last_sequence` CAS (`sqlite.go:666-686`); events immutable by trigger (`sqlite.go:327-331`); cross-process ordering proven with real subprocesses (`process_integration_test.go:15-81`).
- **Fencing**: `verifyFence` binds attempt+owner+generation to `runs.current_attempt_id` (`sqlite.go:985-1003`), and every mutation adds a second CAS predicate — stale writers cannot renew, append, or win terminal.
- **Terminal arbitration**: exactly one winner via `terminal_outcome IS NULL` CAS (`sqlite.go:769-786`), raced against cancellation in tests (`TestCancellationAndTerminalRacePreservesOneWinnerAndIdempotentCommand`).
- **Conservative liveness**: exact `/proc` birth identity (starttime + boot_id + host digest), PID-reuse → interrupted, live-owner → stalled only, uncertainty → cleanup_uncertain (`reconcileProcessDecision`, `lifecycle_test.go:176-231`). Lease comparisons in production use SQLite's own clock (`julianday('now')`, `sqlite.go:1136-1141`) — a deliberate, tested defense against process clock skew.
- **Retention/quota**: soft/hard quota layering with reserved headroom for lifecycle-critical writes, per-run journal compaction with explicit replay boundary, bounded migration backups with integrity gates.
- Typed error taxonomy with retryability flags; startup + periodic reconciliation is a sprint requirement (CURRENT-CONTRACT), implemented idempotently.

Stress is concentrated at two seams, both in the *app* layer, not the repository: (1) the event-payload contract between producers (`app`, `web`) and the runcontrol storage allowlist is maintained by hand on both sides and has already drifted; (2) the owner supervision protocol (tick/heartbeat/cancel-ack/reconcile/fail-closed) exists twice with diverging failure semantics.

### Candidate findings

---

#### ID: SPECIALIST-07B-F01
- **Priority:** P1
- **Claim:** The runcontrol storage allowlist silently strips most of the agent-stream payload fields that commit c455510 deliberately promotes for observability, so the run timeline never receives `text`/`delta`/`title`/`detail`/`content`, and their loss is mis-accounted as `"unsafe event detail omitted"`.
- **Evidence:**
  - Producer promotes those exact keys: `internal/app/run_control.go:435` (`promote := {..."title", "detail", "text", "delta", "content"...}`), built into drafts at `run_control.go:400-486`, appended at `run_control.go:197`.
  - Storage gate drops any key outside `allowedEventPayloadFields`, which contains none of them: `internal/runcontrol/sanitize.go:10-17`, drop path `sanitize.go:29-45` (increments `omitted`, reason "unsafe event detail omitted").
  - Consumer expects them: `internal/web/run_handlers.go:288` — `firstNonEmptyPayload(event.Payload, "text", "delta", "detail", "message", "content", "title", "output")` ("Prefer richest observable text").
  - Commit c455510 message: "keeps SSE plumbing … but now carries tool/action/title", "expose DetailText in run event view".
  - Empirical probe (overlay test, repo unmodified): appending a draft with `text/delta/title/detail/content/tool` committed only `tool`; stored omission = `{Reason:"unsafe event detail omitted", Count:5}`. Same gate also drops `detail` from web-operation events built in `durable_operations.go:148-152`.
  - No test covers the `runtimeEventDraft → Append → Events → view` round trip for these keys (only `secret` stripping is asserted, `run_control_test.go:82`).
- **Architectural reason:** boundary / drift — the allowlist is the documented redaction authority (`docs/architecture.md:179-182`), but the producer half of the contract was widened without updating it; the silent-omission failure semantics designed for security redaction now mask a functional regression and lie about *why* content was dropped.
- **Concrete consequence:** On any real agent run, message/reasoning deltas and tool titles persist as omissions, so `run show`, `/runs/<id>`, SSE replay, and support exports show the generic rows c455510 claimed to fix; operators reading "Omitted N detail item(s): unsafe event detail omitted" wrongly suspect redacted/unsafe content. Every such event inflates `omission_total`.
- **Counter-evidence searched:** Checked whether another persistence path bypasses `sanitizeEventDraft` (none — `Append` is the only write path, `sqlite.go:617`); whether platform/runtime pre-filtering explains key absence (no — `agentwrap.go:119-130` passes up to 64 fields/depth 3/8k strings, so keys arrive); whether sprint 35 planning declares this deferred (no — `flow-state.json` shows requirements-only, no reasoning/plan); whether tests assert the drop intentionally (none found).
- **Confidence:** high
- **Smallest useful action:** Add the promoted observable keys to `allowedEventPayloadFields` (or derive both sides from one shared allowlist definition) and add one round-trip test asserting a promoted draft's keys survive `Append` and reach `newRunEventView`.

---

#### ID: SPECIALIST-07B-F02
- **Priority:** P2
- **Claim:** Owner supervision loops treat a single unretried `Snapshot`/`Heartbeat`/`Reconcile` failure as instantly fatal for the local run — including failures of the workspace-wide `Reconcile` maintenance call, which says nothing about this run's liveness — although the repository explicitly classifies busy/unavailable as retryable and the app already has retry helpers it applies only to `Append`/`ProposeTerminal`.
- **Evidence:**
  - `internal/app/run_control.go:222-259`: first error from `Snapshot` (223-231), `AcknowledgeCancellation` (232-238), `Heartbeat` (239-248), or `Reconcile` (249-258) → `setPersistenceErr` → `cancel()` → loop exits. Same shape in `durable_operations.go:189-217` (`owned.cancel(); return`).
  - Retry asymmetry: `appendRunEventWithRetry`/`proposeRunTerminalWithRetry` (`run_control.go:305-332`) retry `ErrUnavailable|ErrBusy`; the three loop calls have no retry. `classifyStoreError` maps `SQLITE_BUSY/LOCKED` to retryable `CodeBusy` (`sqlite.go:1089-1097`).
  - Contention is real by design: every owner reconciles the whole workspace every 10s, every repository open (including each CLI command, `run_control_state.repository`, `run_control.go:64-68`) runs a startup reconcile, and long writer txs exist (`compactRunJournal` up to 32 delete batches in one tx, `retention.go:58-95`).
- **Architectural reason:** failure-semantics / lifecycle — fail-closed is the documented contract for *required* writes (acceptance/claim/start, `docs/architecture.md:162-167`), but reconcile/snapshot are observation/maintenance; coupling global store health to every active run's continuation converts a transient stall into permanent run termination.
- **Concrete consequence:** One >5s writer stall (WAL checkpoint pressure, disk hiccup, retention compaction) during a 1s tick kills all healthy operations in every attached process mid-flight; `controlledRuntime` then proposes `persistence_degraded` ("durable event persistence failed", `run_control.go:282-291`) even though persistence recovered milliseconds later, permanently recording a false degradation in the immutable terminal field. Long study run-loops are the highest-cost victims.
- **Counter-evidence searched:** Fail-closed doctrine covers heartbeat (kept as intended in the claim's scope-out); busy_timeout=5000 makes plain busy rare (likelihood lowered accordingly, hence P2 not P1); no test or doc states reconcile failure must stop owners (TRD:2223 asks for reasoned "safe behavior… when heartbeat or terminal persistence fails" — sprint 35 reasoning stage does not yet exist, so this specific coupling is undocumented either way).
- **Confidence:** medium
- **Smallest useful action:** Route the loop's `Snapshot`/`Heartbeat`/`Reconcile` calls through the existing bounded-busy retry helper, or at minimum stop treating `Reconcile` failure as fatal for the owning run (log-and-skip; heartbeats alone preserve liveness).

---

#### ID: SPECIALIST-07B-F03
- **Priority:** P2
- **Claim:** The owner supervision protocol is implemented twice with already-divergent failure semantics: identical persistence failures produce `persistence_degraded` for runtime runs but `cancelled` for CLI/TUI/web operations, and the divergence is invisible until a fault occurs.
- **Evidence:**
  - Runtime path: any persistence error → dedicated proposal `TerminalPersistenceLost`/"durable event persistence failed" (`run_control.go:156-163, 282-301`).
  - Operation path: `controlOperation` cancels silently on the same errors (`durable_operations.go:190-216`, no cause captured); `durableCLICommand.Finish` maps `Context.Err() != nil` → `OperationCancelled` (`durable_operations.go:56-62`); `FinishOperation` then wins `TerminalCancelled`/"operation cancelled" (`durable_operations.go:246-247`) — including when the real trigger was quota (`CodeQuota` from `Heartbeat`, `sqlite.go:24-28`) or a failed event append (`RecordOperationEvent` → `owned.cancel()`, `durable_operations.go:168-170`). Web path inherits this via `operations.go:234-241`.
  - Note the asymmetry is reachable: `ProposeTerminal` is not quota-gated, so "cancelled" commits successfully in exactly the scenario the other path labels `persistence_degraded`.
- **Architectural reason:** change-surface / drift — one lifecycle protocol (tick cadence, ack semantics, fail-closed policy, outcome taxonomy) owned by two hand-maintained copies in the same package family; every future protocol change must be applied twice and re-derived, and terminal vocabulary — the field recovery.md tells operators to trust — already disagrees.
- **Concrete consequence:** An operator investigating a quota incident sees some runs `cancelled` and others `persistence_degraded` for the same root cause, breaking the "conservative, truthful terminal" property the model is built around; fixes to the loop (e.g., F02's retries) will predictably land in one copy only.
- **Counter-evidence searched:** Considered whether the two products warrant different outcomes (runtime children vs app operations) — but both use the same repository, same lease/fence machinery, same `TerminalOutcome` enum, and the same recovery documentation; considered whether `FinishOperation`'s ctx-based state is intentional operator-cancel detection — it cannot distinguish user Ctrl-C from loop-initiated cancel because both surface as `Context.Err()`; no test exercises either loop's failure paths (fault injection exists only below the Repository interface).
- **Confidence:** high (divergence is code-certain; impact assessment medium)
- **Smallest useful action:** Capture the loop's failure cause in `ownedDurableOperation`/`controlledRuntime` closure state and map persistence-class causes to `TerminalPersistenceLost` in both finish paths; longer term, extract one shared supervisor tick used by both callers.

---

#### ID: SPECIALIST-07B-F04
- **Priority:** P3
- **Claim:** Reconciliation evidence writes are fire-and-forget (`_ = r.recordReconciliation(...)`), so decisions can take effect with their evidence record silently lost, while the report and health projections give no signal.
- **Evidence:** `internal/runcontrol/lifecycle.go:352` and `:411` discard the error; `recordReconciliation` (`lifecycle.go:511-527`) can fail on busy/store faults. CURRENT-CONTRACT wording: reconciliation "records its evidence and decisions" (workspace `TRD.md:2208`).
- **Architectural reason:** lifecycle / observability — the evidence row is the auditable "why" behind an immutable terminal written by a third party (the reconciler); losing it is quiet and unrecoverable after the fact.
- **Concrete consequence:** A support bundle after a mass interruption shows terminals (`interrupted`/`cleanup_uncertain`) with missing corresponding `reconciliation_log` rows, degrading exactly the forensic path `run diagnostics --support-export` exists for.
- **Counter-evidence searched:** Decisions themselves remain visible via snapshots/events and `report.Decisions`; evidence is support-only, not authority; failure window is narrow (single insert inside no tx). Real but low-frequency, hence P3.
- **Confidence:** medium
- **Smallest useful action:** Count evidence-write failures into `ReconcileReport` (e.g., `EvidenceLost int`) and surface via `Health.Diagnostics` instead of discarding.

### Defended architecture / rejected hypotheses

- **"Single attempt per run makes fencing dead weight."** Investigated whether reclaim/resume within a run was missing: `current_attempt_id` is only ever set by `Claim` and never cleared (`grep` across schema and all mutations), generation is effectively always 1, and a late claim after reconciliation correctly fails with `ErrTerminal` (tested, `lifecycle_test.go:304-306`). Rejected as a defect: no-adoption is the documented decision (`docs/architecture.md:174-177`; `docs/recovery.md:206-209`), sprint 35 leaves adoption as an explicit open question (`requirements.md` Q5), and product-level resume (study run-loop, sprint flow rerun) starts new durable runs. The generation machinery is forward-compatible headroom, correctly unique-constrained.
- **"Reconciler impersonating the owner's fence is unsound."** It builds the fence from the attempts row (`lifecycle.go:399`), but `verifyFence` still requires `current_attempt_id` binding and the terminal CAS still enforces single-winner; races are covered by `TestCancellationAndTerminalRacePreservesOneWinner…`. Sound.
- **"Multi-process clock skew breaks leases."** Production predicates compare against SQLite's own clock (`julianday('now')`, `sqlite.go:1136-1141`); injected clocks are confined to tests; backward/forward clock-jump behavior is regression-tested (`TestReconcileClockJumpNeverExpiresAnOwnerEarly`). Sound.
- **"Per-append quota scan and journal compaction checks are unbounded hot-path costs."** `storageBytes` reads a ~6-entry directory; `compactRunJournal` scans ≤4096 rows/run inside the committing tx; append rate is bounded by coalescing. Within PERSIST-READ-001 for local SQLite scale. Rejected.
- **"Startup reconcile on read-only commands is a hidden mutation."** It is required by the sprint contract ("startup and periodic reconciliation", `35-…/requirements.md:24`), batched (≤64), CAS-idempotent, and repeat-safe (tested). Rejected.
- **"Shutdown drops durable operations because the hub swaps contexts."** Traced `startConfirmed`/`AcceptOperation`: the manager's `operationCtx` is a child of the hub's cancellable ctx, so `record.cancel()` propagates; terminal proposals use a detached 30s ctx and survive draining. Correct.

### Open questions

1. Sprint 35 has requirements only — no reasoning/plan/review artifacts exist yet in the workspace. Is the c455510 payload-promotion behavior (F01) slated for review against the storage allowlist, or should the allowlist widening be treated as the fix now?
2. When an unclaimed run that carries a persisted `cancellation_requested` is later reconciled, the terminal records `interrupted`/"owner_never_claimed_after_grace" and the user's cancellation never appears in the arbitrated outcome (cancellation facts survive only in the snapshot). Is folding cancellation into the terminal reason intended future work, or acceptable conservatism?
