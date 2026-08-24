# SPECIALIST-24B — Evolution/change locality review

### Scope inspected

- **Docs**: `docs/architecture.md`, `docs/configuration.md`, README; planning workspace `projects/ultraplan-go/roadmap.md`, `ultraplan.yml`
- **Stage/pipeline**: `internal/sprint/domain.go`, `flow.go`, `service.go`, `state.go`, `state_database.go`, `freshness_policy.go`, `review.go` (PrepareReview), `store_fs.go`; tests `code_context_test.go`, `plan_test.go`, `reasoning_test.go`, `handbook_test.go`
- **Surfaces/operations**: `internal/app/app.go`, `operations.go`, `run_control.go`, `run_control_inventory_test.go`, `durable_operations.go`, `operation_runner.go`, `sprint_commands.go`, `study_commands.go`, `health_commands.go`; `internal/web/run_handlers.go`, `operations_contract_test.go`, import-boundary tests (web, runcontrol)
- **Runtimes**: `internal/platform/runtime/runtime.go`, `opencode.go`, `agentwrap.go`; `internal/platform/config/config.go`
- **State versions/events**: `internal/runcontrol/migration.go`, `sanitize.go`, `sqlite.go` (Append gate), `model.go`; git commits `c455510`, `eeaa034`
- **Empirical probe**: compiled `runcontrol/sanitize.go+model.go` in isolation and ran a draft through `sanitizeEventDraft` (read-only; target repo untouched)

### Architecture assessment

The module-driven structure absorbs change well at the boundaries that were designed for it: surfaces compose through injected runners (`app.go:36-38,122-124`) with import-boundary tests enforcing `web -> app`-only and runcontrol-as-leaf; browser operation kinds are pinned by a producer/consumer contract table (`operations_contract_test.go:26-52`); runtime-backed CLI entries are pinned by a source inventory test (`run_control_inventory_test.go`); runcontrol schema migration is properly versioned with backups, identity-checked locks, and integrity gates (`migration.go:25-82`). The one prior stage insertion (`code-context`, roadmap Phase 5) was absorbed end-to-end including legacy-state reinterpretation without a schema bump.

The stress point is **cross-package vocabularies with no owner and no inventory test**: event payload facts and governed-input lists are enumerated independently in platform, app, runcontrol, and web. The last two observability commits demonstrate the failure mode concretely.

### Candidate findings

---

**ID: SPECIALIST-24B-F1**
**Priority: P1**
**Claim**: The durable event vocabulary has three uncoordinated promotion/filter layers plus a downstream reader; new observable fields are promoted upstream but silently dropped by the runcontrol storage allowlist, so recent observability work does not survive durability and every such event accrues omission noise.

**Evidence**:
- Producer layer 1 (adapter): `internal/platform/runtime/runtime.go:592-604` promotes `tool,name,title,detail,text,delta,output,message,state,status,action,phase` to top level (commit eeaa034).
- Producer layer 2 (app): `internal/app/run_control.go:435` promotes `tool,title,detail,text,delta,content,message,action,state,status,native_type,line` plus namespaced keys; `:479-486` force-extracts `tool,action,title,detail,text,delta` (commit c455510); `internal/app/durable_operations.go:150` writes `"detail"` directly.
- Storage gate: `internal/runcontrol/sanitize.go:10-17` allowlist contains none of `title,detail,text,delta,content,output,phase,name,native_type,line`; `sqlite.go:617` applies it on every `Append`. Allowlist unchanged since initial commit e09d394.
- Consumer: `internal/web/run_handlers.go:288,293` renders durable payload keys `text,delta,detail,message,content,title,output` into the run.html timeline — only `message` is allowlisted.

Empirical probe of the real `sanitizeEventDraft`: from a draft carrying all promoted keys, survivors were `{action,count,kind,message,reason,runtime_event_id,runtime_run_id,session_id,state,status,tool,type}`; `title,detail,text,delta,content,output,phase,name,native_type,line` all dropped, with `Omission{Reason:"unsafe event detail omitted", Count:11}` attached. No test asserts survival of these keys through Append→Events (`run_handlers_test.go:276` fixture uses only `kind/type/tool`).

**Architectural reason**: boundary + drift (change-surface). runcontrol owns the vocabulary gate but nothing ties it to producers/consumers; two overlapping promotion mechanisms with divergent key sets and depths now coexist (adapter: depth-4 deterministic incl. `name/output/phase`; app: one-level + non-deterministic `findNestedString` incl. `content/native_type/line`).

**Concrete consequence**: The feature c455510 claims ("surface agent stream details in durable run events … render in run.html timeline") is defeated at the storage gate: replayed timelines still show empty DetailText whenever only dropped keys exist, while each event gains an "Omitted N detail item(s)" row (`run.html:18`), degrading signal-to-noise. Live SSE built from `OperationEvent` (`web/operations.go:264`) shows `detail`, so live and replay views diverge. Every future observability fact must be synchronized across ≥4 files with zero compile/test protection.

**Counter-evidence searched**: Checked for a second unsanitized read path (none: page, pagination, and `followRunSSE` all use repository events, `run_handlers.go:202,467,490`); checked whether omissions make the drop visible (they do, but as noise, not content); checked adapter-level promotion as sufficient (insufficient — same allowlist applies post-hoc); checked tests asserting end-to-end survival (none).

**Confidence**: high

**Smallest useful action**: Decide that platform/runtime owns promotion (delete the app-level duplicate in `run_control.go`), extend `allowedEventPayloadFields` to cover the promoted set, and add one round-trip test asserting promoted producer keys survive `Append`→`Events` and reach `newRunEventView`.

---

**ID: SPECIALIST-24B-F2**
**Priority: P2**
**Claim**: New-stage/new-input absorption silently degrades guarantees through closed lists in foreign packages: `governedOperationInputs` omits the area-reasoning directory from Prepare→Run fingerprints, and both it and the review base-input list must be hand-extended per stage with no failure when forgotten.

**Evidence**:
- `internal/app/operations.go:506-518`: fingerprint inputs enumerate individual sprint artifacts (`requirements/code-context/sprint-index/technical-handbook/reasoning/plan.md`) plus `docs/` as a directory, but omit `projects/<p>/sprints/<s>/reasoning/` (the `StageAreaReasoning` output directory, path per `domain.go`/`sprint_test.go:33`).
- Reviewers *do* consume those bytes dynamically: `internal/sprint/review.go:253-274` reads every `reasoning/*.md` into the manifest as governed inputs; plan prerequisites validate them too (`service.go:963-977`).
- Fingerprint authority: `operations.go:120-123, 272-278, 289-291` — Prepare issues `ExpectedFingerprint`, Run rejects mismatches (`ErrStaleOperation`). Area-reasoning edits between confirm and run escape this check while changing what reviewers will read.
- Same-list hazard for future stages: adding `StageX` requires editing `operations.go:506-518` and `review.go:213-220` (hardcoded base list) or coverage/fingerprints silently miss it; unlike the flow switches, omission produces no error anywhere.

**Architectural reason**: change-surface + drift. The authoritative stage inventory lives in `sprint.PlanningStages()` (`domain.go:264-273`), but app-layer security-relevant lists re-enumerate it by hand with no inventory test (the repo already uses this pattern elsewhere: `import_boundary_test.go`, `run_control_inventory_test.go`, `operations_contract_test.go`).

**Concrete consequence**: A user confirms a review/smoke operation in the browser, an area-reasoning artifact is edited, and Run proceeds against changed evidence under a "inputs unchanged since preparation" guarantee that is quietly weaker than advertised. For the next roadmap stage, the likely miss is invisible: no error, just reviewers or staleness checks that don't see the new artifact.

**Counter-evidence searched**: Verified review manifests compensate for review-time content (they do, so verdict provenance is intact); verified `docs/` directory handling proves directories are supported by `fingerprintOperationInputs` (`operations.go:546-569`), so the omission isn't a mechanical limitation; searched for a test tying `governedOperationInputs` to `PlanningStages()` (none); checked whether sprint-side tests would catch a missed app list entry (they cannot — different package).

**Confidence**: high (current-stage gap demonstrable today; future-stage cost inferred from code-context insertion history)

**Smallest useful action**: Derive sprint governed inputs from `sprint.ArtifactRelPath` over `PlanningStages()` (including the reasoning directory) instead of literals, or add an inventory test asserting every planning stage artifact appears in `governedOperationInputs` and in `PrepareReview`'s input enumeration.

---

**ID: SPECIALIST-24B-F3**
**Priority: P3**
**Claim**: Stage-set evolution is absorbed by structural shape-sniffing rather than schema versions, and the reinterpretation exists only on the file path, leaving the database load path unable to absorb the next mid-pipeline insertion.

**Evidence**:
- `FlowStateSchemaVersion` remained 2 across the code-context insertion; old shapes are detected positionally: `internal/sprint/state.go:74-76,83-94` (`isPreCodeContextStages`: exact length `len(PlanningStages())-1` plus hardcoded legacy order) and rewritten by `interpretPreCodeContextStages` (`state.go:96-113`); `Status` suppresses persisting such states via a second file re-read (`service.go:150,191`).
- The SQLite load path runs none of this: `state_database.go:19-36` unmarshals straight into `LoadFlowState`'s validate branch (`state.go:21-31`), which enforces `len(state.Stages) == len(PlanningStages())` (`state.go:307`) — a 6-stage DB record would be rejected as malformed rather than interpreted. Today unreachable only because every current writer validates against 7 stages first (`state_database.go:75`, `storage_commands.go:155`).
- Each further insertion needs a bespoke detector/interpreter pair and correct chaining order; the ordinal switch `flowStages` (`flow.go:176-201`) hardcodes end indexes 1..8 that must be manually renumbered.

**Architectural reason**: lifecycle (state-version ownership) + change-surface. Schema version no longer identifies state shape, so shape knowledge lives in ad-hoc predicates scattered beside the loader.

**Concrete consequence**: Second insertion (roadmap explicitly anticipates more workflow stages: "Product Phase 5 adds the *first* new workflow") costs a growing family of positional predicates, and any writer that ever persisted a pre-insertion shape into productstate turns into an unrecoverable `ErrFlowStateMalformed` on the DB path.

**Counter-evidence searched**: Tests pin stage positions per target (`code_context_test.go:173-479`, `plan_test.go:63`, `reasoning_test.go:169-191`), so renumbering mistakes in `flowStages` would be caught — why this is P3, not P2; searched for any historical writer of 6-stage DB records (could not establish; see open questions); confirmed file-path migration is exercised (`migrateFlowStateV1`, `PreviousFlowStateSchemaVersion`).

**Confidence**: medium

**Smallest useful action**: When the next stage lands, bump `FlowStateSchemaVersion` and route both file and DB loads through one shared migration chain keyed on version (the runcontrol `migration.go` pattern), instead of adding another `isPreXxxStages` predicate.

---

**ID: SPECIALIST-24B-F4**
**Priority: P3**
**Claim**: Runtime construction evolved into three divergent seams, so introducing a second runtime requires discovering three different extension points and contradicts the repo's own stated composition doctrine in one case.

**Evidence**:
- Sprint: injectable factory through the composition root — `internal/app/sprint_commands.go:21-24`, defaulted at `app.go:122-124`.
- Study: package-global mutable var overridden by tests — `internal/app/study_commands.go:22-25`, mutated at `study_run_commands_test.go:194-198`, `study_status_commands_test.go:89-94`; introduced early (37633e0) and never migrated when the sprint factory pattern arrived later (76711a7).
- Health: no seam, direct construction — `internal/app/health_commands.go:113`.
- `docs/architecture.md:22-24` states there is "no package-global mutable registry"; `var studyRuntimeFactory = …` is one.

**Architectural reason**: drift. Same concern, three idioms, across adjacent files.

**Concrete consequence**: When `runtime.default` grows a second value (config currently hard-rejects others at `config.go:409-411`), health will keep probing opencode and study keeps its hidden global unless the implementer knows all three sites; until then the inconsistency is latent.

**Counter-evidence searched**: Single-runtime scope is explicit CURRENT-CONTRACT (`docs/configuration.md:181`), so the lack of dispatch today is intentional and not reported as a defect; the underlying `runtime.Runtime` interface (`platform/runtime/runtime.go:245-249`) is genuinely runtime-neutral, so the abstraction itself is earned and sound.

**Confidence**: high

**Smallest useful action**: Route study and health through the existing `SprintRuntimeFactory`-style injection (one shared runtime factory on `app.Config`), deleting the package var.

### Defended architecture / rejected hypotheses

- **"Config advertises runtimes it can't use"** — rejected as defect: validation error and docs (`configuration.md:181`) explicitly scope to `opencode`; deliberate, documented contract.
- **"Disabled snapshot freshness is dead/broken policy"** — rejected: `freshness_policy.go:1-14` is documented intentional debt with a precise re-enable condition, and digest/existence checks remain enforced.
- **"`flowStages` ordinals are a defect"** — rejected standalone: fail-closed default, and position-pinning tests across `code_context_test/plan_test/reasoning_test/handbook_test` catch renumbering; folded into F3's checklist framing.
- **"Runcontrol migration is unprepared for version changes"** — rejected: forward-only migration with WAL checkpoint, bounded backups, process-identity-checked lock, double-check after lock, integrity verification (`migration.go:25-82,236-266`).
- **"Web/TUI duplicate workflow semantics"** — rejected: surfaces consume typed use cases; browser kinds pinned by contract table; TUI reads the same run snapshots/events.

### Open questions

1. Could any pre-code-context binary have written 6-stage sprint flow records into the productstate SQLite DB (i.e., did productstate writes precede the code-context stage)? If yes, F3's DB-path asymmetry is a live data-compatibility bug, not latent.
2. Is the fidelity difference between live operation SSE (`detail` present) and durable journal replay (dropped) intended, or should both render identically? This decides whether F1's fix belongs in the allowlist or in a deliberate two-tier vocabulary.
3. Does the next roadmap chunk commit to additional planning stages? If yes, F2/F3 move from hygiene to near-term correctness work.
