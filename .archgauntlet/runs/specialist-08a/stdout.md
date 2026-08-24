I'll investigate the SQLite product-state store in the target repository. Let me start by exploring the structure and locating relevant code.
The core file is small (206 lines). Now let me trace consumers, check the driver version, and find authoritative docs.
No tests exist for the productstate package. Now let me read the consumers and verify how the modernc.org/sqlite driver interprets those DSN parameters — that's critical for the concurrency claims.
The driver does support those params in v1.57.0. Let me confirm the exact semantics, then look at the study consumer and the storage command.
There's a second SQLite store (`runcontrol.OpenSQLite`). Let me compare the two, read the study consumer fully, and search workspace docs.
runcontrol has full schema governance; productstate doesn't. Critical question: do they share the same physical database file?
Same file, two schema regimes confirmed. Now let me check runcontrol's retention/quota behavior toward the shared file, and the read-precedence logic in sprint/study.
Read precedence: DB authoritative, file fallback. Now let me check the authoritative planning workspace for storage contracts.
Now let me verify concurrency controls (sprint locks), SchemaVersion usage on read, and who writes concurrently.
The DB load path skips all the version gating that the file path enforces. Let me confirm what role `path` plays in validators, then check runcontrol's pragma verification block for exact citation.
All evidence gathered. Here is my specialist report.

---

# UltraPlan Architecture Gauntlet — Specialist Report (specialist-08a)

## Scope inspected

**Implementation repo** (`ultraplan-go` @ eeaa034):
- `internal/productstate/store.go` (206 lines — entire package; no tests exist)
- Consumers: `internal/sprint/state_database.go`, `internal/sprint/state.go` (`LoadFlowState`:20-77, `SaveFlowState`:201-237, `ValidateFlowState`:291+), `internal/sprint/execute_state.go` (:35-67, :105-131, :184-215), `internal/study/state_database.go`, `internal/study/state.go` (`SaveRunState`:59+, `ValidateRunState`:121+), `internal/app/storage_commands.go`
- Sibling store for comparison: `internal/runcontrol/sqlite.go` (DSN :74-99, `verifyPragmas`:207-228), `internal/runcontrol/migration.go` (whole file), `internal/runcontrol/retention.go`
- Locking: `internal/sprint/locks.go` (mutation lease), `internal/study/cleanup_uncertain.go`, `internal/study/locks.go`
- Driver verification: `modernc.org/sqlite@v1.57.0/sqlite.go` DSN param handling (~:249-433) in module cache
- History: `git log/show 02e2ec4` ("Move mutable product state to SQLite", 2026-08-21 17:57:45), `e09d394` ("Add durable run control…", 17:26:41), `docs/migration-product-state.md`

**Planning workspace** (@ 368a789): `projects/ultraplan-go/docs/TRD.md` (SQLite authority gates, lines 126, 2235, 2239), `docs/PRD.md` (:181, :1189), sprint 35 requirements, roadmap.md.

## Architecture assessment

The product-state store itself is a deliberately minimal, well-scoped KV-with-items design: `(kind, scope)` records, JSON header + ordered item payloads, hash-guarded idempotent upserts, full-replace semantics inside one transaction (`store.go:150-205`). It correctly leaves payload semantics (marshaling, validation, schema gating) to the owning modules (sprint, study) — that division is sound module-driven ownership, not a missing abstraction.

What is stressed is the **relationship between this store and its physical container**, and the **durability contract at the authority switch point**:

1. The store shares one physical SQLite file with `runcontrol`, but only runcontrol governs that file (versioning, locks, backups, restore, quota). productstate co-locates by an independently-declared constant with none of that governance.
2. Authority flips **per record** ("SQLite authoritative once imported", per `docs/migration-product-state.md`), but while a sprint is mid-execution the DB is the *only* durable copy (checkpoint files are written only when every stage/task is terminal — `executeStateCheckpoint`/`flowStateCheckpoint`). Any whole-file lifecycle operation owned by runcontrol therefore silently destroys product-state authority.
3. Reads are two autocommit statements — a snapshot-consistency gap in a WAL database designed for concurrent access.
4. The entire DB-backed mode has zero test coverage, while the file mode it supersedes is heavily tested.

## Candidate findings

---

### SPECIALIST-08A-F01

- **Priority:** P1
- **Claim:** `productstate` and `runcontrol` own schemas in the same physical database file under irreconcilable lifecycle regimes: runcontrol implements whole-file versioning/backup/restore/quota; productstate adds tables to that file outside all of it, so runcontrol's sanctioned recovery operations silently destroy product-state authority and future schema evolution has no owner.
- **Evidence:**
  - Duplicate independent constants naming the identical file: `internal/productstate/store.go:19` and `internal/runcontrol/sqlite.go:22`, both `.ultraplan/run-control.db`.
  - HISTORY: committed 31 minutes apart on 2026-08-21 (`e09d394` 17:26:41, then `02e2ec4` 17:57:45); the migration doc calls it "the workspace database" — co-location is intentional, but governance was not extended.
  - runcontrol whole-file machinery: `PRAGMA user_version` + `app_schema(component='run_control')` (`migration.go:20,84-90,236-248`), migration lock file (`migration.go:92-138`), pre-migration backup (`migration.go:58-74,186-200`), **`RestoreBackup` replaces the entire file via rename** (`migration.go:296-352`), retention/quota measures whole-file bytes and runs `PRAGMA incremental_vacuum` (`retention.go:39,152`).
  - productstate side: bare `CREATE TABLE IF NOT EXISTS` (`store.go:92-116`); no version stamp, no migration hook, no lock, no participation in backups.
  - Consequence anchor: mid-execution execute state exists **only** in the DB (`sprint/execute_state.go:105-118` skips the file write unless all tasks terminal; `sprint/state.go:216-227` likewise). A `RestoreBackup` of an initial-schema `.bak.` (which by construction predates every `storage migrate` import) reverts the file to pre-import state: all product-state records vanish, loads silently fall back to stale-or-missing checkpoint files, and in-flight task ownership is unrecoverable.
  - Reverse coupling: `integrity_check` at every runcontrol open covers the whole file (`migration.go:250-266`), so corrupted *product* tables block the server's run-control store it doesn't own; conversely product-state growth counts against run-control quota and can trigger `CodeQuota` attributed to run events.
- **Architectural reason:** ownership / lifecycle (two authorities over one durable substrate; whole-file operations escape one owner's model)
- **Concrete consequence:** Operator follows documented recovery ("stop processes, restore matching backup") after a bad upgrade; server restarts clean, but every migrated sprint resumes from stale terminal checkpoints or loses resumable execute state entirely, with no error pointing at the cause. Independently: the first future `user_version=2` bump will "migrate the workspace database" without any contract about the three product tables living beside it, and productstate can never use `user_version` itself because runcontrol owns it.
- **Counter-evidence searched:** `docs/migration-product-state.md` confirms sharing is deliberate and files remain checkpoints — intent acknowledged, but the doc is silent on backup/restore/migration interplay; checked whether RestoreBackup restricts to run-control-only content (it doesn't — file-level copy); checked whether backups are retaken after imports (no — backups are created only on initial schema creation); checked TRD line 2235 ("select one product-artifact authority… prohibit silent dual writes") — the implementation satisfies the authority rule but violates the spirit of managed composition by splitting stewardship of one file.
- **Confidence:** high (mechanics verified end-to-end; severity depends on how often RestoreBackup/schema-bump paths get exercised)
- **Smallest useful action:** Register product state as a governed component in the existing regime: stamp `app_schema(component='product_state', version=…)` alongside runcontrol's record and have `RestoreBackup`/backup docs enumerate both components (or make productstate open refuse when the app_schema record disagrees). Do **not** split into a second database file — that would break the documented "workspace database" intent.

---

### SPECIALIST-08A-F02

- **Priority:** P2
- **Claim:** `Store.Load` reads the header and the items as two separate autocommit statements, so under WAL each statement observes a different snapshot; a concurrent `Save` between them yields a torn record (header from commit N, items from commit N±1) that consumers treat as authoritative truth.
- **Evidence:** `store.go:127-148` — `QueryRowContext` for `schema_version, header_json`, then `QueryContext` for items, no `BeginTx`. `Save` writes header+items atomically in one tx (`store.go:154-205`), so the stored data is consistent; only the read tears. Reachable across processes: e.g., CLI status/review commands call `LoadFlowState` without holding the sprint mutation lease while a server-side lease holder saves (`locks.go:13-16` protects writers, not readers); startup reconciliation makes recovery decisions from exactly these fields (`locks.go:46-78` flips running tasks to failed based on loaded items/header).
- **Architectural reason:** boundary / failure-semantics (consistency contract of the read API weaker than the write API it mirrors)
- **Concrete consequence:** A reader pairs new header (e.g., updated attempt metadata) with old task rows during resume/reconcile, mis-classifying which tasks were running — the exact scenario `ReconcileInterruptedMutation` exists to adjudicate — producing wrong interrupted-evidence instead of a detectable error. Low probability per read, silent when it hits.
- **Counter-evidence searched:** Checked whether `database/sql` implicitly wraps the two queries (it doesn't — no tx in scope); checked whether all read sites hold the mutation lease (they don't — status/report/read paths in `review.go`, `smoke.go`, `service.go` call `LoadFlowState` lock-free); checked driver snapshot semantics (standard WAL per-statement read snapshots).
- **Confidence:** high on mechanism, medium on practical impact frequency
- **Smallest useful action:** Wrap Load's two statements in one deferred read transaction (snapshot isolation) — a few lines inside `store.go`, no API change.

---

### SPECIALIST-08A-F03

- **Priority:** P2
- **Claim:** The DB-backed persistence mode — authority switching, dual-write/checkpoint rules, import, and the store itself — has zero test coverage, while the file mode it overrides is extensively tested; the riskiest transition in the state lifecycle is the least protected.
- **Evidence:** `grep -rln productstate --include="*_test.go"` over the repo returns nothing; `internal/productstate/` contains only `store.go`; greps for `InDatabase|Migrate*ToDatabase` in tests return nothing. Contrast: `internal/runcontrol/sqlite_test.go` (470 lines), `fault_test.go`, `migration_test.go`, `lifecycle_test.go`.
- **Architectural reason:** change-surface (the branch `authoritative → saveFlowStateDatabase (+conditional checkpoint)` in `sprint/state.go:216-227` and `execute_state.go:106-118` runs on every mutation post-import yet is exercised only by real workspaces)
- **Concrete consequence:** Any refactor of `Save`'s hash-guarded upsert, stale-item sweep, or the precedence logic regresses invisibly — e.g., breaking the stale-item delete would leave ghost tasks that validators don't catch until reconcile; nothing catches it before a user's workspace is migrated. Change cost is amplified precisely because the code is correct-by-inspection today.
- **Counter-evidence searched:** Looked for coverage via web integration tests, app-level tests, fault tests — none touch this path; looked for a documented decision to defer tests (none found; migration doc describes behavior as finished).
- **Confidence:** high
- **Smallest useful action:** Table-driven tests around the three pinned behaviors: save→load round-trip incl. stale-item removal; authority switch (file ignored once record present; terminal-only checkpointing); torn-read guard from F02 once added.

---

### SPECIALIST-08A-F04

- **Priority:** P3
- **Claim:** `storage migrate` flips per-record authority (file → DB) while acquiring neither the sprint mutation lease nor the study run-loop lock nor any quiesce guidance, unlike every other writer and unlike runcontrol's migration discipline on the same file.
- **Evidence:** `internal/app/storage_commands.go:85-121,142-186` — stat file → `InDatabase?` → load → import with no `acquireMutationContext`/`AcquireRunLoopLock`; help text (`storageHelp()`:199-207) says nothing about stopping the server; contrast runcontrol's "stop other UltraPlan processes" posture (`migration.go:33-35,181`) and sprint writers' unconditional leasing (`service.go` 56 call sites).
- **Architectural reason:** boundary / failure-semantics
- **Concrete consequence:** Running `storage migrate` against an actively executing sprint: migrate imports file snapshot C1; the lease-holding server (still file-mode for that record until it re-checks `InDatabase`) writes C2 to the file; DB now holds older C1 as authority; a crash before the server's next save resumes from C1, losing C2's progress silently. Narrow window, single-user tool hence P3.
- **Counter-evidence searched:** Verified the web server cannot be mid-write without holding the lease (all sprint saves go through leased service methods); verified import is idempotent on re-run (`skipped` when `InDatabase`), limiting damage duration; checked docs for a stated quiesce requirement (absent).
- **Confidence:** medium-high
- **Smallest useful action:** Either acquire the existing leases per record during import, or document/enforce "server stopped" in the command (one help line + a cheap lock probe).

---

### SPECIALIST-08A-F05

- **Priority:** P3
- **Claim:** productstate surfaces raw driver errors and trusts DSN pragmas blindly, giving the shared file two different failure vocabularies: runcontrol classifies `SQLITE_BUSY` as retryable `CodeBusy`, verifies durability pragmas at open, and types every failure; productstate leaks opaque strings after a 5s busy timeout.
- **Evidence:** `store.go:67-82` sets `_busy_timeout/_journal_mode/_synchronous/_txlock` but never verifies them (contrast `runcontrol/sqlite.go:207-228` verifying `journal_mode=wal, synchronous=2, foreign_keys=1, busy_timeout=5000` and `Health()` reporting them :857-867); errors returned verbatim from `database/sql` (e.g., `store.go:164,178,205`) with no classification, versus `classifyStoreError` codes throughout runcontrol.
- **Architectural reason:** failure-semantics / drift (divergent contracts over one substrate)
- **Concrete consequence:** When `storage migrate` contends with a live server past 5s, the user sees `sqlite: database is locked (5) (SQLITE_BUSY)` with exit classification `ExitWorkspace` and no retry guidance, while the equivalent condition in run-control paths reports typed, actionable busy errors. A future driver/DSN regression (e.g., journal mode silently not applied) would go undetected in productstate while remaining guarded in runcontrol.
- **Counter-evidence searched:** Confirmed the driver does honor all five params in v1.57.0 (see rejected hypotheses) — so this is inconsistency, not malfunction; confirmed no other error-mapping layer sits between productstate and CLI/web surfaces.
- **Confidence:** high on observation, low-medium on impact
- **Smallest useful action:** Reuse the pragma-verification idea (a 6-line check after open) and map busy/timeouts to a sentinel error (`ErrBusy`) consumers can classify; full error-code taxonomy is not warranted for this package size.

## Defended architecture / rejected hypotheses

- **"The modernc DSN pragmas are silently ignored"** — Disproved. `modernc.org/sqlite@v1.57.0/sqlite.go` explicitly handles `_busy_timeout`/`_timeout`, `_foreign_keys`/`_fk`, `_journal_mode`/`_journal`, `_synchronous`/`_sync`, `_txlock` (~lines 307-390 of the module source). The concurrency configuration is real.
- **"SchemaVersion is stored but never enforced on DB reads"** — Disproved. Although `record.SchemaVersion` is unused after load, all three DB load paths validate the unmarshaled header against current constants: `ValidateFlowState` rejects `!= FlowStateSchemaVersion` (`sprint/state.go:293-295`, invoked at :28), `ValidateExecuteRunState` likewise (`execute_state.go:186-190`, invoked at :43), `ValidateRunState` for studies (`study/state.go:123-127`). Version gating travels inside `header_json` and works; the redundant column is inert, not unsafe.
- **"Last-writer-wins Save enables lost updates"** — Largely mitigated by design. Sprint mutations serialize through the cross-process product-owned mutation lease (`locks.go:13-16`); study saves occur under the run-loop lock (`cleanup_uncertain.go:74-87`, `run_loop.go`). The DB-level LWW is a backstop within a lease holder, not the primary guard. Residual exposure is limited to F04's unlocked importer.
- **"Read paths create the database as a side effect"** — Disproved. `Existing` stats first and returns disabled when absent (`store.go:41-51`); only explicit write/import paths call `Ensure`.
- **"Concurrent Ensure/first-open corrupts schema"** — Not a defect. `CREATE … IF NOT EXISTS` under SQLite write serialization plus 5s busy timeout handles multi-process cold start; the per-process `sync.Map` cache (`store.go:39,60-62,84-88`) correctly collapses pools and closes race losers.
- **"Two stores should be two packages' worth of shared abstraction (common sqlite kit)"** — Rejected as a fix. The overlap (DSN construction, MaxOpenConns=4, WAL settings) is ~15 lines; extracting a platform-sqlite package now would add indirection without resolving F01's actual problem, which is governance ownership, not code sharing.

## Open questions

- Is `RestoreBackup` intended as whole-workspace-authoritative recovery (making product-state loss accepted-but-undocumented), or should it be record-component-scoped? Answer determines whether F01's smallest action is a doc fix or a schema-registry change.
- Does any roadmap item plan product-state schema evolution (new columns/kinds)? If SQLite authority remains frozen at v1 per the TRD evidence gate (TRD.md:2239), F01's migration half drops from P1 mechanics to latent risk.
