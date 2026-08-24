Now the SSE handlers:
Now let me look at the run handlers SSE, security wrapper, and server setup:
The `Unwrap()` at security.go:39 means `ResponseController.SetWriteDeadline` does reach the underlying conn. Now let me confirm routing/middleware wiring and the client-side reconnect behavior:
Hub-level isolation is solid and tested. Now let me check the durable run-control storage that the SSE poll loops hit (`RunEvents`/`Run`) and how it contends with `RecordOperationEvent`:
WAL + busy-timeout SQLite: SSE pollers (readers) can't block event-appending writers. Now checking authoritative docs for streaming contracts:
Strong contract evidence found. Now verifying the Go net/http write-deadline behavior against the actual toolchain, and checking which SSE endpoints the browser JS actually consumes:
Confirmed from Go 1.26.6 source: server.go:986-990 arms an absolute `now+WriteTimeout` (30s) write deadline after header read unless the handler overrides it. Only `handleOperationEvents` overrides. Let me check remaining counter-evidence: middleware-level deadlines elsewhere, and how tests exercise SSE (httptest has no WriteTimeout, so the truncation would be invisible to the suite):
All evidence gathered. Here is my report:

---

### Scope inspected

- `internal/web/operations.go` (hub: subscribe/appendEventLocked/finish/drainAndWait), `operation_handlers.go` (`handleOperationEvents`, `followDurableOperationEvents`), `run_handlers.go` (`handleRunEvents`, `followRunSSE`), `server.go`, `server_policy.go`, `security.go` (`trackedWriter`, `securityMiddleware.wrap`), `routes.go`
- Client side: `static/js/sse.js`, `static/app.js` (`followDurableRun`, EventSource usage)
- Durable layer touched by SSE poll loops: `internal/runcontrol/sqlite.go` (pragmas, conn bounds), `internal/app/web_usecases.go:277-298`, `internal/app/durable_operations.go:124`
- Tests: `operations_test.go:234-268` (slow-subscriber isolation), `sse_test.go`, `run_handlers_test.go`, `server_policy_test.go`; suite run green (`go test ./internal/web -run 'TestOperationHub|…'`)
- Contracts: `docs/architecture.md:110-125`, `docs/local-web.md:185-237`, `docs/web-compatibility-baseline.md`, `docs/recovery.md:100-103`, `docs/plans/server-shutdown-run-cancellation-contract.md`
- Go runtime verification: `$GOROOT/src/net/http/server.go:986-990` under go1.26.6

### Architecture assessment

The hub-side answer to the assignment is **sound**: a slow or disconnected SSE subscriber cannot block or corrupt operation progress. Publishing (`publishAppEvent` → `appendEventLocked`) sends to subscriber queues **non-blockingly** and evicts slow subscribers (operations.go:455-464); durability precedes projection (`RecordOperationEvent` before fan-out, operations.go:246-255); disconnect only ends the handler goroutine and never touches operation cancellation ownership (operations.go:275-276; cancel authority is `hub.cancelOperation`). This matches the written contract “Slow or disconnected SSE subscribers cannot block or cancel product work” (architecture.md:116) and is directly tested (operations_test.go:234).

What is **stressed** is the transport layer below the hub: three hand-rolled SSE loops with divergent deadline treatment, sitting inside a global admission semaphore shared with the whole control plane.

### Candidate findings

#### FAILURE-08-F01
- **Priority:** P1
- **Claim:** Two of the three SSE follow paths never override the server's absolute `WriteTimeout` (30 s), so every `/api/v1/runs/{id}/events` stream — and the durable-fallback branch of `/api/v1/operations/{id}/events` — is force-closed ~30 seconds after headers are read, regardless of client health. Their designed 30-minute lifetime and heartbeat cadence are unreachable.
- **Evidence:**
  - server.go:19,104 — `WriteTimeout = 30 * time.Second` set on `http.Server`.
  - Go 1.26.6 `net/http/server.go:986-990` — after reading request headers the server arms an absolute conn write deadline `now+WriteTimeout`; writes fail once it passes unless the handler calls `SetWriteDeadline`.
  - operation_handlers.go:242 — only `handleOperationEvents` extends the deadline (`MaxStreamLifetime+SSEHeartbeat`); its presence proves the author knew the mechanism.
  - run_handlers.go:473-527 (`followRunSSE`) and operation_handlers.go:448-498 (`followDurableOperationEvents`) — no `SetWriteDeadline`; both write at least a heartbeat every ≤1 s, so the first write past t≈30 s errors out and ends the stream. Their `time.NewTimer(MaxStreamLifetime)` (run_handlers.go:484 area, operation_handlers.go:460) can never fire to completion.
- **Architectural reason:** drift / failure-semantics — three parallel implementations of the same transport concern, one patched, two not; invisible to tests because they drive handlers via `httptest.ResponseRecorder` (no real conn, no `WriteTimeout`), e.g. run_handlers_test.go:208-229, 362 and run_handlers_test.go:317.
- **Concrete consequence:** Every open run page's live timeline dies and reconnects on a ≤30 s cycle (app.js:1425-1431 reconnect loop), producing constant request/snapshot/replay churn per tab and flickering "Live" status; any consumer that honors the documented contract (local-web.md:193 “Heartbeat / stream lifetime | 15 seconds / 30 minutes”; baseline doc lines 105-107) but does not auto-reconnect loses live follow mid-stream. Not a blocker of product work — the engine and durable store are untouched — but a standing violation of the fixed compatibility surface.
- **Counter-evidence searched:** Searched for a middleware-level deadline refresh (none — only one `SetWriteDeadline` in the package); searched for comments/tests declaring 30-s chunks intentional (none); checked HTTP/2 semantics (plain-loopback HTTP/1.1 only, so the deferred deadline applies); confirmed `trackedWriter.Unwrap()` lets `ResponseController` reach the real conn, so the override on the third path genuinely works — the asymmetry is not an artifact.
- **Confidence:** high
- **Smallest useful action:** Add the same `SetWriteDeadline(now+MaxStreamLifetime+SSEHeartbeat)` line (or a small shared SSE-writer helper) at the top of `followRunSSE` and `followDurableOperationEvents`, plus one recorder-based test asserting the deadline call reaches the writer.

#### FAILURE-08-F02
- **Priority:** P3
- **Claim:** Every SSE handler occupies one of the global `MaxInFlight = 32` admission slots (shared with all pages/APIs) for the full stream lifetime, including the time its writer goroutine is blocked in `Write` on a stalled client — up to ~30 m15 s on the hub-events path. Enough stuck streams stall all web traffic, including operation start/cancel and `/api/v1/health`.
- **Evidence:**
  - security.go:94,121-131 — semaphore acquired before dispatch, released only when the handler returns; on exhaustion new requests park on `<-r.Context().Done()` (i.e., wait, effectively indefinitely for browsers).
  - routes.go:71 — `security.wrap(h)` wraps every route including `api_operation_events`/`api_run_events`.
  - operation_handlers.go:242 — blocked writers are released only at the absolute write deadline (~30 min), not by the hub's slow-subscriber eviction: eviction frees `h.streams` (operations.go:459-463) while the goroutine stays parked in kernel `Write`, so dropped subscribers free stream capacity but not admission capacity.
- **Architectural reason:** boundary / lifecycle — one coarse admission budget couples an observability transport to control-plane availability; the documented “32 streams” bound (local-web.md:210) is enforced at two different layers with different release conditions.
- **Concrete consequence:** A pathological accumulation (≥32 black-holed connections, e.g., via a local port-forward/proxy, suspended sessions across restarts, or reconnect churn leaving zombie conns) makes the dashboard unable to even cancel running operations for up to 30 minutes. Product/runtime work itself continues (hub.run goroutines, product lease, CLI), and shutdown recovers via `server.Close()` (server.go:141-143), so no corruption and no permanent wedge.
- **Counter-evidence searched:** Loopback-only listener policy (server.go:43,72) sharply limits exposure; docs frame 32 streams as intended capacity, so holding slots per stream is arguably deliberate; hub-level caps (`MaxSubscribersPerOperation=8`, `MaxConcurrentStreams=32`) plus tested slow-subscriber eviction make accidental accumulation unlikely without an intermediary hop; graceful shutdown is unaffected.
- **Confidence:** high (mechanism), low-medium (real-world trigger likelihood)
- **Smallest useful action:** Refresh a short per-write deadline on each heartbeat/event write instead of one absolute 30-min deadline, so a stalled peer costs a slot for seconds, not minutes; alternatively exempt streaming GETs from the `MaxInFlight` budget and rely on `MaxConcurrentStreams`.

### Defended architecture / rejected hypotheses

- **“Slow subscriber blocks the publisher”:** rejected. Non-blocking send with eviction under the hub mutex (operations.go:455-464); publishing never performs client I/O under the lock; proven by TestOperationHubBoundsReplayAndSlowSubscriberIsolation.
- **“Disconnect cancels or corrupts runs”:** rejected. `r.Context().Done()` terminates only the handler; cancellation authority is centralized in `hub.cancelOperation`/durable `CancelRun`; explicitly contracted (server-shutdown contract line 20; recovery.md:100-103).
- **"`trackedWriter` swallows the write deadline":** rejected — `Unwrap()` (security.go:39) lets `http.NewResponseController` reach the underlying conn.
- **"SSE poll loops starve SQLite writers":** rejected — WAL journal, `_txlock=immediate`, short transactions, `busy_timeout=5s`, capped pool (sqlite.go:73-99); readers don't block the event-appending writer under WAL.
- **"Replay gaps corrupt event truth":** rejected — gap detection emits `recovery_required` + fresh snapshot + refresh path (operations.go:386-394); durable cursors are validated with explicit `cursor_ahead`/`replay_gap` conflicts (run_handlers.go:448-459).
- **"Channel double-close / slot leak on unsubscribe":** rejected — delete-before-close under `h.mu` in all three teardown paths (eviction, finish, unsubscribe `sync.Once`), and `defer unsubscribe()` is registered before the flusher-capability early return (operation_handlers.go:236-241).

### Open questions

- Was the 30-second truncation of `followRunSSE`/`followDurableOperationEvents` consciously accepted as a backstop (making F01 accepted debt rather than drift)? No comment, changelog, or test states either way.
- Is `MaxInFlight` intended to double as the documented "32 streams" budget (local-web.md:210), or are those meant to be separate budgets that happen to coincide today? This determines whether F02 is a bug or an undocumented coupling.
