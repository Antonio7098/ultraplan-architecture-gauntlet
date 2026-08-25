The runs live in the working-directory repo, not the `gauntlet-run` path. Let me inspect them there.
Now let me read the key prior outputs for my domain: specialists 23/24 (simplicity, evolution), change probes, and challenges.
Now the change probes. Reading all eight change-stage outputs.
Now the challenge-stage outputs (all six domains).
challenge-06 is my exact domain — my job is to adversarially re-verify it plus the specialist/change-probe evidence. Let me check sibling chairs for format and boundary, then verify claims in the repo.
chair-01 is empty and chair-05 died mid-run, so my domain must stand on its own verification. Launching parallel bounded evidence-gathering subagents while I re-derive the highest-stakes claims myself.
Stage-vocabulary inventory returned rich detail. Retrying the docs-drift agent and launching the workspace-authority and counter-evidence agents.
All linchpins are now verified firsthand. I have enough to synthesize. Composing the chair-06 report.

# Chair-06 — Evolution tribunal synthesis (simplicity, abstraction, change locality, agent legibility, documentation drift, roadmap fitness)

### Scope inspected

**Prior-stage outputs** (`.archgauntlet/runs/`): specialist-23a/b (simplicity) and specialist-24a/b (evolution) read in full; change-01..08 (change probes) read in full; challenge-01..06 read; chair-02/03/04 scanned for domain boundaries (chair-01 empty; chair-05 aborted after one line — its sanitize-drift verification was re-done by me). Scout/generalist reports consumed indirectly via the tribunals.

**Implementation repo** `/home/antonioborgerees/coding/ultraplan/gauntlet-run/ultraplan-go` @ `eeaa034` (clean, HEAD = "observability: promote nested runtime payload facts"): personally re-read `internal/runcontrol/sanitize.go:10-50`, `internal/platform/runtime/runtime.go:585-604`, `internal/app/run_control.go:428-490`, `internal/web/run_handlers.go:275-300`, `internal/sprint/service.go:735-845` (both dry-run branches), `internal/sprint/flow.go:40-133`, `internal/sprint/handbook_test.go:71`, `internal/sprint/reasoning_test.go:150`, `internal/app/operations.go:176-190`, `internal/app/operation_runner.go:70-80`, `internal/sprint/prompts.go:241-258`, `internal/tui/model.go:450-456`, `internal/runcontrol/interfaces.go:67-72`, atomic-writer census (`grep` over sprint/study), `SaveFlowState` count in service.go (56). Delegated bounded probes (returned evidence I spot-checked): full stage-vocabulary inventory across app/tui/web/config/workspace/templates/JS; docs-vs-code audit of `docs/{local-web,recovery,architecture,cli-reference,stage-skills,migration-product-state}.md`, `docs/plans/`.

**Planning workspace** `/home/antonioborgerees/coding/ultraplan/gauntlet-run/ultraplan-workspace` @ `368a789`: `roadmap.md:1368,1398,1462-1469` (fence, reasoning mandate, Gate D), `sprints/35-durable-run-observability/` (directory listing, `requirements.md:13,20,72-73`, `flow-state.json` stage statuses), `projects/ultraplan-go/docs/TRD.md:55,57`, `sprints/34-shared-context/requirements.md:52-54`, `.agents/skills/` listing, workspace-wide search for any pre-Gate-D authorization record (none found). Timeline established via `git log -S`/`git show` in both repos (see F02).

### Architecture assessment

The evolution story is fundamentally sound and repeatedly proven where ownership was designed in: `sprint.PlanningStages()` derives validity, flow-shape enforcement, and status derivation inside the owning module; import boundaries are machine-enforced (`web→app`, runcontrol leaf); the change-06 strategy probe ("add review/smoke protocol or harness") is absorbed with **zero Go changes**; runcontrol lifecycle vocabulary has one owner with DDL-checked transitions and graceful degradation everywhere else; `platform/runtime` is earned translation, not ceremony; the dual JSON+SQLite persistence is coherent per-record at runtime. Abstractions are generally earned — the interface census (~25 non-test interfaces, nearly all consumer-side with live fakes) supports the doctrine rather than violating it.

Four stress patterns account for essentially every sustained finding:

1. **Enumeration without authority**: ordered vocabularies owned by one module are re-declared by hand elsewhere with no parity test. This failed twice in opposite directions (TUI never received `code-context`; five app-layer lists omit `area-reasoning`), and the repo already invented the cure elsewhere (`import_boundary_test.go`, `run_control_inventory_test.go`, `operations_contract_test.go`) without applying it to stages.
2. **Copied policy instead of owned policy**: the persist-on-failure decision is re-made in 56 `SaveFlowState` sites across seven hand-copied flow functions and has already diverged (two unguarded dry-run branches); the durable-event payload vocabulary is promoted at two producer layers but deleted by a frozen storage allowlist.
3. **Docs treated as pinned truth instead of projections**: `local-web.md`'s 8-state contract is pinned verbatim by a test while the producer emits 11 values; `recovery.md` states 24 h where code enforces 2 h.
4. **Authority pre-empted from the roadmap**: product-state SQLite persistence crossed the Sprint-35 fence and Gate D sequencing with no recorded decision, and the sprint that was supposed to select storage representation has no `reasoning.md`.

### Candidate findings

---

**ID: CHAIR-06-F01**
- **Priority**: P1
- **Claim**: The ordered planning-stage vocabulary is re-enumerated by hand in ~15 places outside `sprint.PlanningStages()` with no parity test tying any of them to the canonical list; the defect class is realized twice in opposite directions — the TUI has **zero** occurrences of `code-context` (capability silently removed), and five app-layer inventories omit `area-reasoning` (findings/fingerprints silently narrowed).
- **Evidence** (verified firsthand + full delegated inventory): TUI `internal/tui/model.go:453` hardcodes 8 stages without `code-context`, driving Validate/Preview/Dry-Run/Flow nav (`:455-466`); artifact nav `:441-451` also lacks it; `grep -r "code-context" internal/tui/` → 0 hits; `internal/tui/model_test.go:81` pins the stale list, cementing it. App omissions of `area-reasoning`: artifact lists `app/sprint_usecases.go:118-130` and `:193-205`, validation sweep `:206` (while its own dispatcher supports `StageAreaReasoning` at `:264-265`), sort map `:240`, governed-input fingerprints `app/operations.go:504-518` (omit the `reasoning/` directory reviewers actually consume, `sprint/review.go:253-274`), preview allowlist `app/usecases.go:215-217`. No production code outside `internal/sprint` calls `PlanningStages()`; no `*parity*` test exists. CURRENT-CONTRACT: workspace `ARCHITECTURE.md:23` (Phase 5 "surfaced uniformly through all three interfaces") and `sprints/34-shared-context/requirements.md:52` (CLI/TUI/browser agree on code-context readiness…findings). Web is currently complete (`templates/sprint.html:40` has all ten options) — proving the lists are maintained by memory, not mechanism.
- **Architectural reason**: change-surface / drift
- **Concrete consequence**: Each future stage insertion repeats a multi-surface lottery whose misses are silent (the missed surface simply lacks the feature; the fingerprint miss weakens `ErrStaleOperation` guarantees without error). The historical probe confirms cost: commit `2d10aba` touched 46 files across 5 packages and still missed the TUI entirely.
- **Counter-evidence searched**: Sprint-24 staged-evolution charter (predates Sprint 33/34 contracts, so fails as defense); deliberate-deferral records in `docs/plans/` (none — 7 plans checked, closest is generic parity language at `integrated-roadmap.md:475`); whether `SprintSummary.Stages` already carries the data server-side (it does — only consumption missing); intentional area-reasoning exclusion rationale (partial: project page dedups area docs, `project_usecases.go:61-62`; multi-file output doesn't fit single-file labels — neither explains the validation-sweep or fingerprint omission).
- **Confidence**: high
- **Smallest useful action**: Export the ordered stage list through `app` once (from `PlanningStages()`); derive TUI nav, the validation sweep, artifact/sort/preview lists, and `governedOperationInputs` from it; add one inventory test asserting every canonical stage appears in TUI nav, web flow targets, and fingerprint inputs. Fix `model.go:453` immediately regardless. Do **not** build a generator for templates/help/config — those are boundary translation (rejected below).

---

**ID: CHAIR-06-F02**
- **Priority**: P1
- **Claim**: The implementation moved sprint flow outcomes and study run state into SQLite (`internal/productstate`, commit `02e2ec4`, 2026-08-21 17:57) ahead of the workspace's reserved authority decision: it crosses the Sprint-35 fence ("must not move Markdown, **flow outcomes**, … into a new canonical store"), contradicts TRD non-goal "SQLite product persistence … before their explicit roadmap evidence gates", and pre-empts Gate D ("Select one authoritative storage mode… Compare filesystem and SQLite modes on real work") — while Sprint 35 itself lacks the `reasoning.md` the roadmap requires to select storage representation, and no authorizing decision record exists anywhere in the workspace.
- **Evidence**: REALITY: `02e2ec4` creates `productstate/store.go` storing kinds `sprint_flow`/`sprint_execute`/`study_run` (`app/storage_commands.go:89-160`); loads prefer the DB row when present (`sprint/state.go:216-227` `FlowStateInDatabase` branch; `docs/migration-product-state.md:24-26` "After import, SQLite is authoritative for that record"). CURRENT-CONTRACT: workspace `roadmap.md:1368` (fence), `:1398` ("Reasoning must select … storage representation"), `:1462-1469` (Gate D); `TRD.md:57`; `sprints/35-.../requirements.md:13,20,72-73`. Timeline: fence text introduced in workspace `2dc3b74` at 2026-08-21 09:15 (+0100); Gate D exists since `1f9d859` (2026-08-15); `02e2ec4` lands 2026-08-21 17:57 — after both. Sprint 35 `flow-state.json` (updatedAt 2026-08-20): sprint-index/technical-handbook/area-reasoning/**reasoning**/plan all `"missing"` while implementation of sprint-scoped work proceeded through `eeaa034` (2026-08-22). Workspace-wide search for authorization (`productstate`, `Gate D`, decision logs, project-index) → none.
- **Architectural reason**: authority / lifecycle (roadmap fitness)
- **Concrete consequence**: Gate D's evidence-gated comparison is pre-decided by shipped code before its experiment runs; the product tables now evolve inside `.ultraplan/run-control.db` whose governance machinery (`PRAGMA user_version`, `app_schema` ledger — `runcontrol/sqlite.go:356` registers only `'run_control'`) does not track them, and `docs/architecture.md:153-160` still describes the file as runcontrol-owned with modules "authoritative for their own product state" elsewhere — so neither the doc nor a migration path describes the actual co-tenancy.
- **Counter-evidence searched**: The fence explicitly permits operational run records in SQLite (`roadmap.md:1368,1398`) — the violation is scoped to product artifacts, not mechanism; `docs/migration-product-state.md` + `scripts/migrate-product-state.sh` document an incremental opt-in mechanism (mitigates blast radius, documents mechanism not authority); searched impl `docs/plans/`, README, workspace studies for pull-forward authorization (none); considered "sprint still in progress, reasoning may land later" — possible (HISTORY-in-progress), which is why the remedy is a record, not a revert.
- **Confidence**: high (facts, timeline); medium (violation vs. unrecorded-decision framing)
- **Smallest useful action**: Backfill the decision into the authoritative workspace — either a dated record pulling Gate D scope forward for productstate, or a sprint-36 requirement owning it — plus the missing Sprint 35 `reasoning.md` storage selection; until then keep DB rows opt-in via `storage migrate` only, and register a `product_state` component in `app_schema`.

---

**ID: CHAIR-06-F03**
- **Priority**: P1
- **Claim**: The durable-event payload vocabulary is coordinated by nothing: two producer layers promote observability keys (`tool,name,title,detail,text,delta,output,message,state,status,action,phase` at `platform/runtime/runtime.go:592-604`; `tool,title,detail,text,delta,content,message,action,state,status,native_type,line` plus forced extraction at `app/run_control.go:435,:479-486`) that the frozen storage allowlist (`runcontrol/sanitize.go:10-17` — unchanged since initial commit) deletes (`title,detail,text,delta,content,output,phase,name,native_type,line` all dropped), while the replay consumer reads exactly the dropped keys (`web/run_handlers.go:293` `firstNonEmptyPayload(...,"text","delta","detail","message","content","title","output")` — only `message` survives). The feature shipped by the last two commits (`c455510`, `eeaa034`) is defeated at the storage gate.
- **Evidence**: All four layers personally re-read at HEAD (quotes above); live SSE renders `detail` directly from `OperationEvent` (`web/operations.go:264`) so live and replay timelines diverge; each dropped value accrues an `Omission{Reason:"unsafe event detail omitted"}` row (`sanitize.go:29-45`). No round-trip test asserts promoted-key survival (`web/run_handlers_test.go` fixtures carry only `kind/type/tool`).
- **Architectural reason**: boundary / change-surface (vocabulary with three uncoordinated owners plus a gate)
- **Concrete consequence**: Replayed run history loses precisely the fields the observability work added, replaced by omission noise; every future observable fact requires synchronized edits in ≥3 packages with zero compile/test protection; the divergence is invisible because each surface is individually self-consistent.
- **Counter-evidence searched**: change-04 defends the allowlist as a deliberate sanitization gate (valid for scoping — but does not explain promoting upstream what the gate deletes); checked for a second unsanitized durable read path (none); checked whether two-tier fidelity is documented intent (nothing found — open question below).
- **Confidence**: high
- **Smallest useful action**: Make `platform/runtime` the single promotion owner (delete the duplicate map in `run_control.go`), extend `allowedEventPayloadFields` to the promoted set, and add one round-trip test: promoted producer key survives `Append`→`Events`→`newRunEventView`. (Overlaps chair-02's execution-lens version; this is the change-mechanism statement.)

---

**ID: CHAIR-06-F04**
- **Priority**: P1
- **Claim**: Two failure branches of the planning-flow template persist `flow-state.json` during **dry runs**, unlike every sibling branch: `service.go:740` (FlowTechnicalHandbook, evidence-validation-failure) and `service.go:818` (FlowReasoning, placeholder-requirements) call `_ = SaveFlowState(...)` unguarded, while adjacent identical checks in the same functions guard (`:746-748`, `:836-838`). Dry runs acquire no mutation lease (`flow.go:121-132`) and the branches sit before the dry-run early return (`:751`), so a failing preview mutates product state lease-free — on the serve surface while the confirmation promises "runtime-free; no runtime-backed writes" (`app/operations.go:178-183`) in explicit contrast to `OperationSprintStatus`'s "MAY REFRESH FLOW-STATE.JSON".
- **Evidence**: Personally re-derived at HEAD (all citations above); tests pin the opposite behavior for siblings (`handbook_test.go:71`, `reasoning_test.go:150` assert no `flow-state.json` after dry runs). Provenance: SPECIALIST-23A-F01 → CHALLENGE-01-F02 → CHALLENGE-06-F01; both branches verified firsthand by me.
- **Architectural reason**: failure-semantics / drift (persist-on-outcome policy re-decided per copied branch)
- **Concrete consequence**: With a real run active (lease held), `sprint flow --to reasoning --dry-run` against placeholder requirements rewrites flow-state from a stale snapshot, reverting the lease holder's recorded stage progress; from the browser the same happens while the UI promises no writes.
- **Counter-evidence searched**: No test/doc records intent that failing previews persist; `WithoutStatusWrites` gates only `Status()`; git history (via 23a) shows the unguarded lines predate the guard convention. **Citation correction to challenge-06**: `docs/cli-reference.md` contains no "--dry-run makes no writes" sentence for sprint flow (that wording exists only for init-workspace/skills/defaults, `cli-reference.md:42,67,86`); the contract basis is the confirmation text, `docs/plans/sprint-code-context-stage.md:234`, sibling consistency, and tests.
- **Confidence**: high
- **Smallest useful action**: Add `if !req.DryRun` guards at `service.go:740` and `:818` plus one regression test per branch; the driver-level consolidation (F05) prevents the next instance.

---

**ID: CHAIR-06-F05**
- **Priority**: P2
- **Claim**: Seven per-stage flow functions are hand-copied instances of one orchestration template with **56** `SaveFlowState` call sites in `service.go` alone; persist-on-failure timing is re-decided per copy, which is the mechanism that produced F04 and will produce the next divergence.
- **Evidence**: Count verified firsthand (`grep -c SaveFlowState internal/sprint/service.go` → 56); structure per SPECIALIST-23A-F02 (hook points confined to prerequisites, prompt renderer, validation spec, success-stages, repair closure); driver `flow.go` owns ordering but delegates persistence timing.
- **Architectural reason**: change-surface / drift
- **Concrete consequence**: Any change to failure/dry-run persistence semantics must be replicated across ~56 statements with no compiler or test signal when one is missed (existing tests exercise only guarded branches).
- **Counter-evidence searched**: Considered whether stage variation forbids extraction (it does not — variation sits at ~5 hook points); rejected the larger "stage-engine registry" as unearned (doctrine: abstractions must be earned; guard-discipline alone is a sufficient smaller fix).
- **Confidence**: high (structure), medium (that consolidation beats guards+tests alone)
- **Smallest useful action**: Minimum: fix F04 and add per-branch regression tests pinning persistence semantics for every stage. If touch budget allows: extract one parameterized helper in `service.go` where dry-run check and failure persistence live exactly once. Nothing more.

---

**ID: CHAIR-06-F06**
- **Priority**: P2
- **Claim**: Documentation contracts are pinned by tests yet describe older vocabularies than the code emits, so the repo mechanically enforces its own doc drift: `docs/local-web.md:146-147` declares 8 stable operation states while `runcontrol/model.go:18-30` defines 11 (reachable extras `queued`, `timed_out`, `persistence_degraded` produced at `app/durable_operations.go:111,244-245`, `app/run_control.go:285,629` and projected raw at `web/operation_handlers.go:415,424`); `web/operations_contract_test.go:82-83` pins the stale 8 verbatim; `web/operations.go:614-621` `terminalOperationState` recognizes only 5 terminals. Second instance: `docs/recovery.md:75` says attempts expire after "more than 24 hours"; `sprint/verify.go:455-467` enforces 2 hours (`:466`) plus immediate dead-PID expiry (`:459`, undocumented).
- **Evidence**: Full both-side citations from the docs audit (verified against source excerpts I read).
- **Architectural reason**: drift (contracts not derived from, nor reconciled with, owner vocabularies)
- **Concrete consequence**: A timed-out durable operation renders in a state the documented contract calls impossible and that `terminalOperationState` misclassifies as non-terminal, so envelope-following clients never treat it as finished; operators following `recovery.md` wait 12× longer than actual behavior. Because the contract *test* freezes the stale list, the usual "update the doc" fix silently breaks CI instead of revealing that the envelope, classifier, and owner must move together.
- **Counter-evidence searched**: Checked whether extras are projected under other names (they pass through raw); whether the contract test intentionally scopes to hub-local states (the doc claims general durable status); whether another threshold supersedes 2 h (none; the separate 24 h cap in `configuration.md:186` concerns configured timeouts).
- **Confidence**: high
- **Smallest useful action**: Decide the envelope once — extend the stable set to the owner vocabulary (doc + contract test iterating `runcontrol`'s terminal set + `terminalOperationState`), or normalize at `durableOperationDocument` as the sibling SSE mapper already does; correct `recovery.md:75` to the implemented rule.

---

**ID: CHAIR-06-F07**
- **Priority**: P2
- **Claim**: Agent legibility lags the code at the authoritative workspace: `ultraplan-code-context` is embedded in the registry (`workspace/skills.go:102-118`), documented (`docs/stage-skills.md:8`), and required by Sprint 34 acceptance ("all-skill materialization includes code-context", `sprints/34-shared-context/requirements.md:53-54`), but the workspace's `.agents/skills/` contains 10 directories and no code-context skill — agents operating from the authoritative workspace cannot invoke the skill the docs tell them to use, and nothing validates deployed workspaces against the registry.
- **Evidence**: Workspace listing verified by delegated probe (10 dirs: requirements, sprint-index, technical-handbook, area-reasoning, reasoning, plan, execute, review, smoke, reconcile-review-smoke). Corroborating count drift: `docs/release-checklist.md:110` says "all nine embedded stage skills" vs `docs/cli-reference.md:53-54` "all eleven skills".
- **Architectural reason**: drift / change-surface (materialisation is manual and ends the stage-addition checklist with no diff check)
- **Concrete consequence**: Workspace-driven agents hit a missing skill for a fully implemented stage; the same gap recurs at the next stage insertion undetected.
- **Counter-evidence searched**: Materialisation is deliberately manual/user-overridable (so absence is not a materialise bug — but no deferral record exists for code-context; Sprint 33 deferred only docs/skill packaging and Sprint 34 then required it).
- **Confidence**: high (state), medium (intent)
- **Smallest useful action**: Re-run skill materialisation against the authoritative workspace; add a registry-vs-directory diff check (e.g., surfaced in `skills materialise --dry-run`) so the gap cannot recur silently.

---

**ID: CHAIR-06-F08**
- **Priority**: P3
- **Claim**: The crash-safe atomic-write protocol (temp-in-dir → write → sync → close → hook → rename → dir-sync) is implemented six times across sprint/study with the `atomicWriteHooks` type and `syncDir` helper duplicated verbatim — past the TRD's own extraction threshold, into the package the roadmap already reserved for it.
- **Evidence**: Writers at `sprint/state.go:239`, `sprint/execute_state.go:132`, `sprint/review.go:1686`, `sprint/smoke.go:692`, `study/state.go:73`, `study/summary.go:94`; `syncDir` twins at `sprint/state.go:374` / `study/state.go:177`; `atomicWriteHooks` twins at `sprint/state.go:16` / `study/state.go:23` (census verified firsthand). CURRENT-CONTRACT: TRD §18.2 sanctions extracting genuinely cross-module mechanical file operations; `internal/platform/filesystem/doc.go` reserves exactly this; dependency direction pre-approved.
- **Architectural reason**: change-surface on a durability-critical primitive
- **Concrete consequence**: Any durability fix (rename semantics, fsync policy, permissions) must be replicated six times; copies already diverge cosmetically, and the next edit to one leaves five stale.
- **Counter-evidence searched**: Semantic differences justifying copies (none — identical step sequence); marshal/validate logic stays local either way, so extraction removes mechanics only.
- **Confidence**: high
- **Smallest useful action**: Land one `WriteFileAtomic(path, data, hooks)` + `SyncDir` in `internal/platform/filesystem` and delegate the six sites.

---

**ID: CHAIR-06-F09**
- **Priority**: P3
- **Claim**: `runcontrol.Control` is a speculative dead alias — declared with a decoration comment, satisfied by nothing, referenced nowhere.
- **Evidence**: `runcontrol/interfaces.go:67-72` (declaration verified firsthand); repo-wide grep finds zero uses in source/tests; composition injects `Repository`/concrete (`app/run_control.go`, `app/run_usecases.go`). Independently found by specialist-23a and 23b.
- **Architectural reason**: speculative generality (authority: implies a seam that does not exist)
- **Concrete consequence**: Readers must evaluate a decoration option that does not exist; a second name for `Repository` can silently diverge.
- **Counter-evidence searched**: Implementers, test doubles, codegen, docs (none).
- **Confidence**: high
- **Smallest useful action**: Delete `Control` (or move its comment onto `Repository`).

---

**ID: CHAIR-06-F10**
- **Priority**: P3
- **Claim**: Runtime construction uses three divergent idioms in one package — injected `dependencies.SprintRuntimeFactory` (`app/app.go:38`, honored at `sprint_commands.go:21-25`), mutable package-global `var studyRuntimeFactory` (`study_commands.go:22-24`, save/restored by three test files), and direct construction ignoring the seam (`health_commands.go:112-113`) — while `docs/architecture.md:22-24` declares "no package-global mutable registry".
- **Evidence**: Both-side citations verified in the docs audit; test mutation at `study_run_commands_test.go:194-198` et al.
- **Architectural reason**: drift (one concern, three ownership stories)
- **Concrete consequence**: Parallel test mutations of the global can race; a second runtime (or even a wiring fix like the smoke-runner case) requires discovering three sites; the architecture doc's absolute claim is false as written.
- **Counter-evidence searched**: Single-runtime scope is documented CURRENT-CONTRACT (`config.go:409-411` rejects alternatives; `configuration.md`), so absence of dispatch today is intentional and not charged; the `Runtime` interface itself is runtime-neutral, so no new abstraction is demanded — only seam consistency.
- **Confidence**: high
- **Smallest useful action**: Add `StudyRuntimeFactory` to `dependencies` mirroring the sprint factory, route health through the shared factory, delete the var; soften the architecture-doc sentence or make it true.

### Defended architecture / rejected hypotheses

1. **"`internal/app` is a pass-through layer"** — Rejected. `PrepareOperation` builds per-kind confirmations with distinct write warnings, fingerprints governed inputs, classifies typed failures (`operations.go:166-200,531-586,619-663`); web issues HMAC refs and bounded projections. Real projection work, not forwarding.
2. **"Consolidate sprint/study twins (locks, cleanup markers, run engines)"** — Rejected. Materially different policies (central retry locks vs per-study force-unlock/fsync; sequential session queue vs parallel workers with retry scheduling); TRD assigns each module its own mutation exclusion; Sprint 11/12 contracts deliberately delivered `run-all` without run-loop orchestration. Only the mechanical file primitive crossed the extraction threshold (F08).
3. **"Dual JSON+SQLite persistence splits authority"** — Rejected at runtime: loads deterministically prefer the DB when present; JSON checkpoints only at terminal states; documented design. The charge that survives is governance/authority *timing* (F02), not split-brain.
4. **"Roadmap forbids SQLite"** — Rejected as stated: `roadmap.md:1368,1398` explicitly permit operational records in "the smallest mechanism proven by reasoning"; the violation scope is product artifacts and sequencing (F02). Precision here matters or the arbiter will over-reach.
5. **"`flowStages` ordinal switch is a standalone defect"** — Rejected standalone: fail-closed validation (`state.go:307-314`) plus position-pinning tests bound it; folded into F01's shared-list remedy only where lists cross package lines.
6. **"TUI code-context omission is legitimate staged evolution (Sprint 24 charter)"** — Rejected: Sprint 33/34 contracts postdate the charter and require tri-surface agreement; no deferral record exists in `docs/plans/` (all seven checked). The missing parity *mechanism* stands regardless of the instance's classification.
7. **"Generate templates/help/config/docs from a stage registry"** — Rejected as unearned indirection: non-Go surfaces are genuine boundary translation across different contracts; Go-side derivation plus one parity test captures the real fix (F01).
8. **Challenge citation corrections**: challenge-06-F01's `cli-reference.md` "makes no writes" quote does not exist for sprint flow (contract basis corrected in F04); challenge-06's scope section contains stray transcript fragments — substance unaffected, all linchpins independently verified.

### Open questions

1. Does any artifact outside the designated workspace (newer planning revision, external decision log) authorize productstate before Gate D? If produced, F02 downgrades to process debt and F04-style co-tenancy documentation remains.
2. Is live-SSE-shows-`detail`-vs-replay-drops-it an intended two-tier vocabulary? This decides whether F03's fix extends the allowlist or documents deliberate tiering.
3. Was persisting state on *failing* handbook/reasoning dry runs ever an explicit requirement? If yes, F04 becomes a docs/test gap; git history suggests the lines simply predate the guard convention.
4. Is area-reasoning's exclusion from dashboard sweeps/fingerprint inputs partly deliberate (project-page dedup, multi-file artifact model)? A recorded rationale would shrink F01's realized-drift list to the TUI instance — but the missing parity mechanism remains the finding either way.
5. Will Sprint 35's `reasoning.md`/`plan.md` be completed retroactively (normal mid-sprint state), confirming F02's mildest framing? The `flow-state.json` timestamps suggest implementation ran well ahead of the governing stages.
