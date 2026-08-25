# CHAIR-01 — Product/domain tribunal synthesis

## Scope inspected

**Prior-stage evidence**: `challenge-01/stdout.md` (primary candidate set; note: its file is corrupted mid-stream — F01's claim text and the assessment tail are interleaved with process chatter; findings F02–F17 intact). Provenance chains to generalist-01..04, scout-01/02/03/05, specialist-03b/05b/23a/23b, change-01/change-07 as cited therein. I did not re-read all specialist transcripts; every promoted claim below was re-derived from primary source (mine or a delegated verifier's) against the target tree.

**Method**: five parallel bounded verification agents (persistence semantics; stage-inventory drift; prompt/context ownership; healing/lifecycle/recovery; mechanical duplication + test stability), each briefed to kill rather than confirm, plus my own firsthand re-derivation of the corrupted F01 and spot-checks of every load-bearing contract quote behind verdict changes.

**Implementation repo** (`ultraplan-go` @ eeaa034, verified clean): read directly — `internal/app/operation_runner.go`, `internal/app/sprint_commands.go` (:425–499), `internal/sprint/service.go` (:53–112), `internal/sprint/smoke.go` (:30–79), `inteChair-01 synthesis written to `.archgauntlet/runs/chair-01/stdout.md` (209 lines). Method: five parallel kill-oriented verification subagents plus firsthand re-derivation of the corrupted F01 claim and every load-bearing contract quote.

**Verdicts on challenge-01's 17 candidates — 14 survive, 2 demoted, 1 reframed:**

- **P1 ×2 confirmed**: F01 smoke-start via TUI/web can never succeed (runner builds runtime-less service at `operation_runner.go:75`; `authorSmokeSuite` fails closed; `NonInteractive` field is dead code) and F02 dry-run failure branches persist *fabricated* flow-state without lease (`service.go:740/:818`).
- **Demotions**: F03 status-write P2→P3 (documented intentional policy: `service.go:67-69`, TUI help disclosure, recovery docs prescribe it) and F08 healing-authority P2→P3 (`web-compatibility-baseline.md:79-81` explicitly delegates startup reconciliation to serve).
- **Strengthened**: F05 TUI code-context omission is CURRENT-CONTRACT violation, test-locked at `model_test.go:81`; F06 prompt resolution violates TRD.md:1111 fail-before-runtime verbatim; F07 test failed 5/5 runs (worse than claimed flaky); F16 undercounted — 8 copies incl. one missing fsync entirely.
- **Rejections upheld with primary evidence**: lock-free run-all (sprint-11 contract verified verbatim), TUI read-only charter (superseded per `tui/doc.go`), web import boundary, recursion hypothesis disproven.

Open questions for downstream stages: TUI code-context intent, productstate Gate D authorization, marker reserved-vocabulary choice, and whether `NonInteractive` was meant to gate authoring in-module.
emotions and one reframing this stage (vs challenge-01): F03 P2→P3, F08 P2→P3, F12 reframed to its only defensible residue. One finding strengthened beyond its challenge form: F07's test is not merely flaky — it failed 5/5 runs here.

## Candidate findings

### CHAIR-01-F01
- Priority: **P1**
- Claim: `OperationSmokeStart` — the smoke path advertised by both TUI ("Run Smoke [EXTERNAL]") and web — can never succeed. The runner builds a runtime-less service, and non-dry-run deep smoke always requires the runtime to author coverage; users get "Configure the smoke model/runtime" though config is fine and the identical CLI command works.
- Evidence: Runner case uses `sprint.NewService(root.Path).WithSmokeSettings(...)` with no runtime (`internal/app/operation_runner.go:75`) unlike siblings Execute/Review/Verify at :49/:60/:93 via `sprintRuntimeService` (which attaches `.WithRuntime(controlled, req)` — `sprint_commands.go:498`). `NewService` leaves runtime nil (`service.go:63-64`). `runSmoke` always authors when `!req.DryRun` (`smoke.go:60-65`); `authorSmokeSuite` fails closed on nil runtime: "runtime is required to author deep-smoke coverage / Configure the smoke model/runtime and rerun smoke." (`smoke_author.go:21-22`). TUI entry points `tui/model.go:485-486`; web maps stage `smoke`→`smoke-start` (`web/operation_handlers.go:654`, `static/app.js:174,:191`). The operation confirmation even advertises `Runtime = true` (`app/operations.go:213`). Aggravator: the runner's flow case sets `Smoke.NonInteractive: true` (`operation_runner.go:40`) but that field is consumed nowhere in `internal/sprint` (defined `smoke_types.go:63`; read only by CLI wiring) — an unwired interactive/non-interactive intent.
- Architectural reason: lifecycle/wiring drift inside the app layer — one operation case skipped the shared service-construction path every sibling uses; capability advertised by two surfaces cannot execute.
- Concrete consequence: A TUI/web user with correct config confirms an expensive external-harness operation, watches preflight pass (the confirmation preview runs fine via `DryRun:true`, `operations.go:210-211`), then fails at authoring with remediation pointing at config. Every attempt, forever.
- Counter-evidence searched: default runtime in `NewService` (none); alternate injection for this case (none); docs disabling interactive smoke on these surfaces (none); tests driving `OperationSmokeStart` through the runner (none — fakes bypass); a NonInteractive consumer that would skip authoring (none in sprint).
- Confidence: high
- Smallest useful action: Build the smoke case via `sprintRuntimeService(deps, root, ...)` like its siblings; add one integration test asserting `OperationSmokeStart` reaches authoring with non-nil runtime.

### CHAIR-01-F02
- Priority: **P1**
- Claim: Two failure branches of planning-flow persist flow-state during dry runs — writing state fabricated from scratch, not merely stale — while every sibling branch guards; dry runs hold no lease, so a failing preview can destroy a concurrent lease holder's durable progress.
- Evidence: `service.go:740` (FlowTechnicalHandbook, selected-evidence validation failed) and `:818` (FlowReasoning placeholder requirements) call save with no `DryRun` check; both branches precede their function's dry-run return (:751-753; flowAreaReasoning :1142 / flowFinalReasoning :1279), so they are reachable with `DryRun=true`. The write is `NewFlowState(sp, package-flowFailedStages(...))` — empty stage states plus fabricated pre-target Complete — discarding durable Stages wholesale; only Review/Smoke survive the merge (`state.go:204-215`); DB-authoritative mode also mutates without lease (`state.go:216-226` → `state_database.go:69-97`). Dry-run lease skip: `flow.go:51-60` (acquire at :71 else-branch), FlowStage guard at :121/:129. Guarded siblings verified at :555/:563/:571, :648/:657, :729, :746, :836, :1258/:1266/:1274, :1129/:1137, plus newest-stage helper `failCodeContext` (`code_context.go:442-446`). Tests pin the opposite behavior (`handbook_test.go:64-73`; `app/sprint_commands_test.go:274-294` byte-identical state across dry runs). History: unguarded lines predate 90e251d's systematic `if !req.DryRun` hardening — drift, not design.
- Architectural reason: failure-semantics — outcome-persistence embedded per copied branch rather than owned once; drifted exactly where the convention was retrofitted incompletely.
- Concrete consequence: `sprint flow --to reasoning --dry-run` against placeholder requirements rewrites flow-state from fabrication (stage statuses revert; Skipped forensics erased; review/smoke resumable records regressed), while serve's `OperationStageDryRun` confirmation claims no-write behavior. Self-heals only on next derived-view refresh.
- Counter-evidence searched: tests asserting failing dry-runs should persist (none — opposite pinned twice); lease held during dry runs (no); `statusWrites` covering flow paths (it gates only `Service.Status`, service.go:191); doc/test rationale for the two branches (none).
- Confidence: high
- Smallest useful action: `if !req.DryRun` guards at `service.go:740`/:818 + regression test per branch; consider making the flow driver own failure-persistence so branches cannot drift again.

### CHAIR-01-F03
- Priority: **P3** (demoted from challenge P2)
- Claim: `Service.Status` persists derived flow state without the mutation lease, so a status refresh racing a completing review/smoke can regress durable verification/resume evidence. Facts stand, but write-on-status is a documented intentional policy; the residual defect is only the missing coordination around that documented convenience write.
- Evidence: load `service.go:151`, PrepareReview (read-only) :170-189, save :191-195 gated `s.statusWrites && !legacyCodeContextState`. Demotion evidence: `service.go:67-69` documents WithoutStatusWrites exists precisely so read-only surfaces opt out "…existing CLI/TUI status behavior remains unchanged"; TUI help discloses "Refresh and sprint status may recompute deterministic sprint flow-state.json status" (`tui_commands.go:82-83`); `SprintSummary.RefreshMayWrite` exposes it to UI (`sprint_usecases.go:116,:161-162`); user docs prescribe refresh as recovery (`cli-reference.md:255`, `recovery.md:51`). Race window real but low-probability: PrepareReview hashing between load and save; incoming non-nil Review/Smoke bypass merge protection (`state.go:204-215`); DB upsert last-writer-wins (`state_database.go:69-97`); productstate store has no CAS. Web is readOnly (`serve_commands.go:50`); TUI dashboard is the exposed writer.
- Architectural reason: lifecycle — a documented presentation convenience write lacking single-writer coordination, not an authority violation.
- Concrete consequence: rare interactive-refresh × completing-review interleave regresses resume checkpoints/flags; recoverable by rerun.
- Counter-evidence searched: any CAS/lease guard (none); docs calling it defect (none); evidence the write is incidental rather than policy (found — see above, hence demotion).
- Confidence: high (mechanism), low (realized frequency)
- Smallest useful action: skip the save when the mutation lease is contended (mirror `ReconcileInterruptedMutation` degradation).

### CHAIR-01-F04
- Priority: **P2**
- Claim: Two same-named builders with different semantics are used interchangeably for "stage N failed": package-level `flowFailedStages` fabricates all pre-target stages `complete` (`flow.go:304-315`); `(Service).flowFailedStages` derives from real artifacts via `DeriveStages` with fallback to the fabricator (`service.go:1077-1092`, fallback :1079-1081 — the recursion hypothesis is disproven). Measured split: 50 call sites, 42 package vs 8 method, mixed within single functions (FlowSprintIndex :554 method vs :562 package; FlowTechnicalHandbook :739 package vs :745 method; FlowReasoning :817 package vs siblings :1128/:1136/:1273 method).
- Evidence: Fabrication marks legitimately-skipped stages Complete; DeriveStages produces `StatusSkipped` for them (`service.go:1366-1374`), and `verify_test.go:114` pins that Skipped must survive round-trips — the distinction is load-bearing. Raw persisted Stages gate behavior: `codeContextPrerequisite` requires `StageCodeContext == StatusComplete` from loaded state (`code_context.go:245-254`), so fabricated Complete can wrongly unblock (bounded by artifact-validity precheck). CLI renders the fabricated history directly (`sprint_commands.go:985`). No test pins the fabrication; target-stage Failed assertions exist (`sprint_index_test.go:92`, `code_context_test.go:229`).
- Architectural reason: drift/failure-semantics — identical event yields materially different durable histories depending on which overload the author picked; ~50 sites to audit for any change.
- Concrete consequence: cumulative flow failing at plan persists `area-reasoning: complete` though skipped; durable record contradicts explicit-outcome doctrine until next DeriveStages rewrite; worst case transiently unblocks code-context on a sprint whose artifact already validates.
- Counter-evidence searched: tests pinning fabrication (none); recursion (disproven); intent comments (none); self-healing bounds noted.
- Confidence: high
- Smallest useful action: route all call sites through the snapshot-derived method, keeping the package fn as its snapshot-error fallback.

### CHAIR-01-F05
- Priority: **P2**
- Claim: TUI omits the code-context stage entirely — zero occurrences in `internal/tui` — against CURRENT-CONTRACT text; the omission is even test-locked, and the catalog is re-enumerated in ≥5 places.
- Evidence: `grep -rn "code-context|CodeContext" internal/tui` → zero. Hardcoded 8-stage list `tui/model.go:453` drives Validate/Preview/Flow nav (:456-466); artifact nav :442-451 drops code-context although the app layer already supplies it in `SprintSummary.Artifacts` (`sprint_usecases.go:195`). Contract verified verbatim: ARCHITECTURE.md:23 "Phase 5 proves that boundary by adding `code-context` … surfaced uniformly through all three interfaces"; Phase 5 shipped (commit 2d10aba); roadmap Sprints 33–34 commit it further; PRD.md:603 and TRD.md:2639 name TUI parity explicitly. Drift locked in by `tui/model_test.go:81` asserting the same 8-item list. Parallel dispatchers: `operations.go:676-699`, `sprint_commands.go:138-161/:177-198`, `sprint_usecases.go:254-279`, `web_usecases.go:511-539`. No free-text stage input exists anywhere in TUI; no deliberate-exclusion comment/doc.
- Architectural reason: change-surface/drift — per-surface copies of a module-owned catalog; the copy that lagged silently removed a shipped capability.
- Concrete consequence: TUI users cannot validate/preview/flow-to code-context; invisible to single-surface tests; stage N+1 repeats the class.
- Counter-evidence searched: alternate TUI entry point (none); exclusion rationale (none); FUTURE-INTENT reading of ARCHITECTURE.md:23 (rejected — phase shipped, roadmap committed).
- Confidence: high
- Smallest useful action: derive the TUI stage/artifact lists from `sprint.PlanningStages()`; update model_test accordingly.

### CHAIR-01-F06
- Priority: **P2**
- Claim: Prompt-default resolution is implemented four times with divergent failure semantics, and the sprint variant embeds literal `"# Prompt Load Error"` / `"# Missing Prompt Default"` pages into runtime prompts where TRD §10.2 mandates failing before execution.
- Evidence: `project/reasoning_defaults.go:52-86` fail-closed; `study/prompts.go:175-191` propagates errors; `sprint/prompts.go:241-257` fail-open — unreadable override → error page (:250), registry miss → `"# Missing Prompt Default"` (:256), and a fifth divergence: `ResolveInside` failure silently falls back to builtin (:243-244); `review.go:1739-1765` fail-closed with content validation. Contract verified verbatim (TRD.md:1111): lookup "**must fail before runtime execution** … when neither source is available or when an existing override is unreadable or invalid." Error pages reach live execution: dry-runs exit early (`service.go:492-493,:576-578`), non-dry-run paths send composed prompts to runtime (`service.go:501-503,:585-587`, `smoke_author.go:52-74`); only guard is byte budget (`prompt_context.go:157-162`). `'builtin:'+rel` duplicated three sites (`reasoning_defaults.go:82-83`, `study/prompts.go:188`, `sprint/prompts.go:254`). Ownership: ARCHITECTURE.md:471-479 assigns this exact mechanical policy to workspace. Consolidation cannot break session continuation (exact-match gating deliberately rejected, `session_state_test.go:58`).
- Architectural reason: ownership/drift/failure-semantics — the mechanism lives in four consumers instead of its documented owner; failure policy differs per stage.
- Concrete consequence: a chmod mishap or registry omission makes plan/review/smoke execute against instructions that are an error page — tokens spent, off-contract artifacts, failure attributed late; adding a tier touches ≥4 files with nothing forcing coherence.
- Counter-evidence searched: downstream validators catching error-text prompts (none); documented fail-open rationale (none — zero hits outside prompts.go); semantic differences justifying separate implementations (semantic validation layers differ legitimately; only the fallback step duplicates).
- Confidence: high (structure/contract), medium (impact — edge-triggered)
- Smallest useful action: `workspace.ResolveOverrideFile(root, rel)` implementing the documented rule once; all four sites delegate keeping local semantic layers; unreadable overrides error on planning paths per TRD §10.2.

### CHAIR-01-F07
- Priority: **P2**
- Claim: The study priority-tier guarantee is guarded only by a positionally-over-specified test that fails effectively deterministically at HEAD in this environment (measured 5 FAIL / 0 PASS across `-count=1` runs — worse than challenge-01's 4/5), and nothing enforces `go test ./...`.
- Evidence: Test asserts `startedTasks[0]/[1]` positional equality (`run_loop_test.go:299-304`); Started is emitted inside per-task goroutines after an mu-guarded update whose throttled persist can sleep 250ms (`run_loop.go:278,:152-161,:357-364`) — launch order imposes no emission order; no synchronization covers ordering. Scheduler ranking itself correct (`runnableTaskIDs`, `run_loop.go:518-549`). History precise: 5839f9a replaced a race-tolerant set-based assertion over `rt.order[:3]` with the positional one.
- Architectural reason: change-surface/failure-semantics — the release gate is red where iteration is fastest; signal-to-noise collapse for the study package.
- Concrete consequence: genuine study regressions hide behind the known-red test; the priority-tier property is effectively untested.
- Counter-evidence searched: scheduler regression (disproved); hidden ordering sync (none); CI enforcement elsewhere (Makefile targets only, nothing wired).
- Confidence: high
- Smallest useful action: restore the set-based assertion over the scheduled wave via the existing `orderedRuntime` seam.

### CHAIR-01-F08
- Priority: **P3** (demoted from challenge P2)
- Claim: Interrupted-product-state healers have exactly one production caller each — the serve startup path — while run-control rows reconcile at composition open and CLI status heals nothing. Facts verified, but this is documented design, not de facto drift: residual is a bounded staleness window in CLI-only workflows.
- Evidence: Caller uniqueness confirmed (production: `web_usecases.go:364,:377` ← `web/server.go:76-81`; rest are tests). Run-control startup reconcile `app/run_control.go:64`. Demotion evidence: `docs/web-compatibility-baseline.md:79-81` (verified verbatim): "**Startup reconciliation delegates to `app.OperationReconciler`**"; explicit invariant `sprint/verify.go:154-156`: "Status derives expired-attempt truth without mutating durable state. The next explicit review/smoke operation owns any persisted transition" — and derived views do show corrected expiry immediately. Self-correction paths exist: sprint resume converts stale running→failed (`execute.go:195-199,:600-618`); study resume resets via `ResumeValidateRunState` (`run_loop.go:62-63`).
- Architectural reason: lifecycle — ownership of crash-state conversion is assigned by contract to the server composition root; cost is discoverability of that assignment, not ambiguity.
- Concrete consequence: after a CLI-only crash, durable task rows stay "running" until next mutation or any future serve start; run-control ledger disagrees visibly in the interim.
- Counter-evidence searched: other production callers (none); doc assigning healing to CLI (none); doc assigning it to serve (found — hence demotion).
- Confidence: high (facts), medium (consequence)
- Smallest useful action: invoke both healers where `run_control.go:64` already reconciles; keep the serve loop as-is.

### CHAIR-01-F09
- Priority: **P2**
- Claim: App-layer inventories silently omit area-reasoning — dashboard artifact lists, the validation sweep, the sort map — and governed-operation fingerprints omit `reasoning/` outputs entirely, so area-output edits never invalidate prepared operations even though final reasoning must reference them.
- Evidence: Artifact lists include code-context but no area-reasoning label (`sprint_usecases.go:118-130,:193-205`); sweep iterates nine stages excluding `StageAreaReasoning` (:206); sort map :240 lacks it. Fingerprints cover `reasoning.md` but not the `reasoning/` directory (`operations.go:504-519` vs `artifacts.go:22-23`), feeding the staleness gate `ErrStaleOperation` (`operations.go:271-290`) — indirect fingerprinting impossible: final-reasoning validation checks only that referenced names appear (`reasoning.go:134` region), so byte drift passes. Dispatcher supports the stage everywhere (`sprint_usecases.go:264-265`, `operations.go:686-687`, `sprint_commands.go:147-148,:186-187`). Mitigation noted: project page labels area docs separately (`project_usecases.go:61-62`) and `summary.Stages` carries the row — partial surfacing, not full invisibility.
- Architectural reason: drift/failure-semantics — derived inventories beside the owning module's canonical list; survived multiple stage additions.
- Concrete consequence: invalid/stale area artifacts produce no visible findings and never invalidate governed operations; next stage addition repeats the class.
- Counter-evidence searched: tests asserting the lists (none); deliberate-exclusion docs (none); full-invisibility claim (weakened to partial — reflected above).
- Confidence: high
- Smallest useful action: derive inventories from `PlanningStages()`+capability metadata, or add a completeness test over summaries and fingerprint inputs.

### CHAIR-01-F10
- Priority: **P2**
- Claim: Study cancellation signals a bare lockfile PID with SIGINT gated only by `kill(pid,0)` liveness — the product's only PID-signalling path — against TRD's own liveness doctrine, while a correct birth-token implementation exists one package away.
- Evidence: `study/locks.go:141-158` CancelRunLoop: liveness-only gate (:142-148), study-name match (:149), self-PID (:152), then `syscall.Kill(info.PID, SIGINT)` (:155); LockInfo carries Command/AcquiredAt but they are unused at cancel (`locks.go:47-53`); no upstream identity gate (`app/operation_runner.go:133-143` calls directly). `processAlive` = `kill(pid,0)` incl. EPERM-as-alive (`locks.go:17-23`); verbatim twin `sprint/verification_lock.go:95-101`. Stale-steal equally PID-recycle-sensitive (`locks.go:66-70` → indefinite `ErrStudyLocked`). Correct pattern in-tree: `runcontrol/process_linux.go:16-45` `/proc` birth token + boot_id + host digest, stored `sqlite.go:554`, compared `lifecycle.go:488-491`. Doctrine verified verbatim (TRD.md:2208): "A PID alone is insufficient because of PID reuse." Docs reject the practice (`recovery.md:191,:197-198`).
- Architectural reason: failure-semantics/boundary — process-identity authority exists in the platform layer and the only signalling caller bypasses it.
- Concrete consequence: recycled PID receives SIGINT from `study cancel`; conversely recycled PID blocks new loops until `--force-unlock`. Stale locks persisting indefinitely widen the recycle window arbitrarily. Mitigations: graceful SIGINT; name guard binds lock→study.
- Counter-evidence searched: additional identity binding (none); doc accepting risk (none — opposite); alternate cancellation channel coexistence (noted, does not remove this path).
- Confidence: high (mechanism), medium (probability)
- Smallest useful action: record owner birth-token in LockInfo and verify before signalling; hoist the shared probe primitive into the platform layer, lock policy stays per-module.

### CHAIR-01-F11
- Priority: **P3**
- Claim: Composed sprint prompts split on the first occurrence of the boundary marker while requirements/code-context bytes are embedded verbatim with no marker rejection — an artifact quoting UltraPlan's own marker misplaces session-continuation instructions and skews explain/cache diagnostics. Mechanics certain; trigger contrived.
- Evidence (line corrections vs challenge): verbatim embedding `prompt_context.go:184-189`; marker constant `prompts.go:33` (51-byte string incl. newlines); first-index splits `prompt_bundle.go:97`, `session_state.go:135`; consumers `insertStageContinuation` (used on real interrupted-session resumes), `explainComposedPrompt`/cache key (`runtime_metrics.go:134`, `execute.go:123`, `review.go:396`, `service.go:1021`); `ValidateCodeContextContent` lives in `code_context.go:34` (not index.go) and rejects no markers; interior extra markers pass `context_pack.go:64,:71` suffix check. Canonical flows never show agents the marker (requirements-flow prompts carry no shared prefix; code-context embeds requirements, not markers) — exposure needs preview-then-edit out of charter. Anchor-based fix is implementable: `sharedSourceEvidenceClose` defined `prompts.go:32`; sequential parse already exists `prompt_bundle.go:117-120`.
- Architectural reason: boundary — frame grammar delimited by content-searchable markers while one frame side is untrusted agent-authored bytes.
- Concrete consequence: dogfooding sprints documenting UltraPlan's markers get continuation instructions injected mid-frame and skewed cache-prefix metadata; no validator bypass or crash.
- Counter-evidence searched: sanitization (none); LastIndex anywhere (none); marker-collision tests (none); realistic author exposure (low — reflected in priority).
- Confidence: high (mechanics), low-medium (impact)
- Smallest useful action: anchor the boundary after `sharedSourceEvidenceClose`, or reserve the marker strings in artifact validation; one test each way.

### CHAIR-01-F12
- Priority: **P3** (reframed)
- Claim: Of the four name-safety predicates, three are legitimate layered checks; the defensible residue is narrower: the app-layer scope validator accepts names the owning modules reject, so identical input yields different verdicts/error quality per surface.
- Evidence: Reframing basis: web's `identifierPattern` (`handlers.go:20`) is TRD-sanctioned transport validation (§18.1A:2095); study's `isSafeName` (`init_yaml.go:98,:224-226`) is creation-time-only (resolution matches discovered studies, `resolve.go:33`); app's `validateOperationScope` (`operations.go:426-442`) is a coarse traversal pre-guard before delegation — but modules always re-validate (`sprint/discovery.go:36`→`discovery.go:51`). Observable divergence verified: `.hidden` passes app, rejected 400 by web; `a.b` passes app+web then dies in module resolution with a different message. No test pins any of it; delegation would be free (`operations.go:17-19` imports project/sprint).
- Architectural reason: boundary consistency — edge may layer stricter checks (accepted), but looser-than-owner checks produce misleading acceptances.
- Concrete consequence: users get past one gate to fail another with different wording; no security hole (modules re-validate).
- Counter-evidence searched: consumer requiring the looser superset (none); tests pinning divergence (none); transport-shape justification for app's rule (partial — traversal guard is legitimate; dot-permissiveness is the gap).
- Confidence: high (divergence), low (severity)
- Smallest useful action: have `validateOperationScope` delegate to module predicates per scope kind; keep web transport-shaped checks.

### CHAIR-01-F13
- Priority: **P3**
- Claim: `Execute --resume` silently discards malformed or unsupported-schema run state when the plan has no checked/deferred tasks, overwriting evidence every sibling path treats as a stop condition — including the forward-compat case (newer binary wrote v(N+1), older binary resumes).
- Evidence: `execute.go:174-180`: fresh state constructed unconditionally; reconcile only when `loadErr == nil && req.Resume`; any load error falls through to unconditional save. `validateResolvedResumeTasks` early-returns nil with zero markers (`:373-374`), so its load-error finding (:376-378) never fires there. Schema rejection routes into the same hole: version≠current → `ErrExecuteRunStateUnsupported` (`execute_state.go:188-190`). Contrasts verified: Status errors/legacy-fallback (`service.go:202-208`), DeferExecuteTask loud (`execute.go:46-49`), flow-state refuses with restore guidance (`state.go:54`). No test exercises the overwrite.
- Architectural reason: failure-semantics — one path treats unsupported durable state as disposable.
- Concrete consequence: corrupt or forward-version `.run-state.json` plus markerless plan quietly resets attempt/diagnostic/evidence history instead of reporting the schema error.
- Counter-evidence searched: documented fresh-start-wins stance (none); atomic writes reduce torn files (true — weakens mitigation, strengthens finding for the schema case); SQLite-authoritative mode bypasses file (bounds blast radius).
- Confidence: medium
- Smallest useful action: fail resume on load errors other than missing-state unless an explicit reset flag is passed.

### CHAIR-01-F14
- Priority: **P3**
- Claim: recovery.md contradicts implemented timing and half-documents the uncertainty-marker scheme: 24h documented vs 2h implemented (+ immediate dead-PID expiry); only the dotted sprint marker documented, study's undotted variant omitted.
- Evidence: `verify.go:455-467`: dead-PID immediate expiry (:459-461), else `> 2*time.Hour` on max(StartedAt, HeartbeatAt) (:466) — heartbeats feed the 2h window (`review.go:1226,:837`, `smoke.go:192`); no mode implements 24h (other 24h constants are retention/config bounds: `config.go:427,:449`, `retention.go:26`, `runcontrol/model.go:423-424`). Markers: dotted `.cleanup-uncertain.json` at sprint root (`sprint/cleanup_uncertain.go:15,:57`) vs undotted under `<study>/.ultraplan/` (`study/cleanup_uncertain.go:15,:152-154`, `domain.go:5`); each reconcile depends on its own (`sprint/locks.go:40-44`, `study/cleanup_uncertain.go:101-105`); recovery.md:112 names only the dotted form; :75's prose otherwise mirrors `verify.go:154-156` correctly — only the constant is wrong. recovery.md linked from README (:7,:136).
- Architectural reason: drift — recovery is where doc fidelity matters most.
- Concrete consequence: operators wait a day for timeouts firing at two hours; study cleanup uncertainty undiscoverable via docs.
- Counter-evidence searched: heartbeat reconciling 24h (does not); third constant (none).
- Confidence: high
- Smallest useful action: correct recovery.md to 2h (noting dead-PID expiry) and document both marker names, or unify filenames in code.

### CHAIR-01-F15
- Priority: **P3**
- Claim: The skill registry defines eleven skills including `ultraplan-code-context`, but the authoritative workspace materializes only ten — generation lagged the stage addition; workspace agents cannot invoke `$ultraplan-code-context`.
- Evidence: `workspace/skills.go:101-118` defines it (CanonicalFlow-flagged); workspace `.agents/skills/` has exactly 10 dirs. Timeline established from git: skills first materialized 6159b0c (2026-07-30, pre-code-context); skill added upstream 90e251d (2026-08-21); workspace dir last touched 2dc3b74 (2026-08-21, guidance edits only) — never re-materialized. Remediation safe: `PlanSkills` emits create ops for absent dirs, skip for differing files (`skills.go:275-295`), so `ultraplan skills materialise` adds it without clobbering hand-tuned skills. No sync test binds workspace to binary skill set; no contract mandates parity (keeps it P3); no intentional-exclusion doc.
- Architectural reason: change-surface — generated artifact lagging its generator after a stage addition.
- Concrete consequence: workspace-driven agents hit a missing skill for a fully supported stage; CLI `flow --to code-context` still works.
- Counter-evidence searched: deprecation signals (none — code/docs/tests treat it as live); alternate location (implementation repo has no .agents).
- Confidence: high
- Smallest useful action: re-run skill materialisation; add a parity check to the release checklist.

### CHAIR-01-F16
- Priority: **P3** (leaning low-P2)
- Claim: The crash-safe atomic-write protocol is copied six times across sprint/study — plus two degraded variants proving drift is already realized — under a TRD clause sanctioning exactly this extraction once shared; one degraded copy omits fsync entirely.
- Evidence: Full protocol ×6: `sprint/state.go:239-289`, `sprint/execute_state.go:132-182`, `sprint/review.go:1683-1717` (third hooks type `reviewWriteHooks` :1681 — undercounted by challenge), `sprint/smoke.go:692-724`, `study/state.go:73-119`, `study/summary.go:94-123`; `syncDir` twins (`sprint/state.go:374-381` ≡ `study/state.go:177-184`). Degraded variants: `runcontrol/migration.go:317-351` (no dir-sync) and `sprint/session_state.go:62-85` (**no fsync at all**) — the named risk is realized. Differences across copies are parameterization only. Contract verified verbatim (TRD.md:1705,:1718): §18.2 sanctions "filesystem helpers that are genuinely cross-module, such as atomic writes" and "If sprint and study both need the same mechanical file operation, extract the file operation." §18A contains no counter-principle (governs web-surface ownership; locks stay in modules :2142). Correction: `platform/filesystem/doc.go` reserves the boundary generically; §18.2 is the actual sanction. Per-copy tests pin semantics, not location — extraction-compatible.
- Architectural reason: change-surface on a durability-critical primitive; contract threshold crossed (six uses ≫ two).
- Concrete consequence: durability fixes need N-way replication; session_state already missed fsync silently.
- Counter-evidence searched: semantic differences justifying copies (none); §18A conflict (none); per-copy instability (each locally tested and stable — keeps severity at P3).
- Confidence: high
- Smallest useful action: move `WriteFileAtomic(path,data,hooks)`+`SyncDir` into `internal/platform/filesystem`; delegate the eight sites; keep marshal/validate local.

### CHAIR-01-F17
- Priority: **P3**
- Claim: The app/web projection re-parses the governed requirements grammar ("## Sprint Goal" heading scan) the sprint module owns and validates, swallowing read errors into blank UI.
- Evidence: `web_usecases.go:559-581`: heading scan, equal-fold match, backtick/star strip; `os.ReadFile` failure returns "" (:562-564) rendered as an empty overview section (called from :478). Grammar owned by `ValidateRequirementsContent` (`index.go:35-50`, seven mandated sections). No reuse alternative was skipped — `SprintSummary` carries no goal field — but none is exported either. Mild mitigation: this is composition-root app code doing direct workspace reads (cf. os.Lstat :187), within local-first tolerance; what is violated is the share-use-cases/no-duplication rule at grammar level plus silent failure semantics. Parser has zero direct tests (templates tested via injected fakes, `templates_test.go:380-384`).
- Architectural reason: boundary — artifact-schema knowledge duplicated across the module line with error swallowing.
- Concrete consequence: renaming/nesting the goal heading yields a silently empty overview while validation passes; no test fails.
- Counter-evidence searched: exported helper/goal carrier (none); TRD local-first tolerance (caps at P3 — accepted); overview test via parser (none).
- Confidence: medium
- Smallest useful action: export `sprint.SprintGoal(content)` or populate goal where sprint reads requirements; delete the app-side scan; surface read errors as typed unavailable state.

## Defended architecture / rejected hypotheses

1. **Lock-free `run-all` (GENERALIST-01-F05) — rejection UPHELD, verified verbatim.** Sprint-11 requirements checkbox: "`run-all` does not introduce durable retry … stale-running recovery, per-study lock files, or the `run-loop` command" and Non-Goals: "Implementing stale running task recovery, per-study lock files, force unlock, or multi-process coordination" (`sprints/11-.../requirements.md:51,:62`). This is CURRENT-CONTRACT, not drift. Removes a prior finding outright.
2. **Dual JSON+SQLite persistence — rejected as a defect (state tribunal owns the governance-timing question).** Loads deterministically prefer DB; JSON mirrors at terminal states as compatibility checkpoints (`sprint/state.go:216-237`); documented in `docs/migration-product-state.md`. My verification confirms coherent runtime mechanics.
3. **Sprint/study twin lock/marker files as accidental duplication — REJECTED as unification target.** Materially different policies match TRD §18A (locks stay in owning modules, :2142; web must not own artifact persistence, :2095). Only the generic `processAlive` primitive genuinely duplicates — folded into CHAIR-01-F10.
4. **Flaky-vs-deterministic dispute (GENERALIST-04-F03) — RESOLVED by measurement:** 5 FAIL / 0 PASS at HEAD here; the positional assertion is unsound regardless of environment. Carried as CHAIR-01-F07.
5. **Systemic web-layer workflow leak — REJECTED, upheld.** `import_boundary_test.go` mechanically restricts internal/web to stdlib+internal/app (verified :30); pockets charged individually (F17, substring classifications).
6. **TUI mutations violating a read-only charter — REJECTED, upheld.** Verified `tui/doc.go`: "The dashboard supports every sprint status, validation, prompt, flow, execute, review, and smoke operation … runtime-backed or mutating actions are confirmed." Read-only was Sprint-24 foundations HISTORY. Note this same doc sharpens CHAIR-01-F01: promised smoke support that cannot work.
7. **Recursive `flowFailedStages` fallback — DISPROVEN:** the method falls back to the package-level function (`service.go:1079-1081`), terminating.
8. **Stage fan-out to ~20 files as broken ownership — rejection UPHELD:** non-Go surfaces (help text, templates, config keys, docs) are genuine boundary translation; only Go-side inventory duplicates are chargeable (F05/F09).
9. **specialist-03a contamination — investigated:** target tree clean at eeaa034 (`git status` empty); no finding derives from a modified tree.
10. **F08 "healing belongs at composition root" as implicit defect — WEAKENED INTO DEBT:** `docs/web-compatibility-baseline.md:79-81` explicitly delegates startup reconciliation to the server; `verify.go:154-156` documents status-not-heal. Residual carried as P3 staleness window.

## Open questions

1. Was the TUI's code-context exclusion a deliberate scope decision? Nothing in code/docs/tests answers; three normative texts (ARCHITECTURE.md:23, PRD.md:603, TRD.md:2639) currently read as violated. An owner's word converts F05 to documented debt.
2. Does out-of-band authorization exist for the productstate SQLite authority move ahead of Gate D? Owned by the state tribunal/chair; noted because it interacts with F02/F03 blast radius (DB-authoritative mode mutates without lease).
3. Marker reserved vocabulary vs anchor-based splitting: picks between the two halves of F11's smallest action.
4. Is the `NonInteractive` field intended to gate smoke authoring inside the sprint module (dead today, `smoke_types.go:63`)? If yes, F01's fix might alternatively wire it; if no, delete the field when fixing F01.
