### Scope inspected

- **Composition/root**: `cmd/ultraplan/main.go`, `internal/app/app.go`, `internal/app/surfaces.go`, `internal/app/tui_commands.go`, `internal/app/serve_commands.go`
- **Shared operation boundary**: `internal/app/operations.go`, `internal/app/usecases.go`, `internal/app/web_usecases.go`, `internal/app/operation_runner.go`, `internal/app/run_usecases.go`, `internal/app/run_control.go`, `internal/app/durable_operations.go`, `internal/app/run_commands.go`, `internal/app/sprint_commands.go`, `internal/app/study_commands.go`, `internal/app/run_control_inventory_test.go`, `internal/app/durable_operations_test.go`
- **TUI**: `internal/tui/app.go`, `model.go`, `verify_test.go`, `model_test.go` (test inventory)
- **Web**: `internal/web/operations.go`, `operation_handlers.go`, `run_handlers.go`, `security.go`, `import_boundary_test.go`, `operations_contract_test.go`, `integration_test.go`, `api_compatibility_test.go`, templates (`sprint.html`, `study.html`, `project.html`), `static/app.js`
- **Product modules touched by surfaces**: `internal/sprint/service.go`, `state.go`, `internal/study/locks.go`, `run_loop.go`, `execution_domain.go`, `internal/runcontrol/sqlite.go`, `interfaces.go`
- **Authoritative docs**: repo `docs/architecture.md`, `docs/cli-reference.md`, `docs/local-web.md`, `docs/plans/integrated-roadmap.md`, `docs/plans/ultraplan-local-server-experiment-plan.md`; workspace `system/contracts/runtime/workflows.md` (WF-IDEMPOTENCY-001), `system/contracts/surfaces/api-contracts.md` (API-IDEMP-001)
- Verified `go build ./...` and `go test ./internal/app ./internal/tui ./internal/web` pass.

### Architecture assessment

The surface composition is sound and deliberately enforced: only `cmd` imports `internal/tui`/`internal/web`; both adapters import only `internal/app` (machine-checked by `internal/web/import_boundary_test.go:12-36`), runners are injected functions (`app.go` Config, `main.go:27-41`), and the prohibited app↔web cycle is avoided. Product semantics (fingerprinting, mutation classes, governed inputs, runtime-backed execution) live in `internal/app`; the web hub correctly owns transport lifecycle (capacity, sessions, SSE, draining). Run observation is genuinely shared through `RunUseCases` (`run_usecases.go:29-35`). Parity of *product semantics* across surfaces is real: CLI, TUI, and browser reach the same `dashboardUseCases.PrepareOperation/RunOperation` and `repositoryRunUseCases`.

What is stressed: the **durable-operation orchestration protocol** (accept → record-events-before-delivery → finish-with-terminal-proposal) is implemented twice more, once per interactive adapter, on top of the shared manager; the **idempotency-key derivation** for that protocol is decided independently by each surface; and the TUI's **observation loop retains write authority** over authoritative product state that the web deliberately gave up (`readOnly`). These are drift-prone seams, not broken behaviour today.

### Candidate findings

---

**SPECIALIST-11A-F01**
- Priority: P2
- Claim: The durable-operation lifecycle protocol is duplicated between the TUI and web adapters, and the two copies have already diverged in failure semantics.
- Evidence: `internal/tui/app.go:232-312` (`beginOperation`, `operationCmd`) and `internal/web/operations.go:150-275` (`startConfirmed`, `run`, `publishAppEvent`) both implement: `AcceptOperation` → `RunOperation` with an emit callback that calls `RecordOperationEvent` and suppresses uncommitted events → `FinishOperation` on a detached 30 s `context.Background()` (`tui/app.go:302-309`; `web/operations.go:234-241`). Divergence: on `RecordOperationEvent` failure the TUI silently drops the event and keeps running (`tui/app.go:288-295`), while the web hub cancels the whole record (`web/operations.go:246-255`). The underlying manager itself fails closed by cancelling the owned context (`app/durable_operations.go:165-171`), so both converge, but only after different observable behaviour.
- Architectural reason: change-surface / drift
- Concrete consequence: any protocol revision (commit gating, coalescing policy, terminal proposal handling) must be replicated in two adapters with no compile-time or test-level coupling forcing them together; the existing event-persist-failure divergence shows the copies are already drifting. A third, parallel copy of the owner loop/coalescing machinery exists for runtime-child runs (`app/run_control.go:175-303` vs `durable_operations.go:137-176,178-219`).
- Counter-evidence searched: `docs/architecture.md:106-125` assigns the hub only transport-lifecycle ownership, and nothing in bubbletea/SSE shapes forces the accept/record/finish sequence to live in adapters — the sequences are structurally identical including the `Existing` short-circuit. No test pins the two implementations to identical semantics.
- Confidence: high
- Smallest useful action: Extract one `Accept → Run(recording events) → Finish` helper beside `DurableOperationManager` in `internal/app` (adapters keep only presentation mapping and their own dedup-key choice).

---

**SPECIALIST-11A-F02**
- Priority: P2
- Claim: Each surface supplies a semantically different idempotency key (`digest`/alias) to the same durable acceptance mechanism, so the cross-surface "matching durable operation already exists" protection the layer was built for can never fire except within one TUI install.
- Evidence: TUI derives a content hash: `sha256(CanonicalRequest + "\x00" + InputFingerprint)` (`internal/tui/app.go:236-238`). Web derives `sha256(session + "\x00" + token)` (`internal/web/security.go:447-450`, used at `operation_handlers.go:144,336`) — unique per prepare cycle. CLI passes `""` (`app/durable_operations.go:47`). The manager turns the digest into a unique-aliased run and resolves conflicts via `ResolveOperationAlias` (`durable_operations.go:88-97`; `runcontrol/sqlite.go:445-454,472-484`); `TestDurableOperationDeduplicatesAcrossManagersAndFailsClosed` (`durable_operations_test.go:65-88`) demonstrates the intended cross-manager dedup. The web already holds the content inputs at start time (`current.CanonicalRequest`, `current.InputFingerprint`, `operation_handlers.go:139-145`) but does not use them for the alias.
- Architectural reason: authority / boundary (idempotency-key derivation delegated to adapters instead of owned once)
- Concrete consequence: starting the same logical operation from two browser sessions (or TUI + browser) accepts and claims two durable runs; the loser dies only later at the product mutation lease, leaving confusing failed-run journal noise. Per-surface stances are implicit: TUI = restart-safe content dedup, web = effectively none beyond single-use tokens plus an in-process map (`web/operations.go:154-159`), CLI = none. Workspace contract WF-IDEMPOTENCY-001 (`system/contracts/runtime/workflows.md:28`) expects the idempotency stance to be explicit.
- Counter-evidence searched: within-session replay is protected (single-use token consumption plus hub dedup map; `web/operations_test.go:375-382` asserts replay returns the same ID), and the product lease bounds damage from duplicates — so harm is bounded noise, not corruption. No doc declares the per-surface key split intentional.
- Confidence: high (facts), medium (harm)
- Smallest useful action: Derive the alias once from the prepared canonical request + input fingerprint in the start path for all interactive surfaces (inputs already present at both call sites), or document the per-surface stance on `DurableOperationManager` (`app/operations.go:40-44`).

---

**SPECIALIST-11A-F03**
- Priority: P2
- Claim: The TUI's 1 Hz run-view polling loop exercises a query path that rewrites authoritative product state (`flow-state.json` for every sprint in the workspace) because the TUI use cases retain write-enabled status refresh, unlike `serve`.
- Evidence: Tick loop `internal/tui/app.go:222-226` → `refreshCmd` → `dashboardAndRuns` (`tui/app.go:357-389`) → `Dashboard` (`internal/app/usecases.go:136-153`) → `SprintSummaries` calling `service.Status(...)` per sprint (`internal/app/sprint_usecases.go:98,109`) → unconditional `SaveFlowState` when `statusWrites` (`internal/sprint/service.go:191-195`), stamping a fresh `UpdatedAt` each write (`internal/sprint/state.go:361-371`; no content-diff skip in `SaveFlowState`, `state.go:201+`). TUI wiring omits `readOnly` (`internal/app/tui_commands.go:37-41`); `serve` sets `readOnly: true` (`serve_commands.go:47-51`), which `architecture.md:69-70` describes as the deliberate "non-persisting projection mode". Side effect: the web's shared `PrepareOperation` still emits `"RUNTIME-FREE; MAY REFRESH FLOW-STATE.JSON"` (`app/operations.go:183-186`) for a web `sprint-status` operation that cannot write.
- Architectural reason: lifecycle / failure-semantics (observation loop mutating authoritative state; write authority expressed as a constructor flag rather than per-operation)
- Concrete consequence: merely viewing a run in the TUI rewrites every sprint's `flow-state.json` once per second — constant file churn, misleading `updated_at` freshness, backup/sync noise — from a surface whose top-level help bills as passive. The same flag entanglement makes the web confirmation text overstate mutation scope.
- Counter-evidence searched: `tuiHelp` (`tui_commands.go:72-84`) documents that explicit Refresh / "Sprint Status" actions recompute `flow-state.json` — user-initiated refresh writing is intended; the automatic poll is not that action. Checked `SaveFlowState`/`Status` for skip-if-unchanged — none exists.
- Confidence: high
- Smallest useful action: Give dashboard-polling queries the non-persisting projection (as `serve` does) while keeping the explicit refresh/status action write-capable — i.e., separate the projection query from the refresh command instead of one `readOnly` flag on the whole use-case set.

---

**SPECIALIST-11A-F04**
- Priority: P3
- Claim: Dashboard/TUI-started study run loops record a fabricated CLI invocation (`ultraplan operation` — not a real command) as the lock-owner attribution, because the study lock format encodes owner identity as CLI argv.
- Evidence: `internal/app/operation_runner.go:115` (`Command: []string{"ultraplan", "operation"}`) vs the CLI's truthful argv (`internal/app/study_commands.go:209`); `Command` flows only into `LockInfo` (`internal/study/run_loop.go:31`, `locks.go:47-53`) and is surfaced verbatim in lock-conflict errors and lock reads (`locks.go:79,83`; `app/status_json.go:91`). `"operation"` is absent from the command switch (`internal/app/app.go:144-177`).
- Architectural reason: boundary (command-as-protocol leakage: a product lock record's vocabulary is a CLI argv, forcing other interfaces to invent fake argv)
- Concrete consequence: an operator who hits `ErrStudyLocked` for a web-started loop is told the lock is held by `ultraplan operation`, a command that cannot be found in `--help`, docs, or process lists — actively misleading recovery.
- Counter-evidence searched: confirmed `Command` is attribution-only (never executed) and `sanitizeCommand` redacts secrets (`locks.go:184-202`); the defect is diagnostic, not behavioural.
- Confidence: high
- Smallest useful action: Pass a truthful descriptor (e.g., `["ultraplan","study",ref,"run-loop","(dashboard)"]`) or evolve `LockInfo` to carry an interface-neutral origin string.

---

**SPECIALIST-11A-F05**
- Priority: P3
- Claim: Top-level help still describes the TUI as "read-only", contradicting its actual mutation authority.
- Evidence: `internal/app/app.go:268` ("tui — Open a read-only terminal dashboard") vs `tui_commands.go:37-52` wiring (runtime-backed flow/execute/review/smoke operations; `readOnly` unset) and `tuiHelp` itself listing mutating operations. HISTORY: commit `7a94a11` "add interactive read-only tui"; operations were added later and the help line was never updated.
- Architectural reason: drift (superseded decision fossilised in the CLI contract surface)
- Concrete consequence: users trusting the main help believe the TUI cannot mutate workspace state (it can, including `flow-state.json` writes and runtime-backed operations).
- Counter-evidence searched: no reading of "read-only" (e.g., "doesn't edit arbitrary files") reconciles it with `tuiHelp`'s own wording; `serve`'s "read-only" line is accurate because `readOnly: true` is actually wired there.
- Confidence: high
- Smallest useful action: Fix the one help string.

---

**SPECIALIST-11A-F06**
- Priority: P3
- Claim: Shared `PrepareOperation` authors web-only HTTP refresh routes (`/api/v1/...`) into `Confirmation.DurableRefreshPath`, putting interface-specific routing vocabulary inside the surface-neutral use-case boundary.
- Evidence: `internal/app/operations.go:241-252` builds the paths; sole consumer is `internal/web/operations.go:203` (the TUI never renders them); compare the boundary's stated intent that capability values "deliberately contain no … surface-specific" types (`app/operations.go:22-23`, `surfaces.go:8-9`).
- Architectural reason: authority (route-shape ownership leaking into the product boundary)
- Concrete consequence: renaming `/api/v1` routes requires editing the product layer; `Confirmation` carries URLs meaningless to the TUI. Low practical cost today since routes are stable and contract-tested (`operations_contract_test.go:82-114`).
- Counter-evidence searched: the field is small, additive, and covered by compatibility tests; relocating it adds a web-side composition step for marginal gain — defensible either way.
- Confidence: medium (leakage factual; impact low)
- Smallest useful action: Either let `internal/web` compose `RefreshPath` from `Confirmation.Paths` + kind, or document the field as a web-compat affordance at the `Confirmation` declaration.

### Defended architecture / rejected hypotheses

1. **"The web hub reimplements workflow authority in the transport layer."** Rejected as a defect. The hub decides only transport lifecycle concerns (capacity, draining, sessions, subscriber/SSE lifecycle, bounded projections — `web/operations.go:18-32,146-219`); preparation, fingerprinting, mutation classification, and execution remain in `internal/app`, and the import-boundary test proves `internal/web` cannot reach product modules. The accept/record/finish steps delegate to the app-provided `DurableOperationManager` rather than reimplementing storage (the duplication problem is captured in F01, not here).
2. **"Per-surface operation menus (TUI `navItems`, HTML forms, JS labels) are harmful duplication."** Rejected. Each surface legitimately owns presentation of the operation catalog over a shared typed vocabulary (`OperationKind`); the catalogs are pinned by `TestBrowserOperationKindContract` (`operations_contract_test.go:22-61`) and `TestSprintNavigationExposesAllSprintOperations` (`tui/model_test.go:67`). A shared menu abstraction would couple presentation structures for no semantic gain.
3. **"CLI bypasses shared use cases by calling `sprint.Service` directly — a parity violation."** Mostly rejected. CLI commands contribute flag parsing, rendering, and exit-class mapping; runtime-backed entries are pinned to the durable boundary by a source-scanning inventory test (`run_control_inventory_test.go:11-54`), and dry-run/read paths hit the same service configuration the operations use. The one genuine asymmetry (interactive surfaces cannot express `--model/--variant` overrides) is an interaction-scope choice, not a semantic divergence.
4. **"Absence of end-to-end CLI/TUI/web parity tests is a current defect."** Rejected as CURRENT-CONTRACT breach; it is FUTURE-INTENT (`docs/plans/integrated-roadmap.md`, `local-server-experiment-plan.md` call for adapter parity suites). Meaningful partial coverage exists today (`integration_test.go`, `api_compatibility_test.go`, contract tests).
5. **"Two owner/control loops (`controlledRuntime.StartRun` vs `durableOperationManager.controlOperation`) are accidental duplication."** Partially rejected: they own distinct durable runs (runtime-child vs operation) so two heartbeat owners must exist. Only the coalescing bookkeeping (~40 lines, `run_control.go:175-205` vs `durable_operations.go:131-164`) is truly parallel logic; noted under F01 rather than as its own defect.

### Open questions

- Is cross-surface duplicate acceptance considered acceptable by design (relying solely on the product mutation lease as the arbiter)? If yes, F02 downgrades to a documentation fix; if no, the alias unification is warranted.
- Does anything depend on the TUI's polled status refresh keeping `flow-state.json` fresh while observing runs (e.g., recovery flows that assume dashboards reconcile state)? An affirmative answer would reshape the F03 remedy toward throttled/explicit writes instead of read-only projection.
- Whether `Confirmation.DurableRefreshPath` is load-bearing for any embedded external consumer beyond the first-party client (would raise F06's cost of relocation).
