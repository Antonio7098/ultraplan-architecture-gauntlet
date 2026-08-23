Dependency graph is clean and acyclic, matching docs/architecture.md:1-19. Now let me read the core wiring files myself while dispatching bounded discovery subagents in parallel.
Core wiring looks clean so far. Now dispatching five bounded discovery subagents in parallel.
Five evidence reports returned with convergent findings. Now I'll re-derive the top hypotheses myself before promoting anything to a candidate finding.
run_control.go confirms the second supervision engine (Accept/Claim/fence/coalescing/control-goroutine/terminal at run_control.go:122-303) plus `jsonMarshalTruncated` returning `fmt.Sprintf("%v")` (run_control.go:570-575). Now verifying the smoke parity break and reading the duplicate engine plus authoritative contracts.
Shared-DB hypothesis: downgraded to a hygiene/doc finding with pragma-conflict sub-claim refuted. The smoke-parity challenger returned empty, so I'll falsify that one myself.
All hypotheses verified or falsified first-hand. Composing the final report.

---

# UltraPlan Architecture Gauntlet — Generalist Review (generalist-01)

## Scope inspected

**Implementation repo** (`ultraplan-go` @ eeaa034, clean): all 16 internal packages inventoried (LOC + import graph via `go list -deps/-f`; graph verified acyclic and equal to docs/architecture.md:11-19). Read in full or in relevant part: `internal/app/{app,operations,operation_runner,durable_operations,run_control,web_usecases,sprint_commands,serve_commands,tui_commands,usecases}.go`, `internal/productstate/store.go`, `internal/runcontrol/{sqlite,migration,retention,lifecycle,model,sanitize}.go` (targeted), `internal/sprint/{smoke,smoke_author,locks,verification_lock,state,execute_state,service}.go` (targeted), `internal/study/{locks,run_all,run,run.go metadata keys}`, `internal/platform/runtime/policy.go`, `internal/project/discovery.go`, `internal/web/{operations,handlers}.go` (targeted). Tests grepped for coverage claims (`run_control_inventory_test.go`, `operations_contract_test.go`, `retention_test.go`, `migration_test.go`, `lifecycle_test.go`). Git history queried (`git log -S/--follow`) for intent on `operation_runner.go`.

**Authoritative workspace**: `system/contracts/runtime/persistence-and-migrations.md` (PERSIST-SCHEMA-001), `docs/architecture.md`, `docs/migration-product-state.md`, `docs/recovery.md`, `docs/configuration.md` (quota section).

**Delegation**: five parallel discovery subagents (state-ownership scout; app-cohesion worker; sprint/study/runtime-seams worker; web/TUI-boundary inspector; runcontrol-lifecycle worker) plus two adversarial challengers on the two highest-stakes hypotheses. Every promoted finding below was re-derived from primary source by me; challenger verdicts incorporated verbatim where they changed severity.

---

## Architecture assessment

**Sound.** The module-driven composition is real, not aspirational: the internal dependency graph is acyclic exactly as documented; `runcontrol` and `codeextract` are true leaves (enforced by AST import-boundary tests); `web`/`tui` import only `internal/app` (web enforced by `import_boundary_test.go:12-36`); products accept injected narrow runtime interfaces. The run-control engine itself is exceptionally disciplined: accept-before-goroutine fail-closed starts, fenced CAS mutations, immutable terminal arbitration, sanitize-before-commit events, and retention that cannot touch active runs — all verified against code *and* multi-process tests. The web hub is genuinely transport-lifecycle state (memory-bounded, no fs capability, durable fallback on restart).

**Stressed.** Three systemic patterns emerged:

1. **The app layer is becoming a second implementation home for cross-cutting contracts.** Supervision (two engines), reference validation (three rule sets), error→exit-class taxonomy (string matching where typed errors exist), and model-resolution fallback chains are each defined more than once across app/module/platform boundaries, with observable divergence already present. This is drift pressure on the cleanest part of the design.
2. **Lifecycle-healing authority is assigned to a transport surface.** Product-state interruption reconciliation is reachable only from the web serve path, while run-control rows are healed at process startup — two liveness regimes that disagree for CLI users.
3. **The study module's concurrency regime is materially weaker than sprint's** (PID-alone signalling, unleased mutation paths), and the platform layer carries a dead consumer vocabulary — small, but they sit exactly at the boundaries the architecture doc promises are disciplined.

---

## Candidate findings

### GENERALIST-01-F01
- **Priority:** P1
- **Claim:** Sprint smoke **start** offered on web and TUI can never succeed: `sharedOperationRunner`'s `OperationSmokeStart` branch builds the sprint service without a runtime, while non-dry-run smoke always authors coverage and authoring fails closed on nil runtime. The CLI attaches the controlled runtime for the identical operation.
- **Evidence:** `internal/app/operation_runner.go:74-75` (`case OperationSmokeStart: service := sprint.NewService(root.Path).WithSmokeSettings(...)` — no `WithRuntime`), versus sibling branches at `:23, :36, :49, :60, :93` using `sprintRuntimeService` (controlled runtime). Non-dry-run smoke unconditionally authors: `internal/sprint/smoke.go:60-64`; nil-runtime failure: `internal/sprint/smoke_author.go:21-23`. A running smoke attempt is persisted **before** authoring (`smoke.go:30`), so every web/TUI attempt durably records a failed attempt. Web/TUI advertise the op as runtime+mutating: `operations.go:210-222`. CLI contrast: `sprint_commands.go:431-445` attaches `sprintRuntimeService` when `!req.DryRun`. History: branch introduced in a221683 and never gained `WithRuntime` (`git log -S "WithRuntime" -- operation_runner.go` empty). No test exercises a successful web/TUI smoke start (`operations_contract_test.go:46` maps the kind only).
- **Architectural reason:** boundary / change-surface — one operation kind, two divergent constructions of the product service; the shared-runner abstraction that exists precisely to prevent surface drift leaks a per-surface special case.
- **Concrete consequence:** A web or TUI user confirms "EXTERNAL HARNESS + SMOKE ARTIFACT WRITE", waits for an agent authoring phase that never starts, gets told to "configure the smoke model/runtime" although configuration is fine and CLI works, and their flow state now holds a failed smoke attempt requiring review-gate recovery. Smoke authoring on interactive surfaces also permanently escapes run-control tracking if the nil check were ever relaxed.
- **Counter-evidence searched:** Searched for a dry-run-only design intent for web/TUI smoke (PrepareOperation forces `DryRun:true` only during prepare, not start); docs claiming interactive smoke is unsupported (none found); tests asserting the `smoke_author_runtime` error on this path (none); alternative web route to full smoke (none — kind maps straight to the shared runner). Challenger subagent returned empty; falsification performed directly by me (git history, call graph, test grep above).
- **Confidence:** high
- **Smallest useful action:** In `operation_runner.go:75`, construct the smoke service through `sprintRuntimeService(deps, root, ...)` like the sibling branches; add one integration test asserting `OperationSmokeStart` via the shared runner reaches authoring with a non-nil controlled runtime.

### GENERALIST-01-F02
- **Priority:** P2
- **Claim:** `internal/app` contains two parallel run-control supervision engines with already-diverged semantics: `controlledRuntime.StartRun` (per-runtime-child runs) and `durableOperationManager` (per-operation runs). Accept/Claim pipelines, heartbeat/cancel/reconcile loops, progress coalescers, and terminal deciders are implemented twice, and the copies disagree.
- **Evidence:** Engines: `run_control.go:122-303` vs `durable_operations.go:82-255`. Coalescing keys: content-hash `payloadHash(draft.Payload)` (`run_control.go:177-178, :590-612`) vs field-concatenation without message content (`durable_operations.go:137`); omission policy differs (`durable_operations.go:154-156` drops presentation messages; runtime engine persists message-like payload keys via `runtimeEventDraft` `run_control.go:400-498`). Terminal deciders: `terminalOutcome` (`run_control.go:625-637`) vs `FinishOperation` switch (`durable_operations.go:241-252`) — only the latter knows `OperationPartial→TerminalInterrupted`. Additional fidelity drift inside the runtime engine: `jsonMarshalTruncated` returns `fmt.Sprintf("%v")`, storing Go-syntax strings where JSON is implied (`run_control.go:570-575`).
- **Architectural reason:** change-surface / drift — record *nesting* (one op + N child runs) is documented intent (architecture.md:163-165, "individual runtime children share that boundary"), but supervision policy (tick/heartbeat/cancel-poll/coalesce/terminal-classify) is one concept owned twice.
- **Concrete consequence:** Any policy change (coalesce window semantics, retry-on-transient poll errors — currently absent from both, `run_control.go:228/:244` vs `durable_operations.go:190-193`, while appends retry 5 s) must be replicated across engines or silently applies to only one record class; terminal classification of the same failure already differs by layer, skewing `run inspect`/support-export correlations.
- **Counter-evidence searched:** Looked for a shared supervisor abstraction behind both (none — only `appendRunEventWithRetry`/`proposeRunTerminalWithRetry` are shared); checked whether different event sources force different implementations (they justify different *payload shaping*, not duplicated control loops); confirmed nesting itself is documented intent and therefore not charged here.
- **Confidence:** high (facts), medium (severity)
- **Smallest useful action:** Extract one fence-scoped supervisor (tick/heartbeat/cancel/reconcile loop + terminal proposal) parameterized by an event-shaping callback; migrate `durableOperationManager.controlOperation` onto it first.

### GENERALIST-01-F03
- **Priority:** P2
- **Claim:** Healing authority for interrupted product state belongs de facto to the web surface: `ReconcileInterruptedMutation`/`ReconcileInterruptedRun` have exactly one production caller each — the serve path — while run-control rows are reconciled at every repository open.
- **Evidence:** Healers: `internal/sprint/locks.go:25-88` (running execute tasks → failed; expired flow attempts), `internal/study/cleanup_uncertain.go:66`. Sole production callers: `web_usecases.go:364` and `:377` (grep over non-test tree). Startup reconcile covers run-control rows only: `run_control.go:64`. CLI heal is deferred to resume-time conversion (`execute.go:176-199, :600-618`); `Service.Status` reads state without healing (`service.go:130`).
- **Architectural reason:** lifecycle / ownership — "who converts crashed-owner state to interrupted evidence" is workflow authority placed in a transport adapter's periodic loop rather than the composition root.
- **Concrete consequence:** After any crash of a CLI session, `sprint … status` keeps reporting running tasks indefinitely (until a resume or someone starts `serve`), while `run … list` already shows the run interrupted — the two ledgers the architecture doc promises stay correlated (architecture.md:158-160) visibly disagree for CLI-only workflows.
- **Counter-evidence searched:** Checked for CLI/startup callers of the healers (none beyond web); checked whether status intentionally projects raw state (no doc found assigning web this duty); considered that resume-time healing makes it self-correcting — true, but only on the next mutation, and status remains misleading until then.
- **Confidence:** high (facts), medium (consequence)
- **Smallest useful action:** Call `ReconcileInterruptedMutation`/`ReconcileInterruptedRun` opportunistically where `runControlState.repository` already performs startup reconcile (same composition-root site), keeping the web loop as-is.

### GENERALIST-01-F04
- **Priority:** P2
- **Claim:** Study cancellation signals a bare lockfile PID with `SIGINT`, gated only by `kill(pid,0)` liveness — the exact pattern the system's own durable-run doctrine forbids ("never signals a PID based on PID alone"), and the only PID-signalling path in the product.
- **Evidence:** `internal/study/locks.go:141-158` (`CancelRunLoop`: liveness `processAlive` = `syscall.Kill(pid,0)` at `:17-23`; signal at `:155`); reached from web/TUI via `operation_runner.go:133-143`. Contrast: architecture.md:174-175; runcontrol uses `/proc/<pid>/stat` birth-token identity (`process_linux.go:16-45`) and signals nothing. Stale-lock stealing is likewise PID-recycle-sensitive (`locks.go:66-70`); sprint's analog probes but never signals (`verification_lock.go:95-101`).
- **Architectural reason:** failure-semantics / boundary — process-identity authority is defined once (birth tokens) and bypassed by the one code path that sends signals.
- **Concrete consequence:** If a run-loop owner dies and its PID is recycled by an unrelated local process, `study cancel` (CLI, TUI, web) delivers SIGINT to that process; conversely a recycled PID makes a dead lock look live, blocking new loops until `--force-unlock`.
- **Counter-evidence searched:** Checked for command-line/lock-content validation narrowing the hazard (only `info.Study` name match and self-PID guard, `:149-154` — neither binds PID→process identity); checked whether runcontrol cancellation is meant to replace this channel (both channels coexist today, see also F02/F05 context); no doc accepts the PID-reuse risk.
- **Confidence:** high (mechanism), medium (probability)
- **Smallest useful action:** Record the owner's birth-token (or `/proc` starttime) in `LockInfo` and verify it before signalling; fall back to "inspect manually" guidance when identity cannot be confirmed.

### GENERALIST-01-F05
- **Priority:** P3
- **Claim:** `study.RunAll` (and single analysis/synthesis runs) mutate study outputs with **neither** the run-loop lock nor run-state persistence, while each underlying runtime child is durably tracked in run-control — operational tracking asserts activity the product module deliberately does not own, and concurrent execution with an active run-loop is unmediated.
- **Evidence:** `internal/study/run_all.go:16-40`: no `AcquireRunLoopLock`, no `SaveRunState`; terminates in `WriteSummary` (`:38-40`). Lock acquired only for `RunLoop` (`run_loop.go:31`) and cleanup reconcile (`cleanup_uncertain.go:74`). Both paths are exposed from CLI (`study_commands.go:544` vs `:201`) and produce runcontrol child records either way via the controlled runtime.
- **Architectural reason:** ownership / lifecycle — sprint guarantees exactly one mutator per sprint (`locks.go` two-tier lease); study's guarantee covers only the run-loop entry point.
- **Concrete consequence:** `study run-all` during an active run-loop interleaves summary/report writes with the loop's history sync (last-writer-wins on `summary.csv`/history), and `run inspect` shows live child runs for work the study's own state machine cannot see or resume.
- **Counter-evidence searched:** Checked whether RunAll shares any lock indirectly via runtime wrapper (it does not — the wrapper owns leases, not product locks); checked for atomicity making interleaving safe (`WriteSummary` is atomic-rename, but ordering/conflict between writers is still unowned); no doc declares run-all intentionally lock-free.
- **Confidence:** high (facts), low-medium (real-world frequency)
- **Smallest useful action:** Either take the run-loop lock in `RunAll`/analysis/synthesis entries (preferred, matches sprint precedent) or document run-all as a single-user, non-concurrent affordance at the command layer.

### GENERALIST-01-F06
- **Priority:** P3
- **Claim:** User-facing contract drift on surface capability: top-level help labels `tui` and `serve` "read-only" dashboards while both execute confirmed runtime/mutating operations, and the same status operation persists flow-state refreshes from the TUI but not from web.
- **Evidence:** `app.go:268-269` ("read-only terminal dashboard" / "read-only local browser dashboard"); accurate description in `tui/doc.go:11-13` and `tui_commands.go:78-83`; mutating menus `tui/model.go:454-500`; persistence asymmetry: TUI composes `dashboardUseCases` without `readOnly` (`tui_commands.go:36-41`), web sets `readOnly: true` (`serve_commands.go:47-51`) → `WithoutStatusWrites()` (`usecases.go:126-128`).
- **Architectural reason:** drift — shipped help text contradicts the current contract; capability flags differ per surface for the same operation kind without documentation.
- **Concrete consequence:** Users (and security reviewers) reasonably infer the dashboards cannot start agent runs; TUI status views quietly rewrite `flow-state.json` while the identical web operation is advertised as non-persisting.
- **Counter-evidence searched:** Checked whether "read-only" means artifact-editing only (plausible reading, but `tui/doc.go` explicitly claims mutation capability, so the help text is still wrong); confirmed web really can start operations (closed capability, `operations.go` kinds).
- **Confidence:** high
- **Smallest useful action:** Update help strings to "operational dashboard (mutations require confirmation)" and note the TUI/Web status-persistence difference in one sentence next to `dashboardUseCases`.

### GENERALIST-01-F07
- **Priority:** P3
- **Claim:** Reference-name validation authority is fragmented into three divergent rule sets: the app allowlist accepts leading/inner dots, the project module rejects all dots, and the web regex rejects leading dots only; CLI positional args are gated by none of them at parse time.
- **Evidence:** `operations.go:426-442` (`validateOperationScope`: charset includes `.`, rejects only exactly `"."`/`".."`); `project/discovery.go:75-85` (`IsSafeName`: no `.` anywhere, no leading dot); `web/handlers.go:20` (`identifierPattern ^[A-Za-z0-9][A-Za-z0-9._-]*$`).
- **Architectural reason:** boundary / drift — one cross-boundary contract (what is a legal project/sprint/study ref) owned three times with different verdicts.
- **Concrete consequence:** The same ref string fails at different layers depending on surface (e.g. `a.b` sails through app+web validation, then dies in module resolution; `.hidden` is accepted by app but rejected by web before ever reaching the module) — error quality and behavior differ per surface for identical input.
- **Counter-evidence searched:** Checked whether modules accept a superset somewhere that makes app's looser rule correct (no — `ResolveSprint`/`study resolve` delegate to `IsSafeName`-style checks); no test pins the divergence as intentional.
- **Confidence:** high
- **Smallest useful action:** Make app's `validateOperationScope` delegate to (or literally reuse) the module name predicates so there is one definition, and let web keep only transport-shaped checks.

### GENERALIST-01-F08
- **Priority:** P3
- **Claim:** Exit-code and HTTP-class decisions are made by substring-matching error message text even though the modules expose typed/classified errors and app owns a structured `OperationError` taxonomy.
- **Evidence:** `sprint_commands.go:259` (`strings.Contains(err.Error(), "runtime")` → ExitRuntime), `:350` ("failed tasks" → ExitPartial), `:353` ("runtime"); typed alternatives used correctly elsewhere (`mapSprintError` `:656-675`, `sprint.AsSmokeError` at `operations.go:627`); web repeats the pattern for HTTP statuses (`operation_handlers.go:766-771, :814-817`, keyword heuristics).
- **Architectural reason:** drift / boundary — error *classification* authority belongs to the producer of the error; consumers re-derive it from prose.
- **Concrete consequence:** Any wording change inside sprint error messages silently changes CLI exit codes and web status classes (script/CI breakage with no compile-time signal).
- **Counter-evidence searched:** Confirmed typed errors cover the needed cases (so this is not missing capability); looked for tests locking the substrings to messages (none found — the coupling is unpinned).
- **Confidence:** high
- **Smallest useful action:** Route the three sprint subcommand classifications through `mapSprintError`/`AsSmokeError` categories (already exported), deleting the substring checks.

### GENERALIST-01-F09
- **Priority:** P3
- **Claim:** `internal/platform/runtime/policy.go` exports a study-metadata vocabulary that is dead code whose key spellings do not match what study actually sends — a fossil contract in the platform layer.
- **Evidence:** `policy.go:3-14`: `MetadataStudy`, `MetadataDimension`, `MetadataSource`, `MetadataSourceKind`, `MetadataOutputPath`, etc.; repo-wide production references: **zero** (verified by symbol grep). Actual keys emitted by study: `"dimension.ref"`, `"output.path"` (`study/run.go:247-248`).
- **Architectural reason:** drift — the platform layer encodes a consumer contract that never existed in its declared form; future readers will bind to the constants and mismatch real traffic.
- **Concrete consequence:** Someone "fixing" study to use the constants, or correlating events by them, gets silent key mismatches; the platform API advertises vocabulary it does not mediate.
- **Counter-evidence searched:** Checked test usage (test-only references absent too); checked whether sprint consumes them (no); concluded genuinely dead.
- **Confidence:** high
- **Smallest useful action:** Delete the unused constants (or align them with the real keys and actually use them in `study/run.go`), whichever matches owner intent.

### GENERALIST-01-F10
- **Priority:** P3
- **Claim:** One physical SQLite file hosts two schema regimes with asymmetric lifecycle ownership: runcontrol owns versioning/migration/backup/quota machinery for the whole file, while `product_states` tables are created unversioned by a second pool; restore and documentation do not account for the co-tenant data, and no test anywhere exercises productstate.
- **Evidence:** Same path constant: `productstate/store.go:19` ≡ `runcontrol/sqlite.go:22`. Unversioned idempotent DDL: `store.go:92-116`; versioned migration + backups: `sqlite.go:356-360`, `migration.go`. `RestoreBackup` renames a whole-file backup including foreign rows and does not remove `-wal`/`-shm` sidecars (`migration.go:296-352`, rename at `:347`). Quota sums every `run-control.db*` byte including co-tenant data (`retention.go:35-56`; tested-intentional per challenger: `retention_test.go:100-129` plants a foreign fixture). Zero productstate tests (package has none; repo-wide grep empty). `recovery.md:217-224` omits any warning that study/sprint state reverts.
- **Architectural reason:** ownership / failure-semantics — PERSIST-SCHEMA-001 requires clear ownership and no mixed-lifecycle structures; the file-level lifecycle (backup/quota/version) is runcontrol's while table-level meaning is other modules'.
- **Concrete consequence:** An operator following `recovery.md` restores run-control history and silently reverts authoritative sprint/study DB rows (which then shadow newer JSON checkpoints because loads prefer the DB, `sprint/state.go:28-35`); a crash-left WAL sidecar can be recovered into the restored file; future productstate schema evolution has no version registry to migrate against.
- **Counter-evidence searched (adversarial challenger, survives-downgraded):** Restore is a documented fully-offline procedure with coherent whole-file snapshots (backups taken post-`checkpointWAL(TRUNCATE)`), so rollback-to-backup semantics are inherent and prescribed; quota-over-footprint is deliberate and tested, and reaching the 496 MiB soft threshold via bounded upserted state rows is implausible at defaults; claimed pragma conflict between pools was **refuted** (both DSNs set `_txlock=immediate`, `store.go:67-73` vs `sqlite.go:73-80`); `hasApplicationSchema()` (`migration.go:58-74`) shows co-tenancy was engineered for, not accidental. Residual is hygiene/documentation, not a live defect.
- **Confidence:** high (facts), high (downgraded severity)
- **Smallest useful action:** Add sidecar checkpoint/removal to `RestoreBackup`; one paragraph in `recovery.md` stating restore reverts migrated product state; register `product_state` in `app_schema` (or its own marker) before its first real schema change.

---

## Defended architecture / rejected hypotheses

1. **Package graph and surface boundary** — Clean. Verified acyclic; `web`/`tui` import only `internal/app` (AST-enforced for web, `import_boundary_test.go:12-36`); transitive closure contains product modules solely via app, exactly as documented. Minor caveats folded into F06/F07, not charged as violations.
2. **Run-control core lifecycle claims** — Verified, not merely asserted: accept+claim precede any goroutine/child with fail-closed persistence loss handling (`run_control.go:125-145`, `durable_operations.go:88-121`, fault tests); timers/lease/grace match docs exactly (`model.go:477-484`); birth-identity reconciliation never adopts/signals/infers success (`lifecycle.go:481-495`, dedicated tests); one immutable terminal winner with cancel-then-success coexistence tested (`lifecycle_test.go:80-174`); events sanitized/committed before delivery, ≤16 KiB, resumable by `(run_id, sequence)`.
3. **"Two connection pools on one SQLite file deadlock"** — Rejected. Both DSNs use WAL + `_txlock=immediate` + 5 s busy timeout; no upgrade-deadlock vector.
4. **"Quota lockout caused by foreign data is a live defect"** — Downgraded to documentation residue inside F10: whole-footprint accounting is tested-intentional; magnitude implausible; health surfaces pressure.
5. **"Web hub persists workflow truth / web-only state required to continue work"** — Rejected. Hub is memory-bounded transport state; restart falls back to durable snapshots; shutdown persists cleanup markers via app into product-owned state. The `ErrWebUnavailable` fail-open branch (`web/operations.go:184-199`) is reachable only when the durable manager is absent from composition, which production `serve` never does — noted as a latent invariant gap, not a finding.
6. **"Operation + child-run double bookkeeping violates single-record intent"** — Rejected as a defect: nesting is documented (architecture.md:163-165). Only the duplicated supervision *implementation* is charged (F02).
7. **"TUI mutability violates design"** — Rejected: `tui/doc.go` and command help define a confirmed-mutation console; only the stale top-level help string is defective (F06).
8. **"Sprint/study twin implementations should be unified"** — Largely rejected: state machines, prompt composition, and metrics shapes reflect genuinely different domain contracts (defensible divergence). The specific cross-twin drift worth attention (strict vs lenient JSON reload inside sprint itself; repair-config triplication in `agentwrap.RepairConfig`) is recorded here as observation, not promoted, since each copy is locally tested and no failure scenario with cost was demonstrated.

## Open questions

1. **F01 intent:** Was interactive smoke-start meant to ship runtime-less (e.g., planned deferred wiring in a later sprint)? The commit trail suggests regression rather than intent; owner confirmation decides P1 vs "known gap".
2. **F03 placement:** Is product-state reconciliation deliberately web-triggered (server-as-janitor model), or should it live in the composition root? The fix differs materially.
3. **F04 remediation direction:** Extend the runcontrol birth-token discipline into study locks, or route study cancellation exclusively through the durable cancellation channel and retire PID signalling?
4. **Productstate schema roadmap:** Before its first real schema change, who owns versioning for the co-tenant tables (runcontrol's `app_schema` vs a productstate-local marker)? This determines whether F10 stays P3.
