Now let me trace correlation wiring and the web SSE layer.
Now let me read the authoritative observability contract.
This is a critical lead — the allowlist vs. promotion drift. Let me verify with tests.
Confirmed empirically. Now let me check the remaining angles: logging wiring, health truthfulness, and the study-side correlation.
Two strong leads confirmed. Let me empirically verify the `*_tokens` allowlist contradiction and inspect the remaining test coverage.
Investigation complete. All key hypotheses were verified against real code and empirically tested. Here is the review:

---

### Scope inspected

**Target repo** (`ultraplan-go` @ eeaa034):
- `internal/runcontrol/`: model.go, sanitize.go (+test), sqlite.go (Accept/Append/ProposeTerminal/Events/Health), retention.go, local_log.go, metrics.go, interfaces.go
- `internal/platform/runtime/`: runtime.go (Adapter.StartRun, event mapping/promotion), events.go, logging/logging.go
- `internal/app/`: run_control.go (controlledRuntime), durable_operations.go (+test), health_commands.go, study_commands.go (progress rendering), run_commands.go, web_usecases.go
- `internal/web/`: run_handlers.go (runs API/SSE/timeline), templates/run.html
- `internal/sprint/service.go` (runtimeRequest/TraceID), `internal/study/run.go`
- Docs: docs/configuration.md, docs/architecture.md

**Workspace** (`ultraplan-workspace` @ 368a789): system/contracts/core/observability.md (full), studies/ultraplan-daemon-events-study/study.json + dimension 01.06 (skimmed, treated as FUTURE-INTENT)

**Empirical verification**: scratch copy in `/tmp/opencode` (since removed); two instrumented tests ran real `OpenSQLite → Accept → Claim → Append → Events` round-trips. No target repo was modified.

### Architecture assessment

The observability core is genuinely well-architected: a single durable authority (`SQLiteRepository`) with fenced appends, an immutable trigger-guarded journal (`sqlite.go:327`), sequence-invariant snapshot validation (`model.go:349-364`), tiered retention that preserves high-value event classes (`retention.go:185-188`), replay-gap detection surfaced to clients (`run_handlers.go:454-460`), and a bounded private diagnostic log whose drop-on-full behavior is documented at the declaration site (`local_log.go:29-31`). Sanitization is defense-in-depth with credential/path markers (`sanitize.go:84-98`). The stress point is not any single layer but **fragmented ownership of the payload-field policy across three layers** (runtime redaction/truncation → app promotion policy → runcontrol allowlist+denylist gate), which has already drifted, and **two parallel implementations of the durable-progress pipeline inside app** that have diverged in failure semantics.

### Candidate findings

---

**SPECIALIST-18B-F01**
- **Priority:** P1
- **Claim:** The durable storage gate silently discards exactly the agent-stream detail the two most recent observability commits (c455510, eeaa034) claim to surface — including fields the gate itself explicitly allowlists.
- **Evidence:**
  - Producer promotion: `internal/app/run_control.go:435` (promote set includes `title/detail/text/delta/content/native_type/line`) and `internal/platform/runtime/runtime.go:592-604`.
  - Storage gate: `internal/runcontrol/sanitize.go:10-17` — `allowedEventPayloadFields` lacks those keys; line 30 drops everything else.
  - Internal contradiction: `sensitiveEventField` ("token" fragment, sanitize.go:76) defeats the explicit allowlist entries `input_tokens/output_tokens/total_tokens` (sanitize.go:14).
  - Consumer: `internal/web/run_handlers.go:288` prefers `text/delta/detail/message/content/title/output` for `DetailText`; `templates/run.html` renders it as the timeline detail paragraph. Only `message` can ever match post-gate.
  - Empirical: appended payload of 16 representative keys → stored keys were only `{runtime_event_id, runtime_run_id, session_id, type, kind, tool, action, state, status, message}`; omission count=7 "unsafe event detail omitted". Second probe: `{input_tokens,output_tokens,total_tokens,turns}` → only `turns` survived; omission count=3.
- **Architectural reason:** drift / change-surface — the final authority (allowlist) was not updated when producer-side policy changed in c455510/eeaa034; no end-to-end test spans producer→gate→view (sanitize_test.go asserts only hostile drops; durable_operations_test.go:60 asserts only `tool`/`kind`, both allowlisted).
- **Concrete consequence:** The web run timeline and durable journal regress to the exact generic rows c455510 claimed to fix ("Fixes generic 'runtime'/'Tool call' rows"); token/cost facts are unobservable durably; every message event accrues routine omissions, degrading `OmissionTotal` as a trust signal for *real* redactions.
- **Counter-evidence searched:** Checked whether live CLI progress compensates — it does (`study_commands.go:499-509` reads pre-sanitization in-memory events), confirming this is specifically a durable-channel defect. Checked whether commit messages/tests indicate deliberate narrowing — no; c455510 explicitly claims "now carries tool/action/title".
- **Confidence:** high
- **Smallest useful action:** Extend `allowedEventPayloadFields` with the promoted observable keys and fix/exempt the token-count fragment collision; add one integration test asserting a representative opencode-shaped event survives `Append → Events → newRunEventView`. Longer term, make runcontrol own the field schema as a shared constant imported by producers.

---

**SPECIALIST-18B-F02**
- **Priority:** P2
- **Claim:** Durable correlation is written once at Accept-time and never enriched: `Correlation.RuntimeRunID/AgentwrapRunID/ProviderSessionID/ExternalHarnessRunID` are dead fields, and retention deletes the only records that do carry runtime/session identity.
- **Evidence:**
  - Model anticipates full correlation: `internal/runcontrol/model.go:224-231`; validation wires them (model.go:234-243).
  - Sole production writer sets only `ProductTaskID`: `internal/app/run_control.go:124`. `result.RunID`/`result.SessionID` are available after `base.StartRun` (run_control.go:262) but never persisted to correlation; `Repository` has no update path (`interfaces.go:49-65`).
  - `agentwrap_run_id` appears only in the allowlist and model — zero emitters, zero readers.
  - Study runs never set `req.TraceID` (no assignment in `internal/study/*.go`; only sprint does, `service.go:1013-1017`) → empty correlation for all study runs.
  - Compaction deletes `progress/message` events first (`retention.go:76,187`) — precisely the only carriers of `session_id`/`runtime_run_id` payloads.
- **Architectural reason:** lifecycle / authority — the snapshot is the designated correlation authority (`Snapshot.Correlation`, consumed via the runs API) but its write window closes before runtime identity exists; OBS-CORR-001 ("correlation identifiers must survive subsystem boundaries… attach correlation metadata to owned work") is only partially met.
- **Concrete consequence:** After compaction/tombstoning (7-day default full history), a run row cannot be joined to its harness session or provider session for audit or incident follow-up; during the run, joining requires scanning event payloads instead of reading the snapshot.
- **Counter-evidence searched:** Privacy rationale rejected — session IDs are already persisted per-event in plaintext payloads (allowlisted, sanitize.go:11), so correlation storage adds no exposure. Daemon-events study dimensions 01.06/01.07 are FUTURE-INTENT and not counted as current defects; the dead fields plus dead allowlist entry show stalled current intent, not absent intent.
- **Confidence:** high (facts), medium (whether enrichment was planned imminently)
- **Smallest useful action:** After `base.StartRun` returns, append one lifecycle progress event carrying `agentwrap_run_id`/`session_id` (keys already allowlisted), or add a narrow `UpdateCorrelation` on the fence; populate `TraceID` in the study path mirroring sprint's scheme.

---

**SPECIALIST-18B-F03**
- **Priority:** P2
- **Claim:** Two parallel durable-progress pipelines in `app` have drifted: divergent coalescing keys and divergent persistence-failure semantics, with one control loop swallowing failures without any structured signal.
- **Evidence:**
  - Coalescer A (runtime runs): content-hash key, `run_control.go:176-180`; Coalescer B (web operations): field-concatenation key, `durable_operations.go:137-146`.
  - Failure path A: snapshot/heartbeat error → `persistence_degraded` terminal with reason (`run_control.go:156-163, 283-291`).
  - Failure path B: identical errors → silent `owned.cancel()` (`durable_operations.go:190-215`); `FinishOperation` then classifies by context/runErr into `cancelled`/`failed` (`durable_operations.go:241-252`) — persistence root cause is unrecoverable, and `persistence_degraded` is proposed only for start failures (durable_operations.go:111). No log or event is emitted at failure time anywhere in `durableOperationManager`.
- **Architectural reason:** drift / failure-semantics — same concept (fenced owner supervision + coalesced progress append) implemented twice; OBS-CORE-001 requires structured signals on persistence failure paths and forbids mislabeled outcomes.
- **Concrete consequence:** An operator investigating why a dashboard operation ended "cancelled" cannot distinguish user cancellation from run-control store failure; future fixes to coalescing windows or heartbeat logic must be applied twice and will predictably diverge further.
- **Counter-evidence searched:** Duration difference (CLI op vs long run) doesn't justify different terminal truthfulness; tests cover only happy paths (`durable_operations_test.go`), so divergence is untested rather than intended.
- **Confidence:** high
- **Smallest useful action:** Extract the shared supervisor loop (tick/heartbeat/reconcile/cancel-watch) and coalescer into one app-internal type parameterized by event-mapping; make path B propose `TerminalPersistenceLost` like path A.

---

**SPECIALIST-18B-F04**
- **Priority:** P2
- **Claim:** The structured-logging subsystem is inert: config and code exist, are validated and documented, but nothing consumes them.
- **Evidence:** `internal/platform/logging/logging.go` has zero importers (`rg "platform/logging"` over internal/ + cmd/ returns nothing outside its own tests); `config.Logging` is validated (`platform/config/config.go:463-467`) and documented (docs/configuration.md:73-75, 187-188); `CLIOverrides.LogFormat/LogLevel` (`config.go:97-98,153-158`) are set by no flag registration anywhere.
- **Architectural reason:** authority / truthfulness — OBS-DEBUG-001 requires verbosity controls be effective and safe; OBS-CORE-001's "immediate channel" currently relies solely on ad-hoc stdout writes (e.g., `study_commands.go:504`), none of which honor configured format/level/redaction pipeline (`logging.go:39-43`).
- **Concrete consequence:** Operators setting `logging.format: json` / `level: debug` get identical output to defaults; the one component with a real redacting JSON logger can never be enabled; diagnostics capacity implied by configuration reference does not exist.
- **Counter-evidence searched:** Possibly intentional scaffolding for the daemon-events roadmap (FUTURE-INTENT) — plausible, but the configuration doc presents these as valid current settings, which makes inertness a present-tense truthfulness gap regardless of roadmap.
- **Confidence:** medium-high (inertness certain; intent uncertain)
- **Smallest useful action:** Either wire `logging.New` into serve/TUI command paths honoring `config.Logging`, or mark the section as reserved in docs/configuration.md until implemented.

### Defended architecture / rejected hypotheses

- **Retention/replay design is sound, not a finding.** Per-run bounded journals compacted inline within the append transaction (`retention.go:58-95`), three-tier record states with invariant checks (`model.go:349-354`), tombstone tier deliberately preserving warning/finding/artifact/cancellation/terminal events (`retention.go:185-188`), and honest `replay_gap` client errors with recovery hints (`run_handlers.go:454-460`). I looked for lost-update or replay-cursor bugs and found none.
- **Sanitization is not leaking** — my findings are about over-retention-blocking, not exposure. Credential/path/newline denylisting (`sanitize.go:84-98`) plus final oversize replacement with a truthful warning event (`sanitize.go:46-53`) is solid; the local log reuses the same predicates (`local_log.go:74-84`).
- **Unwired `Notifier` + polling SSE is acceptable.** The interface documents itself as best-effort latency optimization with recovery via `Repository.Events` (`interfaces.go:41-45`); `followRunSSE` adaptively polls at 250ms–1s with heartbeat comments (`run_handlers.go:489-526`). Not wiring it is a missed optimization, not a defect.
- **Process-local unlabeled metrics are deliberate** (`metrics.go:8-9` comment; exposed via `Health`, `sqlite.go:856-858`). Stable codes exist in events/errors for machine use (OBS-ALERT-001); I rejected "metrics are insufficient" as manufacturing criticism.
- **Small duplicated sanitize helpers** (`boundedSafe` vs `safeEventValue` vs `boundedPayloadValue`) — investigated as a standalone duplication finding; rejected because each is ~15 lines, local, and behaviorally equivalent; the real problem is the policy-authority split already captured in F01.
- **Per-process Reconcile every 10s** — potential multi-process SQLite contention, but writes are immediate-tx, busy_timeout=5s, retried upstream; no evidence of failure. Left as open question only.

### Open questions

1. Do live opencode event payloads reliably carry `tool/state/message` at top level (reducing F01's practical impact for tool rows, if not text deltas)? Requires a live capture; static evidence (commit eeaa034's rationale) says nesting motivated the promotion work.
2. Is correlation enrichment (F02) scheduled under the daemon-events study (dimensions 01.06/01.07)? If the envelope will be redesigned soon, the smallest action should be the event-payload variant, not a new repository method.
