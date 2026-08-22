Repo structure is clear. Now let me build the actual import graph with `go list`.
Full graph captured. Now checking test-only edges, unused packages, and boundary tests.
Two orphaned packages found (`platform/filesystem`, `platform/logging`). Now reading the import-boundary enforcement tests and main.
Explicit enforced boundaries found. Now measuring package weights and reading main.
main.go confirms dependency inversion via callbacks. Now dispatching parallel subagents for bounded semantic questions behind the graph edges.
runcontrol report reveals a shared-DB finding. Now dispatching the remaining two discovery tasks.
All three reports in. Now verifying the load-bearing claims myself before writing findings.
Orphaned packages are documented future-boundary placeholders (initial commit, never wired). Now verifying the web capability-assertion pattern.
Capability assertions degrade gracefully by design. Verifying the migration backup mechanism against the shared-file risk.
Backup/checkpoint fails closed under contention — good counter-evidence context. Checking the rest of productstate for symlink handling, plus planning-workspace contracts.
Docs confirm the intended topology. One anomaly left to pin down: sprint/study import `agentwrap` directly instead of going through platform/runtime.
agentwrap stays behind the platform/runtime facade for app/frontends. Final verification: tui boundary enforcement gap and tui's test-only imports.
Investigation complete. All claims verified against source. Writing the scout report.

### Scope inspected

**Commands:** `go list -f/-deps -test ./...` (full intra-module import graph incl. test-only edges), `go.mod`, per-package LOC census, targeted greps for `agentwrap`/`filesystem`/`logging` symbol usage, git history of orphaned packages.

**Source:** all 16 non-test packages; in depth: `cmd/ultraplan/main.go`, `internal/app` (via subagent trace: usecases.go, operations.go, run_usecases.go, web_usecases.go, run_control.go, durable_operations.go, storage_commands.go), `internal/runcontrol` (import_boundary_test.go, sqlite.go, migration.go, model.go, interfaces.go), `internal/productstate/store.go`, `internal/web` (import_boundary_test.go, server.go, operations.go, routes.go), `internal/tui` (imports, tests), `internal/sprint` + `internal/study` (structure, locks.go, verification_lock.go, cleanup_uncertain.go, state_database.go, flow.go, service.go), `internal/platform/{config,runtime,process,filesystem,logging}`.

**Docs:** docs/architecture.md (full), docs/migration-product-state.md, planning workspace layout.

**Subagents used:** three bounded discovery tasks (runcontrol consumption/state ownership; sprint-vs-study parallelism; app hub API surface). All load-bearing conclusions re-derived from cited files above.

### Architecture assessment

The production import graph is a clean DAG that exactly matches the documented contract (docs/architecture.md:11–27):

```text
cmd/ultraplan → app, tui, web          tui → app    web → app
app → {codeextract, platform/config, platform/runtime, productstate,
       project, runcontrol, sprint, study, workspace}
sprint → {agentwrap, platform/config, platform/process, platform/runtime,
          productstate, project, workspace}
study  → {agentwrap, platform/runtime, productstate, workspace}
project → workspace        platform/runtime → {agentwrap(+opencode), config}
leaves: codeextract, config, process, productstate, runcontrol, workspace
orphans: platform/filesystem (6 LOC), platform/logging (72 LOC, imports config)
```

No cycles, including test-only edges. The `app → web` half of the classic cycle is broken by injected runner callbacks (`main.go:27–41` feeding `TUIRunner`/`WebRunner` into `app.Run`). Two boundaries are **test-enforced**: runcontrol may import only stdlib+sqlite (`internal/runcontrol/import_boundary_test.go:12–38`) and web only stdlib+app (`internal/web/import_boundary_test.go:12–36`).

Fan-in/fan-out profile is healthy: highest fan-in is `workspace` (4: app/project/sprint/study) and `platform/config` (4); `app` fans out to 9 but is overwhelmingly DTO-projection/boundary-translation work (42 result types mapping product modules to frontends), which the doctrine values. Weight concentrates where ownership lives: sprint 12,984 LOC / app 9,213 / study 7,148 / web 4,072 / runcontrol 3,560.

Sound: hermetic durability leaf (runcontrol) with versioned schema, OS-lock migration, fail-closed busy checkpoint (`migration.go:175–184`); disjoint product key spaces in shared state (`kind` column separates `sprint_flow`/`sprint_execute` from `study_run`, `sprint/state_database.go:12–17` vs `study/state_database.go:13`); sprint and study never import each other; app holds no package-global mutable state beyond test seams.

Stressed: one physical resource has two owners with asymmetric rigor (F01); two boundaries are enforced while an equally documented third is not (F03).

### Candidate findings

---

#### ID: SCOUT-02-F01

**Priority:** P2

**Claim:** `.ultraplan/run-control.db` is double-tenanted by two modules with independent, unequal schema-governance regimes: runcontrol owns file lifecycle (versioning, migration locks, backups, integrity checks, symlink hardening); productstate creates its tables ad hoc in the same file with none of that machinery.

**Evidence:**
- Identical target constants: `runcontrol/sqlite.go:22` and `productstate/store.go:19` both define `DatabaseRelativePath = ".ultraplan/run-control.db"`; both open independent pools against it (`sqlite.go:81`; `store.go:74`, process-wide cache `store.go:39`).
- productstate schema: plain `CREATE TABLE IF NOT EXISTS product_states/product_state_items` (`store.go:92–114`) — no `user_version`, no migrate lock, no backup; runcontrol versions via `PRAGMA user_version` + `app_schema` + OS lock-file migration with stale-lock reclaim (`migration.go:25–99`, `sqlite.go:229–342`).
- Hardening asymmetry: runcontrol rejects symlinked paths, dir 0700/file 0600 (`sqlite.go:154–191`); `productstate.open()` (`store.go:55–90`) does `MkdirAll(0700)` but no symlink rejection on the same path.
- One CLI command drives both regimes: `app/storage_commands.go:60` (runcontrol `OpenSQLite` → migration) and `:65` (`productstate.Ensure`).
- Neither `docs/architecture.md` §"Durable run control" (:153–191, names runcontrol as owner of the db) nor `docs/migration-product-state.md` mentions the co-location or who governs the file.

**Architectural reason:** ownership | boundary | failure-semantics

**Concrete consequence:** productstate has no sanctioned evolution path: any future table change must be hand-rolled inside a file whose `user_version`, backups, and integrity contract another module owns — and runcontrol's raw-file migration backups (`copyPrivateFile`, `migration.go:186–234`) silently include (or, if productstate ever moves out, silently stop covering) table data it doesn't own. The "private database" invariant runcontrol enforces on open is defeatable through the sibling open path (symlink follow). Failure mode today is benign; the constraint bites at the first product-state schema change.

**Counter-evidence searched:** checkpoint before backup fails closed under contention (`CodeBusy`, `migration.go:180–182`) so no cross-pool corruption; identical pragma baselines both sides (WAL/FULL/immediate/busy-timeout, minus `_defensive` on productstate); `integrity_check` covers the whole file regardless of tenant (`migration.go:250–266`); kind namespaces disjoint so no logical collision; architecture.md assigns each product module authority over its own state — consistent with productstate owning its rows, but not with undocumented file-level co-tenancy.

**Confidence:** high (facts), medium (consequence is latent)

**Smallest useful action:** document the co-tenancy contract in docs/architecture.md §Durable run control (who owns the file, that productstate tables ride in it, backup implications); optionally add runcontrol-equivalent symlink rejection to `productstate.open()`.

---

#### ID: SCOUT-02-F02

**Priority:** P3

**Claim:** web discovers optional app capabilities by runtime type assertion on the `Operations` field rather than declared fields or constructor wiring, so durability silently degrades to in-memory mode if the composed value lacks the capability.

**Evidence:** `h.ops.(app.DurableOperationManager)` at `internal/web/operations.go:181,234,246`; `(app.OperationCleanupRecorder)` at :504; `(app.OperationReconciler)` at `server.go:76`; `(app.WebPromptQueries)` at `handlers.go:440`. On failed assertion the operation proceeds purely in-process (`operations.go:200–218`); main.go always passes the full 19-method `WebUseCases` as all three Options fields (`cmd/ultraplan/main.go:34–36`), so production assertions succeed.

**Architectural reason:** failure-semantics | change-surface

**Concrete consequence:** a future second composition root (or a refactor splitting `WebUseCases` construction) that passes a narrower `WebOperations` compiles fine and serves traffic with run-control recording silently off — observability loss without error signal.

**Counter-evidence searched:** degradation is designed, not accidental: durable availability is surfaced to clients (`durableStatusDTO.Available`, `operations.go:194,203`); the web import-boundary test forbids importing anything but app, making assertion the only negotiation channel; TUI deliberately receives the narrower `OperationalUseCases` and never asserts. Pattern is consistent, suggesting intentional optional-capability protocol.

**Confidence:** medium

**Smallest useful action:** none required if intended; otherwise log once at handler construction when `Operations` lacks `DurableOperationManager`.

---

#### ID: SCOUT-02-F03

**Priority:** P3

**Claim:** the `tui → app` boundary is documented (architecture.md:16–17) like web's, but unenforced — and tui tests already reach past app into sprint and runcontrol, so the production invariant is currently accidental.

**Evidence:** web enforcement exists (`internal/web/import_boundary_test.go`); no equivalent file under `internal/tui/`. Production tui imports only app today (`app.go:13`, `model.go:9`, `views.go:8`); tests import `sprint` (`verify_test.go:8`) and `runcontrol` (`run_view_test.go:10`).

**Architectural reason:** boundary | drift

**Concrete consequence:** a future `tui → sprint` production import (one keystroke away given existing test imports and shared fixture habits) would erode the frontend boundary with nothing failing, unlike the identical erosion in web.

**Counter-evidence searched:** tui is smaller and UI-lib-heavy, so temptation surface is lower; test-only imports are legal and exempted by convention in both packages. No doc exempts tui from the rule.

**Confidence:** high (facts), low-medium (risk)

**Smallest useful action:** ~30-line port of the web boundary parser test into `internal/tui`.

---

#### ID: SCOUT-02-F04

**Priority:** P3

**Claim:** sprint and study duplicate a small cross-cutting protocol: the cleanup-uncertain record shape and the mandatory `"server_shutdown"` reason string exist as independent copies with silent-divergence risk.

**Evidence:** identical `CleanupUncertainRecord` struct and `Reason == "server_shutdown"` validation in `internal/sprint/cleanup_uncertain.go:19–26,49` and `internal/study/cleanup_uncertain.go:19–26,49`; byte-equivalent PID-liveness helpers (`sprint/verification_lock.go:95–101` vs `study/locks.go:17–23`); the string is emitted by the app/web shutdown path (architecture.md:118).

**Architectural reason:** drift

**Concrete consequence:** changing the shutdown-reason vocabulary or record schema in one module leaves the other accepting/rejecting different markers; reconciliation consumers in app (`OperationCleanupRecorder`) would see inconsistent data.

**Counter-evidence searched:** the surrounding logic is genuinely different (sprint adds an in-process composite lease layer plus legacy-state reconciliation, `service.go:77–98`; study reconciles to cancelled states, `cleanup_uncertain.go:66–137`) — extracting the *behavior* would add indirection, which the doctrine forbids. Only the ~15-line record/constant core is duplicated, and both copies are directly unit-tested.

**Confidence:** medium

**Smallest useful action:** defer unless a third product module repeats the pattern; then hoist only the record type + reason constant into a minimal shared location.

### Defended architecture / rejected hypotheses

- **"There are import cycles or layering violations."** Rejected. Full `go list -deps -test` graph is acyclic; the prohibited `app → web → app` cycle is avoided exactly as architecture.md:21–27 prescribes, via injected runners, with no registry/init/service-locator tricks.
- **"`internal/platform/filesystem` and `internal/platform/logging` are dead code."** Rejected as a defect. Both are explicit reserved boundaries ("deferred to owning modules in later sprints", `filesystem/*.go:1–5`; "deferred to a later sprint", `logging/*.go:1–4`), present since the bootstrap commit, unwired ever since. This is FUTURE-INTENT placeholder, not accidental cruft. Not reported as a finding per doctrine.
- **"sprint/study bypassing platform/runtime to import agentwrap directly breaks the runtime boundary."** Rejected. Their public runtime seams are typed over `runtime.Request/Result` (`sprint/flow.go:13–15`, `study/service.go:25–27`); direct agentwrap usage constructs payload vocabulary (`ValidationSpec`, permission policies) that the facade itself re-exposes (`runtime.Request.Validation *agentwrap.ValidationSpec`, `runtime.go:38`). No agentwrap type reaches app/tui/web (their 8 hits are config-display strings). Re-wrapping ~50 SDK types would be pure indirection.
- **"app is a god package needing division."** Rejected for now. Its 9.2k LOC is dominated by typed projections and operation normalization — necessary boundary translation between product modules and two frontends; fan-in is only 3 (cmd, tui, web) and no global mutable state exists beyond three documented test seams.
- **"Shared SQLite file risks corruption."** Rejected as a current failure mode: immediate-tx everywhere, fail-closed busy checkpoint, whole-file integrity checks, matching pragma baselines (see F01 counter-evidence). The residual issue is governance/evolution, captured in F01.
- **"tui/web test duplication of repository adapters indicates a missing exported constructor."** Rejected: exporting `repositoryRunUseCases` would let frontends mint raw DB handles, weakening app's authority over the durability boundary; the test-local adapter (`web/run_handlers_test.go:26–42`) is the correct price.
- **"sprint vs study dependency asymmetry (config/process) is drift."** Rejected: differences track real needs — sprint runs the smoke harness subprocess (`smoke.go:75,128`) and resolves execute models from config (`execute_model.go`); study spawns solely via the injected runtime boundary plus one `git clone` (`init_clone.go:44–50`).

### Open questions

1. Is productstate's co-location in `run-control.db` a deliberate, documented decision anywhere outside code (planning-workspace system docs I did not exhaustively search)? If yes, F01 downgrades to documentation-drift P3; if no, it stays P2.
2. Is there FUTURE-INTENT for product-state schema evolution (a second table regime version)? That would raise F01's consequence materially.
3. Are web's type-assertion capability checks intended as a stable negotiation protocol for alternative compositions (multiple server embedders), or transitional? Determines whether F02 warrants an startup-capability log line.
