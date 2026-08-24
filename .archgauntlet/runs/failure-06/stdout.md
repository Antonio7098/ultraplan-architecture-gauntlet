## Failure-stage report — FAILURE-06: Graceful shutdown with active workers

### Scope inspected

Read in full or substantial part:

- **Web lifecycle**: `internal/web/server.go`, `internal/web/operations.go`, `internal/web/operation_handlers.go`, `internal/web/run_handlers.go`, `internal/web/handlers.go`, `internal/web/server_policy.go`
- **App/durable wiring**: `internal/app/serve_commands.go`, `internal/app/durable_operations.go`, `internal/app/web_usecases.go`, `internal/app/run_control.go`, `internal/app/operations.go`
- **Run control**: `internal/runcontrol/lifecycle.go` (Heartbeat/RequestCancellation/AcknowledgeCancellation/Reconcile), `internal/runcontrol/sqlite.go` (ProposeTerminal/Events/Health), `internal/runcontrol/model.go` (constants/states), `internal/runcontrol/fault_test.go`
- **Process boundary**: `internal/platform/process/process.go`, `process_unix.go`, `process_other.go`
- **Product markers**: `internal/sprint/cleanup_uncertain.go`, `internal/sprint/locks.go` (reconcile), `internal/sprint/state.go` (outcome validation fragment), `internal/study/cleanup_uncertain.go`
- **Entry**: `cmd/ultraplan/main.go`; **UI consumer**: `internal/web/static/app.js` (durable-follow logic)
- **Tests**: `internal/web/server_test.go`, `operations_test.go`, `operations_contract_test.go`, `sse_test.go`, `internal/app/web_operations_test.go`, `internal/runcontrol/lifecycle_test.go`, `fault_test.go`, `process_integration_test.go`
- **Authoritative docs**: `docs/plans/server-shutdown-run-cancellation-contract.md` (CURRENT-CONTRACT for this exact scenario), `docs/local-web.md` (shutdown/SSE sections), `docs/recovery.md`
- Cross-cutting greps: `RequestCancellation` call sites, `server_shutdown`, `.Reconcile(`, `SetWriteDeadline`, `httptest.NewServer|event-stream`

### Architecture assessment

The shutdown skeleton is sound and unusually deliberate. Ownership layers are coherent: the ephemeral hub (`operationHub`) owns projection/subscribers only; `durableOperationManager` owns persisted acceptance/claim/events/terminal; product modules own `.cleanup-uncertain.json` markers; run-control SQLite owns authoritative lifecycle with fences, leases, heartbeats, and process-identity reconciliation at repository open (`internal/app/run_control.go:64`). Contract sequence (§3) is implemented in order: draining gate (`operations.go:160-166`), exactly-once cancellation (`cancelOnce`, `operations.go:358-370`), bounded 10s grace, persist-before-project uncertainty ordering (`persistCleanupUncertain` then `markCleanupUncertain`, `operations.go:495-497`), terminal arbitration via fenced CAS (`sqlite.go:722-806`). Process trees get their own group with TERM→KILL escalation (`platform/process/process_unix.go:14-35`). Slow-subscriber handling is correct: non-blocking drop under lock with counters (`operations.go:455-465`), tested.

The stress concentrates at two seams: (1) the handoff between the ephemeral hub and the durable journal — cancellation *reason* and *persistence-loss* classification do not cross it; (2) `net/http` deadline plumbing for the three distinct SSE implementations, only one of which extends the write deadline.

### Candidate findings

---

**ID: FAILURE-06-F01**
**Priority: P1**

**Claim**: Graceful shutdown cancels durable operations in memory but never persists a cancellation request or the `server_shutdown` reason to the run-control journal; the durable record ends as plain `cancelled` with reason `"operation cancelled"` (or, on crash-before-finish, stays `running` with zero cancellation evidence until generic restart reconciliation).

**Evidence**:
- `internal/web/operations.go:488` — `drainAndWait` calls `h.cancelOperation("", id, "server_shutdown")`, which only mutates the in-memory doc and calls `record.cancel()` (`operations.go:345-372`).
- `internal/app/operations.go:40-44` — `DurableOperationManager` interface exposes only `AcceptOperation/RecordOperationEvent/FinishOperation`; no way for the hub to persist a cancellation request.
- `internal/app/durable_operations.go:241-252` — `FinishOperation` maps `context.Canceled` to `TerminalCancelled` with hardcoded reason `"operation cancelled"`; `controlOperation` (`:185-186`) returns silently on `ctx.Done()` without writing anything.
- Grep across repo: `repository.RequestCancellation` is invoked only from user-facing paths (`internal/web/run_handlers.go:197,378`, `internal/tui/app.go:110,124`, `internal/app/run_commands.go:216`) — never from the shutdown path.
- CURRENT-CONTRACT: `docs/plans/server-shutdown-run-cancellation-contract.md` §3.2 ("persist or publish `cancellation_requested` … record `reason: server_shutdown`"), §3.5 (forbids representing shutdown cancellation as "ordinary user cancellation without the shutdown reason"), and `docs/local-web.md:229-231` ("requests `server_shutdown` cancellation exactly once for every active server-owned operation").

**Architectural reason**: drift / authority — the durable journal is the designated truth owner for lifecycle and cancellation, but the shutdown actor (hub) cannot reach it through the capability interface it holds.

**Concrete consequence**: `ultraplan run show`/diagnostics cannot distinguish shutdown-cancelled work from user-cancelled work; if the process dies after cancellation but before `FinishOperation` commits (the exact window the 10s budget can miss), restart reconciliation records generic `interrupted`/`cleanup_uncertain` with reasons like `owner_process_missing_after_grace` (`lifecycle.go:481-495`), losing the shutdown context entirely. Contract §9.2-style tests are unpassable, and none exist asserting a durable shutdown reason.

**Counter-evidence searched**: checked whether `RunOperation` emits an event carrying `Reason` during cancellation (`RecordOperationEvent` payload does include `event.Reason`, `durable_operations.go:148-152`, but only if the product code volunteers one — incidental, not authoritative lifecycle); checked `controlOperation` for any persist-on-cancel (none); checked tests (only the product-marker path asserts `"server_shutdown"` — `web_operations_test.go:138`).

**Confidence**: high

**Smallest useful action**: extend `DurableOperationManager` (or a narrow optional interface, like `OperationCleanupRecorder`) with a cancellation-request call backed by `repository.RequestCancellation(runID, "server_shutdown")`, invoked from `drainAndWait` before `record.cancel()`; let `FinishOperation` prefer the snapshot's `cancellation_reason` over its hardcoded string.

---

**ID: FAILURE-06-F02**
**Priority: P2**

**Claim**: Production `WriteTimeout=30s` silently caps two of the three SSE streaming paths — `followRunSSE` (`/api/v1/runs/{id}/events`) and `followDurableOperationEvents` — at 30 seconds wall clock, contradicting the 30-minute `MaxStreamLifetime`/heartbeat design; only `handleOperationEvents` extends the deadline.

**Evidence**:
- `internal/web/server.go:19` (`WriteTimeout = 30 * time.Second`), `:100-107` (set on `http.Server`); Go sets this deadline once per request, so activity does not renew it.
- `internal/web/operation_handlers.go:242` — `_ = http.NewResponseController(w).SetWriteDeadline(h.now().Add(MaxStreamLifetime + SSEHeartbeat))` proves the mechanism is known and intended for streams.
- `internal/web/run_handlers.go:473-527` (`followRunSSE`) and `internal/web/operation_handlers.go:432-499` (`followDurableOperationEvents`) contain no `SetWriteDeadline`.
- Consumer: `internal/web/static/app.js:1410` opens `EventSource('/api/v1/runs/{id}/events?after=…')` against this exact path.
- Tests never exercise real transport deadlines: `sse_test.go` checks frame formatting only; the operation SSE test uses `httptest.NewRecorder` (`operations_test.go:348-365`); no `httptest.NewServer` SSE test exists.

**Architectural reason**: change-surface / failure-semantics — one concern (bounded SSE lifetime) is implemented three times with divergent deadline handling; the divergence is invisible to CI because recorder-based tests bypass `net/http` deadlines.

**Concrete consequence**: every durable-run follow stream terminates at exactly ~30s regardless of activity (heartbeat writes fail once the deadline passes). The browser masks it with reconnect churn (`app.js` error handler retries after 1s, replaying from `Last-Event-ID`), but any scripted/API consumer sees silent stream closure indistinguishable from a crashed server mid-operation; reconnect loops also re-enter replay/gap logic unnecessarily.

**Counter-evidence searched**: verified `IdleTimeout`/`ReadTimeout` are not the cause here; checked whether `MaxStreamLifetime` is documented as operation-endpoint-only (`docs/local-web.md:193` bounds table is under "Operation bounds", but the same constant governs the run-events loop timers, showing shared intent); checked whether the JS client polls instead of streaming (it streams).

**Confidence**: high (mechanics), medium (real-world impact given browser auto-reconnect)

**Smallest useful action**: add the same one-line `http.NewResponseController(w).SetWriteDeadline(...)` to `followRunSSE` and `followDurableOperationEvents`, plus one integration test that serves SSE through a server constructed with the production timeouts.

---

**ID: FAILURE-06-F03**
**Priority: P2**

**Claim**: Persistence loss during a web-owned durable operation collapses into a generic `cancelled` terminal instead of `persistence_degraded`, while the identical condition on the CLI/runtime acceptance boundary is recorded truthfully as `persistence_degraded`.

**Evidence**:
- Web path: `internal/app/durable_operations.go:165-171` — event-append failure calls `owned.cancel()`; `:190-216` — control-goroutine `Snapshot`/`Heartbeat`/`Reconcile` failures call `owned.cancel()`; `:241-252` — `FinishOperation`'s outcome switch has no persistence case, so the resulting `context.Canceled` becomes `TerminalCancelled`/"operation cancelled".
- Runtime path: `internal/app/run_control.go:156-163` and `:282-291` map the same failures to `TerminalPersistenceLost`/"durable event persistence failed"; `model.go` defines `TerminalPersistenceLost` and `persistence_degraded` appears in schema CHECKs (`sqlite.go:245-287`), UI cues (`run_handlers.go:319`), and `docs/recovery.md:193-198`.
- Both boundaries are live in production: serve wires `repositoryRunUseCases` + `newDurableOperationManager` (`serve_commands.go:63-66`) and `controlledRuntimeFor` (`run_control.go:103-114`).

**Architectural reason**: failure-semantics / parity — two acceptance boundaries translate the same storage-failure class into different durable truths, breaking the parity requirement (`server-shutdown…contract.md` §9.5) and making the `persistence_degraded` recovery guidance unreachable for browser-started operations.

**Concrete consequence**: after a transient SQLite failure (quota, busy-beyond-retry, permissions), a sprint flow started from the dashboard is recorded "cancelled" by operator choice rather than "persistence degraded"; diagnostics and the runbook branch on the wrong state, and cross-surface metrics under-report degradation.

**Counter-evidence searched**: checked whether `publishAppEvent`'s `ErrWebUnavailable` distinction covers this (it is a different mode: durable manager absent entirely, not failing mid-run); checked whether `FinishOperation` inspects prior persistence state on `owned` (it does not); checked fault tests — `runcontrol/fault_test.go` verifies typed failures surface, but no test pins the web-path terminal mapping.

**Confidence**: high

**Smallest useful action**: record a persistence-loss flag on `ownedDurableOperation` when `RecordOperationEvent`/control-tick persistence fails, and let `FinishOperation` propose `TerminalPersistenceLost` when set — mirroring `run_control.go:282-291`.

---

**ID: FAILURE-06-F04**
**Priority: P3**

**Claim**: Request contexts derive from the serve context that SIGINT cancels, so all SSE streams die at signal time — before `drainAndWait` projects `cancel_requested`/`terminal` frames. The documented shutdown step "publishes terminal SSE, then stops HTTP" (`docs/local-web.md:230`; contract §3.6) cannot occur.

**Evidence**: `cmd/ultraplan/main.go:19` (`signal.NotifyContext`) → `web.Run` ctx → `http.Server.BaseContext` returns that same ctx (`server.go:106`), cancelling every request context immediately; handlers select `<-r.Context().Done()` (`operation_handlers.go:275,495`, `run_handlers.go:518`); the hub dutifully appends terminal events and closes subscriber channels (`operations.go:296-306,543-554`) but subscribers are already gone.

**Architectural reason**: boundary / lifecycle — SSE lifetime is coupled to server-lifetime context instead of to the subscription lifecycle the hub owns.

**Concrete consequence**: browsers see abrupt disconnect + reconnect churn during the ≤10s drain window (reconnects hit draining reads), and the carefully built terminal/`cleanup_uncertain` projections never reach any client; observability-only, since durable status remains authoritative and refresh links are documented.

**Counter-evidence searched**: considered that instant stream termination may be intentional boundedness for shutdown; rejected as sole explanation because the contract and local-web.md explicitly promise terminal SSE delivery before HTTP stop, and hub code invests in projecting those events. Note the same coupling also aborts ordinary in-flight reads at t=0, stricter than contract §3.1's "allow bounded reads … where practical".

**Confidence**: high (mechanics)

**Smallest useful action**: run the SSE serve loop on a context detached from request cancellation but shut down explicitly by the hub (e.g., derive from `context.WithoutCancel(r.Context())` plus the hub's drain completion channel), so terminal frames flush before handler return.

### Defended architecture / rejected hypotheses

- **"Shutdown ordering deadlocks exit"** — rejected: `drainAndWait` and `server.Shutdown` share one bounded `shutdownCtx`; expiry forces `server.Close()`; SSE handlers exit promptly (closed subscriber channels + cancelled request contexts), so `serveErr` always arrives (`server.go:131-153`).
- **"`markCleanupUncertain` races `finish` into double-close/decrement"** — rejected: both check `terminalOperationState` under `h.mu` before mutating (`operations.go:281,511,536`); `done` is closed exactly once; covered by `TestOperationHubDeadlinePersistsCleanupUncertaintyBeforeTerminalProjection`.
- **"Late successful completion can be overwritten by shutdown cancellation (or vice versa)"** — rejected at both layers: in-memory terminal check in `finish`/`cancelOperation`; durably, `ProposeTerminal` is first-writer-wins behind fence CAS (`sqlite.go:746-786`), asserted in `lifecycle_test.go:88-96`. Contract §6 satisfied.
- **"Slow subscribers block product work or leak goroutines"** — rejected: non-blocking send-or-drop with close and counters (`operations.go:455-465`); `unsubscribe` is `sync.Once`; tested.
- **"Cleanup-uncertain markers are orphaned or startup fails open"** — rejected: sprint/study reconciliation consumes markers under the mutation lease at startup and fails closed otherwise (`sprint/locks.go:25-85`, `study/cleanup_uncertain.go:101-147`), matching `local-web.md:232-236`; marker writing deliberately avoids the lease because the original owner may still hold it at deadline exhaustion (`sprint/cleanup_uncertain.go:28-31`) — necessary boundary translation, not duplication.
- **"Orphaned agent processes survive server exit"** — rejected for owned work: dedicated process groups with group-SIGTERM then group-SIGKILL, and `CleanupComplete` surfaced for uncertainty (`platform/process/process_unix.go:14-35`, `process.go:113-137`).
- **Held back**: `startConfirmed` holds `h.mu` across `PrepareOperation`-confirm and `AcceptOperation` (multiple SQLite transactions, `operations.go:150-218`), briefly stalling all hub reads/fan-outs. Given WAL mode, busy-timeout, a single loopback user, ≤8 active ops, and the simplicity benefit of serialized admission, I judge the current design defensible and do not report it as a defect.

### Open questions

- Was `local-web.md:229`'s "requests `server_shutdown` cancellation" intended as durable-journal semantics or as in-memory/SSE-only phrasing? If the former, F01 is straightforwardly P1 drift; if the latter, the contract doc still requires the durable form.
- Do any shipped non-browser consumers follow `/api/v1/runs/{id}/events` SSE past 30s? If yes, F02 warrants promotion to P1; the bundled UI self-heals.
- Repeated Ctrl-C during a hung cleanup: `signal.NotifyContext` keeps capturing signals after the first, so only SIGKILL breaks a stuck grace period. Contract §4.2 makes second-signal shortening optional ("may"), so this is recorded as a question, not a defect.
