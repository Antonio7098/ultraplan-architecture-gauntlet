### Scope inspected

Implementation repo (`ultraplan-go` @ eeaa034):

- Composition root and dispatch: `cmd/ultraplan/main.go`, `internal/app/app.go`
- Surface boundary types: `internal/app/surfaces.go`, `tui_commands.go`, `serve_commands.go`
- Shared use-case layer: `internal/app/usecases.go`, `operations.go`, `operation_runner.go`, `web_usecases.go`, `run_usecases.go`, `durable_operations.go`, `run_control.go`
- Existing local surfaces: `internal/tui` (`app.go`, `model.go`, `doc.go`, `test_fakes_test.go`), `internal/web` (`operations.go`, `operation_handlers.go`, `security.go`, `import_boundary_test.go`, `operations_contract_test.go`)
- Durable substrate: `internal/runcontrol/sqlite.go` (accept/alias path)
- Boundary tests: `app/run_control_inventory_test.go`, `app/tui_commands_test.go`, `app/serve_commands_test.go`

Planning workspace: `system/contracts/**` grep for surface/TUI-parity contracts (`review-sprint-protocol.md:78-80`, `deep-smoke-sprint-protocol.md:55-57`); repo docs `docs/architecture.md`, `docs/local-web.md`, `docs/ui-audit.md`.

Probe traced: change-probe #2 — "Add another local interface."

### Architecture assessment

**The sanctioned extension path is real and narrow.** For a hypothetical third local surface (`ultraplan console`), the required edit surface is:

1. New `internal/<surface>` package importing only `internal/app` (contract in `internal/tui/doc.go:7-9`; enforced for web by `internal/web/import_boundary_test.go:12-36`).
2. `cmd/ultraplan/main.go`: one runner closure in `app.Config` (pattern at main.go:27-41).
3. `internal/app`: option struct + `Runner` func type (pattern: `surfaces.go:10-20`, `tui_commands.go:13-19`), a `Config` field (app.go:36-38), `dependencies` fields (app.go:188-190), one switch case (app.go:169-174), help text (app.go:250-276).
4. `internal/app/<surface>_commands.go`: preflight composing the stack.

Product modules (`sprint`, `study`, `project`, `runcontrol`), platform modules, `internal/tui`, and `internal/web` need **zero edits**. Workflow semantics are genuinely shared today: `PrepareOperation`/`RunOperation` (`operations.go:166-407`) and the runtime-backed runner (`operation_runner.go:15-18`, explicitly documented as the single implementation for terminal and browser adapters). The web's wire-string→kind table is boundary translation, pinned by `operations_contract_test.go:22-61`. Locality verdict: structurally sound.

**Where the probe's "without duplicating workflow semantics" condition is stressed:** the *client side* of the durable-operation protocol — accept with digest → adopt accepted context → gate each event on commit → finish with 30 s deadline — exists once per surface, hand-orchestrated in surface code, with divergent digest policy. A third surface author cannot call this protocol; they can only copy one of two variants that already disagree.

### Candidate findings

---

#### ID: CHANGE-02-F01
**Priority:** P2

**Claim:** The durable-operation client protocol (accept → adopt context → committed-event gating → bounded finish) is re-implemented independently by each interactive surface with divergent idempotency semantics; adding a second new local UI requires copying workflow semantics that `app` already owns the inputs for.

**Evidence:**
- TUI: `internal/tui/app.go:232-269` (`beginOperation`: digest = `sha256(CanonicalRequest + "\x00" + InputFingerprint)` at :236-238, `AcceptOperation`, `Existing` short-circuit, context adoption), `:282-312` (`operationCmd`: per-event `RecordOperationEvent` gating at :287-295; `FinishOperation` with `context.Background()` + 30 s timeout + `errors.Join` at :302-309).
- Web: `internal/web/operations.go:150-219` (`startConfirmed`: hub dedup map then `manager.AcceptOperation(ctx, prepared, dedupKey)`), `:245-255` (same commit gating), `:234-241` (same 30 s finish + join). Digest basis differs: `confirmationDedupKey(session, token)` (`internal/web/security.go:447-450`).
- A third variant already lives app-side for sync CLI: `durableCLICommand` (`internal/app/durable_operations.go:33-76`) — proof the protocol is expressible as an app-owned unit.
- The digest is persisted permanently as a unique alias (`runcontrol/sqlite.go:445-454`; no code ever deletes `operation_aliases`), so the basis choice is semantically load-bearing: TUI's content basis makes an identically-confirmed operation idempotent **forever**; web's session-token basis makes cross-process dedupe effectively unreachable and permits distinct sessions to accept duplicate targets (serialized later only by the product lease).

**Architectural reason:** boundary / lifecycle / change-surface. `DurableOperationManager` was designed as the shared capability ("used by interactive adapters to persist acceptance … before they create an execution goroutine", `operations.go:37-44`), but the abstraction stops at per-call granularity; the orchestration sequence around it drifted into each adapter.

**Concrete consequence:** Surface #3 copies whichever variant its author reads first. If the finish timeout, gating rule, or alias policy changes (e.g., "digest must be content-derived so retries across surfaces dedupe"), three files in two packages must change in lockstep, and nothing fails loudly if one is missed — the surfaces will simply disagree about what constitutes "the same confirmed operation." Cross-surface: a TUI-refused rerun is accepted by `serve`.

**Counter-evidence searched:** Checked whether the manager itself enforces the sequence (it does cancel on event-append failure, `durable_operations.go:165-171`, but does not perform accept/gate/finish ordering); checked `docs/local-web.md:130-136` — it documents web's token-replay contract but nowhere blesses per-surface ownership of the accept/finish protocol; `docs/architecture.md:162-165` says CLI, TUI, and web "share that boundary," which supports centralizing the client sequence rather than refuting it. Checked tests: none pin either digest basis (`grep AcceptOperation` in `internal/tui/*_test.go` → zero hits; web tests cover token store, not alias policy).

**Confidence:** high (duplication facts), medium (that digest-basis divergence is unintentional rather than a deliberate per-surface policy).

**Smallest useful action:** Give `Confirmation` its durable dedup digest at preparation time (inputs already exist at `operations.go:270-277`) and collapse the four-step sequence into one app-owned call (generalize `durableCLICommand` or add `Begin/EmitCommitted/Finish` helpers beside `DurableOperationManager`). Both surfaces shrink; surface #3 gets semantics by construction, not by copying.

---

#### ID: CHANGE-02-F02
**Priority:** P2

**Claim:** The TUI's declared capability type understates what the surface actually requires; the missing capabilities (`RunUseCases`, `DurableOperationManager`) are discovered by unchecked runtime type assertions, and no test or compiler check forces the composition root to supply them. A new local surface cloned from this pattern compiles while silently skipping durable acceptance and durable cancellation.

**Evidence:**
- `TUIRunOptions.UseCases` is `OperationalUseCases` = `ReadOnlyUseCases + WebOperations` only (`tui_commands.go:13-19`; `operations.go:22-27`).
- The implementation additionally asserts `app.RunUseCases` (`internal/tui/app.go:109,123,362`) and `app.DurableOperationManager` (`:235,287,302`); on failed assertion the durable paths are skipped without error (`beginOperation` falls through to plain execution at :256).
- Contrast with the newer surface: `app.WebUseCases` declares everything it needs in the type (`web_usecases.go:58-64` embeds `WebQueries`, `WebOperations`, `RunUseCases`; hub uses direct calls, `operations.go:181,231,234`).
- The trap is exported: `NewOperationalUseCases(root)` (`operations.go:724`) returns a value satisfying the declared TUI interface with nil `runs`/`durable`/`runner`.
- Wiring is manual and unguarded: `runTUI` sets `useCases.runs/.durable/.runner` as three extra steps (`tui_commands.go:46-48`); `TestTUICommandHelpAndRunner` asserts only `opts.UseCases != nil` (`tui_commands_test.go:24-28`); `TestServePreflightAndRunnerOptions` likewise only checks `UseCases != nil` plus `Health` (`serve_commands_test.go:42-48`); the durable inventory test covers only CLI files (`run_control_inventory_test.go:22-35`).

**Architectural reason:** authority / change-surface. Durability-before-execution is stated as universal ("Each start is accepted and claimed in SQLite before a goroutine… Direct CLI commands, TUI actions, web operations… share that boundary", `docs/architecture.md:162-165`), but for interactive surfaces it is enforced by injection convention, not by type or test.

**Concrete consequence:** Adding surface #3 via the documented minimal interface (option struct typed `OperationalUseCases`, or the convenience constructor) yields a working dashboard whose runtime-backed mutations execute with no durable run record — losing observability, cross-surface run visibility, and durable cancellation — with zero compile-time or test-time signal. The product lease still guards the mutation itself, so this degrades auditability and recovery, not safety.

**Counter-evidence searched:** Looked for an intentional optional-capability rationale: the assertion pattern does keep test fakes tiny (`test_fakes_test.go:9-43`) and would permit a pure-read TUI mode; however, every production wiring supplies the full set, and `docs/local-web.md:46-49` describes full capability parity as the contract, not an option. Searched for a test asserting the wired dynamic type satisfies the wider interfaces — none exists.

**Confidence:** high.

**Smallest useful action:** Declare the full interactive capability set in the option type (e.g., `TUIRunOptions.UseCases` becomes an interface embedding `OperationalUseCases + RunUseCases + DurableOperationManager`), update the one fake, and add compile-time `var _` assertions in the preflight files — or route all interactive preflights through one app-owned constructor that returns the complete value. ~30-line diff, removes assertion-based discovery entirely.

---

#### ID: CHANGE-02-F03
**Priority:** P3

**Claim:** Interactive-surface preflight composition is duplicated per command and will grow a third copy; it also embeds an undocumented per-surface policy bit (`readOnly`).

**Evidence:** `runTUI` (`tui_commands.go:29-52`) and `runServe` (`serve_commands.go:36-66`) repeat the same eight-step composition: `discoverWorkspace` → `loadEffectiveConfig` → `dashboardUseCases{root, stageRuntime, reviewConcurrency, smokeSettings}` → `runRepository` → `repositoryRunUseCases` → `newDurableOperationManager` → `sharedOperationRunner`. `runServe` additionally computes `planningStageRuntime(effective.Config)` and `smokeSettings(...)` twice (:48/:50 and :60-62) — visible copy-paste. The only semantic delta is `readOnly: true` (serve, :50) vs unset (TUI), i.e., whether status refresh may write `flow-state.json` (`usecases.go:121-130`); this policy survives only in help prose (`tui_commands.go:78-84`).

**Architectural reason:** change-surface / drift.

**Concrete consequence:** Surface #3's preflight author must re-derive the ingredient list and make an undocumented write-policy decision; omitting `.runs`/`.durable` reproduces CHANGE-02-F02's silent degradation, and choosing `readOnly` wrong changes governed-state write behavior with no failing test.

**Counter-evidence searched:** Considered rejecting as trivially small duplication (~12 lines, in-package, each step a named function). It stays reportable only because two of the steps are the exact ones implicated in F01/F02 and because the `readOnly` bit is a real semantic fork with no owner; a shared `composeInteractiveUseCases(deps, root, effective, readOnly)` is earned, unlike generic abstraction.

**Confidence:** high (facts), medium (that a third surface is ever built).

**Smallest useful action:** One app-private constructor used by both existing preflights; document the `readOnly` fork at its parameter.

---

#### ID: CHANGE-02-F04
**Priority:** P3

**Claim:** Surface-neutral workflow plumbing is filed under one surface's name, misdirecting the next surface's author.

**Evidence:** `sharedOperationRunner` — documented as "used by terminal and browser adapters" (`operation_runner.go:15-18`) — routes all runtime progress mapping through `tuiSprintRuntimeProgress` (`tui_commands.go:58-66`), called five times from the shared runner (`operation_runner.go:23,36,49,60,93`). The symbol is neither TUI-specific nor located where shared code lives.

**Architectural reason:** change-surface (naming-induced locality error only; no behavioral issue).

**Concrete consequence:** An author tracing "what does surface #3 receive on the progress path" starts in the wrong surface's file, reinforcing the belief that this mapper belongs to the terminal adapter.

**Counter-evidence searched:** Verified it carries no TUI-specific state (pure closure over `emit`); verified web consumes the same events via the same runner, so the name is factually wrong, not merely stylistic.

**Confidence:** high.

**Smallest useful action:** Rename to `runtimeProgressMapper` and move next to `sharedOperationRunner`.

### Defended architecture / rejected hypotheses

- **"Web's `mapOperationRequest` kind table and DTO family duplicate workflow semantics."** Rejected. It is one-directional wire translation into app-owned `OperationKind`s, compatibility-pinned (`operations_contract_test.go:22-61`), with scope validation mirroring `validateOperationScope` as transport input filtering. App remains the sole semantics owner.
- **"`WebDashboardResult` vs `DashboardResult` means two read paths."** Rejected. Web projections wrap the same `dashboardUseCases` summaries (`web_usecases.go:416-654`); the extra types are presentation shaping (bounds, HMAC artifact refs, nil-normalization), matching `docs/architecture.md:31-36`'s assignment of the query facade to app.
- **"`readOnly` asymmetry between TUI and serve is drift."** Rejected as intentional: `WithoutStatusWrites` is explicit service configuration (`usecases.go:126-128`), and TUI help documents status refresh recomputation (`tui_commands.go:78-84`). Only its documentation locus is a problem (F03).
- **"TUI drops failed durability events while web cancels — material failure-semantics drift."** Investigated and rejected as material: `RecordOperationEvent` cancels the owned context itself on append failure (`durable_operations.go:165-171`), so both surfaces converge on cancellation; the residual difference (local pane messaging) is presentational. Web's extra `record.cancel()` (`operations.go:248-251`) is redundant, not divergent.
- **"Adding surface #3 requires editing `internal/web` or `internal/tui`."** Rejected. Runner injection plus the import boundary keep both untouched; the probe's expected-vs-actual edit surface matches.

### Open questions

1. Is the TUI's content-derived, retention-immutable alias idempotency (an identically-confirmed completed operation is refused until governed inputs change, steering reruns to `*-resume` kinds) an intended product rule or incidental? The answer decides which digest basis F01's consolidation should standardize on.
2. Should a future non-HTTP local surface share the web hub's capacity/draining model (max 8 active, graceful-drain cleanup markers), or is that transport-lifecycle-only per `docs/architecture.md:62-67`? If the latter, F01's extraction should deliberately exclude it.
