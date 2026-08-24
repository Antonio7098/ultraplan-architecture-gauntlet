The recent commit history shows event evolution happening via payload fields, not new types. Now let me empirically verify two hypotheses about the enum-widening path: (1) what happens when a new `EventType` meets a v1 schema's CHECK constraint, and (2) whether a chatty non-whitelisted event type wedges per-run compaction.
Probe 1 confirmed: v1 CHECK rejects new types. Now let me verify how that constraint error is classified by `classifyStoreError`, then run the compaction-wedge probe.
Both probes confirmed. Let me complete the trace: find all Append call sites (other producers), check the web/TUI DTO layer, and look for any doc/test guarding the enum-vs-policy coupling.
I have a complete trace plus empirical verification. Writing the report.

### Scope inspected

- **Implementation repo** (`ultraplan-go` @ eeaa034): `internal/runcontrol/{sqlite,model,migration,retention,sanitize,local_log,errors,interfaces}.go`, producers `internal/app/{run_control,durable_operations,operation_runner}.go`, surfaces `internal/app/{run_commands,run_usecases,tui_commands}.go`, `internal/web/run_handlers.go`, `internal/web/templates/run.html` (via c455510 diff), `internal/tui/{views,model,app}.go`; recent commits eeaa034/c455510 (`git show --stat`); docs/architecture.md "Durable run control".
- **Planning workspace** (`ultraplan-workspace` @ 368a789): `studies/ultraplan-daemon-events-study/dimensions/*` (esp. 01.06 typed facts/schema evolution).
- **Experiments** (on a `/tmp/opencode` copy; target untouched): two Go probes against the real `SQLiteRepository`.

### Architecture assessment

The event pipeline is well-layered for *content* evolution: closed 12-value `EventType` vocabulary with open `map[string]string` payloads; a single storage sanitization gate; fully generic replay (`Events` sqlite.go:809), SSE (`followRunSSE` run_handlers.go:473), CLI follow (run_commands.go:172), TUI (views.go:337), and web timeline (newRunEventView run_handlers.go:282). Recent history confirms the intended evolution path: c455510 and eeaa034 added rich observability by widening payloads inside existing types, touching only producer mapping + view rendering — good locality.

What is stressed is *vocabulary* evolution. The type set is independently encoded in at least six places with no single authority and no guard test, and the schema has create-only migration machinery (`CurrentSchemaVersion = 1`, `migrateSchema` handles only version 0). Two probes confirmed the forgotten-step failure modes are severe: a schema-misaligned append misclassifies as retryable unavailability, and a retention-unregistered type permanently wedges all further appends to the run at sequence 4097.

### Candidate findings

**ID: CHANGE-04-F01**
- **Priority:** P2
- **Claim:** Widening the event-type vocabulary requires simultaneously authoring the first stepwise schema migration, and there is no test or shared constant keeping the Go enum and the SQL CHECK aligned; the divergence failure mode is misclassified as retryable infrastructure unavailability.
- **Evidence:** Enum duplicated in `model.go:137-146` (`EventType.IsValid`) and `sqlite.go:296` (`CHECK (event_type IN (...))`). Migration machinery is create-only: `migration.go:20` (`CurrentSchemaVersion = 1`), `migration.go:58-74` (only `version == 0` branch); bumping the constant alone makes existing v1 workspaces fail `verifySchemaRecord` (`migration.go:244-246`) with `CodeUnsupportedSchema` on every open. Probe on v1 DB: `INSERT ... 'metrics'` → `constraint failed: CHECK constraint failed: event_type IN (...) (275)`; `classifyStoreError` maps extended code 19 to `unavailable, retryable=true` (`sqlite.go:1085-1108`, verified empirically).
- **Architectural reason:** change-surface | drift | failure-semantics
- **Concrete consequence:** A developer adds `EventMetrics` to `IsValid()` but defers the migration: every append of the new type spins in `appendRunEventWithRetry` (~5s, `app/run_control.go:305-316`) then fails as "durable event persistence failed … unavailable", cancelling the run into `persistence_degraded` (`run_control.go:282-292`) — masking a binary/schema vocabulary mismatch as transient infra trouble. Doing it "right" instead requires inventing the v1→v2 rebuild path (SQLite cannot ALTER a CHECK) before the feature itself.
- **Counter-evidence searched:** Backup/checkpoint/lock scaffolding in `migration.go` shows future steps were anticipated (intent, not neglect); the closed-vocabulary-plus-open-payload design is deliberate and recently exercised (c455510, eeaa034 evolve payloads, not types), so the expensive path is rarely taken — this mitigates frequency, not cost when taken.
- **Confidence:** high
- **Smallest useful action:** Derive both enumerations from one package-level `[]EventType` constant rendered into `initialSchema`, and add a test appending every declared type against a freshly opened v1 DB; separately map SQLITE_CONSTRAINT(19) from the `append_event` insert to non-retryable `CodeInvariant` instead of `unavailable`.

**ID: CHANGE-04-F02**
- **Priority:** P1
- **Claim:** Retention tier membership is three inline SQL literals; any event type not registered defaults to "required forever", and a chatty unregistered (or existing non-disposable) type permanently wedges the run's journal at 4096 events — including today's vocabulary.
- **Evidence:** Removable sets hardcoded at `retention.go:76` (`IN ('progress','message','omission')`), `retention.go:185` and `retention.go:187` (compacted/tombstone tiers); quota bypass set separately at `sqlite.go:713-720` (`reservedEventType`); capacity constants `model.go:427-428`. When compaction can't delete, `Append` fails (`retention.go:86`) and is invoked inside `Append` itself (`sqlite.go:687`). Probe result: 4096 `finding` appends succeed, append #4097 fails `quota_exceeded: required durable event history reached its bounded capacity`, and a subsequent `progress` append **also fails** — permanent, unrecoverable except via terminal proposal.
- **Architectural reason:** change-surface | failure-semantics | drift
- **Concrete consequence:** For the assigned change: add a new event type, forget the three retention lists, emit it at runtime volume → mid-run the journal saturates, every later event (including heartbeats' coalesced progress) is rejected, the producer marks `persistenceErr` and cancels the run into `persistence_degraded`. Same mechanics already reachable today: a long agent session emitting >4096 warnings/errors mapped to `EventWarning` (`run_control.go:382-383`) wedges identically. Nothing in `docs/` documents the tier policy or this fail-closed bound.
- **Counter-evidence searched:** Bounded-capacity fail-closed doctrine ("required durable event history") suggests intent for genuinely required facts; `reservedEventType` shows type-specific policy is a recognized axis. But intent does not explain the scattered encoding or the absence of any exhaustive registration test (`retention_test.go` exercises only progress/warning).
- **Confidence:** high
- **Smallest useful action:** Centralize per-type retention class in one Go table (e.g., `typeRetention map[EventType]Tier`) next to the `EventType` declarations, generate the SQL `IN` lists and `reservedEventType` from it, and add a test that fills a run past `MaxRetainedEventsPerRun` with each declared type asserting either successful compaction or a documented, deliberate refusal.

**ID: CHANGE-04-F03**
- **Priority:** P3
- **Claim:** Two independent producer mappings with duplicated progress-coalescing bookkeeping must both learn a new event type if operations and runtime streams should emit it.
- **Evidence:** `runtimeEventDraft` switch (`app/run_control.go:377-399`) vs `RecordOperationEvent` state→type mapping (`app/durable_operations.go:133-136`); parallel coalescing state machines (`run_control.go:151-205`: key/hash/window bookkeeping; `durable_operations.go:131-175,230`: near-identical `progressKey/omitted` logic). Only the window constant is shared (`ProgressCoalesceWindow`, model.go:13).
- **Architectural reason:** drift | change-surface
- **Concrete consequence:** A new type with coalescing or payload-shape semantics gets inconsistent treatment across CLI/web/TUI operations versus runtime-backed runs; omission accounting diverges silently between the two journals.
- **Counter-evidence searched:** The producers serve different sources with different payload shapes (`runtimepkg.Event` vs `app.OperationEvent`), so full unification risks indirection; duplication is currently small and both funnel through the same `appendRunEventWithRetry`/sanitize gate, bounding the blast radius.
- **Confidence:** medium
- **Smallest useful action:** At minimum, extract the coalescing decision (key construction + window check) into one helper owned beside `ProgressCoalesceWindow`; defer deeper unification until a third producer appears.

### Defended architecture / rejected hypotheses

- **"Surfaces will fan out the edit."** Rejected. `RunUseCases` is pass-through aliases over `runcontrol` types (`run_usecases.go:13-24`); web/TUI/CLI/SSE render `Type` as an opaque string and select payload keys generically (`run_handlers.go:282-303`, `views.go:336-337`, `run_commands.go:172`, SSE raw JSON at `run_handlers.go:494-503`). A new type with no special presentation needs zero surface edits. Strong locality.
- **"The sanitizer allowlist is another scattered enum to update."** Rejected as a defect: `allowedEventPayloadFields` (`sanitize.go:10-17`) is a deliberate single storage gate with explicit omission accounting (`sanitize.go:19-22`); producer pre-filtering in `run_control.go:411-486` is boundary translation, not drift.
- **"Closed vocabulary is over-engineering."** Rejected. Bounded safe-journal doctrine (`docs/architecture.md` "Durable run control"; doc.go ownership statement) plus demonstrated payload-based content evolution show the cheap path covers most needs; the finding is about guarding the rare widening, not replacing the design.
- **"Missing v1→v2 migration means neglect."** Partially rejected: backup/WAL-checkpoint/lock/prune scaffolding anticipates future steps; what's missing is only the first step function plus its guard test — hence F01's small action.

### Open questions

- Is the >4096-required-events fail-closed bound documented as accepted doctrine anywhere authoritative (sprint notes outside `docs/`)? If yes, F02 downgrades to the registration-locality half; the wedge mechanics remain as probed.
- Does `studies/ultraplan-daemon-events-study` dimension 01.06 (typed facts, schema evolution) represent FUTURE-INTENT to widen the vocabulary soon? If scheduled, F01 rises to P1 since the migration machinery becomes a blocking prerequisite rather than contingent debt.
