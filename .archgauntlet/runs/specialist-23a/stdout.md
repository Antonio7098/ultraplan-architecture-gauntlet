I'll investigate this large codebase systematically, focusing on my assignment: deep vs god modules, speculative interfaces, duplicated knowledge, pass-through abstractions, and ceremony.
Now let me examine the suspicious duplication between `sprint` and `study` packages:
Now let me examine the sprint service structure — the largest module:
Let me trace the runtime abstraction layers — potential pass-through chain:
Now let me check the authoritative planning workspace for contract evidence on dry-run semantics and related decisions:
Let me verify the repo builds and the relevant tests pass to confirm my reading matches reality:
All investigation complete. Here is my specialist report.

---

# SPECIALIST-23A — Simplicity and earned complexity (independent a)

## Scope inspected

- **Repo**: `/home/antonioborgerees/coding/ultraplan/gauntlet-run/ultraplan-go` @ `eeaa034` (64,635 LoC Go; full `go build ./...` and `go test ./...` green)
- **Packages read in depth**: `internal/sprint` (service.go, flow.go, state.go, state_database.go, code_context.go, verification_lock.go, review.go outline, prompts/prompt_bundle/context_pack outlines), `internal/app` (app.go, usecases.go, operations.go, web_usecases.go, run_usecases.go, durable_operations.go, run_control.go, operation_runner.go, tui_commands.go, serve_commands.go, sprint_usecases.go, study_commands.go), `internal/tui/app.go`, `internal/web` (operations.go, handlers.go outline, security.go excerpt), `internal/runcontrol` (interfaces.go, sqlite.go structure, id.go), `internal/productstate/store.go`, `internal/study` (run_loop.go, run.go, run_all.go, locks.go, service.go, init_clone.go, state_database.go, cleanup_uncertain.go, prompts.go), `internal/platform/runtime/runtime.go` (Adapter), `internal/platform/process`
- **Docs**: `docs/architecture.md`, `docs/cli-reference.md`, `docs/migration-product-state.md`
- **Planning workspace**: `ultraplan-workspace/projects/ultraplan-go` (roadmap.md, sprints/05/06/11/12/13/15 requirements, PRD/TRD excerpts)
- **History**: `git log -L` on the technical-handbook flow branch
- Interface census: every non-test `interface {` declaration in `internal/` traced to consumers

## Architecture assessment

The repository is largely **earned complexity done well**. The layering matches the declared doctrine: product modules (`sprint`, `study`, `project`, `workspace`, `runcontrol`) own state and semantics; `internal/app` adds real projection/aggregation work (operation fingerprints, confirmation construction, HMAC ref issuing, bounded collections, error classification — `app/operations.go:166-280`, `app/web_usecases.go:384-935`), not pass-through forwarding. Interfaces are few (~25 non-test) and almost all consumer-defined with live second implementations or test fakes (`process.Runner`, `study.CloneRunner`, `runcontrol.IDSource/Notifier/Repository`). `controlledRuntime` (`app/run_control.go:95-262`) and `durableOperationManager` (`app/durable_operations.go`) are deep decorators, not ceremony. `productstate` is a 206-line deep store behind two products' migration paths. The one structurally stressed area is the **sprint planning-flow template family** in `service.go`, where a single orchestration sequence has been hand-copied per stage and has already drifted in a safety-relevant semantic.

## Candidate findings

### SPECIALIST-23A-F01
- **Priority**: P1
- **Claim**: Two branches of the planning-flow template persist `flow-state.json` during **dry runs**, unlike every sibling stage. A failing dry run mutates product state without holding the sprint mutation lease, contradicting the CLI contract ("dry-run prints planned operations without writing files"), the web confirmation text ("runtime-free; no runtime-backed writes", `app/operations.go:182`), and this repo's own regression tests.
- **Evidence**:
  - Unguarded writes reachable with `DryRun=true`: `internal/sprint/service.go:740` (`FlowTechnicalHandbook`, "selected evidence validation failed" branch) and `internal/sprint/service.go:818` (`FlowReasoning`, empty/placeholder requirements branch) call `_ = SaveFlowState(...)` before any dry-run early return.
  - Guarded siblings of the *identical* checks: `service.go:554-557` and `:562-565` (sprint-index prerequisite/placeholder), `service.go:647-650` (plan prerequisites), `service.go:728-731` (handbook's own placeholder branch), `service.go:836-839` (reasoning manifest findings), `code_context.go:441-446`.
  - Dry runs skip lease acquisition entirely: `flow.go:44-52` and `flow.go:117-131` (`if !req.DryRun` guards around `acquireMutationContext`).
  - Contract: `docs/cli-reference.md` ("--dry-run ... makes no writes"); tests pin the guarded behavior: `handbook_test.go:83-85` and `reasoning_test.go:150-152` assert `flow-state.json` is **not created** after dry runs.
- **Architectural reason**: failure-semantics (+ boundary): the "does this outcome persist?" policy is embedded per copied branch instead of owned once by the Flow driver, so it drifted.
- **Concrete consequence**: With a real run active (lease held, e.g. `execute` checkpointing), an operator previewing `sprint flow --to reasoning --dry-run` against a sprint whose requirements contain a placeholder triggers `NewFlowState(sp, failedStages)` written by whole-file atomic replace from a stale snapshot with no lease — reverting the lease holder's recorded stage progress (Review/Smoke sections are re-merged by `state.go:201-215`, but stage statuses revert to the dry-run's stale view). On the read-only `serve` surface, `OperationStageDryRun` reaches the same code via `webUseCases → dashboard.sprintService() → FlowStage` while its confirmation claims no writes (`statusWrites=false` gates only `Status()` at `service.go:191`, not `SaveFlowState`).
- **Counter-evidence searched**: Searched tests for any assertion that handbook/reasoning dry-run failures *should* persist (none — the nearest tests assert the opposite on adjacent branches). Checked whether `WithoutStatusWrites` was intended to gate these writes (it does not). Checked git history: the handbook branch predates the guard convention (see F02). Checked whether the write is harmless because dry runs hold the lease (they do not).
- **Confidence**: high
- **Smallest useful action**: Add `if !req.DryRun` guards at `service.go:740` and `service.go:818` to match all sibling branches, plus one regression test per branch asserting no `flow-state.json` after a failing dry run.

### SPECIALIST-23A-F02
- **Priority**: P2
- **Claim**: The seven per-stage flow functions (`FlowRequirements`, `FlowCodeContext`, `FlowSprintIndex`, `FlowTechnicalHandbook`, `FlowReasoning`/area/final, `FlowPlan`) are hand-copied instances of one ~70-line orchestration template containing **40+ verbatim copies** of the failure-persistence statement `_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))`. Cross-cutting policy (persist-on-failure, persist-on-dry-run, when to return the prompt) is re-decided per copy, which is the mechanism that produced F01 and will produce the next drift.
- **Evidence**: `service.go:486-545, 546-630, 631-712, 713-801, 803-1100+`; statement count via grep (40 occurrences in service.go alone); each instance repeats: resolve inputs → prerequisite check → compose shared prompt → dry-run return → nil-runtime check → `runtimeRequest` + validation spec → `startPlanningStageRun` → read artifact → validate → `repairGeneratedArtifact` → success stages/save/clear-session. Stage-specific variation is confined to ~5 hook points (prerequisites, prompt renderer, validation spec, success-stages computation, repair closure).
- **Architectural reason**: change-surface / drift. The driver (`flow.go:44-107`) owns ordering and skip logic but delegates persistence timing to each copy.
- **Concrete consequence**: Any future change to failure/dry-run semantics must be replicated across seven functions and ~40 statements; the F01 defects show replication already fails silently (no compiler or test signal — existing tests exercise only the guarded branches). Adding an eighth stage copies the problem again.
- **Counter-evidence searched**: Considered whether the variations are material enough to forbid extraction (requirements skips shared-prefix composition; reasoning fans out per-area entries; code-context has target resolution) — they differ only at the hook points listed above; the surrounding skeleton is byte-similar. Also considered the doctrine against generic engines: the proposal here is not a new abstraction layer, it is consolidating an *existing* repeated sequence into one parameterized helper inside the same module (or, minimally, making the driver own the dry-run/failure persistence decision). Rejected the larger "stage engine registry" idea as unearned.
- **Confidence**: high (structure), medium (that consolidation is net-positive vs. guard-discipline alone)
- **Smallest useful action**: Either (a) fix F01 and add per-branch regression tests pinning persistence semantics for every stage, accepting the copies; or (b) extract one `runPlanningStage(ctx, sp, req, stageHooks)` helper in `service.go` where the dry-run check and failure persistence live exactly once. Do both (b) only if touch budget allows; (a) is sufficient to stop the bleeding.

### SPECIALIST-23A-F03
- **Priority**: P3
- **Claim**: `runcontrol.Control` is a speculative, completely unused interface.
- **Evidence**: `internal/runcontrol/interfaces.go:67-72`: declared with the comment "Keeping this interface separate permits app composition to decorate durable operations", embeds `Repository` and nothing else. Repo-wide search (source + tests) finds zero references outside the declaration; composition uses `*runcontrol.SQLiteRepository` concretely (`app/run_control.go:23`) and `runcontrol.Repository` (`app/run_usecases.go:37`, `app/durable_operations.go:14`).
- **Architectural reason**: speculative generality (authority: implies a decoration seam that does not exist).
- **Concrete consequence**: Readers must determine whether decoration exists or is planned; the alias invites a second name for the same capability set that can silently diverge from `Repository`.
- **Counter-evidence searched**: Verified not even tests or doc strings consume it; checked `Control` isn't satisfied-by accident somewhere (no implementer assertions).
- **Confidence**: high
- **Smallest useful action**: Delete the `Control` declaration (and its comment) or rename the comment's intent onto `Repository`.

### SPECIALIST-23A-F04
- **Priority**: P3
- **Claim**: The client-side protocol of `DurableOperationManager` — accept → adopt `accepted.Context` → `Existing` short-circuit → per-event `RecordOperationEvent` with "tolerate `ErrWebUnavailable`, drop when `committed==false`" → `FinishOperation` under a 30-second background timeout joined into the run error — is implemented twice, once per transport adapter, and the same capability set is additionally re-forwarded twice inside `app`.
- **Evidence**:
  - TUI: `internal/tui/app.go:235-253` (accept/existing/context adoption), `:285-296` (event relay with committed-drop + ErrWebUnavailable tolerance), `:298-305` (finish, 30s timeout, `errors.Join`).
  - Web hub: `internal/web/operations.go:180-200` (accept/existing/context), `:247-260` (record relay), `:233-245` (finish, 30s timeout, join).
  - Duplicate forwarder sets for the same fields: `dashboardUseCases` (`app/usecases.go:72-119`) and `webUseCases` (`app/web_usecases.go:263-317`), each nil-guarding to `ErrWebUnavailable`; both are live (TUI consumes the former via `tui/app.go:109,235` type assertions; web the latter), but the eight delegations exist in two shapes in one package.
- **Architectural reason**: duplicated knowledge (change-surface): five contract points × two surfaces must evolve in lockstep with `durableOperationManager`.
- **Concrete consequence**: A change to the manager contract (e.g., a third `RecordOperationEvent` outcome, a different unavailable sentinel, changed coalescing visibility) requires synchronized edits in `tui` and `web`; missing one yields surfaces that disagree about which events were durably committed — precisely the kind of divergence the run-control event log cannot reveal, since each surface would be individually self-consistent.
- **Counter-evidence searched**: Confirmed the dedup *keys* legitimately differ per surface (session+token digest in `web/security.go:447-450` vs canonical+fingerprint digest in `tui/app.go:240-241`) — that part is boundary identity, not duplicated knowledge, and was excluded from the claim. Confirmed no existing shared helper covers the relay. Assessed whether extraction merely adds indirection: the shared core is ~15 lines per site with identical semantics, small enough that one `app`-level relay helper removes more than it costs; the surrounding hub/bubbletea machinery stays surface-owned.
- **Confidence**: medium
- **Smallest useful action**: Extract the event-relay and finish-timeout steps into one helper next to `DurableOperationManager` in `app/operations.go` (e.g., `relayOperationEvent(manager, runID, event, deliver)`), leaving accept/existing handling (which differs meaningfully in hub vs TUI flow control) local.

## Defended architecture / rejected hypotheses

1. **"sprint and study duplicate lock/state/marker/validation files"** — Rejected as findings. Side-by-side diffs show materially different implementations: sprint locks live centrally under `.ultraplan/locks/sprint/` with two-attempt retry and no fsync (`verification_lock.go:26-60`); study locks live per-study with force-unlock, command sanitization, and fsync (`study/locks.go:34-120`); `cleanup_uncertain.go` pairs differ in storage layout and lock interaction; `runtime_validation.go` pairs share only the concept of repair specs. Consolidating them would couple two deliberately independent product modules against the stated ownership doctrine ("product modules remain authoritative for their own state"). Only trivial residue: filename asymmetry `.cleanup-uncertain.json` vs `cleanup-uncertain.json`.
2. **"Dual JSON + SQLite persistence duplicates state knowledge"** — Rejected. This is a documented incremental migration design (`docs/migration-product-state.md`): SQLite becomes authoritative per record; writers consult `FlowStateInDatabase`/`ExecuteStateInDatabase` (`state_database.go:129-142`, `state.go:216-227`) and JSON remains a compatibility checkpoint. One authority per record at runtime; not drift-prone duplication.
3. **"`internal/app` is a pass-through layer over product services"** — Rejected. Spot-refutation: `PrepareOperation` builds confirmations, fingerprints governed inputs (SHA-256 over allowlisted trees with symlink rejection, `operations.go:531-586`), and classifies failures into typed operation errors (`operations.go:619-663`); `webUseCases` owns HMAC ref issuing/resolution and bounded projections (`web_usecases.go:847-910`). None of this exists in the product packages.
4. **"Two study execution engines (`RunAll` vs `RunLoop`) is engine duplication"** — Rejected. Sprint 11 contract explicitly delivered `run-all` "without introducing durable run-loop orchestration," and Sprint 12 was instructed to reuse lessons, not code (`sprints/11-run-all-batch-execution/requirements.md`, `sprints/12-durable-run-loop/requirements.md`). Both remain documented, supported commands (`cli-reference.md:196-207`).
5. **"`sprint.Service` is a god object"** — Mostly rejected. Its 16 fields are cohesive dependencies of one artifact set; extension seams (`Runtime`, `pprocess.Runner`, clock, `FSStore`, `WithoutStatusWrites`) are each consumed (`smoke.go:75,128`; `execute.go` session logic). The genuine stress is localized to the flow-template copies (F02), not the module boundary itself; `review.go` (1768 lines) decomposes into named, singly-purposed functions around one workflow.
6. **"`WebUseCases` is an over-wide interface"** — Rejected. It is the single facade that lets `internal/web` import nothing but `app` (verified via `import_boundary_test.go`), it is deliberately split additively (`WebPromptQueries`, `web_usecases.go:47-52`) to protect compatibility embedders, and the architecture doc records this as intentional (`docs/architecture.md:29-55`).

## Open questions

1. Was persisting failure state on handbook/reasoning **dry runs** ever an explicit requirement (e.g., "record that a preview detected broken evidence")? Git history shows the unguarded lines predate the guard convention and no test pins them; if there was intent, F01 becomes a docs/test gap rather than a defect.
2. Is `study run-all` scheduled for retirement now that the durable run-loop exists (FUTURE-INTENT)? If yes, the correct action is deprecation, not consolidation, and today's dual-engine cost is transitional rather than permanent.
