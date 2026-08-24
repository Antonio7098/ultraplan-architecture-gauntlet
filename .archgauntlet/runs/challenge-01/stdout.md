# CHALLENGE-01 — Product/domain tribunal

## Scope inspected

**Prior-stage evidence** (`.archgauntlet/runs/`): populated reports generalist-01..04, scout-01/02/03/05, specialist-03b, 05b, 23a, 23b, change-01, change-07. 24 prior runs have **empty `stdout.md`** despite "succeeded" status; their `stderr.log` transcripts were mined instead. Notably, both project-catalogue specialists (04a/b), both artifact-persistence specialists (15a/b), and both agent/prompt specialists (22a/b) died mid-investigation with no conclusions; the areas they owned were re-covered directly by me. specialist-03a's transcript shows it inserted a DEBUG println into `internal/study/run_loop.go` during testing; I verified the target tree is clean at `eeaa034` (`git status`, `git log -3`) — no contamination persisted.

**Implementation repo** (`ultraplan-go` @ eeaa034, clean): `internal/app/{operation_runner,sprint_commands,sprint_usecases,usecases,tui_commands,serve_commands,web_usecases,operations}.go`; `internal/sprint/{service,state,state_database,flow,domain,smoke,smoke_author,prompts,prompt_bundle,prompt_context,session_state,execute,verify,reasoning,artifacts,index,cleanup_uncertain,verification_lock,locks}.go`; `internal/study/{locks,run_all,run_loop,run_loop_test,cleanup_uncertain,init_yaml}.go`; `internal/project/{discovery,validation}.go`; `internal/web/{handlers,server}.go`; `internal/workspace/skills.go`; `docs/recovery.md`.

**Planning workspace** (@ 368a789): `projects/ultraplan-go/docs/ARCHITECTURE.md` (:23), `roadmap.md`, `sprints/11-run-all-batch-execution/{requirements,reasoning}.md`, `docs/TRD.md` (:2208), `.agents/skills/` listing.

Every promoted claim below was re-derived from primary source by me; provenance to prior findings noted per item.

## Architecture assessment

The product/domain core is structurally sound: one canonical stage list (`sprint.PlanningStages()`), fail-closed flow-state validation, lease discipline for mutating operations, artifacts-primary/state-secondary authority, a model 3-tier reasoning resolver (`project.ResolveReasoningDefault`), and an import boundary (`internal/web` → `internal/app` only) that is mechanically enforced. Most broad "duplication" hypotheses from earlier stages did not survive contact with contracts (see rejected hypotheses).

What is stressed is narrower and specific: **persistence-timing policy is decided per hand-copied branch inside the sprint flow functions** (producing three concrete defects below), **two surfaces advertise capabilities that cannot work** (TUI/web smoke start; TUI code-context), **prompt-default resolution has four owners with divergent failure semantics**, and **study's concurrency regime (PID-only locks, unlocked run-all, web-only healing) is materially weaker than its documentatiAll evidence is pinned. Writing the challenge-01 report now:
Report written. Final citation spot-checks before closing out:
Report complete and verified. Summary of what challenge-01 produced:

**Output:** `.archgauntlet/runs/challenge-01/stdout.md` (209 lines)

**Key results of the challenge:**

- **1 prior finding killed outright:** GENERALIST-01-F05 ("RunAll unlocked is an ownership defect") — rejected via sprint-11's explicit contract requiring no per-study locks/multi-process coordination for the ephemeral `run-all`.
- **1 prior finding corrected:** GENERALIST-04-F03 claimed the study priority-tier test was *deterministically* red; my 5 isolated runs showed 4 FAIL / 1 OK — flaky, not deterministic. Carried at P2 with corrected mechanism.
- **17 surviving findings**, each re-derived firsthand: two P1s (web/TUI smoke-start can never succeed; dry-run flow-state writes on unguarded failure branches), nine P2s (unleased `Status` persistence, twin `flowFailedStages`, TUI code-context omission, prompt-resolution quadruplication + error-text prompts, flaky release-gate test, web-only healing authority, area-reasoning inventory/fingerprint drift, PID-signalling vs TRD doctrine), six P3s — including **one new discovery** (F11): composed prompts split on first boundary-marker occurrence while agent-authored artifacts are embedded verbatim with no marker validation, completing the probe specialist-22a abandoned.
- **8 hypotheses defended/rejected** with evidence; process note that 24 prior-stage outputs were empty and their transcripts mined, with project-catalogue and agent/prompt areas re-covered directly since both twin specialists died without conclusions.
- Target repo verified uncontaminated (clean at `eeaa034`) despite specialist-03a's transient debug edit.
tarts, gets "configure the smoke model/runtime" though config is fine and CLI works — and flow-state already holds a running→failed smoke attempt needing recovery.
- **Counter-evidence searched:** Default runtime in `NewService` (none); alternate injection via `SmokeSettings` (none); forced-dry-run on interactive start (only prepare forces `DryRun:true`); docs disabling interactive smoke (none); tests driving `OperationSmokeStart` through the runner (none — fakes bypass it).
- **Confidence:** high
- **Smallest useful action:** Build the smoke case via `sprintRuntimeService(deps, root, ...)` like its siblings; add one integration test asserting `OperationSmokeStart` reaches authoring with a non-nil runtime.

### CHALLENGE-01-F02
- **Priority:** P1
- **Claim:** Two failure branches of the planning-flow persist `flow-state.json` **during dry runs**, unlike every sibling branch. Dry runs acquire no mutation lease, so a failing preview writes stale state over a concurrent lease holder's progress, violating the documented no-write contract and the repo's own tests.
- **Evidence:** Unguarded writes reachable with `DryRun=true`: `internal/sprint/service.go:740` (FlowTechnicalHandbook, "selected evidence validation failed") and `service.go:818` (FlowReasoning, placeholder requirements). Guarded siblings of the identical checks: `:555/:563/:571` (sprint-index), `:648/:657` (plan), `:729` (handbook placeholder), `:746` (handbook prompt error), `:836` (reasoning prerequisites). Lease skipped on dry runs: `flow.go:44-52` and `flow.go:117-133` (guard at `:123`). Tests pin the opposite behavior: `handbook_test.go:64-73` asserts no `flow-state.json` after dry run. Provenance: SPECIALIST-23A-F01/F02; both branches and guards verified firsthand.
- **Architectural reason:** failure-semantics — "does this outcome persist?" is embedded per copied branch rather than owned once, so it drifted.
- **Concrete consequence:** With `execute` checkpointing under lease, `sprint flow --to reasoning --dry-run` against placeholder requirements rewrites flow-state from a stale snapshot (stage statuses revert; review/smoke records re-merged only because they were nil in the dry-run state, `state.go:204-215`). On serve, `OperationStageDryRun` reaches the same code while its confirmation claims runtime-free/no-write behavior.
- **Counter-evidence searched:** Tests asserting failing dry-runs *should* persist (none — adjacent tests assert the opposite); lease held during dry runs (no); `WithoutStatusWrites` covering these paths (it gates only `Status`, `service.go:191`); git history showing intent (unguarded lines predate the guard convention).
- **Confidence:** high
- **Smallest useful action:** Add `if !req.DryRun` guards at `service.go:740` and `:818` plus one regression test per branch; optionally make the flow driver own failure-persistence so the next branch cannot drift.

### CHALLENGE-01-F03
- **Priority:** P2
- **Claim:** `Service.Status` persists derived flow state — including `Review`/`Smoke` verification records taken from the loaded snapshot — without the mutation lease, so a status refresh racing a completing review/smoke regresses durable verification evidence to a pre-completion snapshot.
- **Evidence:** Read-modify-write window in `Status`: load `service.go:151`, derive + `PrepareReview` `:170-189`, save `:191-195` (`if s.statusWrites && !legacyCodeContextState`). No lease anywhere in the path. `SaveFlowState` merge protection applies only when incoming Review/Smoke are nil (`state.go:203-215`) — Status sets them from the snapshot, so protection never fires. DB-authoritative mode is equally last-writer-wins: `saveFlowStateDatabase` serializes the whole header (`state_database.go:69-97`). Unprotected callers: CLI `sprint status` builds a default service (`sprint_commands.go:81-88`); TUI `dashboardUseCases` without readOnly calls `Status` per sprint (`tui_commands.go:34-39`, `sprint_usecases.go:109`); web sets `readOnly: true` (`serve_commands.go:47`, `usecases.go:126-128`). Provenance: SPECIALIST-05B-F01; verified firsthand including DB path.
- **Architectural reason:** authority / lifecycle — presentation participates in durable writes outside the single-writer discipline every other writer obeys.
- **Concrete consequence:** User polls `sprint status --json` (or refreshes the TUI) while review completes in another session: between load and save the review persists `ReviewCompleted`+`LastComplete`; the status save rewrites the old record — status regressed to running, coverage evidence erased, rerun forced. Same clobber path exists for Smoke.
- **Counter-evidence searched:** CAS/UpdatedAt guard in either persistence path (none); lease inside `SaveFlowState` (none); web exposure (excluded — readOnly); merge backfill reaching non-nil stale records (does not).
- **Confidence:** high (mechanism), medium (realized frequency)
- **Smallest useful action:** Route the `SaveFlowState` inside `Status` through `acquireMutationContext`, skipping the save on conflict (mirroring `ReconcileInterruptedMutation`'s graceful degradation).

### CHALLENGE-01-F04
- **Priority:** P2
- **Claim:** The failure path of the stage machine uses two same-named builders with different semantics interchangeably — package-level `flowFailedStages` fabricates all pre-target stages `complete`; `(Service).flowFailedStages` derives from real artifacts — sometimes within one function for the same conceptual event.
- **Evidence:** Definitions: `internal/sprint/flow.go:304-315` (fabricated) vs `internal/sprint/service.go:1077-1092` (snapshot-derived via `DeriveStages`, silent fallback to fabricated at `:1079-1080`). Intra-function split: FlowTechnicalHandbook uses the derived variant for prompt errors but the fabricated one at `:739-740`; FlowSprintIndex `:554` vs `:562`; FlowReasoning prerequisite failure uses the fabricated variant (`:816-818`) unlike siblings. Fabrication marks legitimately `skipped` stages (area-reasoning with no templates) `complete`. ~50 call sites. Provenance: convergent SPECIALIST-05B-F02 + GENERALIST-02-F07; definitions and splits verified firsthand.
- **Architectural reason:** drift / failure-semantics — identical event ("stage N failed") yields materially different durable histories depending on which overload the author picked.
- **Concrete consequence:** A cumulative flow failing at plan persists `area-reasoning: complete` although it was skipped; `renderSprintFlow` displays the fabricated history immediately and the durable record stands until the next `DeriveStages` refresh — conflicting with the repo's explicit-outcome doctrine; editing failure semantics requires auditing ~50 sites.
- **Counter-evidence searched:** Whether fabrication is approximately true at its sites (usually, since runtime-error paths follow prerequisite gates — bounds but does not eliminate divergence: skipped≠complete, `LastRunAt` erasure); infinite-recursion suspicion (disproven — fallback targets the package-level function); tests pinning either semantics (none).
- **Confidence:** high
- **Smallest useful action:** Delete the package-level variant and route all call sites through the snapshot-derived method (keeping its bounded fallback); rename if a temporary alias eases migration.

### CHALLENGE-01-F05
- **Priority:** P2
- **Claim:** The TUI omits the `code-context` stage entirely — zero occurrences in `internal/tui` — against the workspace contract that Phase 5 adds code-context "surfaced uniformly through all three interfaces"; the stage list is hardcoded per surface, and the same catalog is re-dispatched in ≥4 app-layer switches.
- **Evidence:** `internal/tui/model.go:453` hardcodes `{"requirements","sprint-index","technical-handbook","area-reasoning","reasoning","plan","execute","review"}` driving Validate/Preview/Flow nav for every stage (`:455-466`); artifact nav `:442-447` also lacks it; `grep -r "code-context" internal/tui` → 0. Contract: workspace `projects/ultraplan-go/docs/ARCHITECTURE.md:23`. CLI supports it fully (`sprint_commands.go:138-161`); web supports it (templates/sprint.html:40, `web_usecases.go:513-514`, contract tests). Parallel dispatchers: `operations.go:676-699`, `sprint_commands.go:177-198`, `web_usecases.go:511-539`. Provenance: GENERALIST-02-F01; TUI omission and contract text verified firsthand.
- **Architectural reason:** change-surface / drift — one catalog owned by `internal/sprint` is re-enumerated per surface; the copy that drifted removed capability rather than narrowing projection.
- **Concrete consequence:** A TUI user cannot validate, preview, or flow-to code-context — invisible to any single-surface test; adding stage N+1 requires synchronized edits in ≥5 places and the failure mode is silent.
- **Counter-evidence searched:** Deliberate-exclusion comment/doc in TUI (none); web template dropdown labels being UX copy (true, but does not explain the TUI capability gap); whether code-context is reachable via generic stage op from TUI (nav list is the only entry point).
- **Confidence:** high
- **Smallest useful action:** Derive the TUI stage list (and ideally the app dispatchers) from `sprint.PlanningStages()`; delete the hardcoded list.

### CHALLENGE-01-F06
- **Priority:** P2
- **Claim:** Prompt-default resolution (workspace override → else embedded builtin) is implemented four times across three packages with divergent failure semantics, and the sprint variant embeds literal `"# Prompt Load Error"` / `"# Missing Prompt Default"` pages into **runtime** prompts instead of failing or falling back.
- **Evidence:** Four implementations of the mechanical rule: `project/reasoning_defaults.go:52-86` (strict, fail-closed), `study/prompts.go:175-191` (propagates errors), `sprint/prompts.go:241-257` (`sprintPromptTemplate`: unreadable override → inline error text at `:250`; registry miss → `"# Missing Prompt Default"` at `:256`), `sprint/review.go:1739-1765`. Error bodies reach execution-time prompts: renderers feed actual flows (`prompts.go:267/:271` → plan/index/etc.), `smoke_author.go:213`. Contract: workspace ARCHITECTURE.md assigns the mechanical rule to `workspace`; TRD §10.2 requires preferring "an intentional readable workspace override". Source-label construction `"builtin:"+rel` duplicated at three sites. Provenance: CHANGE-07-F01/F02; verified firsthand.
- **Architectural reason:** ownership / drift / failure-semantics — the mechanism lives in consumers, not in its documented owner; failure policies differ per stage.
- **Concrete consequence:** A chmod mishap or registry omission makes plan/review/smoke stages execute against instructions reading "# Prompt Load Error…" — tokens spent, off-contract artifacts produced, failure attributed much later; adding a tier or renaming labels touches ≥4 code files + ~5 test files with nothing forcing coherence (history shows the strict resolver arrived later and older sites were never migrated).
- **Counter-evidence searched:** Downstream validators catching error-text prompts (none; exact-match gating explicitly rejected in `session_state_test.go:58`); intentional dry-run-affordance rationale (undocumented, untested); boundary-translation justification for the *lookup itself* (each site's semantic validation layers are defensible — only the fallback step is duplicated).
- **Confidence:** high (structure), medium (impact — edge-triggered)
- **Smallest useful action:** Add `workspace.ResolveOverrideFile(root, rel) (content, source, err)` implementing the documented rule once; have all four sites delegate, keeping their semantic layers; make non-NotExist read failures return errors on planning paths like the reasoning path already does.

### CHALLENGE-01-F07
- **Priority:** P2
- **Claim:** The study run-loop's priority-tier property is guarded only by a test that over-specifies cross-goroutine Started-event emission order; it fails on the majority of runs at HEAD (flaky, not deterministic — correcting GENERALIST-04-F03), and no CI enforces the roadmap's `go test ./...` gate.
- **Evidence:** Test asserts positional starts `startedTasks[0]`/`[1]` == priority dimension (`run_loop_test.go:299-304`); tasks spawn as goroutines and emit Started inside them (`run_loop.go:357-364`), so emission order follows scheduler runqueue behavior, not schedule order. The scheduler ranking itself is correct (`runnableTaskIDs`, `run_loop.go:527-549` — verified). My measurement: 4×FAIL / 1×ok across five `-count=1` runs (explains why specialist-23a/b reported green suites while generalist-04 reported red). No CI configuration exists. History: `5839f9a` rewrote a race-tolerant set-based assertion into this positional one.
- **Architectural reason:** change-surface / failure-semantics — the verification architecture cannot enforce the repo's own release gate exactly where iteration is fastest; signal-to-noise collapses for the study package.
- **Concrete consequence:** Genuine study regressions hide behind the known-flaky failure; contributors re-run until green, training the wrong habit; the real priority-tier property is effectively untested.
- **Counter-evidence searched:** Scheduler regression (disproved — ranking function verified); parallelism throttling explaining reorder (`effectiveParallelism` doesn't); CI elsewhere (Makefile targets exist, nothing enforces).
- **Confidence:** high
- **Smallest useful action:** Restore the set-based assertion over the scheduled wave via the existing `orderedRuntime` seam; wire `go test ./...` into any pre-push hook or minimal CI.

### CHALLENGE-01-F08
- **Priority:** P2
- **Claim:** Healing authority for interrupted product state belongs de facto to the web surface: `ReconcileInterruptedMutation`/`ReconcileInterruptedRun` have exactly one production caller each — the serve startup path — while run-control rows reconcile at every composition open.
- **Evidence:** Healers: `sprint/locks.go:25-88`, `study/cleanup_uncertain.go:66`. Sole production caller chain: `app/web_usecases.go:355-383` (`ReconcileOperations`) invoked from `web/server.go:75-79` before serving. Run-control startup reconcile: `app/run_control.go:64`. CLI heal deferred to resume-time task conversion (`execute.go:190-194`); `Status` reads without healing. Provenance: GENERALIST-01-F03; caller uniqueness verified by grep over non-test tree.
- **Architectural reason:** lifecycle / ownership — "who converts crashed-owner state to interrupted evidence" sits in a transport adapter instead of the composition root.
- **Concrete consequence:** After a CLI crash, `sprint … status` keeps reporting running tasks indefinitely (until next resume or someone starts `serve`), while `run … list` already shows the run interrupted — the two ledgers visibly disagree for CLI-only workflows.
- **Counter-evidence searched:** Other production callers (none); deliberate server-as-janitor documentation (none found); self-correction on next mutation (true for resume, not for read paths).
- **Confidence:** high (facts), medium (consequence)
- **Smallest useful action:** Invoke both healers where `run_control.go:64` already reconciles at composition open; keep the serve loop as-is.

### CHALLENGE-01-F09
- **Priority:** P2
- **Claim:** App-layer stage inventories silently omit `area-reasoning`: dashboard artifact lists, the validation sweep, and the artifact sort map all skip it, and governed-operation fingerprint inputs omit `reasoning/` outputs even though final reasoning must reference them.
- **Evidence:** Artifact lists omit area-reasoning twice (`sprint_usecases.go:119-127`, `:193-205` — include code-context but no area-reasoning label); validation sweep iterates nine stages excluding `StageAreaReasoning` (`:206`) while the dispatcher supports it; sort map `:240` lacks the label; `governedOperationInputs` fingerprints requirements/code-context/sprint-index/handbook/reasoning.md/plan but not `reasoning/` (`operations.go:504-519`), although `ArtifactRelPath(StageAreaReasoning)` = `…/reasoning` (`artifacts.go:22-23`) and final-reasoning validation requires referencing selected area outputs (`reasoning.go:134`). Provenance: CHANGE-01-F2 (corrected: code-context *is* present in these lists; area-reasoning is the omission); verified firsthand.
- **Architectural reason:** drift / change-surface — parallel hand-maintained inventories beside the owning module's canonical list; survived three stage additions unnoticed.
- **Concrete consequence:** Area-reasoning findings/artifacts never surface in dashboards (invalid area artifacts produce no visible findings), and durable-operation invalidation ignores area outputs that downstream stages consume; the next stage addition repeats the omission class.
- **Counter-evidence searched:** Partial deliberate-exclusion evidence exists (project page surfaces area docs separately, `project_usecases.go:61-62`; multi-file output doesn't fit single-file `DisplayArtifact`) — neither explains the validation-sweep or fingerprint omissions; nothing marks the exclusions intentional.
- **Confidence:** high (drift factual; deliberateness unresolved)
- **Smallest useful action:** Derive the artifact-label/validation-sweep/fingerprint inventories from `PlanningStages()`+per-stage capability metadata, or add a test asserting every planning stage appears in summaries and fingerprints.

### CHALLENGE-01-F10
- **Priority:** P2
- **Claim:** Study cancellation signals a bare lockfile PID with SIGINT gated only by `kill(pid,0)` liveness — the one PID-signalling path in the product — against the system's own doctrine that "PID alone is insufficient because of PID reuse"; the liveness helper is additionally duplicated verbatim in sprint.
- **Evidence:** `study/locks.go:141-158` (`CancelRunLoop`: name match + self-PID guard only, then `syscall.Kill(info.PID, SIGINT)`); `processAlive` = `kill(pid,0)` incl. EPERM-as-alive (`locks.go:17-23`); verbatim twin `sprint/verification_lock.go:95-101` (probes but never signals); run-control solves identity properly with `/proc` birth tokens (`process_linux.go`). Contract: workspace TRD.md:2208. Stale-lock stealing is likewise PID-recycle-sensitive (`locks.go:66-70`). Provenance: GENERALIST-01-F04 + GENERALIST-02-F06; mechanisms verified firsthand.
- **Architectural reason:** failure-semantics / boundary — process-identity authority exists in the codebase and is bypassed by the only signalling caller.
- **Concrete consequence:** A recycled PID receives SIGINT from `study cancel`; conversely a recycled PID keeps a dead lock looking live, blocking new loops until `--force-unlock`.
- **Counter-evidence searched:** Lock-content validation binding PID→identity (only study-name match); alternate cancellation channel coexistence (both channels live today); doc accepting the risk (none).
- **Confidence:** high (mechanism), medium (probability)
- **Smallest useful action:** Record owner birth-token/starttime in `LockInfo` and verify before signalling; hoist the shared probe into `platform/process` while keeping lock policy per module.

### CHALLENGE-01-F11
- **Priority:** P3
- **Claim:** Composed sprint prompts split on the **first** occurrence of the stage-boundary marker, but requirements.md/code-context.md bytes are embedded verbatim with no validation rejecting the marker — an artifact quoting UltraPlan's own boundary string misplaces session-continuation instructions and skews explain/cache diagnostics. New this tribunal (completes the probe specialist-22a abandoned).
- **Evidence:** Verbatim embedding: `prompt_context.go:178-186` (`b.WriteString(requirements)` / codeContext inside EXACT frames); true boundary appended last (`:225-226`). No marker rejection: `ValidateRequirementsContent` (`index.go:35-47`) checks headings/placeholders only; `ValidateCodeContextContent` (`code_context.go:34+`) none. First-Index consumers: `insertStageContinuation` (`session_state.go:134-142`) and `explainComposedPrompt`/cache-key derivation (`prompt_bundle.go:97-110`). Marker constant: `prompts.go:33`.
- **Architectural reason:** boundary — frame grammar is delimited by content-searchable markers while one side of the frame is untrusted agent-authored bytes; the collision class was left unresolved by the dead specialist (its final probe was inconclusive).
- **Concrete consequence:** For a dogfooding sprint whose requirements document UltraPlan's own markers (this repo's exact use case), resumed sessions inject the continuation instruction inside the frame labeled "reproduced exactly", weakening the directive and shifting cache-prefix metadata; `--explain` misclassifies prefix/suffix spans.
- **Counter-evidence searched:** Sanitization between artifact read and composition (none); budget/length guards preventing the pattern (unrelated); tests covering marker collision (none); LastIndex usage anywhere (none).
- **Confidence:** medium-high (mechanism certain from code; impact conditional on marker appearance in artifacts)
- **Smallest useful action:** Locate the boundary relative to the known preceding anchor (`sharedSourceEvidenceClose`) instead of raw Index, or reject/reserve the marker strings in artifact validation; one test each way.

### CHALLENGE-01-F12
- **Priority:** P3
- **Claim:** Reference-name rules are re-derived in four places with different dot policies: module-owned rules may legitimately differ per namespace, but app and web re-implement transport checks instead of delegating, so the same ref fails differently per surface.
- **Evidence:** `project.IsSafeName` rejects all dots (`discovery.go:75-85`); app `validateOperationScope` allows inner dots, rejects only "."/".." (`operations.go:426-442`); web `identifierPattern` allows inner dots (`handlers.go:20`); study has its own `isSafeName` regex allowing dots but rejecting ".." (`init_yaml.go:98,:224-226`). Provenance: extends GENERALIST-01-F07.
- **Architectural reason:** boundary / drift — coarse transport safety nets duplicate knowledge the modules already own.
- **Concrete consequence:** `a.b` passes app+web then dies in module resolution with a module-level message; `.hidden` passes app but is rejected by web before resolution — error quality and verdicts differ by surface for identical input.
- **Counter-evidence searched:** A consumer requiring app's looser superset (none); tests pinning divergence as intentional (none); defense-in-depth rationale for *stricter* web checks is accepted — the charge is app's looser-than-module rule, not web's stricter one.
- **Confidence:** high (divergence), low-medium (severity)
- **Smallest useful action:** Let `validateOperationScope` delegate to the module predicates per scope kind (app already imports `project`/`sprint`); keep web transport-shaped checks.

### CHALLENGE-01-F13
- **Priority:** P3
- **Claim:** `Execute --resume` silently discards malformed or unsupported-schema execute run state when the plan has no checked/deferred tasks, overwriting evidence that every sibling path treats as a stop condition.
- **Evidence:** `execute.go:175-177` — only `loadErr == nil && req.Resume` reconciles; any load error falls through to `NewExecuteRunState`+save (overwrite). Contrast: `Status` errors/falls back to legacy detection, `DeferExecuteTask` fails loudly, flow-state loading refuses unsupported schemas with restore guidance; `validateResolvedResumeTasks` surfaces the problem only when checked/deferred markers exist (`:366-379`). Provenance: SPECIALIST-05B-F04; verified firsthand.
- **Architectural reason:** failure-semantics — the execute twin treats unsupported durable state as disposable in one path.
- **Concrete consequence:** A corrupted or newer-version `.run-state.json` plus an unchecked plan quietly resets attempt/diagnostic/evidence history instead of reporting the schema error.
- **Counter-evidence searched:** Documented fresh-start-wins stance (none); atomic-write making corruption unlikely (true — which weakens the "garbage anyway" justification and strengthens the finding).
- **Confidence:** medium
- **Smallest useful action:** Fail resume on load errors other than missing-state unless an explicit reset flag is passed.

### CHALLENGE-01-F14
- **Priority:** P3
- **Claim:** Recovery-critical documentation contradicts code and half-documents the uncertainty-marker scheme: attempt expiry documented as 24h vs implemented 2h; the dotted/undotted cleanup-marker filename split is undocumented in the direction that matters.
- **Evidence:** `docs/recovery.md:75` ("more than 24 hours") vs `attemptExpired` `> 2*time.Hour` + immediate dead-PID expiry (`verify.go:455-467`); marker constants `sprint/cleanup_uncertain.go:15` (`.cleanup-uncertain.json`) vs `study/cleanup_uncertain.go:15` (`cleanup-uncertain.json`), recovery.md documenting only the dotted form. Provenance: convergent GENERALIST-04-F08 + SPECIALIST-05B-F03; verified firsthand.
- **Architectural reason:** drift — recovery is where doc fidelity matters most; `ReconcileInterruptedRun` depends on the study marker.
- **Concrete consequence:** Operators wait a day for timeouts that fire at two hours; study cleanup uncertainty is undiscoverable via docs.
- **Counter-evidence searched:** A third constant reconciling the delta (none — other 24h values are smoke timeout caps); heartbeat updates reconciling the doc (they don't).
- **Confidence:** high
- **Smallest useful action:** Correct recovery.md to 2h (noting dead-PID immediate expiry) and document both marker names — or unify the filenames in code.

### CHALLENGE-01-F15
- **Priority:** P3
- **Claim:** The agent-facing skill registry defines eleven skills including `ultraplan-code-context`, but the authoritative workspace materializes only ten — the code-context skill is absent where agents following the documented workflow would invoke it.
- **Evidence:** Code: `workspace/skills.go:103-118` defines `ultraplan-code-context`; README/docs present it as current. Workspace `.agents/skills/` contains 10 directories, no `ultraplan-code-context` (listed directly). Provenance: GENERALIST-04-F09; verified firsthand.
- **Architectural reason:** drift / change-surface — skill materialization lagged the stage addition; agents operating in the workspace hit a missing skill for a stage the implementation fully supports.
- **Concrete consequence:** Workspace-driven agents cannot invoke `$ultraplan-code-context`; the gap also hides whether other materialized skills lag (spot-check: the existing ten are current).
- **Counter-evidence searched:** Intentional deprecation (none — code/docs/tests all treat it as live); alternate materialization location in the implementation repo (no `.agents` there).
- **Confidence:** high
- **Smallest useful action:** Re-run skill materialisation against the workspace; note that stage additions require it.

### CHALLENGE-01-F16
- **Priority:** P3
- **Claim:** The crash-safe atomic-write protocol (temp→write→sync→rename→dir-sync) plus its hook type and `syncDir` helper are implemented six times across sprint/study, although the TRD names exactly this as the sanctioned extraction once two modules share it — a threshold that was crossed.
- **Evidence:** Copies: `sprint/state.go:239` (`saveFlowStateWithHooks`), `sprint/execute_state.go:132`, `sprint/review.go:1686-1712`, `sprint/smoke.go:~692-718`, `study/state.go:73`, `study/summary.go:~94-117`; duplicated `atomicWriteHooks` (`sprint/state.go:16` ≡ `study/state.go:23`) and `syncDir` (`sprint/state.go:374` ≡ `study/state.go:177`). Contract: workspace TRD §18.2 ("If sprint and study both need the same mechanical file operation, extract the file operation"); `internal/platform/filesystem` reserved with a doc saying exactly this. Provenance: SPECIALIST-23B-F002; duplication verified firsthand (count and twins).
- **Architectural reason:** change-surface on a durability-critical primitive; contract threshold crossed.
- **Concrete consequence:** Any durability fix must be replicated six times; copies already diverge cosmetically (temp patterns, hooks presence).
- **Counter-evidence searched:** Semantic differences justifying separate implementations (none — identical steps); per-copy tests (exist but pin the copies, not the duplication). Severity tempered from P2: each copy is locally tested and stable.
- **Confidence:** high (facts), low stakes today
- **Smallest useful action:** Move `WriteFileAtomic(path, data, hooks)` + `SyncDir` into `internal/platform/filesystem`; delegate the six sites, keeping marshal/validate local.

### CHALLENGE-01-F17
- **Priority:** P3
- **Claim:** The web projection layer re-parses the governed requirements format ("## Sprint Goal" heading scan) that the sprint module owns and validates, so template evolution can silently degrade the overview while validation still passes.
- **Evidence:** `app/web_usecases.go:559-581` (`sprintOverview` hand-scans headings, swallows errors) vs the section grammar defined by `sprint.ValidateRequirementsContent`/`markdownHeadingPresent` (`index.go:35-50`); app already imports `sprint`. Provenance: SPECIALIST-23B-F005; verified firsthand.
- **Architectural reason:** boundary — artifact-schema knowledge duplicated across a module line without need.
- **Concrete consequence:** Renaming/nesting the goal heading yields an empty overview string while validation passes; no test fails.
- **Counter-evidence searched:** TRD §18.2 local-first tolerance (caps severity at P3 — accepted); goal carried in `SprintSummary` already (no).
- **Confidence:** medium
- **Smallest useful action:** Export `sprint.SprintGoal(content) string` (or populate once where sprint reads requirements) and delete the app-side parser.

## Defended architecture / rejected hypotheses

1. **"`study run-all` lacking the run-loop lock is an ownership defect" (GENERALIST-01-F05) — REJECTED.** Sprint 11's contract explicitly required the opposite: "`run-all` does not introduce … per-study lock files, or the `run-loop` command" (`sprints/11-run-all-batch-execution/requirements.md:51`), lists "stale running task recovery, per-study lock files, force unlock, or multi-process coordination" as Non-Goals (`:62`), and the reasoning records the trade-off — "Ephemeral `run-all` … avoids misleading resumability, locks, stale recovery" (`reasoning.md:119`). Lock-free `run-all` is HISTORY-documented intentional design, not drift. At most a one-line doc note that it is a single-user affordance. This removes a prior-stage finding.
2. **"Dual JSON+SQLite persistence duplicates state knowledge / risks split-brain" — REJECTED as a defect** (and mostly out of my lane — state tribunal owns it): loads deterministically prefer the DB when present; JSON mirrors only at terminal states as compatibility checkpoints (`sprint/state.go:216-237`); one authority per record at runtime; the design is documented in `docs/migration-product-state.md`. The governance-timing question (Gate D) is carried by other stages; my verification confirms the runtime mechanics are coherent.
3. **"Sprint/study twin lock/marker files prove accidental duplication" — REJECTED as a unification target.** Side-by-side reading shows materially different policies (central two-attempt retry locks vs per-study fsync'd locks with force-unlock; different marker layouts), matching TRD §18A's instruction that each product module owns its own mutation exclusion. Only the generic `processAlive` primitive is genuinely duplicated — folded into CHALLENGE-01-F10's action.
4. **"TestRunLoopStartsPriorityTierBeforeLaterTiers is deterministically red" (GENERALIST-04-F03) — CORRECTED, not accepted as written.** My five isolated runs: 4 FAIL / 1 OK — flaky, not deterministic; the specialists that reported green suites were not wrong. Mechanism confirmed as goroutine-emission-order over-specification, not a scheduling regression. Carried as CHALLENGE-01-F07 with corrected claims and P2 instead of P1.
5. **"The web layer leaks workflow semantics systemically" — REJECTED.** `import_boundary_test.go` mechanically restricts web to stdlib+app; the pockets that exist (overview parsing F17, substring classifications) are charged individually, not as a systemic breach.
6. **"TUI mutations violate the read-only charter" — REJECTED.** The read-only charter was Sprint 24 foundations-phase HISTORY; `tui/doc.go` and command help define the current contract as a confirmed-mutation console. (Residual help-text drift is an interface-tribunal item.)
7. **"Package-level `flowFailedStages` recursively falls back to itself" — DISPROVED**: the method's fallback targets the non-recursive package-level function (`service.go:1079-1080`).
8. **"Stage fan-out to ~20 files proves broken ownership" (change-01 rejected hypothesis) — rejection UPHELD.** Non-Go surfaces (help text, templates, config keys, docs) are genuine boundary translation; only the Go-side inventory duplicates are chargeable (F05/F09).
9. **specialist-03a repository contamination** — investigated: target tree clean at `eeaa034`; the debug edit did not persist. No evidence in my findings derives from a modified tree.

## Open questions

1. Was the TUI's code-context exclusion a deliberate scope decision (softening F05 to documented debt) or oversight against ARCHITECTURE.md:23? Nothing in code/docs/test answers; the contract text currently reads as violation.
2. Did any requirement ever ask failing handbook/reasoning dry-runs to persist state (would convert F02 from defect to docs/test gap)? Git history suggests the unguarded lines simply predate the guard convention.
3. Does an out-of-band authorization exist for the productstate SQLite authority move ahead of Gate D? It changes the largest cross-tribunal finding from authority violation to documentation debt — owned by the state tribunal/chair, noted here because it touches artifact authority.
4. Are the prompt frame markers (`<<< ULTRAPLAN … >>>`) intended as reserved vocabulary (validate-and-reject in artifacts) or should the splitter become anchor-based? The answer picks between the two halves of F11's smallest action.
