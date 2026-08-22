Dependency graph is a clean DAG. `runcontrol` is a leaf that only `app` consumes; `sprint`/`study` don't import it. Now I'll delegate bounded discovery to four parallel explore agents while I check manifests and generated areas myself.
Three detailed reports returned. Now filling remaining gaps: sprint package detail (largest package, unmapped), platform/runtime internals, generated-code markers, and the authoritative architecture doc.
Now verifying the most consequential claims myself before writing findings.
All key claims independently verified. Composing the scout report.

# SCOUT-01 — Repository Cartography: ultraplan-go

## Scope inspected

**Repo**: `/home/antonio/gauntlet-run/ultraplan-go` @ eeaa034 (clean tree). ~291 Go files, ~64k LOC, 11 packages under `internal/`, single binary `cmd/ultraplan`.

**Direct inspection**: `go.mod` (deps: yaml.v3, agentwrap, bubbletea/glamour/lipgloss/goldmark, modernc.org/sqlite); import DAG via `go list`; `docs/architecture.md` (authoritative, 191 lines, read in full); verification reads of `internal/sprint/execute_target.go`, `internal/sprint/freshness_policy.go`, `internal/app/run_control.go` (:520–625), `internal/app/durable_operations.go` (:82–135), diff of `study/cleanup_uncertain.go` vs `sprint/cleanup_uncertain.go`; targeted greps (generated-code markers, `productstate` in tests, `antonioborgerees` references, git history of `execute_target.go`).

**Delegated bounded discovery** (4 explore agents, evidence-only): entrypoint/composition-root wiring; study/runcontrol/productstate/project/workspace/codeextract; web/tui transport layer; sprint/platform internals. All load-bearing claims re-verified by me where cited below.

### Package map (REALITY)

```text
cmd/ultraplan ──> app, tui, web            (main.go:62 lines; delegates to app.Run)
tui ──> app                                (bubbletea)
web ──> app                                (stdlib net/http; import-boundary test enforced)
app ──> codeextract, platform/{config,runtime}, productstate,
        project, runcontrol, sprint, study, workspace   (composition root, 13.7k LOC)
sprint ──> platform/{config,process,runtime}, productstate, project, workspace  (18.3k)
study ──> platform/runtime, productstate, workspace                             (10.5k)
project ──> workspace                       (read-only FS store)
platform/config, filesystem, logging, process, runtime; productstate;
runcontrol; workspace; codeextract          (leaves; no internal imports)
```

Clean acyclic graph. `runcontrol` enforces its leaf status with an import-boundary test (`runcontrol/import_boundary_test.go:12-39`). Both `web` and `tui` are restricted to `internal/app` by tests/doc contracts (`web/import_boundary_test.go:12-36`). No `init()` functions exist repo-wide; global mutable state limited to test-seam function vars in app.

### Entrypoints & lifecycle

- `cmd/ultraplan/main.go:19-42`: signal ctx, injects `TUIRunner`/`WebRunner` closures (avoids documented app↔web cycle; `app/surfaces.go:18-20`); real dispatch/composition in `app.Run` (`app/app.go:88-178`) over hand-rolled CLI dispatch.
- Durable-run control plane: `runControlState` process singleton (`app/run_control.go:21-93`, opened lazily per workspace, closed via defer at `app.go:109`).
- Background work: two durable control-loop families (heartbeat 5s / lease 15s / tick 1s / reconcile 10s — `runcontrol/model.go:476-484`), web hub drain-with-deadline shutdown (`web/server.go:131-152`, `cleanup_uncertain` escape hatch `operations.go:503-555`), study run-loop workers (`study/run_loop.go:357-371`), review fan-out pool ≤16 (`sprint/review.go:492-538`). Graceful-shutdown paths use detached 30s contexts for terminal proposals (`run_control.go:283-301`).
- Event flow: runtime adapter pump (`platform/runtime/runtime.go:277-300`) → sanitize/persist in `controlledRuntime` (`run_control.go:164-209`) → SQLite events (immutable-by-trigger, `runcontrol/sqlite.go:327-331`) → consumers replay by `(run_id, sequence)`; web adds an in-process bounded SSE hub (256 events/256KiB rings, `web/operations.go:18-32`) with durable-poll fallback (`operation_handlers.go:429-499`); TUI uses channel push + 1s polling; CLI `run follow` polls adaptively.

### Generated areas & manifests

- No `// Code generated` markers anywhere. Hand-written everything incl. config parser (`platform/config/loadFile`, custom line scanner despite yaml.v3 being a declared dep), router matcher (`web/routes.go:276-374`), and vanilla JS assets (`web/static/`, embedded via `go:embed`, no build step).
- `dist/` contains **git-tracked release binaries** (4 GOOS/GOARCH binaries + checksums + smoke-evidence.md referencing another machine's paths).
- Planning workspace `ultraplan-workspace` @368a789 holds sprints/studies/docs (not analyzed in depth; out of scout assignment scope).

### Hotspots (fan-in / fan-out)

| Unit | Evidence |
|---|---|
| `config.Config` | imported by app, sprint, study, platform/{runtime,logging}; ~50 leaf settings, precedence default→workspace→env→CLI (`config.go:142-179`) |
| `workspace.ResolveInside` / marker discovery | consumed by project, sprint, study |
| `productstate.Store` | sole generic state DB (`.ultraplan/run-control.db`, shared file with runcontrol's disjoint tables); consumers: sprint, study, app storage migrate |
| `sprint.Service` construction | ≥4 distinct assembly paths in app: per-call `dashboardUseCases.sprintService()` (`usecases.go:121-130`), `sprintRuntimeService` (`sprint_commands.go:477-499`), inline in `sharedOperationRunner` (`operation_runner.go:75`), direct in `runSprint` (`sprint_commands.go:81`) |
| `study.Service` construction | 5 sites (`study_commands.go:340`, `operation_runner.go:134`, `study_usecases.go:74`, `web_usecases.go:235`, operations validate path) |
| `runtime.StartRun` choke points | sprint: single `startSprintRuntime` (`runtime_metrics.go:115-121`); study: interface seam (`service.go:25-27`); both decorated by `controlledRuntime` |

## Architecture assessment

The module-driven structure is real and largely enforced, not aspirational: charters are stated in per-package `doc.go` files, boundary rules are test-enforced (runcontrol, web), and `docs/architecture.md` matches observed behavior in every spot I cross-checked (loopback-only serve, lease ownership, runcontrol-as-projection charter at :153-160, disabled freshness at :147, hub-as-transport-not-authority at :57-75). Dependency direction is consistently inward: presentation → app → product modules → platform leaves. State truth is clearly partitioned (product state in workspace files + productstate DB; operational identity in runcontrol SQLite; transport state ephemeral in web hub). Lifecycle handling (fencing, terminal arbitration, cleanup-uncertain markers, detached terminalization contexts) is unusually deliberate for a local tool.

Stress concentrates in `internal/app`, the composition root: it hosts two structurally similar durable-run adapters and several scattered per-handler assembly paths, and the sprint execute feature carries a machine-specific constant. Persistence's newest layer (DB-backed product state) is the least-tested part of the state machinery.

## Candidate findings

### SCOUT-01-F01
- **Priority**: P1
- **Claim**: Sprint execute target approval is hard-coded to one developer machine's absolute path in domain validation logic.
- **Evidence**: `internal/sprint/execute_target.go:11` — `const ApprovedExecuteTargetPath = "/home/antonioborgerees/coding/ultraplan/ultraplan-go"`; `ResolveExecuteTarget` rejects any other absolute path (:22-24, finding text "this sprint only approves %s"). Repo currently lives at `/home/antonio/...`. Commit `9545e91` ("fix sprint execute target after repository move") shows the constant was already source-edited once for a repo move. `dist/smoke-evidence.md:5,43-45` corroborates development on the other machine.
- **Architectural reason**: drift / authority — an environment binding lives in the domain-validation layer instead of configuration (`platform/config` exists precisely for such settings, and `config.Config` already flows into every sprint service construction).
- **Concrete consequence**: `sprint execute` fails validation ("unsupported execute target") on every machine except one home directory; each repo/worktree relocation requires a source edit and re-review of safety logic. Safety-critical containment checks (`ValidateExecuteWorkdir`) are entangled with the relocatable constant, raising the cost of legitimate retargeting.
- **Counter-evidence searched**: `docs/cli-reference.md:314` describes execute generically and never documents the approved path as contract; README/user-guide/configuration docs don't mention it; no test pins the constant as intended behavior beyond accepting it; validation wording ("this sprint only approves") suggests deliberate temporary scoping, i.e., possibly intentional debt — but nothing marks it as such in code or docs, and the prior repo-move commit demonstrates the failure mode is active, not hypothetical.
- **Confidence**: high
- **Smallest useful action**: move the approved-target value to `ultraplan.yml` (existing `Planning` config section) or an env override, keeping `ResolveExecuteTarget`'s absolute-path + stat checks unchanged; record the current pin as the shipped default if desired.

### SCOUT-01-F02
- **Priority**: P2
- **Claim**: The DB-authoritative half of product-state persistence has zero test coverage anywhere in the repository, including the migration command.
- **Evidence**: repo-wide `grep -l productstate --include="*_test.go"` returns nothing. Uncovered units: `sprint/state_database.go` (kinds `sprint_flow`/`sprint_execute`, mapping :69-127, migrate helpers :143-148), the authority gates `SaveFlowState`→`FlowStateInDatabase` (`sprint/state.go:216-237`) and `SaveExecuteRunState` (`execute_state.go:105-118`), `study/state_database.go` (kind `study_run`, DB-first read semantics `study/state.go:59-71`), and `app/storage_commands.go` (`storage migrate`, legacy JSON import). By contrast, the JSON-side strict validators are heavily tested (22 sprint test files, 5.3k LOC).
- **Architectural reason**: lifecycle / change-surface — this is crash-recovery-critical dual-write logic (DB authoritative when present, JSON rewritten only at all-terminal checkpoints) whose correctness branch is never executed by tests.
- **Concrete consequence**: a refactor of the header/items mapping or the authority predicate can silently corrupt or strand sprint/execute/study resume state; CI cannot catch it. The checkpoint-vs-live write asymmetry is exactly the kind of invariant that drifts unobserved.
- **Counter-evidence searched**: checked for indirect coverage via app-level integration tests (`storage_commands_test.go` does not exist; `run_control_inventory_test.go` covers only CLI durable-call inventory); checked planning-workspace docs for a declared "phase 3 DB migration still in flight" caveat — `docs/phase3-migration.md` exists but documents the design, not a testing gap rationale. Found none.
- **Confidence**: high (coverage absence), medium (materiality — depends on how often DB mode is exercised in practice)
- **Smallest useful action**: one table-driven test per kind covering save→load round-trip, DB-vs-JSON authority flip, and `storage migrate` idempotency against a fixture workspace.

### SCOUT-01-F03
- **Priority**: P2
- **Claim**: Two structurally parallel durable-run adapters live side-by-side in `internal/app`, each re-implementing the accept→claim→fence→coalesce→tick→terminal protocol over the same `runcontrol.Repository`.
- **Evidence**: `controlledRuntime.StartRun` (`app/run_control.go:122-303`: Accept :126, Claim :136, fence :145, OnEvent wrapper with content-hash progress coalescing :164-205, control loop goroutine :211-261, terminal proposal :282-301) vs `durableOperationManager.AcceptOperation`/`controlOperation` (`app/durable_operations.go:82-135`, :137-176 coalescing, :178-255 loop). Verified shape of both by direct read.
- **Architectural reason**: change-surface — protocol-level changes (event draft schema, coalescing window semantics, fencing/retry behavior like `appendRunEventWithRetry`, cancel-ack handling) have two edit sites in one package with no parity test.
- **Concrete consequence**: e.g., changing the progress-coalescing rule or adding a new payload redaction must be applied twice; divergence produces runs that observe differently depending on whether they were started by a runtime child (`sprint`/`study` runtime ops) or accepted as an app operation (web/TUI/CLI durable commands) — observable as inconsistent `run list/show/follow` histories for equivalent work.
- **Counter-evidence searched**: the two adapters wrap genuinely different subjects (external runtime child processes vs in-process operation closures), so full unification could be forced abstraction; however the shared skeleton (accept/claim/event-append-retry/coalesce/heartbeat-tick/terminal-propose) is already parameterizable over an "emit draft" callback, and both already share helpers (`appendRunEventWithRetry`, `proposeRunTerminalWithRetry`, `run_control.go:305-328`), showing partial convergence is the existing style. Also confirmed `run_control_inventory_test.go` enforces call-site coverage of CLI commands, not adapter behavioral parity.
- **Confidence**: medium-high
- **Smallest useful action**: extract the shared control-loop skeleton (tick/heartbeat/cancel-ack/terminal) behind the existing helper functions, or add a parity test asserting both adapters produce equivalent event sequences for a scripted scenario.

### SCOUT-01-F04
- **Priority**: P3
- **Claim**: `jsonMarshalTruncated` neither marshals JSON nor truncates; durable event payloads with structured values persist as Go `%v` renderings.
- **Evidence**: `app/run_control.go:570-575` returns `fmt.Sprintf("%v", v), nil` unconditionally (comment even claims "marshal then truncate"); sole caller `payloadValueString` map/slice branch :558-563 feeds `runtimeEventDraft` payloads persisted to the immutable `events` table and replayed to `run show/follow` (NDJSON) and web SSE projections.
- **Architectural reason**: failure-semantics / drift — name and doc promise a durability-boundary guarantee (bounded JSON) that the implementation doesn't provide; string values elsewhere are capped via `boundedPayloadValue` (`MaxSafeValueBytes`), map/slice values are uncapped after formatting.
- **Concrete consequence**: NDJSON consumers parsing `payload_*` fields receive `map[a:b c:d]` strings instead of JSON; oversized structured values bypass the 16 KiB payload bound stated in `docs/architecture.md:180`.
- **Counter-evidence searched**: checked whether any consumer parses these payloads as JSON — web `durableOperationEvent` and TUI render them as display strings only, so impact is display/log-fidelity, not corruption; checked whether `%v` ordering is deterministic (Go prints maps sorted, so coalescing hash `payloadHash` remains stable). Real but low-severity today.
- **Confidence**: high
- **Smallest useful action**: implement the function as written (compact `json.Marshal` + byte cap) or rename to reflect reality; one test on `payloadValueString`.

### SCOUT-01-F05
- **Priority**: P3
- **Claim**: The platform/runtime translation boundary leaks external `agentwrap` types into product modules through the `Validation` field and direct imports.
- **Evidence**: `platform/runtime/runtime.go:38` (`Request.Validation *agentwrap.ValidationSpec`, passed through untranslated :534); direct agentwrap imports outside platform: `sprint/runtime_validation.go:8`, `sprint/review_runtime_validation.go:11`, `study/runtime_validation.go:8` (repair loops built on `RepairConfig`, `SessionActionContinue/Fresh`).
- **Architectural reason**: boundary — `platform/runtime` otherwise maintains a neutral vocabulary (own Event/Result/Usage/Policy types, sanitization bounds), so validation/repair is the one axis where the shim is pass-through.
- **Concrete consequence**: an agentwrap API change touches four packages instead of one; the "Runtime interface is product-neutral" property (`interfaces.go:8-65` in runcontrol is fully neutral by contrast) is weakened for the repair-loop feature.
- **Counter-evidence searched**: agentwrap is a same-author SDK explicitly designed to expose these contracts ("supervising agentic coding runtimes from product workflows"); duplicating `ValidationSpec` in platform/runtime would be pure translation with one consumer shape per package — plausibly earned-but-deferred abstraction rather than accident. No doc declares the leak intentional.
- **Confidence**: medium
- **Smallest useful action**: none required now; if agentwrap versions churn, own the `ValidationSpec`/`RepairConfig` shapes in `platform/runtime` first.

### SCOUT-01-F06
- **Priority**: P3
- **Claim**: Frontend-facing use-case assembly is scattered across command handlers rather than one composition function, producing capability sets that differ per entry surface.
- **Evidence**: `runTUI` builds a mutable `dashboardUseCases` then attaches `runs`/`durable`/`runner` field-by-field (`app/tui_commands.go:37-48`); `runServe` builds a second read-only `dashboardUseCases` plus a separately constructed `NewWebUseCases` (`serve_commands.go:47-66`); optional capabilities are reached downstream by type assertion (`handlers.go:440-441`, `operations.go:181`, `tui/app.go:235,287,302`); per-call service reconstruction noted above (`usecases.go:121-130`) with repeated config loads (`sprint_commands.go:478`, `run_commands.go:50`).
- **Architectural reason**: change-surface / lifecycle — capability availability is implicit (nil-check + type assertion) instead of explicit per-surface grants; adding a use-case means touching each assembler.
- **Concrete consequence**: TUI and serve can silently diverge in which operations are available (e.g., a capability attached in one assembler but not the other degrades to assertion-failure paths); unused-in-production constructors (`NewReadOnlyUseCases` `usecases.go:132`, `NewOperationalUseCases` `operations.go:724`, referenced only from tests) confirm the assembly matrix already exceeds production needs.
- **Counter-evidence searched**: `docs/architecture.md:6-27` explicitly prescribes injected-runner composition and preflight-before-init, which this satisfies; differing TUI vs serve graphs may be deliberate (serve adds HMAC/refs machinery). The scatter itself is not contradicted by docs.
- **Confidence**: medium
- **Smallest useful action**: consolidate per-surface assembly into two named constructors (`assembleTUIUseCases`, `assembleServeUseCases`) in one file; no interface changes.

## Defended architecture / rejected hypotheses

1. **"runcontrol duplicates sprint/study workflow state."** Rejected. Its enums describe operation ownership/liveness, not work-item status; it calls nothing back into product packages; the projection-only charter is explicit (`docs/architecture.md:153-160`, `runcontrol/interfaces.go` "Product workflow state and runtime supervision are intentionally absent") and matches observed code (locking stays in product packages: `sprint/verification_lock.go:26`, `study/locks.go:25-27`).
2. **"Disabled freshness switches are latent dead code / accidental."** Rejected as defect. `sprint/freshness_policy.go:3-14` documents attribution-based rationale; `docs/architecture.md:147` states "Exact-match dependency freshness remains disabled" as current contract; non-freshness checks (existence, digest, allowlist) remain enforced per the same comment. Intentional debt, properly recorded. Residual note: some tests exercise branches behind dead switches (e.g., `smoke_test.go:130`), so their greenness proves little while the constants are false.
3. **"Sprint↔study shared idioms should be extracted into common infra."** Rejected as an architecture change. The near-twin pairs (`cleanup_uncertain.go` field-identical structs and predicates; interrupted-run reconciliation skeletons; 4× atomic-write implementations; ref-resolver trio) carry deliberately divergent comments stating different lock/charters (`study/cleanup_uncertain.go:28-30` vs `sprint/cleanup_uncertain.go:28-30`), separate lock namespaces, and disjoint persistence scopes per `docs/architecture.md:112-113`. Marker filenames have already diverged cosmetically (`cleanup-uncertain.json` vs `.cleanup-uncertain.json`) without harm. Extraction would create a cross-module coupling the doc charters forbid; the duplication is small, stable infrastructure idiom. Watch-item only: recovery-protocol fixes must be applied twice.
4. **"Web and TUI duplicate query/view-model logic wholesale."** Partially rejected. Read models are produced once in app (`WebQueries`, `DashboardResult`); what's duplicated is projection/formatting (field enumerations in `operations.go:260-268` vs `views.go:134-160`, operation-kind vocabularies in three places, lifecycle cue mapping in three places) and the durable-operation protocol wrapper (covered by SCOUT-01-F03). Given the doctrine against forced abstractions across transports, only the protocol-wrapper duplication rises to a finding.
5. **"Tracked `dist/` binaries indicate hygiene problems."** Unclassified — consistent with `docs/release-checklist.md` release-evidence practice (`dist/smoke-evidence.md` is a curated artifact); flagged as open question rather than defect.

## Open questions

1. Is `ApprovedExecuteTargetPath` slated to become config-driven in an upcoming sprint in the planning workspace (would downgrade F01 to transitional)? A sprint search for "target implementation directory" / execute-target work items would settle it.
2. Is committing release binaries to git in `dist/` the documented distribution channel (release-checklist), or vestigial from before an installer script existed (`scripts/install-ultraplan.sh`)?
3. Does any operational consumer parse durable-event `payload_*` values as JSON (external tooling, dashboards)? If yes, SCOUT-01-F04 rises above P3.
4. `platform/filesystem` (doc-only placeholder, zero code/importers) and `platform/logging` (implemented, zero importers): reserved seams awaiting a planned consumer, or abandoned scaffolding? Planning-workspace sprints would clarify.
