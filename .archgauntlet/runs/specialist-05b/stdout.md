### Scope inspected

- `internal/sprint`: `flow.go`, `service.go`, `state.go`, `state_database.go`, `domain.go`, `locks.go`, `verification_lock.go`, `session_state.go`, `execute.go`, `execute_plan.go`, `execute_state.go`, `code_context.go` (prerequisite/promotion paths), `verify.go`, `review.go` (state/resume sections), `freshness_policy.go`
- Callers/surfaces: `internal/app/sprint_commands.go`, `operations.go`, `usecases.go`, `sprint_usecases.go`, `tui_commands.go`, `serve_commands.go`, `web_usecases.go`; `internal/tui/model.go`
- Authoritative docs: workspace `system/contracts/runtime/workflows.md`, `system/protocols/plan-sprint-protocol.md`; repo `docs/recovery.md`, `docs/user-guide.md`, `docs/cli-reference.md`, `docs/architecture.md`, `docs/plans/sprint-code-context-stage.md`
- Tests: `go test ./internal/sprint/` (pass); `TestCumulativeFlowMaterializesMissingSprintBeforeMutationLock`, `TestFlowToPlanSchedulesCodeContextExactlyOnceInCanonicalOrder`

### Architecture assessment

The sprint state machine is fundamentally sound and unusually disciplined for its size. Ordering is canonical (`PlanningStages()`, domain.go:264) and enforced structurally by `ValidateFlowState` (state.go:307–347); prerequisites are validated from governed inputs before any runtime dispatch; the mutation lease is product-owned with correct re-entrancy for composite workflows (locks.go:16–21, 90–104); resume semantics are layered coherently (stage sessions keyed per stage, review coverage checkpoints fingerprint-gated, execute task records reconciled by deterministic IDs); verification derives expiry without mutating and gives durable transitions only to explicit operations (verify.go:154–156). Artifact-vs-state authority is coherent: artifacts are primary, persisted outcomes are secondary evidence, the context pack is explicitly non-authoritative.

The stress points are (a) a second durable writer — `Status` — operating outside the lease discipline every other writer obeys, and (b) drift inside the failure path of the state machine itself, where two same-named helpers with different state-derivation policies are used interchangeably.

### Candidate findings

---

**ID: SPECIALIST-05B-F01**
**Priority:** P2

**Claim:** `Service.Status` persists derived flow state (including review/smoke verification records) without acquiring the mutation lease, so a status refresh racing a completing review/smoke can regress durable verification evidence to a pre-completion snapshot.

**Evidence:**
- Read-modify-write window: `LoadFlowState` at internal/sprint/service.go:151 → expensive middle section (`PrepareReview`, hashing, content validation, service.go:170–189) → `SaveFlowState` at service.go:191–195. No lease is taken anywhere in `Status`.
- `SaveFlowState` backfills `Review`/`Smoke` only when nil (state.go:204–215); `Status` sets them from the loaded snapshot, so the merge protection does not apply.
- Unprotected writers: CLI `sprint status` builds a default read-write service (app/sprint_commands.go:81–88); the TUI constructs its use cases without `readOnly` (app/tui_commands.go:37–41) and `SprintSummaries` calls `Status` per sprint (app/sprint_usecases.go:98, 109) — while the same TUI session can concurrently run flow/review/execute operations through `sharedOperationRunner`.
- Leased counterpart: review completion writes go through `saveReviewState` under `acquireMutationContext` (review.go:407–413, 619–624).
- Authors knew of the hazard class: `WithoutStatusWrites` exists (service.go:67–73) and both server wirings set `readOnly: true` (serve_commands.go:50, web_usecases.go:250) — but the CLI/TUI paths remain write-enabled.

**Architectural reason:** authority / lifecycle — two synchronization disciplines over one authoritative record; the presentation surface participates in durable writes.

**Concrete consequence:** user polls `sprint status --json` (or presses refresh in the TUI) while a review finishes in another terminal/session. Between the TUI's load and save, the review completes and persists `ReviewCompleted` + `LastComplete` (coverage results) + cleared `ActiveAttempt`. The status save rewrites the old snapshot: status regressed to `running` with a dead `ActiveAttempt`, `LastComplete` and resume coverage erased. `VerificationStatus` then reports not-fresh/incomplete; retained reviewer sessions and focused-review capability are lost; rerun review is forced. Same clobber path exists for `Smoke`.

**Counter-evidence searched:** looked for lease acquisition inside `SaveFlowState`/`Status` (none); looked for UpdatedAt compare-and-swap or last-writer-wins guard (none); checked whether the web path is affected (no — readOnly); checked whether TUI refresh can overlap operations (bubbletea commands run on goroutines; `Refresh` is user-triggered, operations emit async `OperationEventMsg`s — overlap plausible though not proven by a test); considered whether derived-stage convergence masks the issue (it fixes `Stages`, not `Review`/`Smoke` records).

**Confidence:** high (mechanism), medium (realized frequency).

**Smallest useful action:** route the `SaveFlowState` inside `Status` through `acquireMutationContext` (skipping the save on `ErrVerificationConflict` instead of failing the read), mirroring how `ReconcileInterruptedMutation` degrades gracefully (locks.go:26–33).

---

**ID: SPECIALIST-05B-F02**
**Priority:** P2

**Claim:** The failure branch of the state machine has two same-named builders with materially different semantics — package-level `flowFailedStages` fabricates all pre-target stages as `complete`, while `(s Service) flowFailedStages` derives from the actual artifact snapshot — and they are used interchangeably at equivalent lifecycle points, sometimes within one function.

**Evidence:**
- Definitions: internal/sprint/flow.go:304–315 (fabricated prefix-complete) vs internal/sprint/service.go:1077–1092 (snapshot-derived via `DeriveStages`, with a silent fallback to the fabricated variant at 1079–1081).
- Intra-function split in `FlowSprintIndex`: prerequisite failure uses the derived variant (service.go:554), the immediately following empty-requirements failure uses the fabricated one (service.go:562). Same pattern at 647 vs 667 (plan), 739 vs 756 (handbook). `FlowReasoning`'s prerequisite failure uses the fabricated one (service.go:835) unlike its sibling stages.
- Fabrication ignores reality: it marks every earlier stage `StatusComplete` regardless of artifacts or legitimate `StatusSkipped` (e.g., area-reasoning with no selected templates).

**Architectural reason:** drift / failure-semantics — identical conceptual event (“stage N failed”) produces materially different durable histories depending on which helper the author picked and where in the stage lifecycle the error occurred.

**Concrete consequence:** cumulative `flow --to plan` on a no-template sprint fails at plan after runtime dispatch; the persisted state records `area-reasoning: complete` although it was skipped and never ran. `renderSprintFlow` prints `result.Stages` directly (sprint_commands.go:1017–1022) so surfaces display the fabricated history immediately; the durable lie persists until the next `DeriveStages` refresh. This conflicts with the repo's own explicit-outcome doctrine (WF-STATE-001 “keep partial completion … explicit”).

**Counter-evidence searched:** verified whether fabrication is *approximately true* at its call sites — runtime-error paths fire only after prerequisite gates passed, so the prefix usually is complete; that bounds but does not eliminate divergence (skipped≠complete, `LastRunAt` erasure, `Error` detail differences, mid-flow deletion of upstream artifacts). Checked tests for pinning either semantics (none found distinguishing the two).

**Confidence:** high.

**Smallest useful action:** delete the package-level variant and route all ~40 call sites through the snapshot-derived method (keeping its bounded fallback), so one failure-semantics policy exists.

---

**ID: SPECIALIST-05B-F03**
**Priority:** P3

**Claim:** The runbook's attempt-expiry threshold contradicts the implementation.

**Evidence:** docs/recovery.md:75 states an attempt lacking terminal updates “for more than 24 hours” is derived timed out; `attemptExpired` uses `now.Sub(lastSeen) > 2*time.Hour` (verify.go:466). No other expiry constant exists in the package (the 24h values elsewhere are smoke timeout caps, unrelated).

**Architectural reason:** drift — recovery-critical operational parameter documented differently than implemented.

**Concrete consequence:** an operator diagnosing a hung review waits up to 24h for auto-expiry that the code applies at 2h (or conversely misjudges freshness windows when reading the runbook).

**Counter-evidence searched:** grep for 24h/heartbeat across `internal/sprint` and docs; checked whether `HeartbeatAt` updates change effective windows (they don't reconcile the doc delta).

**Confidence:** high.

**Smallest useful action:** correct docs/recovery.md:75 to the implemented 2h window (or make the window configurable and document that).

---

**ID: SPECIALIST-05B-F04**
**Priority:** P3

**Claim:** `Execute --resume` silently discards a malformed or unsupported-schema execute run state when the plan has no checked/deferred tasks, contradicting the loud-error stance used for the same condition everywhere else.

**Evidence:** execute.go:175–177 — `if existing, loadErr := LoadExecuteRunState(...); loadErr == nil && req.Resume { reconcile }`; any load error falls through to `NewExecuteRunState` + `SaveExecuteRunState`, overwriting the unreadable file. Contrast: `Status` fails or falls back to legacy detection (service.go:202–209), `DeferExecuteTask` fails loudly (execute.go:46–49), `requireCompleteExecute` wraps the error (verify.go:102–118), and flow-state loading refuses unsupported schemas with restore guidance (state.go:53–55). `validateResolvedResumeTasks` surfaces the error only when checked/deferred markers exist (execute.go:366–379).

**Architectural reason:** failure-semantics — the twin flow-state machine treats unsupported/malformed durable state as a stop condition; the execute machine treats it as disposable in one path.

**Concrete consequence:** a corrupted (or written-by-a-newer-version) `.run-state.json` plus an unchecked plan means `flow --to execute` quietly resets all attempt/diagnostic/evidence history instead of reporting `ErrExecuteRunStateUnsupported`, undermining the "durable evidence" contract the rest of the module enforces.

**Counter-evidence searched:** looked for a documented fresh-start-wins stance (none); checked atomic-write guarantees making truncation unlikely (yes — which weakens the "garbage anyway" justification); confirmed `Resume:true` is forced on the flow path (flow.go:160), so the overwrite path is routinely reachable.

**Confidence:** medium.

**Smallest useful action:** on `loadErr` other than `ErrExecuteRunStateMissing`, fail the resume (or require an explicit reset flag) rather than overwriting.

---

**ID: SPECIALIST-05B-F05**
**Priority:** P3

**Claim:** Stage ordering is encoded redundantly in several parallel structures, raising the change cost of inserting a stage — a cost demonstrably paid during the code-context insertion.

**Evidence:** canonical list domain.go:264–274; numeric prefix table in `flowStages` (flow.go:182–199, `end=1..8`); long membership chains in `validateFlowTarget` (flow.go:348–353) and `parseSprintFlowArgs` (sprint_commands.go:720); hand-chained success builders `flowRequirementsSuccessStages → … → flowPlanSuccessStages` (flow.go:243–302); per-stage config map (sprint_commands.go:618–653). The code-context insertion additionally required legacy shims `interpretPreCodeContextStages`/`preCodeContextFlowState` and a v1 migration (state.go:83–148).

**Architectural reason:** change-surface.

**Concrete consequence:** adding/removing a stage requires synchronized edits in ≥5 places; a missed edit (e.g., forgetting to bump `end`) compiles fine and only the pinned-order test catches it — and that test pins today's list, not consistency.

**Counter-evidence searched:** stages are few and stable; tests cover canonical order and code-context-once; the success-chain builders correctly thread `noTemplates`. Risk is maintenance cost, not a live defect.

**Confidence:** medium.

**Smallest useful action:** replace the numeric `end` switch with an index lookup into the ordered stage slice (`end = pos(target)+1`), collapsing one parallel encoding.

### Defended architecture / rejected hypotheses

- **Lease deadlock between composite Flow and child Execute/Verify** — rejected: the context marker makes `acquireMutationContext` re-entrant (locks.go:96–98); `Flow` holds one lease for the whole traversal.
- **`flowStageAlreadyValid` skips code-context artifact validation** — rejected: `codeContextPrerequisite` validates the artifact content *and* requires a persisted complete outcome (code_context.go:237–255); it is stricter than the other stages, consistently with `DeriveStages` requiring prior-complete for code-context.
- **Stage-session continuation ignoring prompt checksum is brittle/unsafe** — defended by design: explicit comment explains checksum matching was deliberately dropped, and the continuation prompt instructs re-reading current state (session_state.go:87–100).
- **`VerificationStatus` mutates expiry behind the caller's back** — rejected: it reconciles a loaded copy only; durable transitions belong to explicit operations, matching the runbook (verify.go:154–156, docs/recovery.md:75).
- **Dual DB/file persistence creates ambiguous authority** — coherent: DB is authoritative when present; the file becomes a crash-consistent checkpoint only at terminal stage boundaries (state.go:216–237, state_database.go); loads prefer DB with strict validation on both paths.
- **Skeleton creation outside the lease races** — deliberate, documented, idempotent (`MkdirAll`), because lease resolution accepts existing sprints only (flow.go:62–69; test at sprint_index_test.go:161–173).
- **Disabled freshness switches silently weaken verification** — intentional, bounded debt: digest, format, existence, and allowlist checks remain enforced; rationale documented at the switches (freshness_policy.go) and docs/architecture.md:147.
- **Stale-running recovery duplicated in three places is redundant** — accepted as defense-in-depth across distinct entry points (reconcile on startup, inline before start, resume reconciliation); each site marks the same explicit `stale-running` diagnostic, preserving inspectability.

### Open questions

- Can a TUI dashboard refresh genuinely interleave with an in-session operation (goroutine scheduling in the bubbletea command loop)? A yes sharpens SPECIALIST-05B-F01's in-process path; cross-process CLI-vs-serve exposure exists regardless.
- Was 2h in `attemptExpired` ever 24h (or intended to be)? Git history on verify.go/docs/recovery.md would settle whether F03 is code-drift or doc-drift.
- Does `productstate` (SQLite) mode serialize concurrent `SaveFlowState` writers sufficiently to narrow F01 in database-backed workspaces relative to file-only workspaces?
