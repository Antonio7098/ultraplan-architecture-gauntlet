Prior outputs inventoried. Now let me re-derive key claims firsthand. Starting with the core store and migration files:
Core store/state files verified firsthand. Now dispatching parallel bounded-discovery subagents while I continue reading the runcontrol core myself:
Two reports returned with rich evidence. Dispatching the remaining two bounded-discovery agents, then verifying key claims myself:
Tests agent confirmed zero-coverage TRUE. Docs agent was rate-limited — retrying while I verify remaining claims myself:
Rate-limited on subagents — proceeding with direct reads myself:
All evidence verified firsthand. Writing the tribunal report.

# CHALLENGE-03 — State tribunal (filesystem, SQLite/productstate, migrations, durable/derived/ephemeral truth, consistency, recovery)

### Scope inspected

**Implementation repo** `/home/antonioborgerees/coding/ultraplan/gauntlet-run/ultraplan-go` @ eeaa034 (clean; read-only):

- Read in full or key-region, firsthand: `internal/productstate/store.go` (entire package, 206 lines), `internal/runcontrol/migration.go` (363 lines, full), `internal/runcontrol/sqlite.go` (DSN :59–80, `preparePrivateDatabase` :152–196, `verifyPragmas` :207–227, schema CHECKs :229–271, Append sanitize call :617, `ProposeTerminal` CAS :769–786, Health :856–914, `classifyStoreError` :1085–1108), `internal/runcontrol/lifecycle.go` (`Reconcile` entry :301–324, `reconcileUnclaimed` INSERT+CAS :452–470), `internal/sprint/state.go` (full), `internal/sprint/state_database.go` (full), `internal/sprint/execute_state.go` (full), `internal/sprint/locks.go` (full), `internal/sprint/code_context.go` (:270–513 incl. `promoteCodeContext`), `internal/study/state.go` (:20–75), `internal/study/state_database.go` (full), `internal/study/run_history.go` (full), `internal/app/storage_commands.go` (full)
- Docs/scripts: `docs/recovery.md` (:40–120, :180–233), `docs/migration-product-state.md` (full), `scripts/migrate-product-state.sh` (10 lines), `internal/platform/filesystem/doc.go`, workspace contract `system/contracts/runtime/persistence-and-migrations.md` (via subagent, verbatim quotes returned)
- Tests: `internal/runcontrol/migration_test.go` (:100–155 incl. `TestBackupRestoreFixture`), `run_history_test.go`; repo-wide greps for `-wal`/`-shm`, `productstate` in `*_test.go`, `wal_checkpoint`, `LoadFlowState(` callers
- Commands: `go build ./...` (OK), `go vet ./internal/{productstate,runcontrol,sprint,study}/...` (OK), `go test ./internal/{productstate,runcontrol,sprint,study}/...` (pass except one unrelated scheduling-order failure in study, noted below), `git log --oneline --follow` on `productstate/store.go` (single commit `02e2ec4`) and `migration.go` (untouched since `e09d394`)
- **Subagents**: filesystem-writer inventory (all durable write sites across sprint/study/project/workspace/app with atomicity classification + contract quotes); SQLite co-tenancy verification (retention, health, error taxonomy, sidecar handling, ownership comments); test-coverage counter-evidence hunt (verdict: zero-coverage claim TRUE)
- **Prior outputs read**: scout-05, specialist-08a, failure-09, failure-10, failure-12, change-05, challenge-01/02 (style/provenance), INDEX.md

Every finding below was re-derived from primary source before inclusion; provenance to prior stages is preserved explicitly.

### Architecture assessment

The durable-state core is genuinely disciplined and survived adversarial re-derivation: runcontrol mutations are single immediate transactions with fence verification and CAS row-count checks; the initial schema migration wraps DDL + `app_schema` stamp + `user_version` in one commit (`migration.go` via `createInitialSchema`); corrupt databases are rejected byte-for-byte intact (`TestMigrationRejectsCorruptDatabaseWithoutReplacingEvidence`); product-file writers overwhelmingly use temp+fsync+rename+dir-fsync; authority precedence (DB wins once a record exists, files are terminal-boundary checkpoints) is implemented identically in all three loaders (`sprint/state.go:20–32`, `sprint/execute_state.go:35–47`, `study/state.go:26–35`) and matches the documented rule ("After import, SQLite is authoritative", `docs/migration-product-state.md`). Module-driven ownership here is sound, not a missing abstraction.

Stress concentrates in three places: (1) the **recovery surface itself** — the shipped restore path has a stale-sidecar hole, no executable entrypoint, and the one test that exercises it carefully avoids the crash condition where the hole bites; (2) the **co-tenancy seam** — `productstate` lives inside runcontrol's physical file while participating in none of its lifecycle machinery, with zero test coverage anywhere on the DB-backed product path (counter-evidence hunt confirmed TRUE, not narrowed); (3) two **append-ledger/durability edges** (study history JSONL, execute summary) where the write discipline drops below the codebase's own standard. Prior-stage findings in my domain were mostly accurate; two carried overstated severity framings, and one carried an understated impact rationale (corrected below).

Side observation (REALITY, outside my domain, for the quality tribunal): `go test -count=1 ./internal/study` fails deterministically on `TestRunLoopStartsPriorityTierBeforeLaterTiers` (`run_loop_test.go:303`, scheduling-order assertion); cached runs mask it.

### Candidate findings

---

**ID: CHALLENGE-03-F01**
**Priority: P2**

**Claim:** The offline restore path replaces `run-control.db` by rename but never removes stale `run-control.db-wal`/`-shm` sidecars, so restoring a pre-incident backup after an unclean writer death is silently undone (or mixed) by SQLite WAL replay on next open — precisely the disaster scenario restore exists for.

**Evidence:**
- `RestoreBackup` (`internal/runcontrol/migration.go:296–352`): validates backup integrity, copies to temp, fsyncs, `os.Rename(tempPath, databasePath)` (:347), chmod — no reference to sidecars anywhere.
- Repo-wide grep: zero occurrences of `-wal`/`-shm` in any `.go` file or doc/script except `scripts/reset-opencode-db.sh` (different database). Nothing in the codebase ever clears them for `run-control.db`.
- SQLite semantics (modernc.org/sqlite v1.57.0, `go.mod:14`): WAL recovery on open validates frames against the WAL's own salts/checksums, not against the main file it now sits beside; a non-empty stale WAL shadows restored pages. Migration backups are taken immediately after `PRAGMA wal_checkpoint(TRUNCATE)` (`migration.go:64–67`), so a surviving post-crash WAL contains exactly the frames newer than any backup — replay reconstructs the pre-restore state.
- The only restore test defuses the trap: `TestBackupRestoreFixture` calls `checkpointWAL` then `repository.Close()` *before* restoring (`migration_test.go:124,133`) — a clean last-connection close deletes the WAL, so the crash condition is never exercised.
- Compounding: recovery.md instructs operators to "use the tested restore path" (`docs/recovery.md:217–224`), but `RestoreBackup` has no CLI surface (confirmed: sole caller is its own test) — operators will hand-copy, and recovery.md warns only "Never copy only the database while a WAL writer is active", not "remove `-wal`/`-shm` left by a dead process".

**Architectural reason:** failure-semantics / lifecycle — the recovery operation's contract ("offline, matching backup") is necessary but insufficient; durability state survives in sidecars the operation does not model.

**Concrete consequence:** power loss or SIGKILL during writes → integrity failure or bad upgrade → operator follows the runbook, restores the timestamped backup, restarts; the first open replays the crashed era's committed transactions back over the rollback. `integrity_check` passes; `ultraplan health` reports OK; the rollback silently never happened (or pages mix eras if the WAL predates the backup point). Diagnosis points anywhere but the sidecar files.

**Counter-evidence searched:** clean last-connection close deletes the WAL (so graceful shutdown + restore is safe — narrows the trigger to unclean death, which is when restores happen); a zero-length/TRUNCATEd WAL is inert; checked whether `validateBackupIntegrity` or `preparePrivateDatabase` touch sidecars (they do not); checked whether `_defensive` or query-only flags alter recovery (they do not). No prior stage reported this.

**Confidence:** high on mechanism, medium on reachability (requires unclean death + subsequent restore).

**Smallest useful action:** after the rename in `RestoreBackup`, remove `<db>-wal` and `<db>-shm` (safe under the documented all-processes-stopped contract; discarding post-backup WAL frames is the intent of rollback), and add the sidecar rule as one bullet beside `recovery.md:222`.

---

**ID: CHALLENGE-03-F02**
**Priority: P3**

**Claim:** The study history ledger (`studies/<s>/.ultraplan/runs/tasks.jsonl`) is appended without fsync and parsed strictly, so a torn trailing line (ENOSPC partial write or power loss) permanently poisons every consumer — the run loop refuses to start or finish until an operator hand-repairs the file, with no recovery-doc guidance.

**Evidence:**
- Append: `os.OpenFile(..., O_APPEND)` + single `Write(data+'\n')`, no `Sync()` (`internal/study/run_history.go:94–105`). A short write under ENOSPC leaves a partial line on disk while returning an error.
- Strict reader: `readRunHistory` fails the whole parse on any malformed line (`run_history.go:208–210`); `readRunHistoryKeys` propagates (`:228–230`).
- Fail-closed consumers: run-loop startup aborts on `SyncRunHistory`/key-read error (`internal/study/run_loop.go:70–72`), and the final sync aborts completion (`:389–391`); `AppendRunHistory` fails the same way (`run_history.go:74–79`).
- No tail-tolerance anywhere (no truncate-on-parse-error, no skip-last-line logic); `go test ./internal/study` covers happy path + dedup only (`run_history_test.go:11–86`); `docs/recovery.md` documents repair paths for every other durable artifact but never mentions `tasks.jsonl`.

**Architectural reason:** failure-semantics / durability — a long-lived ledger ("rendered as Ledger", consumed for restart-safe idempotency keys) is written below the codebase's own temp+fsync+rename standard, yet read stricter than any other log in the tree (`run-loop-memory.jsonl` deliberately swallows errors; `tasks.jsonl` errors are fatal).

**Concrete consequence:** disk-full during a long study (quota pressure is an explicitly supported condition — recovery.md has a quota section) tears line N; every subsequent `study <s> run-loop` dies at startup with a raw `json.Unmarshal` error pointing at no file and no remedy; cost/token history and dedup become unreachable until manual surgery.

**Counter-evidence searched:** dedup keys give idempotent re-append (partial reconciliation *after* successful parse — does not help a poisoned parse); single small writes are atomic between processes (irrelevant to page-cache/power-loss tearing and ENOSPC short writes); considered whether callers downgrade history errors to warnings (they do not — both sites return).

**Confidence:** high on mechanism; low-medium on frequency.

**Smallest useful action:** treat only the final line as tolerable when malformed (skip-with-warning, standard append-log repair) in `readRunHistory`, plus one recovery.md bullet; optionally fsync the append.

---

**ID: CHALLENGE-03-F03**
**Priority: P2**

**Claim:** Adjudication of the co-tenancy cluster: `productstate` deliberately shares runcontrol's physical file (`store.go:19` == `sqlite.go:22`, committed 31 min apart, HISTORY) but participates in none of the file-level lifecycle machinery its co-owner built — no `app_schema` component registration, no migration-lock coordination for its open-time DDL, no quota attribution, no restore semantics, and no version gate on DB loads. The design is defensible; the missing registration is the actual defect. Upstream P1 framings are recalibrated down; the today-sharp edges stay.

**Evidence (mechanics re-derived firsthand):**
- Ungoverned tenant: bare `CREATE TABLE IF NOT EXISTS` executed on every fresh-process open including read paths (`store.go:79,92–116`; readers route through `Existing`→`open`: `study/state_database.go:18`, `sprint/state_database.go:58`), versus runcontrol's `user_version` + `app_schema(component='run_control')` + lock + bounded backups + integrity gate (`migration.go:19–90,236–266`). No comment or doc acknowledges the sharing (grep; `runcontrol/doc.go` scopes ownership to "operational run identity" only).
- Quota coupling is real but benign-in-direction: `storageBytes()` sums every regular file prefixed `run-control.db*` (`retention.go:35–56`), so product growth consumes run-control quota and can trip `CodeQuota` attributed to run events; compaction SQL touches only `events`/`runs`.
- Version asymmetry: DB loaders ignore `record.SchemaVersion` entirely (`state_database.go` ×3) while validators accept only the current constant (`sprint/state.go:295–296`, `study/state.go:125–126`, `execute_state.go:188–189`) — the first future payload-version bump hard-fails DB-authoritative workspaces that the file path auto-heals via `migrateFlowStateV1` (`state.go:68–73,150–192`). Not reachable today (single commit wrote the store; imports go through validating loaders), so the evolution half is structural debt, not live breakage.
- Today-sharp edges confirmed separately: silent fallback to stale frozen checkpoints when the DB record disappears with no staleness signal (FAILURE-12-F01, verified: pure row-existence authority via `Has`, checkpoint files written only at terminal states — `state.go:63–65`, `state.go:222–224`, `execute_state.go:112–114`); `storage migrate` imports without mutation lease or quiesce guidance (`storage_commands.go:33–140`; help text :199–207 silent; wrapper script is bare exec) — importer-vs-server race bounded but real (SPECIALIST-08A-F04).
- Zero coverage confirmed by exhaustive counter-evidence hunt: package has no test files; no `*_test.go` references `productstate`/`InDatabase`/`Migrate*ToDatabase`/`storage migrate`; app/web tests build the shared DB exclusively through `runcontrol.OpenSQLite`, so product branches are transitively unreachable in CI; `git show --stat 02e2ec4`: 766 insertions, zero tests, none deleted since.

**Architectural reason:** ownership / boundary — one substrate, two owners, one unaware of the other's protocol; change-surface for the untested half.

**Concrete consequence:** the first product-schema evolution or the first real restore/migration accident lands on a persistence layer with no governance hook, no test, and diagnostics that attribute its failures to the wrong subsystem — and today, a documented recovery procedure can silently regress live workspaces to weeks-old checkpoints with status surfaces presenting them as current (F12-F01).

**Counter-evidence searched:** co-location is intentional per `docs/migration-product-state.md` and the migration plan (FUTURE-INTENT embedded-schema work exists in plans — not reportable as absent); `migrateSchema` handles a foreign pre-existing schema safely (backup-before-create, `migration.go:58–74`), so fresh-workspace ordering is defended; `RestoreBackup` produces a coherent whole-file snapshot (see rejected hypotheses); immediate-txlock + busy timeout + O_EXCL creation make dual-pool corruption unlikely (fault posture verified in `fault_test.go` scope). What survives challenge is the registration/test gap and the two sharp edges above — hence P2 cluster, not upstream P1s.

**Confidence:** high on structure; calibration medium-high.

**Provenance:** confirms-and-recalibrates SPECIALIST-08A-F01 (P1→cluster-P2), CHANGE-05-F1/F2 (stand as the evolution half), FAILURE-12-F01/F03 (stand as the sharp edges), SPECIALIST-08A-F03/CHANGE-05-F3 (zero-coverage verdict independently reproduced TRUE).

**Smallest useful action:** register a `product_state` component row in the existing `app_schema` ledger inside `createSchema` and have restore/quota documentation enumerate both components; add store round-trip + load-precedence + terminal-checkpoint-gating tests; one quiesce line in `storage migrate` help. No second database file, no shared sqlite-kit abstraction.

---

**ID: CHALLENGE-03-F04**
**Priority: P3**

**Claim:** `WriteExecuteSummary` writes governed evidence with plain `os.WriteFile` (`internal/sprint/execute.go:583–586`) — confirming FAILURE-09-F04's mechanism, but correcting its severity rationale: `execute.md` is not a regenerable bystander; its exact bytes are fingerprinted into the frozen review manifest and gated by verify, so a torn summary can enter the evidence chain undetected.

**Evidence:**
- Non-atomic write firsthand: `execute.go:586 os.WriteFile(path, ...)`; contrast sibling writers (`atomicWriteFile` smoke.go:692–724, `atomicWriteReviewWithHooks` review.go:1686–1717, `saveFlowStateWithHooks` state.go:239–289 — all temp+fsync+rename+dirsync).
- Governed status (missed by FAILURE-09-F04's counter-evidence): review input fingerprint manifest includes `{"execute","governed",StageExecute}` (`review.go:219`); approved verification commands must be recorded *in* execute.md (`review.go:333–337`); `requireCompleteExecute` gates on non-empty execute.md (`verify.go:127–130`).
- Ordering: called only after `SaveExecuteRunState` commits (`execute.go:70–75, :317`), so machine truth (.run-state.json) is safe; the error-after-commit uncertainty of FAILURE-09-F02 applies here too.

**Architectural reason:** failure-semantics / drift — one governed artifact written below the contract its siblings meet (contract wording: "temporary files plus same-directory rename for important file state where practical", PERSIST-ATOMIC-001 Required clause).

**Concrete consequence:** crash or disk-full mid-write leaves truncated `execute.md`; the verify gate's emptiness check passes on partial content and review freezes a fingerprint over garbage bytes; staleness detection only fires on *later* edits, so the corrupted evidence can ride through one full review cycle.

**Counter-evidence searched:** artifact is regenerable via `flow --to execute` and recovery.md documents reruns (bounds blast radius, keeps this P3); checked whether any validator hashes execute.md at gate time (none found beyond the fingerprint freeze).

**Confidence:** high on facts, low on impact frequency.

**Provenance:** confirms FAILURE-09-F04; corrects its "not validated/not truth" mitigation.

**Smallest useful action:** route `WriteExecuteSummary` through the existing `atomicWriteFile`.

---

**ID: CHALLENGE-03-F05**
**Priority: P3**

**Claim:** Confirmed with downward calibration: `productstate.Store.Load` reads header and items as two autocommit statements (`store.go:127–148`, no tx), so under WAL a concurrent `Save` between them yields a torn record; real but narrow, because every writer is lease-serialized and the window is two adjacent statements.

**Evidence:** mechanism verified firsthand (no `BeginTx` in Load; `Save` fully transactional :150–205). Lock-free production readers exist: `smoke_protocol.go:177`, `verify.go:150`, `service.go:151` (status), `review.go:654,693,828,1195` — none hold the mutation lease; writers do (`locks.go:13–15`, ~56 leased call sites).

**Architectural reason:** consistency — read API weaker than the write API it mirrors, at the boundary that feeds reconcile decisions (`ReconcileInterruptedMutation` classifies from loaded items, `locks.go:46–78`).

**Concrete consequence:** a torn read during concurrent server-side save could flip running→failed classification on wrong item/header pairing — silent, durable wrong evidence. Probability: two statements racing one commit while a CLI reader overlaps a web save; rare in a single-user tool.

**Counter-evidence searched:** no implicit wrapping by `database/sql` (none); no reader-side retry/validation that would catch pairing (validators check shape, not cross-statement coherence); hash columns are write-skip metadata, never verified on load (`store.go:99,109` unused at :127–148 — folds in FAILURE-12-F02's observation).

**Confidence:** high on mechanism, low on practical incidence. Calibrated P2→P3 relative to SPECIALIST-08A-F02 given lease serialization of all writers.

**Smallest useful action:** wrap Load's two statements in one deferred read transaction (~3 lines).

### Defended architecture / rejected hypotheses

1. **"RestoreBackup destroying product-state authority" as a P1 current defect (SPECIALIST-08A-F01's headline framing).** Rejected as framed: restore yields a coherent point-in-time snapshot of the whole file (both tenants included); recovery.md provides the re-derivation path (`sprint status` persists derived stage state); the Gate-D dual-write prohibition is explicitly FUTURE-INTENT and unopened; only one schema generation exists. What survives is F03's registration gap and F01's sidecar hole — narrower, and differently fixed.
2. **"The modernc DSN pragmas are ignored / concurrency config is fake."** Disproved: driver v1.57.0 honors `_busy_timeout/_foreign_keys/_journal_mode/_synchronous/_txlock`; runcontrol additionally verifies them at open (`sqlite.go:207–227`), productstate merely doesn't (asymmetry already reported by FAILURE-09-F01, standing).
3. **"Two connection pools on one file is a corruption hazard."** Largely rejected: O_EXCL creation, immediate txlock, busy timeout, and `CREATE IF NOT EXISTS` serialize correctly; residual windows (DDL interleaving a migration backup copy; cold-start `CodeBusy`) are fail-closed or already reported (FAILURE-12-F03).
4. **"Extract a shared sqlite kit package."** Rejected as a fix: overlap is ~15 lines of DSN construction; the problem is governance ownership, not code sharing. Indirection would not have caught anything in this report.
5. **"DB-vs-file precedence is inconsistent (split-brain)."** Disproved: all three loaders prefer the DB unconditionally when the row exists (`state.go:21–32`, `execute_state.go:36–47`, `study/state.go:26–35`); saves gate identically on `*InDatabase`. The genuine defect is the absence of staleness detection on fallback (FAILURE-12-F01), not wrong precedence.
6. **"`internal/platform/filesystem` being an empty doc.go proves a missing layer."** Rejected: it declares deliberate deferral ("deferred to owning modules in later sprints"); the per-module atomic-write helpers are cohesive and small; a shared helper today would be an unearned abstraction. (Noting the duplication is factual: `atomicWriteFile` ×2, `syncDir` ×2.)
7. **"`promoteCodeContext` renames without fsyncing the candidate — standalone defect."** Downgraded to a note: visibility is atomic-by-rename, the state-persist failure path has tested compensating restore with proper sync (`code_context.go:484–509`), and regeneration is documented; only power-loss durability of new bytes is weaker than siblings.
8. **"`SaveFlowState`'s authority check is a separate TOCTOU defect."** Folded into the unlocked-importer edge (F03): the check-then-write race is only exploitable by the one writer that skips leases (`storage migrate`); normal writers are serialized.

### Open questions

1. Does modernc.org/sqlite delete the WAL on clean last-connection close exactly like stock SQLite (assumed yes — it is the amalgamation translated)? If it instead persists WAL, F01's trigger widens to every restore; if it never deletes, the existing test's pass needs re-examination.
2. Will Gate D (single storage authority selection) be ratified, and does any roadmap item plan `product_states` DDL evolution? Answers set whether F03's evolution half escalates back toward P1 or stays latent.
3. Does any supported external tooling read `flow-state.json`/`run-state.json` directly while DB-authoritative mid-run? If yes, the silent-stale-checkpoint edge (F03/F12-F01) escalates from confusing to breaking.
4. Root cause of the deterministic `TestRunLoopStartsPriorityTierBeforeLaterTiers` failure (found during REALITY checks; out of this tribunal's scope — flagged for the quality tribunal).
