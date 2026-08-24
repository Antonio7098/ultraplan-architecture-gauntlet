### Scope inspected

- **Implementation** (`ultraplan-go` @ eeaa034): `internal/web/{server.go,server_policy.go,security.go,operations.go,operation_handlers.go,run_handlers.go,routes.go,handlers.go}`; tests `{operations_test.go,run_handlers_test.go,security_test.go,operations_contract_test.go,sse_test.go,server_test.go,test_fakes_test.go}`; `internal/web/static/js/{app.js,sse.js}`; app boundary `internal/app/{web_usecases.go,durable_operations.go,run_control.go,serve_commands.go}`; `cmd/ultraplan/main.go`; `docs/local-web.md`; git history (`git log -S SetWriteDeadline`).
- **Planning workspace**: sprints `31-web-operations` (reasoning.md decision 5, api-design.md, plan.md), `32-hardening-and-release`, `35-durable-run-observability/requirements.md`.
- Verification commands: greps for `SetWriteDeadline|Flusher|heartbeat|recover()`, test-inventory greps. No repo modifications.

### Architecture assessment

Sound overall. Ownership is clean: the ephemeral `operationHub` owns transient pub/sub, backpressure (bounded ring buffer, queue-full slow-subscriber eviction), and drain semantics; durable truth lives in `runcontrol` behind `app.RunUseCases`; the web package holds only HTTP/SSE connection state (matches sprint-35 requirement line 83). Lifecycle wiring is unusually careful: `BaseContext` propagation (server.go:106) makes SIGINT cancel all request contexts so SSE loops exit during drain; `drainAndWait` → `cancelOperations` → `server.Shutdown` ordering (server.go:135–137) is correct; cleanup-uncertain persistence is re-checked under the hub mutex to avoid double-close races; startup reconciliation runs before serving (server.go:76–81).

The stress point is that **the SSE transport layer exists as three independently hand-rolled streaming loops** (`handleOperationEvents`, `followDurableOperationEvents`, `followRunSSE`) sharing constants (`SSEHeartbeat`, `MaxStreamLifetime`) but not mechanics — and they have already drifted apart in deadline enforcement, heartbeat cadence, and lifetime bounding.

### Candidate findings

---

**ID: SPECIALIST-12B-F01**
**Priority: P2**

**Claim:** Two of the three SSE endpoints are hard-capped at ~30 seconds by the server's `WriteTimeout`, defeating the documented/policy 30-minute stream lifetime and 15-second heartbeat model; the enforcement logic exists in only one of three duplicated stream loops, which is drift introduced when the durable paths were added.

**Evidence:**
- `server.go:19,104` — `WriteTimeout = 30 * time.Second` applied to the whole server. In `net/http` this is an absolute per-request write deadline set when headers are read; once it lapses every subsequent write errors and the handler returns.
- `operation_handlers.go:242` — the **only** `SetWriteDeadline` in the tree: `_ = http.NewResponseController(w).SetWriteDeadline(h.now().Add(MaxStreamLifetime + SSEHeartbeat))`, present solely in `handleOperationEvents`.
- `operation_handlers.go:448–499` (`followDurableOperationEvents`) — installs a 30-min `lifetime` timer (line 460) but never extends the write deadline, so it can never be reached.
- `run_handlers.go:473–527` (`followRunSSE`) — neither extends the write deadline nor has any `MaxStreamLifetime` bound; additionally emits a `: heartbeat` comment every ~1 s poll cycle (lines 512–524), drifting from the `SSEHeartbeat = 15s` constant used by the other two loops.
- History: `git log -S SetWriteDeadline` shows it was added in `a221683` (guarded web operations/SSE); the polling loops arrived later in `e09d394` (durable run control) without carrying it over.
- Tests cannot catch this: all streaming tests use `httptest.ResponseRecorder` (e.g., `run_handlers_test.go:362`, `operations_test.go:348–356`), which imposes no deadlines; no test sends `Accept: text/event-stream` to `/api/v1/runs/{id}/events`.

**Architectural reason:** lifecycle + drift + change-surface. Stream-lifetime authority is split across three copies of the same loop instead of one owned transport helper; the policy object (`ServerPolicy`, `server_policy.go:16,47`) validates `SSEHeartbeat < MaxStreamLifetime` coherently, but the runtime only enforces the pair on one endpoint.

**Concrete consequence:** every open `/api/v1/runs/{id}/events` or durable-fallback `/api/v1/operations/{id}/events` stream dies on its first write after ~30 s. The bundled client masks it (`app.js:1425–1431` closes and reconnects from `durableLast` after 1 s, re-running preflight replay + snapshot reads each cycle), so behavior degrades to a 30-second forced reconnect churn per open run tab instead of the intended 15 s-heartbeat/30-min-lifetime regime; `hub.streams` accounting and `MaxConcurrentStreams` never apply to these paths. Any future non-auto-reconnecting consumer breaks outright.

**Counter-evidence searched:** docs promise 30-min lifetime only in the *operation bounds* table (`docs/local-web.md:193`), and sprint-35 doctrine says "the transport is not the source of truth" (requirements.md:23) — reconnect-tolerance is intentional. But the presence of unreachable 30-minute `lifetime` timers *inside* those very loops plus the sibling endpoint's explicit deadline extension shows long-lived intent; nothing documents 30 s recycling as desired.

**Confidence:** high (mechanism certain; impact self-healing hence P2, not P1).

**Smallest useful action:** extend the write deadline in both polling loops exactly as `handleOperationEvents` does (`http.NewResponseController(w).SetWriteDeadline(...)` — `trackedWriter.Unwrap()` at `security.go:39` already supports it); ideally extract one shared stream-loop helper (headers, deadline, heartbeat ticker, lifetime timer, disconnect select) consumed by all three endpoints so the trio cannot drift again.

---

**ID: SPECIALIST-12B-F02**
**Priority: P3**

**Claim:** The transient-hub subscribe path has no cursor-ahead rejection (unlike both durable paths) and broadcasts one client's gap-recovery events to all subscribers of the operation.

**Evidence:**
- `operations.go:374–400` — `subscribe()` replays `event.ID > lastID`; a `Last-Event-ID` ≥ `nextEventID` yields an empty replay, no error, and a silently idle stream until the next event or lifetime expiry. Contrast `run_handlers.go:448–452` and `operation_handlers.go:440–443`, which return explicit `cursor_ahead` / `replay_gap` conflicts.
- `operations.go:386–394` — gap detection appends `recovery_required` + `snapshot` via `appendEventLocked`, which fans out to **every** subscriber (`operations.go:455–465`), so client A's private reconnect injects frames into client B's stream and permanently into the retained ring (consuming the 256-event budget).

**Architectural reason:** boundary / failure-semantics consistency. Cursor validation semantics differ across the ephemeral/durable boundary for the same resource family.

**Concrete consequence:** a buggy or stale client cursor pins a silent connection for up to 30 minutes with no diagnostic signal, while the same mistake against a durable run gets an immediate 409; concurrent reconnect storms add spurious recovery frames to other live viewers' streams.

**Counter-evidence searched:** `docs/local-web.md:173–178` documents gap→`recovery_required`+snapshot as intended, and the browser treats all stable names idempotently (`sse.js:3`, contract test `operations_contract_test.go:116–151`), so the broadcast is benign today; no contract text promises `cursor_ahead` on the transient path. This is why it stays P3.

**Confidence:** medium.

**Smallest useful action:** in `subscribe()`, return a typed error when `lastID >= record.nextEventID` (mapped to the existing `cursor_ahead` shape), and consider emitting gap/snapshot only to the requesting subscriber's queue.

---

**ID: SPECIALIST-12B-F03**
**Priority: P3**

**Claim:** Long-lived SSE streams consume the same 32-slot in-flight semaphore as interactive requests, and `MaxConcurrentStreams == MaxInFlight` means saturated streams block every other request, including `/api/v1/health` and cancel POSTs, with no policy-level headroom rule.

**Evidence:**
- `security.go:73,94,121–131` — semaphore acquired in middleware before routing; released only when the handler returns, i.e., after up to 30 minutes of streaming for hub streams.
- `operations.go:27–28` — `MaxConcurrentStreams = 32` equals `MaxInFlight = 32` (`server.go:22`); `server_policy.go:47–50` validates `MaxSubscribersPerOperation ≤ MaxConcurrentStreams` but no relation reserving non-stream capacity. Durable/run SSE streams are bounded only by the semaphore (they never register in `hub.streams`).

**Architectural reason:** boundary / capacity ownership — the concurrency budget couples two independent workloads (observation vs interaction) invisibly at the policy layer.

**Concrete consequence:** a browser restoring several run/operation tabs (each auto-opening EventSources) can wedge dashboard navigation and health checks until streams end; requests block (with client-abort escape) rather than failing fast, making the condition hard to diagnose from diagnostics alone.

**Counter-evidence searched:** single-user loopback posture (`docs/local-web.md:86–113`) and browser per-host HTTP/1.1 connection caps (~6/tab) make exhaustion unlikely day-to-day; documented bounds table lists both limits without claiming independence. Genuine but low-probability coupling — hence P3.

**Confidence:** medium.

**Smallest useful action:** either reserve headroom (`MaxConcurrentStreams` strictly `< MaxInFlight`, enforced in `ValidateServerPolicy`) or fail fast (429 `subscriber_capacity`) instead of blocking when the semaphore is exhausted by streams.

---

**ID: SPECIALIST-12B-F04**
**Priority: P3**

**Claim:** `publishAppEvent` persists progress with `context.Background()` and no timeout, the sole unbounded background persistence call site in the drain/lifecycle machinery; an in-flight hung journal write cannot be interrupted by operation cancellation or shutdown.

**Evidence:**
- `operations.go:245–254` — `manager.RecordOperationEvent(context.Background(), ...)`; contrast `FinishOperation` wrapped with a 30 s timeout (`operations.go:235`), cleanup-uncertain persistence with 1 s (`operations.go:516`), and app-side retry discipline (`appendRunEventWithRetry` 5 s deadline, `run_control.go:305–316`).
- On persist error the op is cancelled (`operations.go:250`), but a *stuck* (not erroring) write ignores `record.cancel()` since it holds a different context.

**Architectural reason:** lifecycle / failure-semantics — cancellation authority does not reach one branch of the event pipeline.

**Concrete consequence:** a wedged store stalls the operation goroutine inside the emit callback; user cancel and drain produce no visible effect until the write returns (shutdown still bounds the process at 10 s → `cleanup_uncertain`, so blast radius is limited to unresponsiveness, not corruption).

**Counter-evidence searched:** `Background()` here is plausibly deliberate so final events survive drain-context cancellation; local SQLite makes indefinite hangs unlikely; retries are deadline-bounded. Only the single non-returning `repository.Append` call escapes all bounds.

**Confidence:** low-medium.

**Smallest useful action:** wrap the `RecordOperationEvent` call in `context.WithTimeout(context.Background(), N)` mirroring the adjacent `FinishOperation` pattern.

### Defended architecture / rejected hypotheses

- **Session-less durable reads/cancels are not a security hole.** Hub records are session-scoped (`operations.go:309–318,345–350`) while durable run status/events/cancel ignore the starting session. Rejected as a defect: sprint 35 requires "Read visibility is not coupled to the browser session" (requirements.md:23) and `docs/local-web.md:301–306` states durable operation reads are cross-session; mutations still pass the middleware's Host/Origin/session-cookie/CSRF gates (`security.go:134–163`), tested in `security_test.go:118–282`.
- **Shutdown does not stall 10 s behind open SSE streams.** I initially predicted `server.Shutdown` waiting on streaming handlers; rejected because `BaseContext: func(net.Listener) context.Context { return ctx }` (server.go:106) makes SIGINT cancel every request context, firing `r.Context().Done()` in all three stream loops, and `finish()` closes subscriber channels anyway (`operations.go:300–306`). Verified ordering in `server_test.go` lifecycle test.
- **Slow-subscriber eviction and drain racing do not panic/deadlock.** Eviction deletes under `h.mu` before `close` (`operations.go:455–465`), unsubscribe is `sync.Once` (`operations.go:410–422`), and both `finish` and `markCleanupUncertain` re-check terminal state under the mutex, so `record.done` cannot be closed twice (`operations.go:277–307` vs `531–555`); covered by `TestOperationHubBoundsReplayAndSlowSubscriberIsolation` and the deadline-drain test.
- **Ring-buffer trimming loses replay safety silently? No** — trim keeps the invariant checked by the gap detector (`lastID < events[0].ID-1` ⇒ `recovery_required`, `operations.go:386–394`), matching the documented "never fabricates missing progress" contract.
- **Ephemeral-only hub is not missing durability by accident** — it is an explicit, reasoned decision (sprint 31 reasoning.md:189,227: durable workers require a separately versioned architecture), later satisfied by the durable manager without relaxing sprint-31 invariants.

### Open questions

- Sprint 35 has only `requirements.md` in the workspace (no plan/reasoning yet): if its eventual contract specifies a stream lifetime or heartbeat cadence for durable-run SSE, SPECIALIST-12B-F01 moves from P2 drift toward a contract violation.
- Whether any consumer besides the bundled browser targets `/api/v1/runs/{id}/events` with `Accept: text/event-stream` (docs say the API is not yet a promised integration surface) — determines how visible the ~30 s recycle is outside `app.js`.
