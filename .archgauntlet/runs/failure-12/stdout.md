### Scope inspected

- **Implementation repo** `/home/antonioborgerees/coding/ultraplan/gauntlet-run/ultraplan-go` @ eeaa034 (read-only):
  - `internal/productstate/store.go` (whole store)
  - `internal/study/state.go`, `state_database.go`, `run_state.go`, `run_loop.go`, `run_history.go`, `locks.go`, `validation_command.go`
  - `internal/sprint/state.go`, `state_database.go`, `execute_state.go`, `verify.go`, `locks.go`, `cleanup_uncertain.go`
  - `internal/runcontrol/migration.go`, `lifecycle.go`, `sanitize.go`, `sqlite.go` (open/snapshot/events/health), `migration_test.go`, `doc.go`
  - `internal/app/storage_commands.go`, `run_control.go`, `study_usecases.go`, error mappers in `study_commands.go` / `sprint_commands.go`
  - Docs: `docs/recovery.md`, `docs/migration-product-state.md`, `docs/architecture.md` (durable run control), `scripts/migrate-product-state.sh`
- **Authoritative planning workspace**: `projects/ultraplan-go/roadmap.md` (Phase 4 gate, Sprint 35, Gate D)
- Targeted searches for quarantine/corruption/stale handling across all `*_test.go`

### Architecture assessment

The malformed/stale-state story is deliberately split across three owners, and each is individually disciplined:

- **Product workflow state** (study `RunState`, sprint `FlowState`/`ExecuteRunState`) lives in one of two stores chosen *per record*: the workspace SQLite DB (`internal/productstate`) if a record exists there, else the legacy JSON file. Authority rule is consistent everywhere: **DB wins on load; files degrade to compatibility checkpoints** (study/state.go:27–35, sprint/state.go:20–32, execute_state.go:35–47). Files are rewritten alongside the DB only at terminal states (study/state.go:66–68, sprint/state.go:222–227, execute_state.go:112–115).
- **Operational run state** (`internal/runcontrol`) is fenced, CAS-guarded, integrity-checked at every open, refuses to rewrite corrupt evidence (migration_test.go:154–176), reconciles stale owners conservatively into `interrupted`/`cleanup_uncertain`/`stalled` rather than inventing outcomes (lifecycle.go:301–495), and quarantines unsafe payloads behind an allowlist with explicit omission metadata (sanitize.go:19–55).
- **Recovery semantics are conservative by construction**: active study tasks reset to pending but completed artifacts are revalidated against disk before trust (run_state.go:277–361); expired review/smoke attempts derive `timed_out` in memory only, leaving the durable transition to the next explicit operation (verify.go:455–498, 154–156); cleanup uncertainty is a separate sidecar marker that never rewrites canonical state (cleanup_uncertain.go:28–31).

What is stressed: the product-state half of this picture was moved onto the run-control database file recently and thinly. The authority-transfer seam (precedence, checkpointing, corrupt-record handling) is essentially untested, its error taxonomy diverges from both the file path and runcontrol, and nothing detects the case where the authoritative store disappears and the system silently falls back to a stale checkpoint.

### Candidate findings

---

**ID: FAILURE-12-F01**
- **Priority:** P2
- **Claim:** When the authoritative DB record disappears (deleted DB, restore of a pre-migration backup, partial cleanup), load silently falls back to the stale JSON checkpoint with no staleness detection, warning, or provenance — the ambiguity between sources resolves toward whichever happens to exist, and re-running `storage migrate` then re-imports the outdated file as current.
- **Evidence:** `internal/study/state_database.go:17–25` (`ErrNotFound`/missing store → `found=false`), `state.go:27–35` (falls through to file read with no annotation); same pattern `internal/sprint/state_database.go:57–67`; authority is pure row existence (`store.Has`, state_database.go:70–76); study file checkpoints are frozen mid-run — `SaveRunState` writes the file only when `state.Complete` (state.go:66–68), sprint only when all stages are terminal (state.go:230–237, execute_state.go:120–130); no migrated/staleness marker exists anywhere (grep for provenance fields: none outside runcontrol's `app_schema.migrated_at`).
- **Architectural reason:** authority + lifecycle — truth-source selection keyed on undifferentiated existence, with no epoch/version binding between the two representations.
- **Concrete consequence:** operator follows recovery.md's offline restore procedure with a backup taken before `storage migrate`; the workspace resumes from weeks-old file checkpoints; `study status` presents stale counts as current with zero indication of fallback; a subsequent `storage migrate` "skips nothing" and re-imports the regressed state, making the rollback invisible and self-confirming. Damage is bounded (resume revalidates completed artifacts, resets actives), but status/truth disagreement across surfaces is exactly what Sprint 35's gate forbids.
- **Counter-evidence searched:** looked for any tombstone, `UpdatedAt` cross-comparison, fallback logging, or status annotation — none found; considered whether terminal-only checkpointing makes files always-current (no: incomplete studies/sprints never refresh files); considered whether `ResumeValidateRunState` fully neutralizes staleness (it heals *task* states on next run-loop, but status-only reads and non-resumable `run-all` still report stale truth).
- **Confidence:** medium-high
- **Smallest useful action:** bind the two representations with one comparable fact — e.g., record last-DB-write time (or a `migratedAt` marker row) and make `LoadRunState`/`LoadFlowState` emit a loud warning (or refuse with actionable guidance) when falling back to a file older than the last known DB write; surface "source: sqlite|file-checkpoint" in status JSON.

---

**ID: FAILURE-12-F02**
- **Priority:** P2
- **Claim:** Corrupt DB-backed product records bypass the malformed-state error taxonomy: the `*Database` loaders return raw `json.Unmarshal` errors, so CLI exit classes, `validate` observations, and repair guidance diverge depending on which store is corrupted — and guidance actively points at the non-authoritative JSON file.
- **Evidence:** `internal/study/state_database.go:30–37`, `internal/sprint/state_database.go:24–33,43–53` return unwrapped decode errors; file-side equivalents wrap sentinels (`study/state.go:45–46`, `sprint/state.go:48–66`, `execute_state.go:60–61`); mappings key off sentinels only — `mapStudyRunLoopError` (app/study_commands.go:347–348) → `ExitValidation`, default → `ExitWorkspace` (study_commands.go:914–923); `mapSprintError` (app/sprint_commands.go:666–669) same; `validateRunStateCheck` reports generic "run state could not be read" and guides the operator to "`inspect or recreate studies/<study>/.ultraplan/run-state.json`" (validation_command.go:165–186) — the file that is *not* the source of truth in this scenario. Contrast: runcontrol classifies identical situations as typed stop conditions (`CodeCorrupt`, sqlite.go:840,845,966; errors.go:20). Additionally, `productstate` persists `header_hash`/`payload_hash` (store.go:99,109) but never verifies them on `Load` (127–148) — they are write-skip metadata only, so bit-rot surfaces as opaque downstream JSON errors.
- **Architectural reason:** failure-semantics + drift — one concept ("malformed product state") has two incompatible classifications depending on storage backend.
- **Concrete consequence:** exactly the scenario this stage probes — a corrupt durable record — produces `ExitWorkspace`-class noise instead of `ExitValidation`, and the printed repair instruction directs the operator to modify a stale compatibility checkpoint, i.e., to edit the wrong copy of state while believing they repaired the real one.
- **Counter-evidence searched:** checked whether callers wrap DB-load errors upstream (they do not: state.go:28–29, state_database callers in app use raw `err.Error()`); checked whether `ValidateRunState` on the DB path restores sentinel classification (only for schema/field violations, not payload decode: state.go:31); confirmed no test exercises these loaders against corrupt payloads (grep across `*_test.go`: zero hits for `productstate`, `Migrate*ToDatabase`, `InDatabase`, `storage migrate`).
- **Confidence:** high
- **Smallest useful action:** wrap decode failures in the existing sentinels inside the three `load*Database` helpers (and optionally verify stored hashes on load, returning a typed corrupt-class error), so file and DB backends present identical failure semantics through the existing mapping/validation layers.

---

**ID: FAILURE-12-F03**
- **Priority:** P3
- **Claim:** Two modules independently own the same physical database file with asymmetric integrity discipline: `runcontrol` gates every open on schema verification plus `PRAGMA integrity_check` under a migration lock, while `productstate` opens the same `.ultraplan/run-control.db` directly and performs DDL even on read paths (`Existing`→`open`→`createSchema`), with no integrity gate, no lock coordination, and no documented co-tenancy.
- **Evidence:** identical `DatabaseRelativePath` constants (`productstate/store.go:19`, `runcontrol/sqlite.go:22`); runcontrol gate: `sqlite.go:112` → `migration.go:36–41,78–80,250–266`; productstate ungated open+DDL: `store.go:41–53,79`; read paths trigger it (`study/state_database.go:18`, `sprint/state_database.go:58` via `status`/`validate` commands that never construct run control — grep confirms `OpenSQLite` appears only in `storage_commands.go`, `run_control.go`); the intended ordering exists only in `storage migrate` (storage_commands.go:60–67). `docs/architecture.md:155–158` assigns product authority to sprint/study modules and says nothing about shared file co-tenancy.
- **Architectural reason:** ownership + boundary — one store, two owners, one of them unaware of the other's safety protocol.
- **Concrete consequence:** (a) during run-control's first-time migration (checkpoint WAL → copy backup), a concurrent `study status`/`sprint status` performs uncoordinated writes against the assumption of exclusivity, with a narrow window for a torn backup that only fails loudly later at restore time; (b) after runcontrol correctly declares the DB corrupt (`CodeCorrupt` stop condition), ordinary product-status commands keep hitting the same file and surface raw driver errors classified as generic workspace failures, undermining the "stop condition" posture recovery.md depends on; (c) fresh-workspace ordering (`study status` creating the DB at `user_version=0` before runcontrol ever initializes) works only because `migration.go:58–74` happens to handle pre-existing schema — an implicit cross-module contract nobody documents.
- **Counter-evidence searched:** verified productstate cannot destroy corrupt evidence (it issues only `CREATE ... IF NOT EXISTS`, no drops/journal resets — migration_test.go:154–176 evidence-preservation property is not threatened); verified WAL mechanics bound the backup-tear window (main file changes only at checkpoints; modernc autocheckpoint could interleave — low probability); looked for a doc establishing the co-tenancy contract (none).
- **Confidence:** high on facts, medium on impact
- **Smallest useful action:** document the shared-store relationship and give `productstate` the same open-time gate (reuse runcontrol's migrate/integrity entry point, or open product-state reads through a mode that defers to runcontrol initialization), so one owner establishes health before either writes.

---

**ID: FAILURE-12-F04**
- **Priority:** P3
- **Claim:** The recovery runbook documents the superseded expiry rule: it says `sprint status` derives a running attempt as timed out after 24 hours without terminal updates; the implementation derives it after 2 hours using heartbeats and PID liveness (deliberately tightened in ce223e4).
- **Evidence:** `docs/recovery.md:75` ("more than 24 hours") vs `internal/sprint/verify.go:455–467` (`now.Sub(lastSeen) > 2*time.Hour` plus `verificationProcessAlive`); git: ce223e4 changed `> 24*time.Hour` (StartedAt-based) to the heartbeat-aware 2h form; `docs/recovery.md` was last touched before this semantic in e09d394 lineage.
- **Architectural reason:** drift — HISTORY captured as CURRENT-CONTRACT in the operational runbook.
- **Concrete consequence:** an operator following the runbook waits up to 22 extra hours for a derivation that already fired, or misjudges why a review shows `timed_out` "early"; recovery instructions are the one place where stale numbers become wrong actions.
- **Counter-evidence searched:** searched for another constant/doc restating 24h as current (none); confirmed the code path is the only expiry derivation for these attempts.
- **Confidence:** high
- **Smallest useful action:** update `docs/recovery.md:75` to describe the heartbeat-aware 2-hour derivation and PID-liveness shortcut.

---

### Defended architecture / rejected hypotheses

1. **"The stale JSON file can win over the DB."** Disproved. Every load path checks the DB record first and only falls back when absent (study/state.go:27–35; sprint/state.go:20–32; execute_state.go:35–47); saves prefer the DB and treat files as terminal-state checkpoints; `storage migrate` skips records already imported and leaves invalid files untouched with a partial-failure exit (storage_commands.go:93–106, 136–138; docs/migration-product-state.md:24–26). Residual risk is covered by FAILURE-12-F01, not by a wrong precedence rule.
2. **"Malformed or stale state is silently accepted."** Disproved. Loaders reject unknown fields, trailing JSON, wrong/missing schema versions, unsafe paths, duplicate IDs (sprint/state.go:44–67, 291–358; study/state.go:121–166); resume revalidates completed tasks against on-disk artifacts and demotes failures ("runtime success is not product success", run_state.go:313–351); legacy v0 flow/execute shapes are recognized and preserved as inert history rather than misinterpreted (state.go:128–148, execute_state.go:69–103).
3. **"A corrupt run-control DB gets overwritten or worked around."** Disproved for runcontrol: `TestMigrationRejectsCorruptDatabaseWithoutReplacingEvidence` proves byte-for-byte preservation (migration_test.go:154–176); newer-schema and failed integrity checks are hard stops (migration.go:30–31, 250–266); restore validates backup integrity before the atomic rename (migration.go:296–362).
4. **"`--reset` destroys prior progress."** Largely defended: the file checkpoint is archived (run_loop.go:477–491), per-task history survives in `runs/tasks.jsonl` (run_history.go), and the crash window between archive and first durable save reconciles conservatively on next resume. Minor note: on a DB-authoritative study the archived object may be the non-authoritative copy while the DB record is replaced silently — acceptable given history retention, but worth folding into F01's provenance work.
5. **"Study's optimistic reset of running→pending is inconsistent with sprint's pessimistic running→failed."** Defended as domain-correct: study tasks produce regenerable, post-hoc-validated reports (safe rerun), sprint execute tasks mutate the target repository (must never double-apply), so locks.go:46–63 marks them failed with explicit `recovery-interrupted` diagnostics under the mutation lease. Different failure semantics per domain, coherently owned.
6. **"SQLite-backed product state violates the roadmap's dual-write prohibition."** Rejected as a current defect: Gate D's "select one authoritative storage mode… prohibit silent dual writes" sits in the explicitly gated post-Sprint-35 section (FUTURE-INTENT). Current design maintains one authority per record with an explicit migration command; the gate remains unopened. F01/F02 address today's defects without importing tomorrow's contract.

### Open questions

- Has Gate D been formally opened in any planning document beyond the roadmap chunk inspected (i.e., is per-record authority selection a ratified interim mode)? That would reframe F03's co-tenancy from undocumented wart toward contract violation — or ratify it.
- Does any supported external tooling or dogfood automation read `.ultraplan/run-state.json` / `flow-state.json` directly while DB-authoritative mid-run? If yes, the frozen-checkpoint behavior in F01 escalates from confusing to breaking.
