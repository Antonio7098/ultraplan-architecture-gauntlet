# CHAIR-03 — State tribunal synthesis (filesystem, SQLite/productstate, migrations, durable/derived/ephemeral truth, consistency, recovery)

Input: challenge-03 (five candidate findings) plus standing state-domain outputs from scout-05, specialist-08a/08b, failure-09/10/12, change-05. Every load-bearing claim below was re-derived firsthand from source at eeaa034, and the highest-consequence mechanism (restore vs stale WAL) was settled empirically, not by argument. Cross-tribunal items are marked as owned elsewhere, not duplicated.

### Scope inspected

**Implementation repo** `/home/antonioborgerees/coding/ultraplan/gauntlet-run/ultraplan-go` @ eeaa034 (clean, read-only):

- Read in full, firsthand: `internal/runcontrol/migration.go` (all 363 lines — `migrateSchema`, `acquireMigrationLock`, `removeStaleMigrationLock`, `checkpointWAL`, `createMigrationBackup`, `RestoreBackup`, `validateBackupIntegrity`, `pruneMigrationBackups`), `internal/runcontrol/sqlite.go:59–108` (DSN construction, `_defensive`, pragma set).
- Verified key-region, firsthand: `internal/study/run_history.go` (:90–110 append, :198–235 parsers), `internal/study/run_loop.go:68–78`, `internal/runcontrol/retention.go:35–56` (`storageBytes` prefix sum), `internal/sprint/freshness_policy.go:1–15`, `internal/runcontrol/migration_test.go:85–152`.
- Bounded-discovery subagents (evidence-only, conclusions re-derived here): (a) `internal/study/run_history*.go` full consumer map incl. `cleanup_uncertain.go:137`, `app/study_runs_commands.go:28`, `run_history_summary.go:13`; contrast reader `run_loop_diagnostics.go:369–401`; (b) `internal/productstate/store.go` full read + runcontrol `app_schema` machinery (`sqlite.go:230–234,344–366`; `migration.go:236–248`) + SchemaVersion handling in `sprint/state_database.go`, `study/state_database.go`, validators `sprint/state.go:295–297`, `study/state.go:125–127`, `execute_state.go:188–190` + exhaustive zero-coverage grep + `storage_commands.go:33–140,199–207` + lease sites (`locks.go`, `service.go:77–98`) + git history of `store.go` (single commit `02e2ec4`); (c) prior-output provenance for specialist-15a/15b, failure-01, failure-10, scout-05 + direct verification of `internal/platform/filesystem/doc.go`, duplicate atomic-write helper inventory, `code_context.go:464–513`; (d) prior-output provenance for specialist-08a/08b, failure-09, failure-12, change-05 + direct verification of `execute.go:556–587`, sibling atomic writers (`smoke.go:692–724`, `review.goChair-03 synthesis written to `.archgauntlet/runs/chair-03/stdout.md`. Target repos untouched (both still clean at eeaa034 / 368a789).

**Verdict: no P1 survives challenge in the state domain.** Six confirmed findings:

- **CHAIR-03-F01 (P2)** — Restore/`-wal` hole: **empirically proven** with modernc v1.57.0 + the modules' exact DSN — restoring a backup over a crash-left WAL resurrects pre-restore data (`[original newer]` vs correct `[original]`); clean close deletes the WAL, so the sole test defuses the trap and challenge-03's open question 1 is resolved.
- **CHAIR-03-F02 (P2)** — productstate co-tenancy cluster: intentional co-location, but no `app_schema` registration, no DB-path version gate, zero test coverage; upstream P1s rejected as framed.
- **F03–F06 (P3)** — tasks.jsonl torn-line poisoning, non-atomic governed `execute.md`, torn two-statement `Store.Load`, orphaned `.migrate.lock` brick (FAILURE-10-F01 recalibrated P2→P3).

Also flagged for the arbiter: specialist-15a/15b and failure-01 never emitted findings — the filesystem lens is formally uncovered.
ment

The durable core is sound and survived adversarial re-derivation twice (challenge stage, then this chair). Run-control mutations are single immediate transactions with fence/CAS checks; the initial schema migration commits DDL + `app_schema('run_control',1)` + `user_version` atomically (`migration.go:71` → `sqlite.go:344–366`); corrupt databases are rejected without replacing evidence; migration backups are bounded, fsynced, pruned; product-file writers overwhelmingly meet temp+fsync+rename+dirsync; DB-over-file authority is implemented identically in all three loaders and matches the documented rule (`docs/migration-product-state.md:24`). None of the loud upstream P1 framings (restore destroying product authority, fake pragmas, dual-pool corruption, split-brain precedence) survived contact with the code.

Stress concentrates in three places, and this tribunal's net contribution is narrowing and hardening them:

1. **The recovery surface itself.** The shipped restore primitive has a stale-sidecar hole that is now *empirically proven* (CHAIR-03-F01), has no executable entrypoint (FAILURE-10-F02), and the migration-lock recovery path can permanently wedge a workspace with a misleading error (FAILURE-10-F01, verified here). Recovery is the least-tested, least-governed part of an otherwise disciplined durability story.
2. **The co-tenancy seam.** `productstate` shares run-control.db's physical file but none of its lifecycle machinery, with zero test coverage on the DB-backed authority path (CHAIR-03-F02). Adjudication: intentional co-location (HISTORY/FUTURE-INTENT), so the defect is the missing registration/governance hook and absent tests — not the sharing, and not worth a second database or a shared sqlite-kit abstraction.
3. **Two append-ledger/writer edges** below the codebase's own standard: the study history ledger is appended without fsync and parsed fatally (CHAIR-03-F03), and `WriteExecuteSummary` writes governed evidence non-atomically (CHAIR-03-F04).

No P1 survives challenge in this domain. Provenance warnings for downstream stages: **specialist-15a, specialist-15b, and failure-01 emitted no final reports** (empty or narration-only stdout.md; their transcripts end mid-exploration). The filesystem/artifact-persistence twin lens therefore never produced findings; this chair partially compensates via direct verification (helper inventory, `promoteCodeContext`, atomic-writer contrasts), but the arbiter should treat that lens as under-covered. Side observation flagged for the quality tribunal, corroborated independently by the specialist-15b transcript: `go test -count=1 ./internal/study` fails deterministically on `TestRunLoopStartsPriorityTierBeforeLaterTiers` (`run_loop_test.go:303`).

Cross-tribunal ownership (not re-reported here): the sanitize-allowlist/event-payload drift (SCOUT-05-F01) is owned by the execution tribunal (CHALLENGE-02, recalibrated P2) and independently re-derived by the quality tribunal (their F01, P1) — the priority dispute between those two belongs to the arbiter. The `reconcileUnclaimed` second terminal-write path (SCOUT-05-F03) was adjudicated by challenge-02 as P3 duplication risk, not a double-arbiter bug; I concur from the same evidence.

### Candidate findings

---

**ID: CHAIR-03-F01**
**Priority: P2**

**Claim:** Restoring `run-control.db` without removing stale `-wal`/`-shm` sidecars silently resurrects the pre-restore committed state through WAL replay. Empirically demonstrated with the project's own driver and DSN parameters. `RestoreBackup` itself never touches sidecars, the sole test defuses the trap by cleanly closing first, and neither the function nor the runbook tells operators to clear them.

**Evidence:**
- REALITY: `RestoreBackup` (`migration.go:296–352`) validates, copies to temp, fsyncs (:341), renames (:347), enforces mode (:351) — no reference to `-wal`/`-shm` anywhere in the function or file.
- REALITY: repo-wide search: only `scripts/reset-opencode-db.sh:131,139,177` removes sidecars — for the opencode database (`database_path="$(opencode db path)"`, line 59), not run-control.db. Nothing in code or docs ever clears them for this file.
- REALITY: the only restore test takes a checkpoint+backup, commits a *post-backup* run (`migration_test.go:131`), then calls `repository.Close()` (:134) **before** restoring (:137). A clean last-connection close deletes the WAL, so the test passes while never exercising the crash condition it exists to cover.
- REALITY: `docs/recovery.md:217–222` instructs "stop every UltraPlan process … use the tested restore path … Never copy only the database while a WAL writer is active" — it never mentions deleting `-wal`/`-shm` left by a dead process. And "the tested restore path" has no CLI surface: sole caller of `RestoreBackup` is its own test (`migration_test.go:137`; grep-verified).
- **EMPIRICAL** (this chair, `/tmp/opencode/waltest`, modernc.org/sqlite v1.57.0, DSN params identical to both modules): seed → checkpoint TRUNCATE → clean close (WAL deleted, confirming open-question resolution) → file-copy backup → reopen, commit `'newer'` → `os.Exit(1)` simulating SIGKILL → `-wal` (4152 bytes) and `-shm` persist on disk → copy backup over db keeping sidecars → next open returns **ROWS-AFTER-RESTORE: [original newer]**. Same setup with sidecars removed after restore → **[original]**. The rollback is silently undone exactly when restores happen (after unclean death).

**Architectural reason:** failure-semantics / lifecycle — the recovery operation models the database file but not the durability state surviving beside it; the contract ("offline, matching binary") is necessary but insufficient.

**Concrete consequence:** power loss or SIGKILL mid-write → integrity failure or failed first migration → operator follows the runbook (or hand-copies) → first open replays the crashed era's committed transactions over the rollback. `integrity_check` passes, `ultraplan health` reports ok, and diagnosis points anywhere but two hidden sidecar files.

**Counter-evidence searched:** clean-close WAL deletion (verified empirically — narrows trigger to unclean death, which is precisely when restores occur); TRUNCATEd/zero-length WAL inertness; whether `validateBackupIntegrity` or `preparePrivateDatabase` touch sidecars (they do not); whether query-only/defensive flags alter recovery (they do not). No prior stage reported the mechanism; this chair converted challenge-03's medium-confidence reasoning into empirical fact.

**Confidence:** high (mechanism proven end-to-end on the pinned driver version).

**Smallest useful action:** in `RestoreBackup`, after the rename, remove `<db>-wal` and `<db>-shm` (safe under the documented all-processes-stopped contract; discarding post-backup frames is the intent of rollback), extend `TestBackupRestoreFixture` to skip the clean close so the crash condition is exercised, and add one bullet beside `docs/recovery.md:222`.

---

**ID: CHAIR-03-F02**
**Priority: P2**

**Claim:** Cluster adjudication: `productstate` deliberately co-locates with run-control (`store.go:19` == `sqlite.go:21–22`; single commit `02e2ec4`, HISTORY) but participates in none of the file-level lifecycle machinery its co-owner built — no `app_schema` component row (only `'run_control'` registers, `sqlite.go:356`), no version gate or migration hook on DB loads, no quota attribution, no restore awareness, no integrity gate — and the entire DB-backed authority path has zero test coverage. Upstream P1 framings (SPECIALIST-08A-F01/08B-F01) are rejected as framed; what survives is this registration/governance gap, three sharp edges, and the coverage hole.

**Evidence:**
- Ungoverned tenant: bare `CREATE TABLE IF NOT EXISTS` on every open including read paths (`store.go:79,92–116`; readers route through `Existing`→`open`: `study/state_database.go:18`, `sprint/state_database.go:58`) vs runcontrol's `user_version` + `app_schema` + migration lock + bounded backups + integrity gate (`migration.go:25–82`). Repo-wide: no `app_schema` row for any product component; no comment acknowledges the sharing.
- Version asymmetry: DB loaders ignore the `schema_version` column entirely (`sprint/state_database.go:24–55`, `study/state_database.go:29–39`) while validators accept only the current constant (`sprint/state.go:295–297`, `study/state.go:125–127`, `execute_state.go:188–190`); the file path auto-heals v1→v2 via `migrateFlowStateV1` (`state.go:53–73,150–192`) but no analogous DB hook exists. Latent today (one schema generation ever shipped) — the evolution half is structural debt, not live breakage.
- Sharp edge A (staleness): pure row-existence authority with no staleness signal; checkpoint files written only at terminal boundaries (`execute_state.go:112–130`, `study/state.go:66–68`); if the DB record disappears while a stale checkpoint remains, loads silently resume from it and a later `storage migrate` re-imports the regressed state as current (FAILURE-12-F01, mechanism verified).
- Sharp edge B (unlocked import): `storage migrate` flips per-record authority with no mutation lease or quiesce guidance (`storage_commands.go:59–68,93–106,147–158,170–181`; help text :199–207 silent), while all normal writers hold the product mutation lease (`locks.go:13–15,26`; seven leased call sites enumerated).
- Coverage hole: exhaustive grep — no `*_test.go` anywhere references `productstate`/`InDatabase`/`Migrate*ToDatabase`/`storage migrate`/`product_states`; the package contains one file, no tests; introduced in commit `02e2ec4` (766 insertions, zero tests), untouched since. Independently reproduced TRUE by challenge-03, SPECIALIST-08A-F03, CHANGE-05-F3.
- Quota coupling (benign in direction): `storageBytes()` sums every regular file prefixed `run-control.db*` including `-wal/-shm/.bak.*` (`retention.go:41–56`), so product growth consumes run-control quota attributed to run events.

**Architectural reason:** ownership / boundary / change-surface — one substrate, two owners, one unaware of the other's protocol; the untested half is the half that became authoritative.

**Concrete consequence:** the first product-schema evolution, or the first real restore/migration accident, lands on a persistence layer with no governance hook, no test, and diagnostics attributing its failures to the wrong subsystem — and today, a documented recovery procedure can silently regress live workspaces to older checkpoints with status surfaces presenting them as current.

**Counter-evidence searched:** co-location intentional per `docs/migration-product-state.md` and embedded-schema plans (FUTURE-INTENT — not reported as absent); `migrateSchema` handles foreign pre-existing schemas safely (backup-before-create, `migration.go:58–74` — and product tables merely count as "has schema", get backed up, left in place); restore yields a coherent whole-file snapshot; immediate-txlock + busy timeout + O_EXCL serialize the two pools correctly (dual-pool corruption rejected); imports go through validating loaders, so stored rows are import-time-current by construction (CHANGE-05 defended item).

**Confidence:** high on structure and coverage; medium-high on calibration.

**Provenance:** confirms-and-recalibrates SPECIALIST-08A-F01, SPECIALIST-08B-F01 (both P1 → cluster-P2), folds CHANGE-05-F1/F2 (evolution half), FAILURE-12-F01/F03 and SPECIALIST-08A-F04/SPECIALIST-08B-F02 (sharp edges), SPECIALIST-08B-F04/F06 and FAILURE-09-F01 (asymmetries, folded), FAILURE-10-F02 (restore has no entrypoint — see F01 action). Rejects SPECIALIST-08A-F02's P2 (see CHAIR-03-F05).

**Smallest useful action:** register a `product_state` component row in the existing `app_schema` ledger inside schema creation and gate loads on it; add store round-trip, load-precedence, and terminal-checkpoint-gating tests; take (or document quiesce requirements for) the mutation lease in `storage migrate`. No second database file; no shared sqlite-kit abstraction.

---

**ID: CHAIR-03-F03**
**Priority: P3**

**Claim:** The study run-history ledger (`studies/<s>/.ultraplan/runs/tasks.jsonl`) is appended without fsync and parsed strictly, so one torn trailing line permanently poisons every consumer — run-loop startup, completion, reconciliation, and `study runs summary` all fail closed until an operator hand-edits a hidden file, with no recovery-doc guidance.

**Evidence (mechanism verified firsthand; consumer map by subagent, spot-checked):**
- Append: `os.OpenFile(..., os.O_CREATE|os.O_WRONLY|os.O_APPEND)` + single `Write(append(data,'\n'))`, no `Sync()` anywhere in the file (`run_history.go:94,103`; repo-wide `.Sync()` sweep confirms this is the only durable study artifact written without fsync).
- Strict parse: `readRunHistory` fails the whole file on any `json.Unmarshal` error (`run_history.go:207–210`); `readRunHistoryKeys` propagates (:228–231); a torn final line from ENOSPC short-write or power loss is unrecoverable.
- Fail-closed consumers: run-loop start aborts (`run_loop.go:70–76` — verified firsthand), completion errors after state save (:389–391), `cleanup_uncertain.go:137` aborts reconciliation, `app/study_runs_commands.go:28` and `run_history_summary.go:13` block summaries.
- Contrast: `run-loop-memory.jsonl` swallows encode/close errors and silently skips malformed lines on read (`run_loop_diagnostics.go:185–191,387`) — same format, opposite corruption policy, in the same package.
- Tests cover happy path + dedup only (`run_history_test.go:11–103`); docs never mention the ledger (grep: zero hits for `tasks.jsonl`/`run_history` in docs/).

**Architectural reason:** failure-semantics / durability — a restart-safe idempotency ledger written below the codebase's own temp+fsync+rename standard, read stricter than any other log in the tree.

**Concrete consequence:** quota pressure or power loss during a long study tears line N; every subsequent run-loop/resume/summary command for that study dies with a raw `json.Unmarshal` error naming no file and no remedy; dedup keys become unreachable, so safe auto-resume is blocked (fail-closed — no double-execution, but a stuck study).

**Counter-evidence searched:** dedup keys give idempotent re-append (only after successful parse — does not help); single small writes are near-atomic between processes (irrelevant to page-cache tearing/ENOSPC); callers downgrading errors to warnings (none — all six consumers return).

**Confidence:** high on mechanism; low-medium on frequency.

**Provenance:** confirms CHALLENGE-03-F02; consumer enumeration extended beyond it (reconciliation + summary surfaces).

**Smallest useful action:** tolerate a malformed *final* line (skip-with-warning — standard append-log repair) in `readRunHistory`, fsync the append, and add one `docs/recovery.md` bullet.

---

**ID: CHAIR-03-F04**
**Priority: P3**

**Claim:** `WriteExecuteSummary` writes governed evidence with plain truncate-and-overwrite `os.WriteFile` (`execute.go:586`) — confirming FAILURE-09-F04's mechanism while correcting its mitigation: `execute.md` is not an unvalidated bystander; its exact bytes enter the review input fingerprint (`review.go:219` entry `{"execute","governed",StageExecute}`, contents hashed via `reviewInput` :1288–1291) and approved verification commands must appear in it (`review.go:335–338`), yet the verify gate checks only non-emptiness (`verify.go:127–130`) and no gate-time hash of `execute.md` exists (`strictCompletedReviewSnapshotFreshness = false`, `freshness_policy.go:12`, comment :3–10).

**Evidence:** non-atomic write firsthand-quoted (`execute.go:556–587`; call sites :73, :317 — both after `SaveExecuteRunState` commits, so machine truth is safe); sibling writers all temp+fsync+rename+dirsync (`smoke.go:692–724`, `review.go:1686–1717`, `state.go:239–289` — verified); contract PERSIST-ATOMIC-001 Required clause: "use temporary files plus same-directory rename for important file state where practical" (`persistence-and-migrations.md:88–97`); Forbidden list includes "partial durable writes … with no compensation".

**Architectural reason:** failure-semantics / drift — one governed artifact written below the contract its siblings meet.

**Concrete consequence:** crash or disk-full mid-write leaves truncated `execute.md`; the emptiness gate passes on partial content; if it precedes `PrepareReview`, garbage bytes freeze into the evidence fingerprint and can ride through a review cycle undetected (no gate-time hashing; snapshot freshness disabled). Bounded: regeneration is possible via `flow --to execute` (re-invokes Execute with Resume:true, rewriting the summary, `flow.go:159–161`), and `execute.md` is the sole writer's output (grep-verified).

**Counter-evidence searched:** any validator hashing `execute.md` at gate time (none); alternative writers (none); FAILURE-09-F04's "not truth" mitigation (rejected — fingerprint membership makes the bytes evidentiary).

**Confidence:** high on facts; low on incidence.

**Provenance:** confirms FAILURE-09-F04; corrects its severity rationale; extends CHALLENGE-03-F04 with the freshness-disabled nuance.

**Smallest useful action:** route `WriteExecuteSummary` through the existing `atomicWriteFile`.

---

**ID: CHAIR-03-F05**
**Priority: P3**

**Claim:** `productstate.Store.Load` reads header and items as two autocommit statements (`store.go:127–148`, no `BeginTx`) while `Save` is fully transactional (:150–205), so a concurrent save between the two reads yields a header/items mismatch presented as truth. Real but narrow: every production writer holds the mutation lease; several legitimate readers do not.

**Evidence:** mechanism verified firsthand (no tx in Load; hash columns written at :160–178 but never read back or compared at :127–148). Lock-free readers: `service.go:151,202` (Status), `verify.go:150` (VerificationStatus), `requireCompleteExecute` when called from `ExecuteComplete` (`verify.go:134–140`), smoke dry-run path (`smoke.go:21–22` bypassing acquisition), study-side `run_loop.go:456`, `cleanup_uncertain.go:106`, `validation_command.go:167`.

**Architectural reason:** consistency — the read API is weaker than the write API it mirrors, at the boundary feeding reconcile classification (`locks.go:46–78` classifies from loaded items).

**Concrete consequence:** a torn read during a concurrent server-side save could flip running→failed classification on wrong pairing — silent, durably recorded misclassification. Probability low: two statements racing one commit, single-user tool.

**Counter-evidence searched:** implicit `database/sql` wrapping (none); readerside retry/coherence validation (validators check shape, not cross-statement pairing); lease coverage of readers (enumerated above — genuinely absent for status/verify-complete/smoke-dry-run).

**Confidence:** high on mechanism; low on practical incidence.

**Provenance:** confirms SPECIALIST-08A-F02/SPECIALIST-08B-F03 with downward calibration P2→P3 (lease serialization of all writers); folds FAILURE-12-F02's unused-hash observation.

**Smallest useful action:** wrap Load's two statements in one deferred read transaction (~3 lines).

---

**ID: CHAIR-03-F06**
**Priority: P3**

**Claim:** Process death inside `acquireMigrationLock`'s creation window (O_EXCL create :94 → PID probe :111 → marshal :117 → write :123 → sync :128) strands an empty or partial `.migrate.lock` that `removeStaleMigrationLock` refuses to reclaim by design (malformed records "cannot authorize lock removal", :146–148), permanently bricking every run-control open for the workspace behind `CodeBusy` with the misleading message "another local UltraPlan process owns the schema migration lock" (:105). Undocumented anywhere in docs/.

**Evidence (mechanism verified firsthand this chair):** `migration.go:92–138` creation sequence with best-effort self-cleanup on error paths only (:113–135 — ineffective against SIGKILL); `:140–165` reclaim logic requiring a complete, valid identity whose PID is provably dead; behavior pinned by `migration_test.go:85–91` (foreign lock content → `ErrBusy`) and valid-dead-owner reclaim proven :94–117; `rg 'migrate.lock' docs/` → zero hits; per FAILURE-10 (not re-verified here), `health` never opens run control, so it reports ok while the workspace is wedged.

**Architectural reason:** failure-semantics / lifecycle — a conservative anti-footgun rule (never delete a lock you cannot attribute) converts a millisecond crash window into an unrecoverable-by-tooling state with an actively wrong diagnostic.

**Concrete consequence:** SIGKILL/power loss during the once-per-workspace first migration leaves the workspace unusable for serve/run/storage until an operator finds and deletes a hidden dotfile whose error message told them something false.

**Counter-evidence searched:** error paths do clean up (true — narrows to hard kill); valid-record reclaim works and is tested (true — narrows to the pre-sync window); whether any liveness surface detects the wedge (health does not open the DB — per FAILURE-10-F01's citation).

**Confidence:** high on mechanism; low on frequency (millisecond window, once per workspace lifetime).

**Provenance:** confirms FAILURE-10-F01 with downward calibration P2→P3 (window width and once-per-lifetime frequency), contra its original rating; the misleading-message and undocumented-remedy aspects stand unchanged.

**Smallest useful action:** document the lock and its remedy in `docs/recovery.md` alongside the restore bullet, and make the `CodeBusy` message name the lock file path so hand-repair is discoverable.

### Defended architecture / rejected hypotheses

1. **"RestoreBackup destroys product-state authority" as a P1 current defect (SPECIALIST-08A-F01/08B-F01 headline).** Rejected as framed: restore produces a coherent point-in-time snapshot of the whole file including both tenants; per-record authority gates make post-restore loads consistent with the restored era; the silent-regression kernel is captured as F02 sharp edge A, and the true restore defect is the sidecar hole (F01) — narrower and differently fixed than the upstream framing implied.
2. **"modernc DSN pragmas are ignored / concurrency config is fake."** Disproved: driver honors the parameters, and runcontrol additionally verifies them at open (`sqlite.go:104,207–227`); productstate's lack of verification is the already-folded asymmetry, not fake config.
3. **"Two connection pools on one file is a corruption hazard."** Largely rejected: O_EXCL creation, immediate txlock, busy timeout, and idempotent DDL serialize correctly; my empirical harness exercised both pools' parameter set without anomaly. Residual windows (DDL interleaving a migration backup; cold-start busy) are fail-closed or already reported.
4. **"Extract a shared sqlite-kit / platform-filesystem helper package."** Rejected as a fix: DSN overlap is ~5 lines; the atomic-write duplication is factual (2× `atomicWriteFile` with different signatures, 2× identical `syncDir`, 1× `atomicWriteReviewWithHooks`, ~5 inline copies) but each is small and locally owned; `internal/platform/filesystem/doc.go` explicitly defers generic helpers to owning modules. Consolidation today buys indirection, not correctness. Revisit only if a third copy appears.
5. **"DB-vs-file precedence is inconsistent (split-brain)."** Disproved: all three loaders prefer the DB unconditionally when the row exists and saves gate identically on the `InDatabase` checks; the documented rule matches implementation. The genuine defect is absence of staleness signaling on fallback (F02 edge A).
6. **"`promoteCodeContext` renames without fsyncing the candidate — standalone defect."** Downgraded to accepted note: visibility is atomic-by-rename; the state-persist failure path has a tested compensating restore that does fsync (`code_context.go:486–509`, tested `code_context_test.go:488–508`); regeneration is documented. Only power-loss durability of the newly promoted bytes is weaker than siblings — acceptable for a regenerable artifact.
7. **"`SaveFlowState`'s authority check is a separate TOCTOU defect."** Folded into F02 edge B: exploitable only by the lease-free importer; normal writers are serialized by the mutation lease.
8. **"`tasks.jsonl` poisoning justifies a rewrite-the-ledger redesign."** Rejected: the ledger is small, append-only, and human-repairable; tail-line tolerance plus fsync restores the codebase's own standard without new structure.
9. **Challenge-03 open question 1 resolved, not carried:** modernc.org/sqlite v1.57.0 deletes the WAL on clean last-connection close (empirically shown), so the F01 trigger is confined to unclean death — which is exactly when restores happen.

### Open questions

1. Will Gate D (single storage authority selection) be ratified, and does any roadmap item plan `product_states` DDL evolution? Answers decide whether F02's version-gate half escalates back toward P1 or stays latent structural debt.
2. Does any supported external tooling read `flow-state.json`/`run-state.json` directly while DB-authoritative mid-run? If yes, F02 edge A escalates from confusing to breaking.
3. Was the strict sanitize allowlist ratified as policy after c455510? (Owned by execution/quality tribunals; listed only because their P1-vs-P2 dispute affects the state domain's "no P1" verdict.)
4. Root cause of the deterministic `TestRunLoopStartsPriorityTierBeforeLaterTiers` failure (`run_loop_test.go:303`; observed by this chair's scope check and independently by the lost specialist-15b transcript) — quality tribunal's to place, but it currently undermines cached-run CI signal for the study state machine.
5. Should the arbiter commission a re-run of the filesystem/artifact-persistence lens (specialist-15 twins and failure-01 produced no findings)? This chair's direct verification covered the highest-value ground, but the lens is formally uncovered.
