Now let me read the supporting pieces: durable operations, run handlers SSE, and the browser-side JS.
Now let me look at the runtime wiring (server setup, policy timeouts) and the web use-cases that drive long operations.
The hub-path SSE sets an explicit write deadline (operation_handlers.go:242), but I need to verify the other two stream paths and the runtime wiring. Let me check the serve wiring, web use-cases, and tests covering disconnect/resume.
The single `SetWriteDeadline` is suspicious given `WriteTimeout: 30s` at server.go:19. Let me verify the serve wiring, the use-case layer, and what tests/docs say about stream lifetimes.
The docs set a clear CURRENT-CONTRACT: browser disconnect never cancels; reconnect must observe current state while the server lives; documented stream lifetime is 30 minutes. Now I need to empirically validate whether `WriteTimeout: 30s` kills the two durable stream paths that lack `SetWriteDeadline`.
The repro was flawed (client closed early). Let me fix it so the client keeps reading until the server finishes.
Mechanism confirmed empirically: without `SetWriteDeadline`, an SSE stream on this exact server pattern dies at `WriteTimeout` (scaled 300ms ≙ 30s); the hub path's extension lets it live to its own cap. Now let me check remaining counter-evidence: integration tests, retention semantics, and coalescing windows.
Let me pull the exact contract passages from local-web.md for accurate citation.
Investigation complete. Here is my report.

---

### Scope inspected

**Implementation repo** (`ultraplan-go` @ eeaa034):

- `internal/web/operations.go` — operation hub: lifecycle, event ring, subscriber fan-out, reaping (`start`, `run`, `publishAppEvent`, `subscribe`, `appendEventLocked`, `reapLocked`)
- `internal/web/operation_handlers.go` — `handleOperationEvents` (hub SSE), `followDurableOperationEvents` (durable projection fallback), `writeSSEEvent`
- `internal/web/run_handlers.go` — `handleRunEvents`, `followRunSSE`
- `internal/web/server.go`, `server_policy.go` — HTTP server timeouts and stream policy bounds
- `internal/web/security.go` — session cookie issuance/binding (`MaxAge: 3600`)
- `internal/app/durable_operations.go`, `web_usecases.go`, `serve_commands.go`, `operations.go` — durable acceptance, event commit/coalescing, owner control loop, production wiring
- `internal/runcontrol/model.go` — coalesce window, owner tick/heartbeat constants; retention via `OldestRetainedSequence`
- Browser side: `static/js/sse.js`, `static/app.js` (`follow()` ~844–883, durable run stream ~1377–1445), `templates/run.html`
- Tests: `operations_test.go`, `run_handlers_test.go` (incl. `TestBrowserRunDurableOperationCompatibilitySurvivesMissingLocalHubRecord`, cross-server observation), `sse_test.go`
- Authoritative docs: `docs/local-web.md`, `docs/web-compatibility-baseline.md`, `docs/configuration.md`, `docs/recovery.md`, `docs/plans/server-shutdown-run-cancellation-contract.md`, `docs/plans/integrated-roadmap.md`
- Empirical check (outside repo): standalone Go repro of the exact server/SSE pattern confirming `http.Server.WriteTimeout` kills un-extended streams and `ResponseController.SetWriteDeadline` prevents it.

### Architecture assessment

The disconnect story is architecturally sound at the ownership level:

- **Work continues on disconnect, by construction.** The hub executes operations in a goroutine whose context derives from the server root or durable acceptance (`operations.go:180-181`, `217`), never from `r.Context()`. The SSE handler exiting on `r.Context().Done()` (`operation_handlers.go:275`) cannot affect execution. This matches CURRENT-CONTRACT: "disconnecting closes only that subscription" (`docs/local-web.md:150-151`), "Browser disconnect never triggers this sequence" (`local-web.md:236-237`), and the plan contract "A browser tab … losing its SSE connection … does **not** cancel a run" (`docs/plans/server-shutdown-run-cancellation-contract.md:20`).
- **Truth layering is correct.** Every projected progress event is durably committed first (`operations.go:245-255` → `durable_operations.go:124-176`); the hub is explicitly ephemeral ("Durable workspace and product run state are the recovery authority", `local-web.md:173-178`). SSE is transport, not authority (`integrated-roadmap.md:337`).
- **Observation resumes through four tiers:** (1) same-process hub replay via `Last-Event-ID` against a 256-event ring with honest gap detection emitting `recovery_required` + snapshot (`operations.go:386-394`); (2) session/process-independent durable projection fallback (`followDurableOperationEvents`, tested in `TestBrowserRunDurableOperationCompatibilitySurvivesMissingLocalHubRecord`); (3) canonical `/api/v1/runs/{id}/events` SSE with `cursor_ahead`/`replay_gap` conflict contracts and retention facts (`run_handlers.go:448-460`); (4) dashboard merging hub + durable active runs, which even rediscovers CLI-started durable runs (`operation_handlers.go:173-192`, proven cross-server in tests). Session-cookie expiry (>1h ops) degrades gracefully into tier 2 since the durable paths need no session.
- **Backpressure cannot stall work:** slow subscriber queues are shed, not blocked (`operations.go:455-465`).

What is stressed: there are **three hand-written SSE serve loops** (`handleOperationEvents`, `followDurableOperationEvents`, `followRunSSE`) with divergent lifetime handling, and **two monotonic event-ID namespaces** (in-memory ring sequence vs durable run sequence) exposed behind one endpoint family.

### Candidate findings

#### FAILURE-05-F1

- **Priority:** P2
- **Claim:** Both durable SSE stream paths terminate ~30 seconds after connection because they never extend the server's `WriteTimeout`, violating the documented 30-minute stream-lifetime contract and reducing the canonical long-run observability surface to a perpetual reconnect cycle.
- **Evidence:**
  - `server.go:19` `WriteTimeout = 30 * time.Second`, applied at `server.go:104`.
  - Only one `SetWriteDeadline` exists in the repo: `operation_handlers.go:242` (hub path, extended to `MaxStreamLifetime + SSEHeartbeat` = 30 min 15 s).
  - `followRunSSE` (`run_handlers.go:473-527`) and `followDurableOperationEvents` (`operation_handlers.go:432-499`) never extend the write deadline. In Go's net/http, the server-level write deadline covers the whole response unless a handler overrides it via `http.ResponseController`.
  - Empirical confirmation (standalone repro, scaled 300 ms ≙ 30 s): un-extended stream died exactly at `WriteTimeout` (`i/o timeout`); the extended stream outlived it up to its own cap.
  - Contract violated: "Heartbeat / stream lifetime | 15 seconds / 30 minutes" (`docs/local-web.md:193`; same figures `web-compatibility-baseline.md:105-107`, `configuration.md:168-169`). At a 15 s heartbeat cadence, a 30 s effective lifetime allows ~2 heartbeats.
- **Architectural reason:** drift + failure-semantics — three parallel implementations of the same transport idiom, only one carrying the deadline fix; the failure is silent (clean handler return, no terminal/error event).
- **Concrete consequence:** Watching an active long operation on its canonical page `/runs/{id}` (browser uses SSE via `app.js:1410`) dies every ~30 s; `app.js:1425-1432` reconnects after 1 s, replays from `durableLast`, and repeats indefinitely — UI status flaps between "Live — committed events only." and "Reconnecting…", each cycle re-runs `RunEvents`+`Run` query pairs, and non-browser consumers of the fixed compatibility matrix (`web-compatibility-baseline.md:36`) receive streams silently terminated mid-work. No truth is lost (durable cursor replay), but stable live observation of the authoritative surface is impossible past 30 s.
- **Counter-evidence searched:** No test exercises any stream beyond sub-second (`httptest` servers carry no `WriteTimeout`, so CI cannot catch this). No doc declares durable streams intentionally short-lived. Heartbeat cadence, terminal-detection logic, and the hub path's careful deadline management all indicate long-lived intent. Confirmed the `trackedWriter` middleware does not touch deadlines.
- **Confidence:** high
- **Smallest useful action:** Mirror `operation_handlers.go:242` at the top of `followRunSSE` and `followDurableOperationEvents` (`SetWriteDeadline(now + MaxStreamLifetime + SSEHeartbeat)`), restoring parity with the documented bound; add a regression test that serves a stream through a server configured with a short `WriteTimeout`. (Note: this also fixes that `followRunSSE` currently lacks any lifetime cap at all.)

#### FAILURE-05-F2

- **Priority:** P3
- **Claim:** In-memory ring event IDs and durable run sequences are different monotonic spaces served behind the same endpoint `/api/v1/operations/{id}/events`; after hub-record loss (server restart/crash) an auto-reconnecting tab that was fully caught up deterministically receives `409 cursor_ahead` instead of resuming.
- **Evidence:** The hub appends two events never committed durably — accepted snapshot (`operations.go:215`) and "operation started" progress (`operations.go:227`) — plus every committed event (`operations.go:274`). Durable sequences count lifecycle-running (`durable_operations.go:106-114`) plus committed events. For C committed events: live lastID = C+2, durable `LastSequence` = C+1. On reconnect after record loss, `subscribe` fails (`operations.go:378-380`), the fallback validates `after > snapshot.LastSequence` → 409 (`operation_handlers.go:440-443`). A fully-caught-up client always carries C+2.
- **Architectural reason:** boundary (cursor-space translation at the hub→durable fallback seam) + failure-semantics (strictness punishes a normal EventSource auto-reconnect).
- **Concrete consequence:** An open operation page across a server restart cannot resume observation automatically: it announces "Live progress is unavailable. Refresh durable status…" (`app.js:876`) and requires a manual reload (which correctly redirects `/operations/{id}` → `/runs/{id}` and resyncs from the server-rendered cursor, `operation_handlers.go:347-352`, `run.html` `data-last-sequence`). Work is unaffected; impact confined to crash/migration windows owned by adjacent scenarios.
- **Counter-evidence searched:** Strict cursors are deliberate and consistently implemented on both endpoints (`run_handlers.go:448-459`), and the recovery path is guided by UI copy plus redirect. Records are only reaped post-terminal (`operations.go:563-573`), so the live-path never hits this; the divergence matters only when the hub record disappears while the durable run persists.
- **Confidence:** medium-high (mechanics verified by code arithmetic and path tracing; not executed end-to-end)
- **Smallest useful action:** When `followDurableOperationEvents` is entered as a hub-miss fallback, treat a modestly-stale `after` (e.g., `after <= LastSequence+N` or simply any `after > LastSequence`) as a resync — clamp to `LastSequence` and open with a synthetic `snapshot` event — rather than a hard 409; alternatively namespace the fallback cursor separately.

### Defended architecture / rejected hypotheses

1. **"Disconnect cancels or orphans the work" — rejected.** Operation context is server-owned (`operations.go:180-181,217`); cancellation requires explicit `DELETE`/shutdown (`cancelOperation`, `drainAndWait`); contract forbids disconnect-coupling in three places (see assessment). Verified no subscriber-count coupling anywhere in `run`/`finish`.
2. **"Events missed while disconnected are silently lost" — rejected.** Ring replay with gap detection emits `recovery_required` + current-state snapshot instead of fabricating history (`operations.go:386-394`); durable history retains `OldestRetainedSequence` bounds with structured `replay_gap` recovery options; UI surfaces the gap honestly (`app.js:859`).
3. **"A disconnected-but-slow consumer can block or corrupt delivery" — rejected.** Per-subscriber queue of 32 with non-blocking send and shed-on-overflow (`operations.go:455-465`), counted via `slowSubscribers`; capacity errors are explicit 429s with `Retry-After`.
4. **"One-hour session cookie expiry permanently hides >1h operations" — rejected.** Expired cookie yields a fresh session; hub `subscribe` then fails session match and falls through to the session-independent durable projection/status/active-merge paths (`operation_handlers.go:224-235,396-405,173-189`). Residual cost is only F1's reconnect churn.
5. **"30-minute hub stream cap breaks long-operation watching" — rejected.** Documented bound (`local-web.md:193`); EventSource auto-reconnects with `Last-Event-ID` and replays from the retained ring; gap path covered above.

### Open questions

- Is the documented "30-minute stream lifetime" intended to cover `/api/v1/runs/{id}/events` as well, or only operation-event streams? The wording is generic ("bounded SSE"); if a future contract deliberately scopes it to hub streams, F1 narrows to the fallback path plus the three-implementation divergence — but the code inconsistency itself stands.
- Do real deployments ever place a proxy in front of the loopback dashboard (`X-Accel-Buffering: no` hints at nginx awareness)? If yes, F1's 30 s churn compounds with proxy idle timeouts; the single-user loopback design suggests no.
