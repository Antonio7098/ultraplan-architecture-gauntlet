### Scope inspected

**Implementation repo (`ultraplan-go` @ eeaa034):**
- `internal/runcontrol/migration.go` (full — `migrateSchema`, `acquireMigrationLock`, `removeStaleMigrationLock`, `createInitialSchema` path, backups, `RestoreBackup`), `sqlite.go:59-366` (`OpenSQLite`, `preparePrivateDatabase`, `initialSchema`, transactional DDL), `lifecycle.go:301-540` (`Reconcile`, `reconcileUnclaimed`, `reconcileProcessDecision`)
- Tests: `migration_test.go`, `fault_test.go`, `process_integration_test.go`, `lifecycle_test.go` names/assertions; `internal/sprint/verify_test.go:62-89`
- Product/run-state formats: `internal/productstate/store.go`, `internal/sprint/state.go`, `state_database.go`, `domain.go:10-11`, `execute_state.go:74-81`, `locks.go:76`; `internal/study/state.go`, `state_database.go`, `validation_command.go:165-198`
- Startup wiring: `cmd/ultraplan/main.go`, `internal/app/app.go` dispatch, `run_control.go:16-93`, `serve_commands.go:17-81`, `run_commands.go:42-60`, `storage_commands.go`, `health_commands.go:12-75`
- Docs/scripts: `docs/recovery.md`, `docs/configuration.md:192`, `docs/migration-product-state.md`, `scripts/migrate-product-state.sh`, `scripts/migrate-legacy-sprint-state.sh`
- Repo-wide sweeps: `rg migrate.lock docs/` (zero hits), `RestoreBackup` call sites, all `productstate.Ensure/Existing` call sites

**Planning workspace:** `projects/ultraplan-go/docs/TRD.md:2132,2354-2356,2498` (startup reconciliation contract), roadmap/plan docs for FUTURE-INTENT classification.

### Architecture assessment

The startup chain is coherent and mostly well-defended:

1. **Schema migration (previous DB formats):** `OpenSQLite` → `migrateSchema` distinguishes newer-than-supported (hard stop, `CodeUnsupportedSchema`), current (fast path without migration lock, verified record + integrity check), and version-0 (lock → re-check → WAL checkpoint → timestamped bounded backup → atomic DDL+version-stamp transaction). `createInitialSchema` wraps DDL, `app_schema` row, and `PRAGMA user_version` in one commit (sqlite.go:344-366), so an interrupted first migration rolls back cleanly and re-runs safely.
2. **Startup run reconciliation:** every repository open runs `Reconcile` (app/run_control.go:64) which terminalizes never-claimed runs after grace and probes exact process birth identity for expired leases before proposing `interrupted`/`cleanup_uncertain` — matching the TRD:2132/2354 contract ("never infer success"). Idempotency and clock-jump behavior are tested.
3. **Product/run-state dual format:** authority is per-record, not per-file: `LoadFlowState`/`LoadRunState` prefer the DB row, fall back to the JSON file, and interrupted `storage migrate` imports leave a tolerated mixed state (per-record atomic import via one `productstate.Save` transaction; re-run skips stored records; files remain checkpoints per docs/migration-product-state.md).
4. **Legacy format handling is layered and tested:** sprint flow v1→v2 migrates in memory only (deliberate, verify_test.go:78-82 asserts no implicit rewrite); pre-code-context stage shapes are interpreted by shape detection; legacy map-based v0 states gate lock acquisition rather than crashing (locks.go:76); study run-states reject unsupported versions loudly by explicit documented contract (configuration.md:192) instead of migrating.

Stressed points are confined to operator-facing failure semantics of the migration machinery itself (findings below), not to state ownership or authority.

### Candidate findings

#### FAILURE-10-F01
- **Priority:** P2
- **Claim:** A process death between creation and synced identity write of the schema-migration lock leaves an orphaned `.migrate.lock` that can never be reclaimed automatically, permanently blocking every run-control open in the workspace with a factually wrong error message, and neither the runbook nor any command documents or surfaces this state.
- **Evidence:**
  - `acquireMigrationLock` creates the file with `O_EXCL` *first*, then probes its own PID, marshals, writes, syncs identity (migration.go:92-137) — the window spans `probeNativeProcess(os.Getpid())` at :111 plus write/fsync.
  - `removeStaleMigrationLock`: malformed/unparseable record → `(false, nil)` — "A legacy or malformed record cannot authorize lock removal" (migration.go:146-149); empty/partial file therefore yields permanent `CodeBusy`.
  - The resulting message claims "another local UltraPlan process owns the schema migration lock" (migration.go:105) — false when the creator died pre-write.
  - Test enshrines the behavior for garbage content (migration_test.go:85-91); reclaim is proven only for valid-dead identity (:94-117).
  - `rg 'migrate.lock' docs/` → zero hits; recovery.md's run-control section (recovery.md:184-228) covers backups, restore, and stale *product* locks but never this file. `ultraplan health` never opens run control (health_commands.go:43-74), so it reports all-ok in this state; only durable commands / `run diagnostics` fail (run_commands.go:42-60).
- **Architectural reason:** failure-semantics (+ drift between code conservatism and operator documentation).
- **Concrete consequence:** SIGKILL/power loss during the once-per-workspace version-0→1 transition bricks all serve/run/storage commands with `ErrBusy` until an operator hand-deletes a hidden file inside the private `.ultraplan/` directory — the very class of action recovery.md tells them to avoid without guidance ("Never delete it merely because...", recovery.md:115-116). Diagnosis is actively misdirected by the message.
- **Counter-evidence searched:** The non-removal is *correct* while a writer may be alive-but-pre-write (indistinguishable at read time), consistent with TRD:2132's prohibition on inferring staleness; the window is narrow and occurs once per database lifetime; `CodeBusy` is marked retryable. None of this repairs the missing diagnosis/doc — the gap is real but bounded, hence P2.
- **Confidence:** high (mechanism); medium (operator impact frequency).
- **Smallest useful action:** Split the malformed-record branch into its own error text that names the exact file, states that no owner identity was ever recorded, and gives the verified-no-ultraplan-process + delete instruction; add one matching bullet to recovery.md. Optionally include migration-lock presence in the `run diagnostics` support export.

#### FAILURE-10-F02
- **Priority:** P2
- **Claim:** The prescribed offline recovery for failed/interrupted schema migration — restoring a migration backup — has no executable surface: `runcontrol.RestoreBackup` is exported but referenced only by its own test, while recovery.md instructs operators to "use the tested restore path".
- **Evidence:** recovery.md:217-224 documents backup restore and declares unsupported-schema/integrity-failure a stop condition whose remedy is restore; `rg RestoreBackup internal cmd` → only definition (migration.go:293-352) and `migration_test.go:137-138`; `scripts/` contains migrate helpers but no restore script; no CLI subcommand, web operation, or TUI action dispatches it.
- **Architectural reason:** change-surface / failure-semantics drift (documented recovery contract without a shipped entrypoint).
- **Concrete consequence:** at exactly the moment the stop condition fires (corrupt/newer/integrity-failed DB after an interrupted migration), the operator has no product-provided way to execute the documented procedure and must improvise file copies — precisely the "never copy only the database while a WAL writer is active" hazard recovery.md:221-222 warns against. The function's own safety logic (symlink rejection, size bound, integrity pre-check, temp+rename, mode enforcement) is what operators silently lose.
- **Counter-evidence searched:** The comment "Callers must stop all UltraPlan processes and restore the matching binary" suggests deliberate API-only design for external orchestration, and today there is exactly one schema version so restores are rare; release-checklist mentions restore only in test context. The doc promise vs. reachable-surface mismatch stands regardless of intent.
- **Confidence:** high (gap exists); medium (whether API-only is intentional).
- **Smallest useful action:** Either add a guarded `ultraplan run-control restore-backup <name>` (offline-enforcing: refuse if any owner identity probes live, reuse `RestoreBackup` verbatim) or amend recovery.md with the exact manual steps mirroring `RestoreBackup`.

### Defended architecture / rejected hypotheses

1. **DB-vs-file split-brain for migrated state.** Hypothesis: runtime could pick conflicting copies after interrupted import. Rejected: authority is defined per record (`FlowStateInDatabase`/`RunStateInDatabase` gates in sprint/state.go:216-227 and study/state.go:59-70); load prefers DB unconditionally (state.go:20-32); checkpoint writes eventually refresh files only at completion boundaries (sprint/state.go:222-225, study/state.go:66-69); interrupted imports leave a documented, tolerated mixed mode (docs/migration-product-state.md:21-26); per-record import is atomic (productstate/store.go:150-205).
2. **`RestoreBackup` silently reverting imported product state.** Hypothesis: restoring run history also rolls back sprint/study execution state while artifacts stay newer. Rejected as defect: both live in one file, so restore yields a coherent point-in-time snapshot; recovery.md:51,64-66 provides the re-derivation path (`sprint status` persists derived stage state from artifacts); the procedure is explicitly offline and deliberate.
3. **productstate bypassing `app_schema`/`user_version` governance as a current defect.** Rejected for now: table shape is single-version, DDL idempotent, and every DB-creating path stamps `user_version` via `OpenSQLite` before product tables exist (storage_commands.go:59-67 is the only cold `Ensure`). Embedded schema-migration machinery for this store is declared future work (docs/plans/ultraplan-local-server-experiment-plan.md:662,899-903 — FUTURE-INTENT, not reportable). Watch-item only.
4. **In-memory-only v1→v2 flow migration leaving stale v1 files.** Rejected: deliberate compatibility (comment at sprint/state.go:128-130 — recovery must preserve the file because no live attempt exists to reconcile), explicitly tested (verify_test.go:62-89), and deterministic on re-load.
5. **Divergent compatibility policies (sprint auto-migrates v1→v2 vs study rejecting old versions) as inconsistency.** Rejected: both policies are explicit and documented (configuration.md:192; sprint migration is mechanical evidence-preserving per migrateFlowStateV1), and the stricter policy governs the higher-risk execution state.

### Open questions

1. Is `RestoreBackup` API-only by decision (awaiting a guarded CLI surface in planned storage work), or an omission? This sets whether F02 is a doc fix or a missing command.
2. Should migration-lock state be part of `run diagnostics --support-export`? If yes, F01's smallest action expands slightly; the diagnosis gap itself is unchanged.
