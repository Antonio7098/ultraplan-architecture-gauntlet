Now let me examine the tests, client-side SSE code, and the wiring.
Let me empirically verify the Go `WriteTimeout` vs SSE streaming interaction, since two of three SSE paths don't extend the write deadline.
The test confirmed the mechanism indirectly (connection died at deadline since writes were buffered). Let me redo it with proper flushing like real SSE:
Confirmed empirically: without `SetWriteDeadline`, a stream dies with `i/o timeout` at the server `WriteTimeout`; with it, the stream survives. Now checking remaining loose ends:
All evidence gathered. Writing the specialist report.

### Scope inspected

- `internal/web`: routes.go, server.go, server_policy.go, security.go, handlers.go, operation_handlers.go, operations.go, run_handlers.go, artifacts.go (skim), static/js/{app,sse}.js, static/app.js (SSE sections), templates (run.html usage via views), tests: operations_test.go, run_handlers_test.go, sse_test.go, server_test.go, integration_test.go
- `internal/app`: web_usecases.go, durable_operations.go, run_control.go (retry helpers), serve_commands.go
- `internal/runcontrol`: sqlite.go (pragmas, bounds), model.go (tick/lease constants), interfaces.go (Notifier)
- Docs (CURRENT-CONTRACT): docs/local-web.md, docs/web-compatibility-baseline.md, docs/plans/server-shutdown-run-cancellation-contract.md, docs/architecture.md (web section)
- Planning workspace: system/contracts/* greps (no transport-specific SSE requirements found)
- Empirical verification: Go 1.26 program reproducing `http.Server.WriteTimeout` vs `SetWriteDeadline` streaming behavior

### Architecture assessment

The transport layer is soundly module-driven. `internal/web` owns DTOs, framing, sessions/CSRF, the ephemeral hub, and SSE framing only; product truth stays in app/product modules (enforced by `TestWebImportBoundary`). Lifecycle ownership is unusually disciplined: one operation root context (server.go:83–85), drain-before-Shutdown ordering (server.go:131–153), startup reconciliation failing closed (server.go:76–81), durable `run_*` identity as recovery authority with legacy `op_*` mapped to `410` (operation_handlers.go:163–166,407). Backpressure is bounded and self-consistent: bounded replay buffers with byte/count caps (operations.go:450–453), slow subscribers disconnected without blocking producers (operations.go:455–465), capacity rejections instead of queues, and a `ValidateServerPolicy` coherence check tying the constants together (server_policy.go:34–52). Cancellation is idempotent and fenced (`cancelOnce`, terminal arbitration under mutex).

The stress point is that SSE streaming exists as **three hand-copied loops** (in-hub push, durable-operation poll, durable-run poll) rather than one owned helper. Connection-level lifecycle facts (headers, heartbeat, lifetime, write deadline) are re-declared per copy, and one of those facts was missed twice.

### Candidate findings

---

**ID: SPECIALIST-12A-F01**
- **Priority:** P1
- **Claim:** `followRunSSE` and `followDurableOperationEvents` never extend the response write deadline, so `http.Server.WriteTimeout = 30s` force-closes both stream types ~30 seconds after connect, contradicting the documented 15s-heartbeat / 30-minute stream-lifetime contract.
- **Evidence:**
  - server.go:19,104 (`WriteTimeout = 30 * time.Second`, applied to the server); net/http sets an absolute write deadline before the handler runs (verified empirically: non-extended stream dies with `i/o timeout` at the deadline; extended stream survives).
  - operation_handlers.go:242 — the in-hub path correctly calls `http.NewResponseController(w).SetWriteDeadline(h.now().Add(MaxStreamLifetime + SSEHeartbeat))`.
  - operation_handlers.go:432–499 (`followDurableOperationEvents`) and run_handlers.go:473–527 (`followRunSSE`) contain no equivalent call — repo-wide grep confirms line 242 is the only `SetWriteDeadline`.
  - CURRENT-CONTRACT: local-web.md:190–193 (“Heartbeat / stream lifetime | 15 seconds / 30 minutes”), web-compatibility-baseline.md:100–107 (documents both “30s write” and “30-minute stream lifetime” — mutually unsatisfiable for these two paths as implemented).
- **Architectural reason:** lifecycle / change-surface — write-deadline ownership is implicit per copied streaming loop instead of owned by a single SSE transport helper; the third copy got it right, proving the hazard is known.
- **Concrete consequence:** every open `/runs/{id}` page and every durable-fallback `/api/v1/operations/{id}/events` stream is severed at ~30s; heartbeats past 30s are impossible; the browser reconnects every ~31s per tab (app.js:1425–1432), producing perpetual “Reconnecting…” churn and doubled SQLite polling. A proxy in front (docs say none intended, but nothing enforces it) would make this worse.
- **Counter-evidence searched:** client-side reconnect plus replay cursor prevents data loss (app.js:1377–1398, 1410); tests pass because they use `httptest.ResponseRecorder`, which has no write deadline (operations_test.go:348–356, run_handlers_test.go:361–366) — masking, not refuting. No doc declares 30s truncation intentional for these endpoints.
- **Confidence:** high
- **Smallest useful action:** add the same `SetWriteDeadline(MaxStreamLifetime+SSEHeartbeat)` to both polling loops (or extract the one shared SSE loop that owns headers, deadline, heartbeat, and lifetime).

---

**ID: SPECIALIST-12A-F02**
- **Priority:** P2
- **Claim:** `startConfirmed` executes durable acceptance I/O (SQLite accept + claim + event append with retry loops) while holding the global hub mutex, contradicting the repository’s own documented lock discipline and stalling all hub-touching requests under DB contention.
- **Evidence:**
  - operations.go:151–199 — `h.mu.Lock()/defer Unlock` spans `manager.AcceptOperation(ctx, …)` at :182.
  - app/durable_operations.go:88–114 — `Accept` + `Claim` + `appendRunEventWithRetry`; app/run_control.go:305–316 — retry loop with a 5s wall-clock budget per append; sqlite.go:23,74–78 — `busy_timeout=5000ms`, `_synchronous=FULL`.
  - CURRENT-CONTRACT violated: web-compatibility-baseline.md:67–70 — “App callbacks, canonical cancellation, cleanup recording, sends, and waits occur outside that lock.”
- **Architectural reason:** boundary / failure-semantics — persistence latency is coupled into the transport coordination lock. Multi-process concurrency is a supported mode (`TestBrowserRunTwoServerRepositoriesShareObservationAndCancellation`, CLI-during-serve), so contention is not hypothetical.
- **Concrete consequence:** while one start waits on a contended SQLite write (seconds under sustained CLI writes), every `status`, `subscribe`, `activeOperations`, and event append blocks on `h.mu`; new SSE setups and page loads queue against `MaxInFlight=32` (security.go:121–131) and the dashboard appears hung. Related inconsistency in the same seam: `publishAppEvent` persists with bare `context.Background()` (operations.go:247), so operation cancellation cannot interrupt a stuck persist — bounded only by the internal 5s retry budget, unlike `FinishOperation`’s explicit 30s timeout (operations.go:235).
- **Counter-evidence searched:** WAL + 5s busy timeout make stalls rare and bounded on a single-user laptop; typical hold is sub-millisecond. No test exercises hub concurrency under a blocked repository, so nothing asserts the lock scope is intentional.
- **Confidence:** medium
- **Smallest useful action:** perform `AcceptOperation` (and the dedup/confirm resolution already split out) before acquiring `h.mu`, admitting the record under the lock afterwards — matching the documented “callbacks outside the lock” order.

---

**ID: SPECIALIST-12A-F03**
- **Priority:** P3
- **Claim:** `/api/v1/operations/{id}/events` serves two incompatible event-ID namespaces behind one endpoint; when a stream fails over from the transient hub to the durable projection, the browser’s `Last-Event-ID` is applied across the switch, so committed progress can be silently skipped or duplicated.
- **Evidence:** operations.go:426–454 — transient IDs are per-record counters starting at 1 (`nextEventID`); operation_handlers.go:501–518 — durable projection uses journal `event.Sequence` as the SSE id (a different series that also contains accepted/claimed/lifecycle events the hub never emitted); operation_handlers.go:218–235 — on `subscribe` failure the raw `lastID` flows straight into `followDurableOperationEvents`. Reachable on reconnect after server restart: the HMAC session secret is regenerated (security.go:96–98), so the browser gets a fresh session and lands in the fallback.
- **Architectural reason:** lifecycle / failure-semantics — cursor continuity is asserted across an ownership transition without a namespace or reset marker; the hub’s in-band `recovery_required` signal (operations.go:386–394) is unreachable on this path because `subscribe` already failed.
- **Concrete consequence:** after a mid-run server restart, an auto-reconnecting EventSource resumes the durable stream from a meaningless offset; the live timeline omits rows (ids ≤ stale lastID) with no gap signal, while the durable journal remains correct.
- **Counter-evidence searched:** terminal reload masks the common case (app.js:860–867); the run-page flow avoids it by using `?after=` from a fresh preflight (app.js:1377–1410); docs steer users to refresh for authoritative state (local-web.md:173–178) — mitigations reduce severity, none restores ID continuity.
- **Confidence:** medium
- **Smallest useful action:** in the subscribe-failure fallback, ignore the inherited `Last-Event-ID` once and emit a `recovery_required` + snapshot pair (mirroring the hub gap protocol) before resuming from the oldest retained sequence.

---

**ID: SPECIALIST-12A-F04**
- **Priority:** P3
- **Claim:** the durable-operation control loop treats any single transient repository error (heartbeat/snapshot/reconcile) as fatal and immediately cancels the running operation, instead of retrying within the 15s lease window it maintains.
- **Evidence:** app/durable_operations.go:189–217 — each tick’s `Snapshot`/`AcknowledgeCancellation`/`Heartbeat`/`Reconcile` error path is `owned.cancel(); return`, with no retry, while `OwnerLeaseDuration = 15s`, `OwnerTickInterval = 1s`, `HeartbeatInterval = 5s` (model.go:477–480) leave ample margin to survive a failed beat; contrast with `appendRunEventWithRetry` (app/run_control.go:305–316), which does retry identical storage errors.
- **Architectural reason:** failure-semantics — a control-plane blip escalates to workload termination; inconsistent with the event-append seam one layer down.
- **Concrete consequence:** one unlucky `_busy` expiry or transient I/O error during a concurrent CLI write burst converts a hours-long sprint execute into a spurious `cancelled` terminal record; the browser faithfully reports it (failure projected as truth).
- **Counter-evidence searched:** fail-fast is defensible (never operate on a possibly-stale lease); `busy_timeout=5000` plus short immediate transactions make a full failure rare; no test pins either behavior. The lease arithmetic (≥2 ticks of margin) suggests retry was affordable, not that cancel was required.
- **Confidence:** medium
- **Smallest useful action:** tolerate one failed heartbeat/tick (log + retry next tick) and cancel only when failures persist across consecutive ticks within the lease window.

### Defended architecture / rejected hypotheses

- **“Slow subscribers lose events silently.”** Disproved: a full queue closes the subscriber channel and drops the stream deliberately (operations.go:455–465), the handler exits cleanly, the client reconnects, and bounded replay plus `recovery_required` covers gaps (operations.go:386–400); counted via `slowSubscribers`. Test: `TestOperationHubBoundsReplayAndSlowSubscriberIsolation`.
- **“Browser disconnect cancels owned work.”** Disproved: unsubscribe only detaches the subscriber (operations.go:411–422); `record.cancel` is reachable solely via `cancelOperation`/drain; docs pin this contract (server-shutdown-run-cancellation-contract.md:20,280).
- **“Cross-session visibility of durable operations is a security leak.”** Rejected as defect: transient hub state is session-scoped (operations.go:309–343) while durable reads are intentionally workspace-visible (local-web.md:304–306). Mutations still require session+CSRF+origin (security.go:134–159).
- **“Shutdown double-closes `record.done` / leaks streams.”** Disproved: `finish` and `markCleanupUncertain` both arbitrate under `h.mu` with a terminal-state guard (operations.go:281,536); subscribers closed exactly once via map ownership; `unsubscribe` is `once`-guarded. Drain ordering matches the documented shutdown contract (baseline.md:74–81).
- **“Security semaphore deadlocks on abandoned SSE.”** Disproved: acquisition selects on `r.Context().Done()` (security.go:121–131); SSE handlers return on request-context cancellation.
- **“WriteTimeout also kills the primary operation stream.”** Disproved for that path by the explicit `SetWriteDeadline` at operation_handlers.go:242 — which is precisely why its absence in the other two loops reads as an oversight (F01).

### Open questions

- Is the 30s truncation of the two polling SSE loops known/intended anywhere outside the reviewed docs? Nothing found states it; a yes would downgrade F01 to a documentation fix plus intent capture.
- Does `modernc.org/sqlite` under concurrent CLI write bursts plausibly exceed `busy_timeout` in practice (affects F02/F04 likelihood)? No benchmark or fault test in-repo covers multi-process contention against the served hub.
