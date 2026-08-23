I'm running the generalist-02 whole-system review. Let me get oriented first.
Now let me survey the planning workspace and launch parallel discovery subagents.
Build and vet pass. Now launching parallel bounded-discovery subagents while I read core files myself.
Strong evidence base returned. Now I'll verify the key claims myself before promoting anything to a finding.
TUI omission of `code-context` confirmed at tui/model.go:442-458. Now verifying the app-layer duplication, shared SQLite file, error classification, and lock helper claims myself.
Shared-file/two-schema confirmed (store.go:19 vs sqlite.go:22, divergent DSN flags `_defensive`). Checking run-control quota semantics against co-located product tables, plus the second poll loop and error-classification sites.
Quota coupling fully confirmed (retention.go:35-56 sums whole-file bytes; gates Accept/Append; Compact can only prune run-control's own tables). Final verification batch on the stage-switch copies and overview parsing duplication.
All verification complete. Writing the review report.

---

# UltraPlan Architecture Gauntlet — GENERALIST-02 (whole-system simplicity)

### Scope inspected

**Implementation repo** `/home/antonioborgerees/coding/ultraplan/gauntlet-run/ultraplan-go` @ eeaa034 (61k LOC, 291 Go files):
- Direct reads/verification: `docs/architecture.md`, `internal/app/{operations,durable_operations,run_control,sprint_commands,sprint_usecases}.go`, `internal/app/run_control_inventory_test.go`, `internal/sprint/{service,flow,domain}.go`, `internal/runcontrol/{sqlite,retention,sanitize,migration}.go`, `internal/productstate/store.go`, `internal/web/{operation_handlers,operations}.go`, `internal/tui/model.go`, `internal/study/locks.go`, `internal/sprint/verification_lock.go`
- Mechanical checks: `go build ./...`, `go vet ./...` (clean); `go list -deps` per package for dependency direction
- Delegated bounded discovery (evidence returned, conclusions re-derived by me): lifecycle/state duplication sweep, command-surface audit, sprint-module complexity survey, authoritative-contract extraction

**Planning workspace** `/home/antonioborgerees/coding/ultraplan/gauntlet-run/ultraplan-workspace` @ 368a789: `system/contracts/core/architecture.md` (+ errors, persistence-and-migrations, performance, llm), `projects/ultraplan-go/docs/ARCHITECTURE.md`, `TRD.md` §18/§18A-C, `project-index.md`, `roadmap.md`, sprint 30/34/35 requirements & reasoning.

### Architecture assessment

**Sound.** The module-driven structure is real, not nominal: mechanical checks confirm platform packages import only `platform/config` (never product modules), `internal/web` imports only `internal/app` (enforced by `import_boundary_test.go`), TUI actions route through app use cases, and workflow sequencing is single-homed in product modules (`verify.go:37-39`). Composition is explicit in `cmd/ultraplan`; no global registries. Normalization/fingerprinting/affected-paths have exactly one home (`app/operations.go:409-586`, grep-verified). Anti-over-abstraction doctrine is respected: I found no speculative ports, factories, or layered trees; `platform/runtime` is earned by the agentwrap contract.

**Stressed.** Complexity concentrates in two patterns: (1) *knowledge replicated across interface surfaces and layers* — stage catalogs, error classifications, redaction deny-lists, and the durable-run client protocol each exist in 3–5 hand-maintained copies, with drift already materialized; (2) *physical coupling where logical boundaries are clean* — two schema owners share one SQLite file, and run-control's quota authority measures bytes it cannot govern.

---

### Candidate findings

---

#### ID: GENERALIST-02-F01
**Priority:** P1

**Claim:** Sprint planning-stage knowledge is dispatched through four-plus hand-maintained switch copies across surfaces, and the TUI copy has already drifted: `code-context` is unreachable from the TUI despite Phase 5's contract requiring uniform three-interface surfacing.

**Evidence:**
- Single source exists: `sprint.PlanningStages()` (`internal/sprint/domain.go:264-274`)
- Duplicate dispatchers: `internal/app/sprint_commands.go:138-161` (validate, 10-case) and `:177-198` (prompt, 9-case); `internal/app/sprint_usecases.go:254-279` (`validateSprintStage`, byte-for-byte same shape); `internal/app/operations.go:676-699` (`promptSprintStage`); `internal/app/web_usecases.go:511-539` (`PromptBundle`)
- Drift materialized: `internal/tui/model.go:453` hardcodes `[]string{"requirements", "sprint-index", "technical-handbook", "area-reasoning", "reasoning", "plan", "execute", "review"}` — omits `code-context`; the artifact nav at `model.go:442-451` also omits it; zero occurrences of "code-context" in `internal/tui`
- Contract: workspace `ARCHITECTURE.md`: *"Phase 5 proves that boundary by adding `code-context` as a new sprint stage surfaced uniformly through all three interfaces."*

**Architectural reason:** change-surface / drift

**Concrete consequence:** Adding sprint stage N+1 requires synchronized edits in ≥5 locations plus the TUI menu list; a missed location fails silently (the TUI case shows exactly this). A TUI user today cannot validate or preview the code-context stage or flow-to-it, an inconsistency invisible to any single-surface test.

**Counter-evidence searched:** Web template dropdowns hardcode stages for human label copy (deliberate UX text, verified in `web_usecases.go:119-131` comment); `PromptBundle` is a documented deliberately-narrow projection — but neither explains the TUI validate/preview/flow omission, which removes capability rather than narrowing projection. `run_control_inventory_test.go` string-matches CLI sources, showing awareness of the sprawl, yet pins only the CLI copies.

**Confidence:** high

**Smallest useful action:** Derive the TUI stage list and the validate/prompt dispatchers from one shared table keyed by `PlanningStages()` (a `map[PlanningStage]validateFn` in the sprint service or app layer); delete the four parallel switches. No new abstraction — one lookup replaces five copies.

---

#### ID: GENERALIST-02-F02
**Priority:** P2

**Claim:** Two independent schema owners open the same database file `.ultraplan/run-control.db` with separately maintained DSNs and migration paths, and run-control's quota/compaction machinery measures whole-file bytes — including product-state tables it cannot prune — creating cross-module failure coupling through shared physical storage.

**Evidence:**
- `internal/productstate/store.go:19` and `internal/runcontrol/sqlite.go:22` declare the identical `DatabaseRelativePath = ".ultraplan/run-control.db"`
- Two `open()` implementations: `productstate/store.go:55-90` (own DSN `_busy_timeout/_foreign_keys/_journal_mode=WAL/_synchronous=FULL/_txlock=immediate`, MaxOpenConns(4), process-global cache `stores sync.Map:39`, `createSchema` at `:92-116`, **no migration lock**) vs `runcontrol/sqlite.go:60-85` (same flags **plus `_defensive=1`**, plus `migration.go:92-165` migrate lockfile)
- Quota measures the whole file: `retention.go:35-56` `storageBytes()` sums every `run-control.db*` file; gates `Accept` (`sqlite.go:384-395`) and event append (`sqlite.go:618-621`)
- Compaction can only delete run-control's own rows: `Compact`/`compactTerminalRun` (`retention.go:97-207`) touch `runs`/`events` exclusively; under pressure it sets `compactCutoff = now` (`:112-113`), aggressively compacting even fresh terminal history

**Architectural reason:** ownership / lifecycle / failure-semantics

**Concrete consequence:** Unbounded growth of `product_states`/`product_state_items` (no retention found) consumes quota that run-control enforces: compaction destroys recent run-control event history without restoring headroom, then `accept_quota` refuses new runs — a failure users will attribute to run control while the actual cause is another module's schema in the same file. Separately, the already-divergent DSNs (`_defensive`) and lockless-vs-locked schema setup will drift further.

**Counter-evidence searched:** Colocation itself is deliberate — `docs/migration-product-state.md:24-29` documents importing product state into this exact DB as authoritative; WAL + busy timeout makes concurrent writers safe. But no doc addresses quota interaction, schema-ownership protocol, or why productstate needs no migrate lock. PERSIST-SCHEMA-001 requires clear per-entity ownership; ownership here is split *within* one file with no coordination mechanism.

**Confidence:** high (facts); medium (practical impact — defaults give ≥64 MiB headroom, so pain is deferred)

**Smallest useful action:** Either exclude non-run-control relations from quota accounting (per-table `page_count` attribution) or extract one shared workspace-DB open/migrate helper used by both packages; do not split the file.

---

#### ID: GENERALIST-02-F03
**Priority:** P2

**Claim:** The durable-run client protocol (owner loop: 1 s tick → snapshot poll → cancellation ack → cancel; 5 s heartbeat; 10 s reconcile) is implemented twice inside `internal/app` with visibly divergent error handling, and the accept/event/finish choreography is again hand-rolled separately in the TUI and web clients.

**Evidence:**
- Loop A: `internal/app/run_control.go:212-261` (inline goroutine in `controlledRuntime.StartRun`)
- Loop B: `internal/app/durable_operations.go:178-219` (`durableOperationManager.controlOperation`)
- Identical protocol skeleton, same `runcontrol.OwnerTickInterval/HeartbeatInterval/ReconciliationInterval` constants; drift visible: Loop A tolerates `runCtx.Err()` races before failing (`:225-227,241-243`) and uses variable lease `runControlLease` (`:240`), Loop B cancels unconditionally on first error and uses `OwnerLeaseDuration` (`:204`)
- Client choreography duplicated again: `internal/tui/app.go:232-269` (digest `sha256(canonical+"\x00"+fingerprint)` → `AcceptOperation` → `RecordOperationEvent` → `FinishOperation`) vs `internal/web/operations.go:181-255`

**Architectural reason:** drift / change-surface

**Concrete consequence:** Any protocol revision (ack semantics, lease policy, reconcile options, coalescing rules — the coalescing window is likewise coded twice: `run_control.go:175-205` vs `durable_operations.go:137-164`) must be replicated across owners that share no code; the existing error-handling fork shows the copies are already semantically different, so reconciliation behavior after transient DB faults now depends on which surface started the work.

**Counter-evidence searched:** The loops belong to two distinct managers wrapping different execution styles (direct runtime vs accepted-operation map) — a real distinction, but both drive the identical repository fence; a ~40-line shared `ownerLoop(repo, fence, cancel, onErr)` helper covers both without merging the managers. TUI/web choreography differences (subscriber lifecycle) justify some divergence but not the duplicated dedup-digest derivation.

**Confidence:** high

**Smallest useful action:** Extract one owner-loop function in `internal/app` parameterized by error policy; leave TUI/web choreography but move digest derivation next to `AcceptOperation`.

---

#### ID: GENERALIST-02-F04
**Priority:** P2

**Claim:** Error-to-outcome classification exists as three parallel taxonomies over the same domain errors, two of which match on error-message substrings, making exit codes and HTTP statuses sensitive to prose wording.

**Evidence:**
- CLI: `internal/app/sprint_commands.go:259,353` `strings.Contains(err.Error(), "runtime")` → ExitRuntime; `:350` `"failed tasks"` → ExitPartial — coexisting with the correct `mapSprintError` (`errors.Is/As`, `:656-675`)
- Web: `internal/web/operation_handlers.go:766-771` substring fallback (`"validation"|"incomplete"|"prerequisite"` → 422; `"lock"|"in progress"` → 409; `"unavailable"` → 424) alongside `errors.Is` cases for web-local sentinels; `safeOperationCause` (`:782-791`) repeats the same substrings a fourth time
- App layer already has typed failures (`failedOperation` uses `errors.Is(sprint.ErrFlowStateMalformed…)`, `operations.go:634-646`)

**Architectural reason:** drift / failure-semantics

**Concrete consequence:** Rewording any wrapped domain error (or introducing a message containing "runtime"/"lock" incidentally) silently changes CLI exit class or maps a server fault to 4xx. Tests assert status codes, so regressions appear as mysterious test failures rather than pointing at the string match.

**Counter-evidence searched:** The roadmap explicitly defers *full canonical structured error payloads* to the Stable Release Gate, so absence of a unified error framework is not a defect. But typed sentinels already exist and are used by one of the three taxonomies; the substring copies are not required interim scaffolding — they bypass the mechanism that is already there.

**Confidence:** medium-high

**Smallest useful action:** Route `sprint_commands.go` classification through `mapSprintError`, and add typed sentinel checks (or an `interface{ HTTPClass() }` carrier) for the six categories web guesses at; delete both substring lists.

---

#### ID: GENERALIST-02-F05
**Priority:** P3

**Claim:** "What counts as sensitive" is re-implemented in ≥5 redaction/bounding engines with materially disjoint deny-lists, so sensitivity classification is distributed knowledge whose weakest instance defines the leak surface.

**Evidence:**
- `internal/runcontrol/sanitize.go:74-82`: keys containing `secret|token|credential|password|prompt|payload|stdout|stderr|path|command`; values with `bearer |sk-|ghp_|github_pat_|-----begin private key` (`:84-98`) — but **no** `authorization|cookie|auth`
- `internal/app/run_control.go:501-509`: adds `authorization|cookie|api_key|apikey|auth` — but **no** `prompt|stdout|path|command`
- `internal/web/operations.go:632-656` `safeProjectedText`: inline markers `token=|secret=|authorization:|cookie:` + `/home/` redaction only
- Plus `study/locks.go:184-202` `sanitizeCommand` and `platform/config/redaction.go` `RedactValue` (consumed via `usecases.go:244-246`)
- Constant duplication: 16 KiB encoded-event cap declared independently (`runcontrol/model.go:12`, `web/operations.go:24`)

**Architectural reason:** drift / boundary

**Concrete consequence:** A new credential-bearing field name (e.g., `session_auth_header`) is caught by app's list but not run-control's; a key like `stdout_path` passes app's list. Each future sensitive class must be added in up to five places; omissions are undetectable locally.

**Counter-evidence searched:** Layering is partly intentional defense-in-depth: `sanitize.go:19-21` positions run-control as the *final* storage gate and its key list is the broadest; web redacts again at browser presentation. This limits blast radius but does not explain divergent deny-*lists* rather than one shared classifier composed per sink. OBS-PII-001/DATA-LOG-001 make sensitivity classification a single normative concept.

**Confidence:** medium

**Smallest useful action:** Move one deny-list classifier into `platform/config` (next to `RedactValue`) and have each engine call it, keeping sink-specific bounds local.

---

#### ID: GENERALIST-02-F06
**Priority:** P3

**Claim:** Cross-process lock plumbing is copy-pasted between the two product modules: a verbatim PID-liveness helper, structurally identical cleanup-uncertain record types, and a third bespoke lockfile scheme in run-control's migration path — none shared despite `platform/process` already existing as the generic process-infrastructure home.

**Evidence:**
- `study/locks.go:17-23` `processAlive` ≡ `sprint/verification_lock.go:95-101` `verificationProcessAlive` (identical bodies incl. EPERM handling)
- `sprint/cleanup_uncertain.go:15-74` ≡ `study/cleanup_uncertain.go:15-62` (`CleanupUncertainRecord`, same validation, both write bare `OwnerPID = os.Getpid()`)
- Third O_EXCL+stale-reclaim lockfile: `runcontrol/migration.go:92-165`
- Meanwhile TRD §18C states the doctrine these copies implement inconsistently: *"Liveness is conservative. A PID alone is insufficient because of PID reuse"* — the product locks rely on exactly kill(pid,0), where a reused PID keeps a dead lock looking live until manual force-unlock (`ForceUnlockRunLoop`, `study/locks.go:161-167`)

**Architectural reason:** drift / change-surface

**Concrete consequence:** Any fix to PID-reuse handling (e.g., adding birth-token checks to product locks) must be applied in two modules plus the migration lock; the modules will continue diverging (they already differ in heartbeat: sprint attempts carry `HeartbeatAt` with 2 h staleness at `verify.go:455-467`; study has none).

**Counter-evidence searched:** Module-owned locks are contractual (TRD §18A: *"Existing study locks stay in `internal/study`; sprint mutation exclusion belongs in `internal/sprint`"*) and `runcontrol/interfaces.go:17-19` deliberately reserves birth-identity rigor for run control. Ownership separation is sound — the duplication is of *generic primitives* (liveness probe, marker record), not of product policy; a tiny shared helper in `platform/process` violates nothing.

**Confidence:** high (facts); low stakes

**Smallest useful action:** Hoist `processAlive` (and optionally the marker-record JSON shape) into `platform/process`; leave lock policies per module.

---

#### ID: GENERALIST-02-F07
**Priority:** P3

**Claim:** Two same-named `flowFailedStages` functions with divergent semantics for prior-stage status live in the same package, one falling back to the other on read failure — in the code path that persists failure state (~49 call sites).

**Evidence:**
- Package-level `internal/sprint/flow.go:304-315`: marks all pre-target stages **complete**, target failed, built from `emptyPlanningStageStates`
- Method `(s Service) flowFailedStages` `internal/sprint/service.go:1077-1092`: derives from actual artifacts via `DeriveStages`; on snapshot-read error delegates to the package-level variant (`:1079-1080`)
- Failure-block boilerplate `stages := flowFailedStages(...); _ = SaveFlowState(...); return FlowResult{…}` repeated throughout `service.go` (49 occurrences counted)

**Architectural reason:** drift / change-surface

**Concrete consequence:** After a failed flow, whether prior stages are recorded `complete` (optimistic, artifact-free) or derived from real artifacts depends on which overload resolved at the call site and whether the snapshot read succeeded — two recovery-state shapes for the same failure, discoverable only by reading both functions. Editing failure semantics requires auditing ~49 sites.

**Counter-evidence searched:** Initially suspected infinite recursion at `service.go:1080`; disproved — the fallback targets the non-recursive package-level function. The artifact-derived variant exists to preserve real progress during resume; the optimistic variant covers unreadable snapshots. Intent is defensible; the shared name and silent divergence are the defect.

**Confidence:** medium

**Smallest useful action:** Rename the package-level fallback (e.g., `fallbackFailedStages`) and add a comment tying the pair together; no behavioral change.

---

#### ID: GENERALIST-02-F08
**Priority:** P3

**Claim:** Review/smoke staleness computation is maintained in three places although the strict-freshness policy that consumes it is disabled by a package constant — pure carrying cost of a switched-off feature.

**Evidence:**
- Switch: `internal/sprint/freshness_policy.go:11-14` `strictCompletedReviewSnapshotFreshness = false` (rationale commented `:3-10`)
- Triplicated staleness logic: `service.go:169-179` (Status recomputes review staleness by **re-running `PrepareReview`**), `smoke_protocol.go:185-194`, `verify.go:178-192`

**Architectural reason:** change-surface

**Concrete consequence:** Every change to `PrepareReview` inputs or manifest shape must keep three staleness derivations consistent, and `Status` pays a full prepare (filesystem enumeration + hashing) just to answer a question the current policy ignores.

**Counter-evidence searched:** `freshness_policy.go:3-10` documents *why* strictness is off (snapshot identity churn), i.e., intentional debt with an in-source rationale; the efficiency test suite pins related behavior. Documented intentional debt lowers severity to P3, but three live copies of disabled-feature logic exceed the minimum needed to re-enable later.

**Confidence:** high (facts)

**Smallest useful action:** Consolidate to one exported `ReviewFreshness(...)` predicate called by all three sites; keep the policy constant as-is.

---

### Defended architecture / rejected hypotheses

1. **"The web layer leaks workflow semantics."** Rejected as a systemic claim. `import_boundary_test.go:12-36` mechanically restricts `internal/web` to stdlib + `internal/app`; handlers are DTO/transport except the two pockets reported in F01/F04. The operation hub correctly holds transport-lifecycle state only, bridged to durable cancellation at one point (`operation_handlers.go:194-201`).
2. **"Multiple lock mechanisms are accidental duplication."** Mostly rejected. Product-owned mutation leases, run-control SQLite leases+fencing, the web hub mutex, and the migration lockfile guard genuinely different authorities, per `docs/architecture.md:111-113`, `runcontrol/doc.go:1-6`, and TRD §18A/§21.2. What survives is the generic-primitive copying (F06), not the plurality of policies.
3. **"Overlapping status enums across subsystems are a modeling defect."** Largely rejected. Each enum serializes its own store per the state-authority table in workspace `ARCHITECTURE.md` (product JSON = machine state for owned concerns; run records = operational observation; hub = transport). Boundary translation between them is necessary, and `phase3-json-schemas.md:23` distinguishes verdicts from process statuses deliberately. Residual cost is absorbed into F03/F04 rather than charged as a separate defect.
4. **"`platform/runtime` is an unearned abstraction over a single provider."** Rejected. The agentwrap/OpenCode SDK is contractually mandated (TRD §11, project-index: *"Do not invent competing runtime contracts"*), `platform/*` imports no product modules (mechanically checked), and sprint-specific semantics demonstrably stayed out of it.
5. **"`Review`'s 232-line body and per-stage copy-paste in `service.go` prove god-object decay."** Downgraded, not filed as its own finding. 143 sprint tests — especially `efficiency_improvements_test.go` and the resume/rebase/repair-bound suite — pin individually specified behaviors; much length is essential spec complexity. The genuinely accidental parts (F07's twin functions, the 49× failure block) are captured there; the rest would demand restructuring beyond "smallest useful action" with tests already defending current shape.
6. **"TUI reimplements product logic."** Rejected — TUI imports only `internal/app` at runtime and routes every action through use cases; its sole sin is the hardcoded stage list (F01).
7. **"Missing correlation IDs / structured errors / ports are defects."** Rejected as premature: the workspace roadmap's Skeleton/Local-CLI Gate explicitly defers these (*"Not required until later gates: full canonical structured error payloads … request/trace correlation IDs … runtime ports"*).

### Open questions

1. Is TUI `code-context` exclusion an undocumented scope decision (which would soften F01 from contract violation to drift), or an oversight against Phase 5's "surfaced uniformly through all three interfaces"? No statement was found either way.
2. Are `product_states` rows expected to remain trivially small forever? If not, F02's quota coupling becomes a foreseeable availability incident; if yes, a one-line size budget in `migration-product-state.md` would close it.
3. Implementation HEAD (eeaa034, "observability: promote nested runtime payload facts…") contains post-Sprint-35 work whose requirements/reasoning do not yet exist in the authoritative workspace (Sprint 35 has only `requirements.md`). Which document version governs review of those commits — relevant to whether the payload-promotion changes conflict with the 16 KiB allowlisted-payload contract in TRD §18C.
