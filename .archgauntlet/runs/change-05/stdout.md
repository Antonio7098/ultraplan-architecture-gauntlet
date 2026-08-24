# CHANGE-05 — Evolve a product-state schema

## Scope inspected

- `internal/productstate/store.go` (schema DDL, Record model, Save/Load/Has, open caching)
- `internal/study/state.go`, `internal/study/state_database.go`, `internal/study/run_state_domain.go`, `internal/study/domain.go`, `internal/study/run_state.go`, `internal/study/discovery.go`
- `internal/sprint/state.go`, `internal/sprint/state_database.go`, `internal/sprint/domain.go`, `internal/sprint/execute_state.go`
- `internal/app/storage_commands.go`, `scripts/migrate-product-state.sh`, `internal/app/app.go` (command wiring)
- `internal/runcontrol/migration.go`, `internal/runcontrol/sqlite.go` (shared DB file, schema governance)
- Docs: `docs/migration-product-state.md`, `docs/recovery.md`; workspace contract `system/contracts/runtime/persistence-and-migrations.md`
- Git history: `02e2ec4` ("Move mutable product state to SQLite") vs `f4d6d38` (FlowState v2) ordering; test-surface greps for `productstate`/`storage migrate`

## Architecture assessment

Sound: each product state kind is owned by its producing module (`study_run` in study, `sprint_flow`/`sprint_execute` in sprint) with scope conventions local to each; the generic `productstate` store knows nothing about payloads. Every load path validates (`ValidateRunState`, `ValidateFlowState`, `ValidateExecuteRunState`), file writes are temp+rename+fsync, and `storage migrate` imports through the normal `Load*` functions, so imported records are always current-version at import time. The file path for FlowState demonstrates a complete in-place schema evolution (v1→v2 via `migrateFlowStateV1`), including legacy-shape detection and recovery-preserving semantics.

Stressed: the same physical database (`.ultraplan/run-control.db`) hosts two schema regimes — a fully governed one (`runcontrol`: `user_version` gate, `app_schema` ledger, lock, backups, restore) and an ungoverned one (`productstate`: bare `CREATE TABLE IF NOT EXISTS` on every open). Version handling is asymmetric between the file and DB backends of the *same* logical record, and none of the product-state persistence surface has tests.

## Candidate findings

### ID: CHANGE-05-F1
- **Priority:** P1
- **Claim:** `product_states` tables live inside the run-control database but outside the repo's only migration-governance machinery, so a DDL-level product-state schema evolution has no ordered, reviewed, backed-up home.
- **Evidence:** `productstate.DatabaseRelativePath = ".ultraplan/run-control.db"` (store.go:19) equals `runcontrol.DatabaseRelativePath` (sqlite.go:22). `createSchema` (store.go:92-116) is create-only DDL executed on *every* `Ensure`/`Existing` open (e.g., state_database.go:63, 92, 122), with zero references to `user_version` or `app_schema` outside runcontrol (grep). Meanwhile runcontrol owns `PRAGMA user_version` (migration.go:84-90), the `app_schema` component ledger with `component TEXT PRIMARY KEY` (sqlite.go:231, :356 inserts only `'run_control'`), a migration file-lock (migration.go:92-138), bounded backups (migration.go:186-200), and `RestoreBackup`.
- **Architectural reason:** boundary / drift — two owners share one durable file; the persistence contract's migration regime (PERSIST-MIG-001) maps to only half the schema.
- **Concrete consequence:** adding a column/index/constraint to `product_states` must be done as ad-hoc DDL in `createSchema` running on ordinary saves (mid-run-loop), outside the migration lock and backup regime, with no idempotency guard beyond `IF NOT EXISTS`. Older binaries still pass runcontrol's `user_version == 1` gate (migration.go:30-31 unchanged) and then fail with opaque SQL errors instead of `CodeUnsupportedSchema`. `PRAGMA user_version` is per-file and already claimed, so the natural version marker is taken.
- **Counter-evidence searched:** initial mixed-schema creation *is* handled carefully — `migrateSchema` backs up a pre-existing foreign schema before initial create (migration.go:58-74) and `storage migrate` deliberately opens runcontrol before `Ensure` (storage_commands.go:59-67); `RestoreBackup` restores the whole file including product rows under "matching binary" discipline (recovery.md:217-224). None of this covers *future* product-state DDL changes. The `app_schema` component PK is a ready-made extension point that is simply unused.
- **Confidence:** high (structure verified; consequence is the standard next iteration of this probe)
- **Smallest useful action:** register and verify a `product_state` component row in `app_schema` inside `createSchema`/`Ensure`, giving future DDL evolutions an ordered, gated home; state the convention in `docs/migration-product-state.md`.

### ID: CHANGE-05-F2
- **Priority:** P1
- **Claim:** Legacy-version upgrade support exists only on the file backend; the DB backend has no version gate or migration hook, so the *next* schema bump permanently hard-fails DB-authoritative records that the file path would auto-heal — the evolution edit surface is not localized and one required site has no extension point.
- **Evidence:** File path: `state.go:53` accepts `PreviousFlowStateSchemaVersion`, `state.go:68-73` applies `migrateFlowStateV1` (state.go:150-192) — seamless v1→v2. DB path: `loadFlowStateDatabase` (sprint/state_database.go:19-36) and `loadRunStateDatabase` (study/state_database.go:17-41) ignore `record.SchemaVersion` entirely and apply no migration; the subsequent `ValidateFlowState`/`ValidateRunState` accept *only* the current constant (state.go:295-296, state.go:125-126, execute_state.go:188-189). No command upgrades stored records: `storage migrate` skips anything already stored (storage_commands.go:98-100, 150-152).
- **Architectural reason:** lifecycle / drift — one logical record, two backends, upgrade logic wired into one branch only; the precedent (`migrateFlowStateV1`) lives exclusively in the file branch, inviting copy-paste evolutions that miss the DB branch.
- **Concrete consequence:** after bumping `FlowStateSchemaVersion` to 3, every migrated workspace's DB rows become unloadable (`ErrFlowStateUnsupported` on every status/verify/flow operation) while an identical non-imported workspace self-heals via `migrateFlowStateV1`-style logic. The only operator remedies are deleting `.ultraplan/run-control.db` (losing run-control history) or hand-editing SQLite. Study and execute states have no previous-version concept at all, so their first evolution has no precedent on either backend.
- **Counter-evidence searched:** not a *live* defect today — the SQLite feature (commit `02e2ec4`) postdates the v2 bump (`f4d6d38`), so no supported path ever wrote a v1 DB row, and import goes through `Load*` so stored rows are import-time-current. Terminal checkpoint files provide a fallback if the DB is deleted (`Existing` returns disabled when the file is absent, store.go:41-51; checkpoints written only at terminal states, state.go:222-225 / state.go:66-68). But that fallback sacrifices run-control history and is documented nowhere as an upgrade remedy.
- **Confidence:** high (asymmetry is structural and verified; reachability is the next iteration of this exact change)
- **Smallest useful action:** route DB loads through the same version gate + migrate function as file loads (extract one validated-decode helper per state type), and add a test asserting a previous-version DB record upgrades identically to its file twin.

### ID: CHANGE-05-F3
- **Priority:** P2
- **Claim:** The entire product-state persistence surface — store, both `_database.go` modules, and `storage migrate` — has zero test coverage, despite being the layer where PERSIST-MIG-001 explicitly demands "migration tests or dry-runs cover upgrade path".
- **Evidence:** grep for `productstate|storage migrate|run-control.db` across `*_test.go` matches only `internal/runcontrol/migration_test.go` and `retention_test.go`. Nothing covers `Save`'s hash-based skip and stale-item deletion (store.go:160-205), `Load`'s item reassembly, either DB loader, or the imported/skipped/partial-exit behaviour of `runStorage` (storage_commands.go:69-140).
- **Architectural reason:** change-surface / failure-semantics — the newest, least-boring persistence code is the only durable-state code with no regression harness.
- **Concrete consequence:** any schema evolution here (this probe) lands blind; the F2 gap class is invisible to CI, and subtle store behaviours (stale-item cleanup, version-overwrite-on-hash-change) can regress unnoticed.
- **Counter-evidence searched:** no indirect coverage via app-level tests found; runcontrol's fault/benchmark tests do not exercise productstate.
- **Confidence:** high
- **Smallest useful action:** add a store round-trip test (save/load/delete-stale), one previous-version-record test per owning module, and a `storage migrate` dry-run/import/skip/partial-failure test.

### ID: CHANGE-05-F4
- **Priority:** P3
- **Claim:** Version/validation failures on DB-authoritative records are misattributed to the JSON checkpoint file path, misleading recovery.
- **Evidence:** DB branches compute `FlowStatePath`/`ExecuteRunStatePath`/`RunStatePath` purely for diagnostics and pass it to validators (state.go:24-31; execute_state.go:39-46; study/state.go:28-34); validator errors embed that path (e.g., state.go:295-297) while the offending bytes are in `product_states`. `recovery.md` remedies ("restore version N or regenerate state", state.go:54) point at files.
- **Architectural reason:** failure-semantics.
- **Concrete consequence:** during the F2 scenario, an operator inspecting a perfectly valid (stale) checkpoint file chases the wrong artifact; the actual record is a DB row keyed `(kind, scope)`.
- **Counter-evidence searched:** reusing the path keeps messages uniform for the dominant file case; acknowledged, but the DB branch could annotate origin cheaply.
- **Confidence:** high
- **Smallest useful action:** wrap DB-origin validation failures with the record origin (`kind/scope in .ultraplan/run-control.db`) instead of (or alongside) the file path.

## Defended architecture / rejected hypotheses

1. **"Migration imports raw stale-version files into the DB."** Rejected: `runStorage` imports via `LoadRunState`/`LoadFlowState`/`LoadExecuteRunState` (storage_commands.go:102, 153, 176), which validate and, for FlowState, apply `migrateFlowStateV1` first. Stored rows are import-time-current by construction.
2. **"Read-modify-write across versions silently strips fields."** Rejected: validate-on-load rejects any non-current version before a save can occur on either backend, so an old binary cannot round-trip degrade a newer record; `productstate.Save`'s unconditional version overwrite is therefore largely unreachable in supported flows (folded into F1 as a missing belt-and-braces gate).
3. **"Two packages opening one SQLite file is a corruption hazard."** Rejected: both DSNs use WAL, busy timeout, immediate txlock, FULL sync (store.go:67-78; runcontrol sqlite.go), the deployment model is single-user local (recovery.md:226-228), and runcontrol explicitly handles the foreign-pre-existing-schema case with backup-before-create (migration.go:58-74).
4. **"Kind/scope strings duplicated across modules need a central registry."** Rejected: each owner defines its kind beside its domain (`study/state_database.go:13`, `sprint/state_database.go:12-15`) — proper module cohesion; only `storage migrate`, whose job is aggregation, enumerates kinds.
5. **"File checkpoints going stale after import breaks recovery."** Rejected as designed behaviour: checkpoints are written only at terminal states (`flowStateCheckpoint`, `executeStateCheckpoint`, `state.Complete`), the file path retains full legacy-upgrade capability, and deleting the DB cleanly re-enables file fallback (`productstate.Existing`). Undocumented, but coherent.

## Open questions

- Is the intended evolution model for `product_states` payload-shape-only (opaque JSON blobs, stable DDL forever)? If yes and it's written down somewhere I didn't find, F1 reduces to documentation debt; the generic blob design hints this was intended but no doc states it.
- Is old-binary/new-data compatibility for DB-authoritative workspaces in scope at all (single-user CLI, but workspace restore-after-upgrade and multi-checkout sharing make it plausible)? An explicit out-of-scope statement would downgrade F2 toward documentation.
