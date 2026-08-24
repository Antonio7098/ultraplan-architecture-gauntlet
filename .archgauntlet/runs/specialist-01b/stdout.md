README's ownership map names 7 modules but the tree shows additional top-level packages (`productstate`, `runcontrol`, `tui`, `web`) not in that list. Let me check the planning workspace's authoritative architecture docs first.
The authoritative ARCHITECTURE.md names modules incl. `runcontrol`'s semantic role (Phase 6 boundary) but leaves its package placement open. Now let me map actual imports against the declared dependency rules.
Import graph is clean — no forbidden edges. Now the deeper question: three modules (`productstate`, `sprint`, `study`) plus `runcontrol` all touch `.ultraplan/run-control.db`. Let me trace who owns what in that file and how JSON artifacts relate to SQLite state.
Sprint 35 requirements explicitly forbid durable records from becoming "a shadow authority for ... flow-state outcomes" (line 42), yet the implementation makes SQLite rows win over `flow-state.json`. Let me verify how deep this goes and whether any doc sanctions it.
Investigation complete. Here is my specialist report.

---

### Scope inspected

**Implementation repo** (`ultraplan-go` @ eeaa034): `internal/sprint/{state.go,state_database.go,execute_state.go,prompts.go,locks.go,execute_model.go}`, `internal/study/{state.go,state_database.go}`, `internal/productstate/store.go`, `internal/runcontrol/{doc.go,sqlite.go,migration.go,retention.go,import_boundary_test.go}`, `internal/app/{storage_commands.go,run_control.go,sprint_usecases.go}`, `internal/workspace/{init.go,skills.go,defaults.go}`, `cmd/ultraplan/main.go`, full `go list` import graph for all packages, `docs/{architecture,recovery,migration-product-state,cli-reference}.md`, `README.md`, `git log/show` for commits `02e2ec4`, `e09d394`.

**Planning workspace** (`ultraplan-workspace` @ 368a789): `projects/ultraplan-go/project-index.md`, `docs/ARCHITECTURE.md`, `docs/TRD.md` (persistence sections), `sprints/35-durable-run-observability/{requirements.md,flow-state.json}`.

### Architecture assessment

The module topology is sound and unusually well-defended. The declared dependency rules hold exactly: `web → app` and `tui → app` only; `platform/* → platform only`; `sprint` does **not** import `study`; `runcontrol` enforces its own genericity with a parse-based import-boundary test (`import_boundary_test.go:26-37`). Flow-state writes funnel through three choke points (`SaveFlowState`, `SaveExecuteRunState`, `SaveRunState`) so the persistence policy is centralized, and composition matches the documented explicit-runner shape in `cmd/ultraplan/main.go`. Prompt/template authority is cleanly split: workspace owns mechanical override resolution (`prompts.go:253` `workspace.DefaultOverrideFile`), sprint owns stage semantics.

The stress point is durability: commit `02e2ec4` ("Move mutable product state to SQLite") introduced a second persistence regime — `internal/productstate` — sharing one physical database file with `internal/runcontrol`, and made SQLite rows win over the package-owned JSON artifacts that both the planning workspace contracts and the implementation repo's own docs still declare authoritative.

### Candidate findings

---

**ID: SPECIALIST-01B-F01**
**Priority: P1**

**Claim:** After `ultraplan storage migrate`, the source of truth for sprint flow state, execute run state, and study run state silently moves from the package-owned JSON artifacts into SQLite, contradicting the current governing contracts (which reserve this decision for Sprint 35 reasoning, still unwritten) *and* the implementation repo's own operator-facing docs — and the entire authority-flip layer has zero test coverage and no divergence detection between the two stores.

**Evidence:**
- Load precedence is DB-first, JSON ignored when a record exists: `internal/sprint/state.go:21-31` (`LoadFlowState` returns the DB record without reading the file), `internal/sprint/execute_state.go:36-46`, `internal/study/state.go` via `loadRunStateDatabase` (`state_database.go:17-41`). No timestamp comparison against the file exists anywhere.
- Save is dual-mode: DB always when authoritative; JSON rewritten only at terminal checkpoints (`state.go:216-227`, `flowStateCheckpoint` at 230-237; `execute_state.go:106-118`; `study/state.go:59-70`) — non-terminal transitions leave the JSON checkpoint stale by design.
- The flip is documented only implementation-side: `docs/migration-product-state.md:24` "After import, SQLite is authoritative for that record."
- Contradicting current contracts: workspace `ARCHITECTURE.md:712` ("package-owned workflow artifacts retain their existing authority"), `:716` ("No silent dual writes or synchronization precede an explicit authority decision"; persistence selection happens "once at composition", not mid-life via CLI), `TRD.md:126` and `TRD.md:2235` ("Sprint 35 operational run storage does not select a new authored-artifact authority"), Sprint 35 `requirements.md:13` and `:42` ("Durable operational records do not become a shadow authority for … flow-state outcomes").
- Contradicting in-repo docs: `docs/architecture.md:57-60` "Workspace files and product-owned flow, execute, review, smoke, and study run state remain authoritative"; `docs/recovery.md:51,86` instructs file-centric repair ("Validate both files and rerun smoke to reconcile"); `README.md:103` still claims study progress "is still stored in `studies/<study>/.ultraplan/run-state.json`".
- Authority placement: `project-index.md:17` declares the planning workspace "the sole authoritative location" for PRD/TRD/Architecture, and Sprint 35 `requirements.md:66` requires authority-affecting decisions "recorded in reasoning before implementation planning". Commit `02e2ec4` (Aug 21) landed after Sprint 35 requirements were completed (Aug 20, per its `flow-state.json`) while `reasoning.md`/`plan.md` are still `missing`.
- Zero tests: `rg 'productstate|InDatabase|ToDatabase|runStorage' --glob '*_test.go'` returns nothing; there is no `productstate` test file at all.

**Architectural reason:** authority (+ drift, change-surface).

**Concrete consequence:** An operator who hand-edits or restores `flow-state.json` from Git after migrating (a workflow `recovery.md` and `README.md:193` "editable Markdown/JSON artifact chains" still imply) gets silent no-op behavior — the edit never loads, with no warning. Conversely, a fresh clone of a migrated workspace (the `.ultraplan/*.db` is private/`0600` and not shown as committable) falls back to terminal-only checkpoints and loses non-terminal resume state. Two repair manuals give opposite instructions depending on which doc the operator reads first.

**Counter-evidence searched:** Migration is explicit, opt-in, dry-runnable, validates before import, skips already-imported records, and can be incremental (`storage_commands.go:85-121`); fresh workspaces never create the DB implicitly because `SaveFlowState` only takes the DB path when `Existing()` finds one; all writers route through the three choke points, so semantics stay sprint-owned (`ValidateFlowState` runs on the DB path too, `state.go:28`). These mitigate severity but do not resolve the contract contradictions, the missing divergence detection, the absent tests, or the stale operator docs.

**Confidence:** high (code behavior and contract text are unambiguous; medium-high overall only because Sprint 35 reasoning may later ratify the topology).

**Smallest useful action:** Record the authority decision where the contracts require it (Sprint 35 reasoning + ARCHITECTURE/TRD update), then align the implementation repo: amend `docs/architecture.md:57-60`, `recovery.md`, `README.md:103/193` to describe DB-primary-with-checkpoints, and add round-trip tests for load/save precedence plus a `storage migrate --dry-run` warning when a JSON checkpoint is newer than its DB record.

---

**ID: SPECIALIST-01B-F02**
**Priority: P2**

**Claim:** One physical database file (`<ws>/.ultraplan/run-control.db`) has two uncoordinated owners with different invariant regimes: `runcontrol` (versioned migrations, quota accounting over the whole file, symlink/permission enforcement, import-isolation test) and `productstate` (unversioned schema, quota-invisible growth, weaker open policy, duplicated path constant). Neither package can reason about the other — by design, in runcontrol's case.

**Evidence:**
- Duplicated path constants with no shared owner: `productstate/store.go:19` and `runcontrol/sqlite.go:22` each independently declare `.ultraplan/run-control.db`.
- Split schema authority: runcontrol registers versioned migrations in `app_schema` and `PRAGMA user_version = 1` (`sqlite.go:230,356-360`; `migration.go:86-87` rejects newer schemas with `CodeUnsupportedSchema`); `productstate.createSchema` (`store.go:92-116`) is bare `CREATE TABLE IF NOT EXISTS` invisible to that registry.
- Quota coupling: `retention.go:35-56` (`storageBytes`) sums every `run-control.db*` file byte — including `product_states`/`product_state_items` rows — against runcontrol's hard quota; ≥80% triggers compaction pressure (`retention.go:109`) and exhausted compaction fails starts with `CodeQuota` (`retention.go:86,94`). Sprint/study state growth therefore consumes the event-retention budget with no coordination or attribution.
- Asymmetric open policy: `preparePrivateDatabase` (`sqlite.go:154-191`) enforces non-symlink directory/file, regular file, `0600`; `productstate.open` (`store.go:55-90`) does `MkdirAll` + `sql.Open` with none of those checks.
- Isolation proof: `import_boundary_test.go:33` permits only stdlib + sqlite + x/sys, so runcontrol cannot even reference `productstate` — the coupling exists solely at the filesystem layer, invisible to the compiler and to the boundary test.
- Backup/restore doctrine covers only the runcontrol regime: `docs/recovery.md:218-224`.

**Architectural reason:** ownership (+ lifecycle, failure-semantics).

**Concrete consequence:** A task-heavy migrated sprint pushes file usage past the soft threshold through no runtime-event growth; operators see quota failures attributed to "run-control storage" while the actual consumer is sprint state, and remediation advice ("free space outside the active database") is wrong. A future runcontrol maintenance step that assumes file ownership (e.g., rebuild-from-backup, `user_version` bump with rewrite) risks clobbering or orphaning authoritative product-state tables that no backup procedure for "run-control" claims to cover; or a change to one `DatabaseRelativePath` constant silently forks the store.

**Counter-evidence searched:** Both openers set identical durability pragmas (WAL, `synchronous=FULL`, `_txlock=immediate`, 5s busy timeout — `store.go:67-72` vs `sqlite.go:73-80`), so multi-pool concurrency on one WAL file is safe; in practice `storage migrate` opens runcontrol first (`storage_commands.go:60-65`), applying the `0600` chmod before productstate writes. Real workspaces are unlikely to hit the 64 MiB minimum quota soon. These reduce immediate risk but leave the structural coupling.

**Confidence:** high (mechanics verified), medium (severity depends on future evolution).

**Smallest useful action:** Make the path constant single-owned (one package exports it, the other consumes), register a `productstate` component row in `app_schema` during migrate, and exclude (or explicitly attribute) known foreign-table bytes from `storageBytes()`.

---

**ID: SPECIALIST-01B-F03**
**Priority: P3**

**Claim:** `ultraplan storage migrate` — the command that flips state authority — is absent from the declared CLI contract surface.

**Evidence:** `grep -c 'storage' docs/cli-reference.md == 0`; `README.md:133` defines cli-reference as "public commands, flags, exit classes, and stable JSON surfaces." The command exists with usage, flags, `--json` output shape, and an `ExitPartial` class (`storage_commands.go:33-140,136-138`).

**Architectural reason:** drift (+ authority).

**Concrete consequence:** Operators and tooling discover an authority-changing command only by reading source or a side doc; scripted environments cannot rely on the documented-stable JSON surface.

**Counter-evidence searched:** `docs/migration-product-state.md` and `scripts/migrate-product-state.sh` document it; nothing marks it experimental or excluded from the contract.

**Confidence:** high.

**Smallest useful action:** Add a `storage` section to `docs/cli-reference.md` (or an explicit experimental marker).

---

**ID: SPECIALIST-01B-F04**
**Priority: P3**

**Claim:** The implementation README's module-ownership map omits half the shipped modules (`tui`, `web`, `productstate`, `runcontrol`).

**Evidence:** `README.md:227-235` lists seven modules; the tree and `docs/architecture.md:153-160` include the durable run-control boundary and both interface modules.

**Architectural reason:** drift.

**Concrete consequence:** Low — README defers to the planning workspace as authoritative (`README.md:7`); new contributors get a stale first impression only.

**Counter-evidence searched:** Planning workspace documents all modules correctly, bounding the damage.

**Confidence:** high.

**Smallest useful action:** One-paragraph README update pointing at `docs/architecture.md` §"Durable run control" and the product-state migration note.

### Defended architecture / rejected hypotheses

- **"web/tui bypass the app layer or own product state"** — rejected. `go list` shows `web → internal/app` and `tui → internal/app` as their *only* internal imports; all flow mutations route through sprint-owned choke points; the status-refresh path also goes through `service.go:192 → SaveFlowState`.
- **"platform imports product modules"** — rejected; platform packages cross-import only each other.
- **"sprint reaches into study"** — rejected; sprint imports project/workspace/platform/productstate only, matching `project-index.md:57-62` reuse rules.
- **"Hidden delegation in stage skills"** — rejected; `$ultraplan-code-context` delegates to the canonical flow and is documented as the narrow sanctioned exception (`workspace/skills.go:107-117`, `README.md:195-199`); no other skill shells out.
- **"Competing model-resolution authorities"** — rejected; `ResolveExecuteModel` (`execute_model.go:15-30`) is the single sprint-owned rule consuming config as data, with sourced candidates and explicit error.
- **"Global runner registry / init() wiring"** — rejected; `cmd/ultraplan/main.go` injects TUI/web runners explicitly, matching `docs/architecture.md:8-27`.
- **"runcontrol owns product semantics"** — rejected; its doc goal (`doc.go:1-7`) plus the import-boundary test keep it a projection/recording boundary; reconciliation is conservative per `locks.go:19-24` (live leases never rewritten).

### Open questions

- Will Sprint 35's pending reasoning ratify the `productstate` topology and amend the workspace ARCHITECTURE/TRD? If yes, F01 reduces to untested-flip + stale-doc residue; if no, the contract conflict stands as written.
- Is whole-file quota inclusion of product-state bytes intentional budget-sharing or an oversight? A one-line comment or exclusion would settle F02's severity.
- Are post-migration JSON checkpoints expected to be committed to Git in target workspaces (they currently would be, while the DB is private)? That determines whether F01's clone/restore scenario is common or exotic.
