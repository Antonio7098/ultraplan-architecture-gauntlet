### Scope inspected

Implementation repo `/home/antonioborgerees/coding/ultraplan/gauntlet-run/ultraplan-go` @ eeaa034 (clean):

- **Run control (owner):** `internal/runcontrol/model.go` (Lifecycle/Liveness/CancellationState/TerminalOutcome vocabularies, `IsValid`/`IsActive`/`IsTerminal`, `TerminalOutcome.Lifecycle()`), `lifecycle.go` (Heartbeat, RequestCancellation, List, Reconcile, markStalled), `sqlite.go` (DDL CHECK constraints :245–269/:287/:296, Accept/Claim/Append/ProposeTerminal, `validActiveTransition` :1028–1044, health query :871, `validateEventDraft` :1005–1026), `interfaces.go`, `doc.go`, `migration.go`
- **App layer:** `internal/app/run_control.go` (controlledRuntime acceptance→claim→events→terminal), `durable_operations.go`, `run_commands.go` (`parseLifecycleFilter`, snapshot rendering), `run_usecases.go`, `operation_runner.go`, `sprint_usecases.go` (product-status summarization)
- **Surfaces:** `internal/web/run_handlers.go` (`runLifecycleView` :312–336, `webLifecycleFilter` :565), `operation_handlers.go` (:176, :409–427, :501–519), `operations.go` (`finish`, `activeOperations`, `terminalOperationState` :614–621), templates `runs.html`, `run.html`, `operation.html`, `components.html`; static `js/app.js` (:271–296, :710, :1377–1414), `js/sse.js`; TUI `views.go` :319–350
- **Product state:** `internal/productstate/store.go` (opaque kind/scope records)
- **Docs:** `architecture.md` §Durable run control, `local-web.md` :126–152, `cli-reference.md` :444–453, `recovery.md` :184–217, `phase3-json-schemas.md`
- **Tests:** `runcontrol/{lifecycle,model,sqlite,fault}_test.go`, `web/operations_contract_test.go`, `web/run_handlers_test.go`, `app/run_control_inventory_test.go`, `tui/run_view_test.go`; `go test ./internal/runcontrol/... ./internal/web/...` green.

Probe measured (change-probes.md #8): add/change an execution lifecycle status.

---

### Architecture assessment

**Sound.** The lifecycle vocabulary has a single authoritative owner: `runcontrol` defines the values, validity, active/terminal classification (`model.go:16–52`), enforces terminal coherence in `Snapshot.Validate` (`model.go:343–356`), gates active-only transitions at Append (`sqlite.go:1009`, `validActiveTransition`), routes all terminal entry through one-arbitration-wins `ProposeTerminal` with a 1:1 `TerminalOutcome↔Lifecycle` identity (`model.go:116–118`, `sqlite.go:771–774`), and freezes the same truth into DDL CHECK constraints plus a table-level terminal invariant (`sqlite.go:245–269`). CLI/TUI filtering validates via `IsValid()` (`run_commands.go:379–392`, `run_handlers.go:565–578`) rather than copied lists, and the run-detail surface renders values generically through the `component/run-state` pill with a cue lookup, degrading gracefully to "Unknown" (verified by the TUI unknown-matrix test). The product-state axis is cleanly separated: `productstate` stores opaque kind/scope records, `ProductStatus` rides snapshots as a length-bounded opaque correlation string (`sqlite.go:402`, `model.go:328`), and product stage/task/review/smoke statuses stay owned by `internal/sprint`/`internal/study` domain enums — adding a *product* status does not touch run control at all, matching `doc.go:3–6` and `architecture.md:156–160`.

**Stressed.** The *operational* vocabulary leaks out of its owner through four hand-copied active-set literals and two hand-copied terminal-set literals in web/template/JS layers, and the web operation compatibility layer models only an 8-state subset of the 11-state producer vocabulary. The schema-version gate that exists precisely to coordinate vocabulary change cannot actually execute a vocabulary-changing migration.

---

### Candidate findings

#### CHANGE-08-F01

- **Priority:** P1
- **Claim:** The durable→web operation projection passes raw `runcontrol.Lifecycle` through to clients, but the operation compatibility surface (docs, contract test, `terminalOperationState`) models only 8 of 11 states. Two *reachable* terminal outcomes — `timed_out` and `persistence_degraded` — escape the documented stable-state set and are misclassified as non-terminal by the web package's own classifier.
- **Evidence:**
  - Producer: `State: string(snapshot.Lifecycle)` with no normalization — `internal/web/operation_handlers.go:415` and `:424`; also seeded from an existing run's lifecycle at `operations.go:194`.
  - Reachability: `FinishOperation` maps `DeadlineExceeded → TerminalTimedOut` (`internal/app/durable_operations.go:244–245`) and start-persistence failures → `persistence_degraded` (`durable_operations.go:111`); same mapping for runtime-backed runs at `internal/app/run_control.go:285, 628–629`.
  - Contract that doesn't hold: `docs/local-web.md:146–147` ("Stable states are `accepted`, `running`, `cancelling`, `succeeded`, `failed`, `cancelled`, `interrupted`, `cleanup_uncertain`") omits `queued`, `timed_out`, `persistence_degraded`; the self-described "producer/consumer contract" test pins exactly that stale list (`internal/web/operations_contract_test.go:83`) including the terminal classification assertion at `:110`.
  - Misclassification: `terminalOperationState` recognizes only 5 terminals (`internal/web/operations.go:614–621`).
  - Contrast proving the asymmetry is drift, not design: the sibling projection `durableOperationEvent` deliberately normalizes unknown inputs into the documented 8-event SSE vocabulary (`operation_handlers.go:501–512`, default→`progress`); the state projection does not normalize.
- **Architectural reason:** boundary / drift / failure-semantics — the web package owns a compatibility envelope but projects an unowned superset through it.
- **Concrete consequence:** Operation times out (user-supplied `Timeout` option, `operation_handlers.go:33/:597`) → durable run reaches `lifecycle=timed_out` → server restarts → `GET /api/v1/operations/{id}` falls back to `durableOperationDocument` and returns a state value that `local-web.md` declares impossible and `TestBrowserLifecycleDocumentContract` never verified. Any consumer written against the documented set (including future in-repo code feeding durable docs into `terminalOperationState`-guarded paths at `operations.go:271/281/326/352/482/511/536`) treats a finished run as ongoing.
- **Counter-evidence searched:** Hub-created records can never hold the extra states today (`operations.go:193–196` returns early without storing; `finish` derives states from a closed switch `:284–292`), so no live hub corruption occurs now; the browser's duplicated terminal list happens to include all 7 (`static/js/app.js:1392`); the HTML operation page redirects durable operations to `/runs` which handles all 11 values generically (`operation_handlers.go:347–352`). These reduce blast radius but do not repair the JSON contract.
- **Confidence:** high
- **Smallest useful action:** Decide the envelope once: either extend the stable set to the full run-control vocabulary in `local-web.md` + `operations_contract_test.go` + `terminalOperationState`, or normalize at `durableOperationDocument` (as `durableOperationEvent` already does). One boundary function, one test-table update, one doc paragraph.

#### CHANGE-08-F02

- **Priority:** P2
- **Claim:** The active-lifecycle set is enumerated as string literals in four places outside `runcontrol`, one copy has already diverged, and the SQL copy structurally defeats the defensive `IsActive()` re-check next to it.
- **Evidence:**
  - `internal/web/operation_handlers.go:176`: `RunQuery{Lifecycle: []app.RunLifecycle{"accepted", "queued", "running", "cancelling"} …}` followed at `:183` by `!snapshot.Lifecycle.IsActive()` — rows excluded by the literal IN-list can never reach the re-check, so the guard provides false confidence.
  - `internal/web/static/app.js:281`: `fetch("/api/v1/runs?lifecycle=accepted,queued,running,cancelling…")`.
  - `internal/web/templates/operation.html:14`: `$active := or (eq .State "accepted") (eq .State "running") (eq .State "cancelling")` — **omits `queued`**, unlike every other copy (existing divergence, harmless only because no writer currently sets `queued`).
  - `internal/web/static/app.js:1392`: full 7-element terminal array duplicated in JS (complete today; hand-maintained).
  - Owner-side duplicates that are acceptable locality but part of the sync burden: health query `sqlite.go:871`, claim mapping `sqlite.go:539–542`.
- **Architectural reason:** drift / change-surface — `Lifecycle.IsActive()` is the classification authority, yet the gating decisions that matter most (what appears as "active work") read copied literals instead.
- **Concrete consequence:** Add an active status (probe: e.g., `paused`/`draining`). Compile passes everywhere; `parseLifecycleFilter`/`webLifecycleFilter` accept it; runs pages render it. But durable operations in the new state silently vanish from `GET /api/v1/operations` navigation (`operation_handlers.go:176`), the dashboard active fetch (`app.js:281`), and the operation page's live panel/cancel affordance (`operation.html:14`) — no test fails, users see running work as absent.
- **Counter-evidence searched:** The SQL IN-list could be justified purely by index usage (`idx_runs_active_updated`, `sqlite.go:333`) — but deriving arguments from one exported slice (e.g., `runcontrol.ActiveLifecycles()`) keeps the index benefit; server-side code already consumes `IsActive()` directly elsewhere (`run_handlers.go:278`, `tui/views.go:343`), proving the pattern works. The `operation.html` omission shows the copies are not being kept aligned by review or tests.
- **Confidence:** high
- **Smallest useful action:** Export the active set once from `runcontrol`, build the SQL args and the runs-page default filter from it, and inject it into templates/JS (or at minimum add a parity test asserting `IsActive()` ⇔ every literal list). Fix `operation.html:14`.

#### CHANGE-08-F03

- **Priority:** P2
- **Claim:** The schema-version gate does not actually gate vocabulary change: lifecycle values are frozen into CHECK constraints, but `migrateSchema` implements only initial creation (v0→v1) — there is no step mechanism for v1→v2 — so the naive additive edit (consts + `IsValid` + CHECK strings, version unchanged) ships mixed-binary semantics that the version-refusal logic exists to prevent, while the honest path (bump version) bricks workspace open until someone hand-writes a table rebuild.
- **Evidence:**
  - Values baked into DDL: `runs.lifecycle` CHECK `sqlite.go:245`; terminal coherence invariant `:268–269`; `terminal_outcome` CHECK `:264`; `attempts.outcome` CHECK `:287` (SQLite cannot ALTER a CHECK; changing it requires table rebuild).
  - Migration machinery is create-only: `CurrentSchemaVersion = 1` (`migration.go:20`), newer-version refusal `:30–31, :55–57`, exact-match schema record `verifySchemaRecord :236–248`, and the only write branch is `version == 0 → createInitialSchema` (`:58–74`). A bump to 2 with no step makes every existing workspace fail open with `unsupported_run_schema` (`app/run_control.go` error path surfaced at `run_handlers.go:537–538`).
  - Mixed-binary semantics when version stays 1: for an unknown lifecycle value `IsValid()==false` ⇒ `IsTerminal()==false` (`model.go:43–52`), so an older binary will (a) poll a finished run forever via `IsTerminal()` SSE exit conditions (`run_handlers.go:509`, `operation_handlers.go:480`), and (b) happily `Claim` a run sitting in a new pre-running active state — `Claim` rejects only `IsTerminal()` (`sqlite.go:533`) and stomps lifecycle to `running` (`:539–542`) — while its write-path `snapshot.Validate()` then rejects what it just wrote (`sqlite.go:460/576/694` call sites), and a reconciler touching such a row propagates a non-terminal-classified error out of `Reconcile` (`lifecycle.go:401–403`) into startup failure (`app/run_control.go:64–68`).
  - Docs advertise migration behavior: "Schema migrations create private timestamped backups next to…" (`docs/recovery.md:217`); backup/lock/prune/restore tooling all exist (`migration.go:92–234, :268–291, :296–352`) but no step runner consumes them for v≥1.
- **Architectural reason:** lifecycle / change-surface — ownership of the vocabulary is correct, but the evolution protocol for the owned artifact is unfinished, so the cheapest-looking edit path is the wrong one and nothing (compiler, test, or version check) says so.
- **Concrete consequence:** First real status addition proceeds as a "small enum change," tests pass (fresh DBs in tests always build the new DDL), and the failure appears later as cross-version weirdness in a long-lived workspace or as an unplanned mid-change project to build the migration framework under pressure.
- **Counter-evidence searched:** Reads degrade softly — `loadSnapshot` does **not** call `Snapshot.Validate` (`sqlite.go:920–983`), so old binaries don't hard-fail merely by listing runs in a new state; single-operator local workspaces make mixed binaries less likely, and the system already refuses forward-compat explicitly, showing coexistence is not a supported goal. `docs/phase3-json-schemas.md:3` freezes enums only for Phase-3 JSON, not the run-control DB. None of this supplies the missing v1→v2 path or forces a version bump on vocabulary edits.
- **Confidence:** medium
- **Smallest useful action:** Two cheap moves, either order: (1) add a parity test asserting the Go vocabulary (`Lifecycle`/`TerminalOutcome` consts) equals the embedded DDL CHECK lists, so any drift fails in CI instead of in a workspace; (2) write down the rule — vocabulary change ⇒ schema bump ⇒ table-rebuild step in `migrateSchema` (the backup/restore scaffolding is already in place) — before the first real status change needs it.

---

### Defended architecture / rejected hypotheses

1. **"Runs-page lifecycle cue switch and the runs.html dropdown should be generated from a registry."** Rejected as premature indirection. The cue mapping (`run_handlers.go:312–323`) is presentation editorializing (Active/Complete/Attention) over a value-generic pill primitive (`components.html:3`), unknown values degrade to a designed "Unknown" fallback covered by the TUI unknown-matrix test (`tui/run_view_test.go:73+`), and the API/filter paths validate via `IsValid()` so a dropped-down-but-valid value remains fully usable. The real gaps are the literal *classification* lists in F01/F02, not this display mapping.
2. **"`ProductStatus` should be a typed enum like `Lifecycle`."** Rejected — it would violate the repository's stated ownership split. Run control deliberately records product status as opaque bounded correlation data (`doc.go:3–6`, `architecture.md:156–160`, bound at `sqlite.go:402`); product vocabularies are typed where they are owned (`sprint/domain.go:47–190`, `study/run_state_domain.go:11+`). Typing product status inside run control would recentralize product semantics and grow run control on every product change — the opposite of the probe's goal.
3. **"`terminalOperationState` is a live routing bug."** Partially rejected: I traced every feed and hub-record states are drawn only from the closed 8-value set (`operations.go:193–196` early return; `:224–225`, `:293`, `:360`, `:539` writers; `finish` switch `:284–292`), so no current code path misroutes. Retained as the latent half of CHANGE-08-F01 because the durable projection feeds the same field and the function sits in shared code.
4. **"Every surface needs exhaustive per-status tests."** Partially rejected: graceful degradation is intentional and tested (`run_view_test.go` unknown matrix); demanding positive coverage everywhere would be ceremony. The targeted fixes are the compatibility table (F01) and set-parity (F02/F03 action 1).
5. **"`queued` is dead vocabulary and should be deleted."** Investigated and left alone: no production writer sets `LifecycleQueued` (repo-wide grep: only transitions `sqlite.go:1034–1035`, filters, and UI options), but reserving a state with full plumbing is consistent with how the vocabulary treats reconciliation-driven futures; deletion would churn DDL/CHECK/docs for zero behavioral gain. Noted as speculative surface cost feeding F02's parity concern.

---

### Open questions

1. Does the operation API intend to **widen** its stable-state contract to the full run-control vocabulary or **narrow** the producer via boundary translation? This decides whether CHANGE-08-F01 resolves by extension or by mapping (`timed_out→failed`, `persistence_degraded→cleanup_uncertain`?) — both are legitimate; silence is what currently breaks the contract.
2. Are **mixed-binary workspaces** (two ultraplan versions sharing `.ultraplan/run-control.db`) a supported operational scenario? If yes, CHANGE-08-F03's cross-version consequences move from P2 toward P1; if explicitly unsupported, a one-line doc statement plus the parity test suffices.
3. Is `queued` awaiting a writer on the roadmap (e.g., deferred/staged starts)? If it is FUTURE-INTENT, surfaces must keep enumerating it (and `operation.html:14` should stop omitting it); if it is vestigial, it should be retired deliberately rather than copied.
