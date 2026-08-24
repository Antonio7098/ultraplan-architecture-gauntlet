### Scope inspected

Implementation repo (`ultraplan-go` @ eeaa034): `internal/productstate/store.go` (entire package), `internal/runcontrol/sqlite.go`, `migration.go`, `lifecycle.go`, `retention.go`, `interfaces.go`, `doc.go`, `import_boundary_test.go`, `sqlite_test.go`, `migration_test.go`; `internal/sprint/state.go`, `execute_state.go`, `state_database.go`, `locks.go`; `internal/study/state.go`, `state_database.go`; `internal/app/storage_commands.go`, `app.go`; `scripts/migrate-product-state.sh`; docs `architecture.md`, `recovery.md`, `migration-product-state.md`. Ran `go test ./internal/productstate/... ./internal/runcontrol/...`.

Planning workspace (`ultraplan-workspace` @ 368a789): `projects/ultraplan-go/docs/TRD.md` (§4, §18C, §18D), `roadmap.md` (Phase 6, Gate D), `sprints/32-hardening-and-release/*`, `sprints/35-durable-run-observability/requirements.md`.

### Architecture assessment

The two stores have very different maturity. `runcontrol` is a disciplined multi-process repository: fixed durability pragmas verified after open (`sqlite.go:207-227`), versioned schema with `user_version` + `app_schema` component registry (`sqlite.go:229-234`), file-locked migrations with stale-owner probing and bounded backups (`migration.go:25-165`), immediate short transactions with CAS `RowsAffected` checks and fencing everywhere (`sqlite.go:561-571, 677-685, 769-786`), conservative reconciliation that never infers success (`lifecycle.go:481-495`). Tests cover concurrent sequence allocation, single-winner terminal CAS, context/lock behavior, fault injection. This side is sound.

`productstate` is the opposite: a 206-line unversioned store with no tests, sharing the *same physical file* (`.ultraplan/run-control.db`) that runcontrol's migration, backup, restore, and quota machinery governs. The semantic decoupling claimed in `runcontrol/doc.go:3-6` and `docs/architecture.md:158-160` ("modules remain authoritative for their own product state") is enforced at import level (`import_boundary_test.go` forbids runcontrol importing product packages) but not at the physical level: one file, two owners, one governance regime. The stress point is exactly this physical-vs-semantic split.

### Candidate findings

---

**SPECIALIST-08B-F01**
- **Priority:** P1
- **Claim:** Product state lives inside the run-control database file but outside runcontrol's schema governance: unversioned DDL re-run on every open, never registered in `app_schema`, invisible to migration backups' intent, and silently reverted by `RestoreBackup`.
- **Evidence:** `productstate/store.go:19` duplicates `DatabaseRelativePath` (`runcontrol/sqlite.go:22`) as the only link between the packages; `store.go:92-116` runs bare `CREATE TABLE IF NOT EXISTS` on every open; `runcontrol/migration.go:167-173` (`hasApplicationSchema`) counts any non-`sqlite_%` object, so a workspace where product tables predate runcontrol init triggers WAL-checkpoint + backup of a database containing only foreign tables; `migration.go:238` verifies only component `'run_control'`; the `app_schema.component` PK (`sqlite.go:230-234`) shows multi-component intent that productstate never uses; `RestoreBackup` (`migration.go:296-352`) renames the whole file, including `product_states` rows.
- **Architectural reason:** ownership / boundary / drift
- **Concrete consequence:** After `storage migrate`, SQLite is canonical and JSON files are checkpoints written only at all-terminal states (`execute_state.go:112-115`, `state.go:222-225`). A documented restore (`docs/recovery.md:216-222`, framed as a *schema-migration* backup procedure) rolls canonical sprint/study execution progress back to an arbitrary pre-migration instant while stale checkpoint files are ignored by the DB-first loaders (`state.go:21-31`, `execute_state.go:36-47`) — silent loss with no re-import path (`storage_commands.go:98-101` skips stored records). Future evolution has no coordinated path: runcontrol bumps `user_version` for the whole file while productstate can only keep piling idempotent DDL forever. This also conflicts with the authoritative contract: `roadmap.md:1368` ("must not move Markdown, **flow outcomes**, … into a new canonical store") and Gate D (`roadmap.md:1462-1464`, TRD §18D.5) which explicitly reserve the product-SQLite authority decision.
- **Counter-evidence searched:** Implementation-side intent exists (`docs/migration-product-state.md:24-26` states SQLite becomes authoritative; commit 02e2ec4). TRD line 126's "explicit product-mode decision" arguably covers an operator-invoked migrate. Quota accounting consistently includes the whole file prefix (`retention.go:41-55`). The dual-write prohibition (TRD §18D.5 "silent dual writes") is respected — files are checkpoints, not parallel truth. None of this addresses the unregistered-component/restore-blast-radius mechanics, and no workspace doc authorizes Gate D work.
- **Confidence:** high (mechanics), medium (whether the contract conflict is intended debt)
- **Smallest useful action:** Register a `'product_state'` row in `app_schema` with its own version at `Ensure`, gate `RestoreBackup` output on warning that product records revert to the backup instant, and record the shared-file decision in the workspace reasoning docs.

---

**SPECIALIST-08B-F02**
- **Priority:** P2
- **Claim:** The file→DB authority switch is a check-then-act across process boundaries with no lock spanning it; `storage migrate` bypasses the product mutation leases entirely.
- **Evidence:** `sprint/state.go:216-227` and `study/state.go:59-63`: query `InDatabase`, then write to either DB or file based on the answer. `app/storage_commands.go:119,142-186` invokes `MigrateFlowStateToDatabase`/`MigrateExecuteRunStateToDatabase` directly — none of them acquire the cross-process mutation lease that all other mutators share (`sprint/locks.go:13-16`). `scripts/migrate-product-state.sh` adds no locking; `docs/migration-product-state.md` carries no "stop processes" warning (unlike restore, `docs/recovery.md:220`).
- **Architectural reason:** concurrency / boundary
- **Concrete consequence:** Interleave: CLI execute saves task progress (authority check says "file"); `storage migrate` imports the older snapshot concurrently; the newer file write lands after import. DB-first load now serves the stale imported record, and re-running migrate cannot heal it (`stored → skipped`). Silent divergence requiring manual surgery.
- **Counter-evidence searched:** Window is milliseconds and requires an operator to run an explicit offline command against an actively mutating sprint/study; per-sprint leases do serialize ordinary writers; both migrate runs racing each other converge (idempotent upserts). Real but low-probability trigger — hence P2, not P1.
- **Confidence:** high (mechanics), medium (operational exposure)
- **Smallest useful action:** Have `storage migrate` acquire the existing per-sprint/study mutation leases (or refuse when any is held), matching its own offline-procedure siblings.

---

**SPECIALIST-08B-F03**
- **Priority:** P3
- **Claim:** `Store.Load` reads header and items in two separate autocommit snapshots, so a concurrent `Save` commit between them yields a mixed record.
- **Evidence:** `productstate/store.go:127-148` — `QueryRowContext` for header, then `QueryContext` for items, no surrounding transaction; contrast with `Save`'s single immediate transaction (`store.go:154-205`). Concurrent readers/writers are a supported topology (`ultraplan serve` read-only dashboard, `app.go:171,269`, while CLI executes mutate).
- **Architectural reason:** transactional semantics / failure-semantics
- **Concrete consequence:** Dashboard load or startup reconciliation (`locks.go:46-67`) can observe header v1 + items v2; strict validators (`ValidateExecuteRunState`) turn the mix into spurious transient failures, lenient ones into briefly inconsistent status. Self-heals on next load; no durable corruption since writers are lease-serialized.
- **Counter-evidence searched:** WAL gives per-statement snapshots only in autocommit — confirmed mixing is possible; consumers validate loaded records, bounding damage; writer serialization prevents persistent inconsistency.
- **Confidence:** high (mechanics), medium (impact)
- **Smallest useful action:** Wrap both queries in one deferred-rollback read transaction inside `Load`.

---

**SPECIALIST-08B-F04**
- **Priority:** P3
- **Claim:** Drift detection is asymmetric between backings: flow-state file loads reject unknown fields, DB loads accept and silently drop them on rewrite.
- **Evidence:** `sprint/state.go:57-61` uses `DisallowUnknownFields` for file decode; `sprint/state_database.go:25,30,44,49` (and `study/state_database.go:30,35`) use plain `json.Unmarshal`. A binary downgrade reading a newer DB record ignores unknown fields, and the next save persists the struct without them.
- **Architectural reason:** drift / failure-semantics
- **Concrete consequence:** The strictness invariant that protects file-backed flow state disappears exactly where SQLite became canonical; field loss is silent instead of an `ErrFlowStateMalformed`-class error.
- **Counter-evidence searched:** Per-record `schema_version` is validated to exact current versions by consumers (`execute_state.go:184-189`), catching version-level drift; only field-level drift within a version slips through. Execute/study file paths were already lenient, so this widens rather than introduces leniency.
- **Confidence:** high
- **Smallest useful action:** Use `DisallowUnknownFields` decoders in `*_database.go` loads, matching the strictest existing file-path behavior.

---

**SPECIALIST-08B-F05**
- **Priority:** P2
- **Claim:** The newest durability seam has zero automated coverage: no tests exist for `internal/productstate`, nor for any DB-backed state path or the `storage migrate` command.
- **Evidence:** `go test ./internal/productstate/...` reports `[no test files]`; repo-wide grep for `storage migrate|MigrateFlowStateToDatabase|InDatabase|productstate` in `_test.go` returns nothing; sibling `runcontrol` has dedicated concurrency/fault/integrity suites.
- **Architectural reason:** change-surface
- **Concrete consequence:** The subtle semantics — hash-guarded upserts, stale-item GC (`store.go:159-204`), authority switching, torn-load behavior, permission handling — can regress unnoticed by any test in the repo; every future refactor of `Save`/`Load` or of the authority check in F02 is unguarded.
- **Counter-evidence searched:** Sprint/study tests exercise the file paths heavily but never the database paths; no build-tag or integration harness references the store.
- **Confidence:** high
- **Smallest useful action:** Add table-driven unit tests for `Save`/`Load` round-trip, stale-item deletion, invalid-record rejection, plus one integration test covering `storage migrate` skip/import/partial-failure statuses.

---

**SPECIALIST-08B-F06**
- **Priority:** P3
- **Claim:** Private-permission and pragma enforcement is one-sided on the shared file: runcontrol enforces and verifies on every open; productstate neither tightens existing modes nor verifies durability pragmas.
- **Evidence:** `runcontrol/sqlite.go:154-191` (`preparePrivateDatabase`: symlink rejection, `O_EXCL` 0600 creation, `enforcePrivateMode`) and `:104 verifyPragmas`; `productstate/store.go:64` only `MkdirAll(..., 0o700)` (no chmod of existing dir/file, no chmod ever of the DB file), DSN lacks `_defensive=1` (`store.go:67-72` vs `sqlite.go:73-80`). `Ensure` is exported and creates the database if absent, so any future caller ordering it before `OpenSQLite` produces a umask-default (~0644) planning-state database.
- **Architectural reason:** boundary / lifecycle
- **Concrete consequence:** Today safe only by incidental call order in `storage_commands.go:60-67`; the private-workspace invariant that runcontrol treats as a hard product invariant is unenforced on half the file's writers.
- **Counter-evidence searched:** All current shipped paths open runcontrol first; `Existing` never creates. So no live defect — latent coupling to call ordering.
- **Confidence:** high (code), low-medium (reachability)
- **Smallest useful action:** Apply `enforcePrivateMode`-equivalent chmod + pragma verification inside `productstate.open`.

### Defended architecture / rejected hypotheses

1. **"Silent dual-write violates TRD §18D.5."** Rejected: post-migration saves write the DB canonically and touch files only as all-terminal checkpoints (`state.go:222-225`, `execute_state.go:112-115`); unmigrated records stay purely file-backed. This is an explicit authority switch, not a shadow store.
2. **"Two connection pools on one SQLite file will corrupt state."** Rejected: both sides set WAL, `synchronous=FULL`, foreign keys, 5 s busy timeout, immediate write transactions; `TestSQLiteRepositoryConcurrentWritersAllocateMonotonicSequence` proves monotonic allocation across repositories, and multi-process same-host is the declared supported topology (`docs/architecture.md:188-191`).
3. **"productstate.Save needs runcontrol-style fencing/CAS."** Rejected as currently scoped: per-sprint/per-study cross-process mutation leases serialize writers (`sprint/locks.go:13-16`); adding fencing would duplicate runcontrol concerns the package deliberately excludes (`doc.go`). The residual gap is the lease-free migrate command (F02), not missing fencing in Save.
4. **"Migration backup logic mishandles foreign tables."** Partially rejected: `hasApplicationSchema` treating product tables as legacy application schema causes only a conservative extra backup/checkpoint before first runcontrol init — harmless, folded into F01 as evidence of the blind spot.
5. **"RestoreBackup should surgically preserve product tables."** Rejected: selective restore across independently-versioned schemas in one file is a second architecture; the smallest honest fix is registering the component version and documenting restore scope (F01 action).

### Open questions

- Does any Gate D evidence or later-sprint authorization for product-SQLite authority exist outside this workspace snapshot? That would downgrade the contract-conflict aspect of F01 (not its mechanical substance).
- Does the `serve` dashboard poll state frequently enough that F03's transient failures are operator-visible in practice?
- Is the shared-file choice in commit 02e2ec4 recorded anywhere beyond `docs/migration-product-state.md` (no reasoning document was found in either repo)?
