# SYNTH-02 — Whole-system synthesis: ownership, cohesion, simplicity, boundaries, drift

Target: `/home/antonioborgerees/coding/ultraplan/gauntlet-run/ultraplan-go` @ eeaa034 (clean) against workspace @ 368a789 (clean). Read-only review; no target files modified.

Provenance legend: **[FH]** = verified firsthand by this synthesis against the target tree at eeaa034; **[chair-N, obs M]** = re-derived and confirmed by chair tribunal N with M independent observers behind it. No claim below rests on an unverified challenge-stage assertion alone.

### Scope inspected

**Directly (firsthand):** `internal/app/operation_runner.go` (:15-149), `internal/app/run_control.go` (:150-299), `internal/app/durable_operations.go` (:185-254), `internal/app/run_control.go` promotion block (:400-489), `internal/sprint/service.go` (:726-840), `internal/sprint/smoke_author.go` (:1-40), `internal/sprint/prompts.go` (:241-257 region), `internal/sprint/verification_lock.go` (:92-101), `internal/study/locks.go` (:14-43), `internal/study/run_loop_test.go` (red test executed 3×), `internal/runcontrol/sanitize.go` (:1-50), `internal/runcontrol/model.go` (:15-59), `internal/runcontrol/interfaces.go` (:60-72), `internal/runcontrol/process_linux.go` (full), `internal/runcontrol/migration.go` RestoreBackup (:296-352), `internal/runcontrol/sqlite.go` app_schema sites (:230, :356), `internal/productstate/store.go` (:1-40 + DSN constants), `internal/tui/model.go` (:443-467), `internal/web/operations_contract_test.go` (:75-119), `internal/web/run_handlers.go`, `run_handlers_test.go` grep, `static/app.js:1392`, `templates/runs.html`, both `import_boundary_test.go` files, repo-A `docs/architecture.md` (:150-162); workspace `docs/ARCHITECTURE.md` (:21-26), `docs/TRD.md` §18.2 (:1703-1720).

**Commands:** `go test ./internal/study/ -run TestRunLoopStartsPriorityTierBeforeLaterTiers -count=3` (FAIL 3/3), skills directory listing of the workspace, greps for `PlanningStages()` callers, `productstate.DatabaseRelativePath` references, `runcontrol.Control` references.

**Delegated (bounded distillation, six parallel workers):** all six chair outputs (`chair-01..06/stdout.md`) cross-read against their challenge inputs (`challenge-01..06/stdout.md`), each distilled to per-finding verdicts, rejections, corrections, and themes; one scout produced the package/import map and doc inventory. ~640KB of prior output consumed via delegation; every load-bearing claim promoted below was either re-verified firsthand or carries named chair provenance.

### Architecture assessment

**Sound and worSYNTH-02 complete. Written to `.archgauntlet/runs/synth-02/stdout.md` (27.5KB); both target repos verified clean.

**Method:** Six parallel subagents distilled all chair/challenge outputs (~640KB) into per-finding verdicts; one scout built the package/import/doc map. Every promoted claim was then re-derived firsthand against eeaa034 — including executing the suspected red test (3/3 deterministic FAIL in `internal/study`), tracing the smoke-runner wiring, the payload promotion→allowlist→consumer chain, both supervision loops, and the co-tenancy/restore path.

**Result: 11 findings** (4×P1, 6×P2, 1×P3), concentrated at three seams rather than in modules:
- **F01 (P1)** TUI/web smoke-start can never succeed — runner skips runtime wiring for one of six kinds (`operation_runner.go:74-75`)
- **F02 (P1)** Stage catalog has an owner nobody calls — ~15 hand-lists diverging two ways
- **F03 (P1)** Observability feature defeated at the storage gate — three unsynchronized key lists, drift active in HEAD commit itself
- **F04 (P1)** Two owner loops, divergent terminal truth — cancellation racing completion records `persistence_degraded`
- Plus bare-PID locks vs unused birth-token authority, productstate's unaware co-tenancy + WAL-sidecar restore hole, test-pinned doc drift, unleased 1Hz TUI writes reconciled against the documented opt-out policy, red main with no CI, and prompt-resolution quadruplication

10 rejected hypotheses documented (no layering refactor needed — the DAG and boundary tests are sound and worth keeping as-is). 5 open questions flagged for the arbiter, including two dead-specialist coverage holes.
on and no round-trip/parity test (F02, F03, F07).
2. **Per-surface re-implementation of workflow concerns.** The shared runner exists precisely so surfaces can't diverge; each confirmed interface defect is a seam where one surface (or one operation kind) re-decided transport/wiring/identity locally (F01, F08).
3. **Two authorities for identity and terminal truth.** Bare-PID product locks vs runcontrol birth-token fencing; two supervision loops with divergent terminal taxonomies; presentation writes racing mutation leases (F04, F05, F08).

**Ambiguous in the docs themselves:** ARCHITECTURE.md deliberately leaves Phase-6 package placement and SQLite adoption to "sprint reasoning" without requiring the decision be recorded; productstate then crossed into run-control.db with no record (F06). The docs' open-endedness, not the code, created the governance gap.

**Drift direction:** drift here is not decay — it is *unowned vocabulary*. Every P1/P2 finding above is a list, digest, taxonomy, or wiring decision made twice with no mechanism forcing agreement.

### Candidate findings

---

**SYNTH-02-F01**

- Priority: **P1**
- Claim: The shared operation runner — the documented single runtime-backed implementation for TUI and web — wires runtime into five of six operation kinds but not smoke, so deep-smoke started from TUI or web deterministically fails after being accepted.
- Evidence: [FH] `internal/app/operation_runner.go:74-75` builds `sprint.NewService(root.Path)` directly while sibling kinds route through `sprintRuntimeService(deps, ...)` (:23, :36, :49, :60, :93); fail-closed authoring at `smoke_author.go:21-22` requires non-nil runtime; web maps dry-runs to a separate kind (`operation_handlers.go:657-665`), so only real starts are affected. Same seam also fabricates the nonexistent lock-owner command `"ultraplan", "operation"` for study ops (`operation_runner.go:115` vs truthful argv `study_commands.go:209`) [chair-04-F08].
- Architectural reason: ownership / boundary — the parity seam itself leaks per-kind wiring decisions.
- Concrete consequence: TUI/web users get a guaranteed-failing operation that has already persisted running→failed state; the failure mode is invisible until used because only kind-mapping is tested (`operations_contract_test.go:46`).
- Counter-evidence searched: dry-run escape hatch exists but only transiently during prepare; CLI path is correct (`sprint_commands.go:436`), isolating the defect to the shared runner; corroborated independently by chair-01-F01, chair-04-F01, FAILURE-11-F01 [4 observers total].
- Confidence: high
- Smallest useful action: add `.WithRuntime(...)` to the OperationSmokeStart case (one line), plus a runner-level test asserting every runtime-requiring kind receives the runtime.

---

**SYNTH-02-F02**

- Priority: **P1**
- Claim: The planning-stage catalog has a declared single owner (`sprint.PlanningStages()`, `domain.go:264`) with zero production callers outside `internal/sprint` [FH grep], so ~15 hand-maintained stage lists across tui/app/templates have already diverged in two opposite directions: TUI omits `code-context`; five app-layer lists omit `area-reasoning`.
- Evidence: [FH] `tui/model.go:453` hardcodes 8 stages (no code-context), pinned stale by `model_test.go:81`; app omissions at `sprint_usecases.go:118-130,:193-205,:206,:240` and fingerprints `operations.go:504-518` [chair-06-F01, chair-01-F09]; contract text ARCHITECTURE.md:23 explicitly demands uniform tri-surface surfacing of code-context (quoted and verified) [FH]; area-reasoning omission means area edits never invalidate operation fingerprints [chair-01-F09].
- Architectural reason: drift / change-surface / ownership — vocabulary owned in name, re-owned in practice by every consumer.
- Concrete consequence: adding stage N+1 repeats the historical change probe (commit 2d10aba touched 46 files across 5 packages and still missed the TUI); parity regressions are invisible because the stale lists are themselves test-pinned.
- Counter-evidence searched: staged-evolution charter (Sprint 24) predates Sprint 33/34 contracts and cannot excuse it [chair-06]; no deferral record in `docs/plans/` (7 checked) [chair-06]; possible deliberate area-reasoning exclusion partially explicable by dedup labeling (`project_usecases.go:61-62`) but not the fingerprint omission. Fix adds no new dependency edge: tui→app→sprint already exist [FH].
- Confidence: high
- Smallest useful action: expose the stage inventory through `app` (it already imports sprint) and add one parity test comparing every surface enumeration to `sprint.PlanningStages()`.

---

**SYNTH-02-F03**

- Priority: **P1**
- Claim: The durable observability feature shipped by c455510/eeaa034 is defeated at the storage gate: two producer layers promote payload keys that the frozen 27-key allowlist deletes, and the replay consumer reads exactly those deleted keys.
- Evidence: [FH end-to-end] promotion maps `app/run_control.go:435` (`title/detail/text/delta/content/native_type/line...`) and force-add loop :479-486 whose own comment says "the run timeline JS expects (payload.tool, payload.action, payload.title, etc.)"; allowlist `runcontrol/sanitize.go:10-17` contains none of those display keys; consumer `web/run_handlers.go:288` reads `"text","delta","detail","message","content","title","output"` — only `message` survives. HEAD commit eeaa034 itself added the second promotion layer plus namespaced `<field>_<sub>` keys (:438) that a fixed allowlist structurally cannot pin.
- Architectural reason: boundary / ownership / drift — three unsynchronized policy lists across three packages; nothing connects them and no round-trip test exists (`run_handlers_test.go` fixtures carry only kind/type/tool) [chair-05-F01].
- Concrete consequence: durable replay shows sparse events while live SSE renders full detail (`operations.go:264`); commit messages claim observability the journal does not retain; every future producer key addition silently degrades.
- Counter-evidence searched: strict-gate-as-security-policy rejected as full defense (exactly the promoted keys absent is not scoping, it is disconnect) [chair-02]; no alternative unsanitized read path (Append sole writer) [FH]; live CLI reads pre-sanitize memory, compensating only for live viewing [chair-05].
- Confidence: high
- Smallest useful action: make `platform/runtime` (or runcontrol) the single owner of the observable-key vocabulary, extend the allowlist once, and pin producer→storage→consumer survival with one round-trip test.

---

**SYNTH-02-F04**

- Priority: **P1**
- Claim: Two hand-copied owner supervision loops hold identical responsibility (lease, heartbeat, cancellation ack, terminal proposal) but classify the same fault differently: a store error during cancellation/completion races durably records `persistence_degraded` in the runtime loop yet plain `cancelled`/`failed` in the operations loop, which has no persistence-loss concept at all.
- Evidence: [FH] `app/run_control.go:156-163` (setPersistenceErr cancels ctx), :262-276 (post-run flush on possibly-dead `runCtx`), :282-291 (persistErr branch proposes TerminalPersistenceLost, never consulting `terminalOutcome`), ack branch :232-235 missing the `runCtx.Err()` guard its siblings have (:225-227, :241-243, :251-253); contrast `durable_operations.go:190-216` (same errors → bare `owned.cancel()`), :241-252 (taxonomy switch without persistence case, cause dropped). ≥6 terminal-classification sites total across hub/reconcilers/shutdown [chair-05-F03].
- Architectural reason: ownership / lifecycle / failure-semantics — "why did this run end" has two owners with different vocabularies and evidentiary standards; `TerminalProposal` carries no structured cause field (`model.go`).
- Concrete consequence: a user cancelling a run at the wrong moment gets a permanently wrong durable verdict (persistence_degraded implies data loss when none occurred); cross-loop incidents are non-comparable; shutdown attribution lands as generic `cancelled` [chair-02-F07/chair-04-F05].
- Counter-evidence searched: wholesale engine merge REJECTED by two chairs (genuinely different event envelopes) — remedy is classification parity, not consolidation [chair-02/04]; `-race` silence explained by synchronous fakes [chair-02]; twin plain-cancel shape recast as "half a correct precedent".
- Confidence: high
- Smallest useful action: add the missing `runCtx.Err()` guards, classify context.Canceled before persistence loss, and give `TerminalProposal` a structured cause so all loops share one taxonomy function.

---

**SYNTH-02-F05**

- Priority: **P2**
- Claim: Product locks (study run-loop, sprint verification) use bare-PID liveness with kill-based signalling while runcontrol owns a correct birth-token process-identity implementation one package away — two process-identity authorities that disagree, violating TRD:2208's explicit "PID alone is insufficient".
- Evidence: [FH] near-duplicate `processAlive` closures at `study/locks.go:17-23` and `verification_lock.go:95-101` (kill(pid,0), EPERM=alive); correct pattern at `runcontrol/process_linux.go:16-45` (PID + boot_id + /proc starttime token), used only by reconciliation; study cancel signals a bare PID via SIGINT (`locks.go:141-158`) — the product's only user-facing signalling path.
- Architectural reason: authority / boundary / drift — the safe mechanism exists in-tree; the modules that need it most don't use it.
- Concrete consequence: PID reuse makes a recycled unrelated process look alive, blocking new run loops (or misdirecting SIGINT at it); recovery guidance points at stale owners.
- Counter-evidence searched: TRD §18A legitimately keeps locks module-owned — the violation is identity mechanism, not file placement [chair-01]; EPERM-as-alive fails safe cross-user; frequency lowered by pid_max [chair-02]. Unification target narrowed to the identity primitive only [chair-01 rejected broader consolidation].
- Confidence: high
- Smallest useful action: move the birth-token probe to `platform/process` and have both lock implementations record/check it alongside PID.

---

**SYNTH-02-F06**

- Priority: **P2**
- Claim: One physical SQLite file has two schema owners, one unaware: productstate co-locates in `.ultraplan/run-control.db` via its own duplicated path constant, registers no `app_schema` component row, has zero tests, and the restore path renames the DB without WAL-sidecar handling.
- Evidence: [FH] `productstate/store.go:19` vs `runcontrol/sqlite.go:22` declare the same relative path as independent constants (no reference, no equality-pinning test — grep empty); `sqlite.go:356` registers only `'run_control'`; RestoreBackup renames over the live DB with no `-wal`/`-shm` removal (`migration.go:296-352`) and chair-03 proved empirically (modernc driver, exact DSNs) that a stale WAL resurrects pre-restore commits precisely in the post-crash scenario restores exist for; sole restore caller is its own test [chair-03-F01/F02].
- Architectural reason: ownership / lifecycle — "one substrate, two owners, one unaware"; recovery is the least-governed lifecycle stage.
- Concrete consequence: restore can silently return pre-backup product truth; schema versioning, quota attribution, and backup semantics skip half the file's contents; a constant edit in one package splits the store in two with no failing test.
- Counter-evidence searched: co-tenancy mechanics are sound (matching pragmas serialize writers; shared-sqlite-kit extraction rejected — ~5 lines DSN overlap, revisit at third copy [chair-03]); roadmap fence permits operational records, so scope limited to product artifacts + missing authorization record [chair-06-F02]; remedy is a record + registration, not revert.
- Confidence: high (mechanism), medium (governance leg)
- Smallest useful action: register a `product_state` app_schema row, delete one of the two path constants, teach RestoreBackup to remove sidecars, and add a minimal productstate test file.

---

**SYNTH-02-F07**

- Priority: **P2**
- Claim: Documentation drift is mechanically enforced by the repo's own contract tests: the web contract test pins an 8-state lifecycle vocabulary while the transport layer handles all 11 runcontrol states, so correcting the doc breaks CI.
- Evidence: [FH] `operations_contract_test.go:83` pins `{accepted,running,cancelling,succeeded,failed,cancelled,interrupted,cleanup_uncertain}` and :110-112 asserts timed_out/persistence_degraded classify as non-terminal, while `runcontrol/model.go:18-30` defines 11 states and web handlers/static JS/filters handle the full set (`run_handlers.go:315,:319`, `app.js:1392`, `runs.html` options); raw lifecycles pass through `operation_handlers.go:415,:424`. Second instance: `recovery.md:75` documents 24h staleness vs implemented 2h + immediate dead-PID expiry (`verify.go:455-467`) [chair-01-F14].
- Architectural reason: drift — the test suite freezes yesterday's contract, making documentation correction a compile error.
- Concrete consequence: agents and integrators trust the documented envelope, then meet undocumented terminals; fixing the doc requires touching the very test that was supposed to protect the contract.
- Counter-evidence searched: blast radius bounded (HTML redirects to /runs; active lists use `IsActive()` correctly) [chair-04-F06, P3 there]; no renaming occurs — values pass through verbatim, so consumers see valid states; severity calibrated P2 here because the *enforcement mechanism* is the finding, spanning multiple docs.
- Confidence: high
- Smallest useful action: regenerate contract fixtures from `runcontrol.Lifecycle` values (single source), then let docs/tests both derive from it.

---

**SYNTH-02-F08**

- Priority: **P2**
- Claim: Presentation paths persist durable flow-state outside the mutation lease: `Status()` saves derived state when status-writes are enabled and the TUI polls at 1Hz, making the dashboard a continuous unleased writer racing lease-holding operations.
- Evidence: [chair-04-F04 ×2 observers, chair-01-F03] `service.go:151`, save `:191-195`, no `acquireMutation` in any Status/TUI path (lock census over execute/flow/locks/review/smoke/verify); web opts out via `WithoutStatusWrites()` iff readOnly (`usecases.go:121-129`); TUI tick `tui/app.go:271-275`.
- Architectural reason: authority / lifecycle — single-writer discipline holds for every mutating path except the loudest one (the UI).
- Concrete consequence: a polled status refresh can overwrite stage truth mid-operation, destroying concurrent lease-holder progress; blast radius bounded by the merge guard preserving nil-only Review/Smoke (`state.go:199-215`) but not other fields.
- Counter-evidence searched: chair-01 demoted the Status-write itself to P3 on documented intentional policy (`service.go:67-73` opt-out comment frames CLI/TUI writes as legacy-behavior preservation; TUI help discloses it; recovery.md prescribes refresh). This synthesis reconciles the two verdicts: the documented policy legitimizes occasional convenience refresh, not a 1Hz continuous write loop from an interactive surface — the residual hazard stands at P2, mechanism uncontested by any domain.
- Confidence: medium-high
- Smallest useful action: apply the existing `WithoutStatusWrites()` option to the TUI service construction (mirroring web), keeping CLI one-shot refresh as documented behavior.

---

**SYNTH-02-F09**

- Priority: **P2**
- Claim: Main is red at HEAD and nothing notices: `TestRunLoopStartsPriorityTierBeforeLaterTiers` fails deterministically (executed 3/3 FAIL [FH], `study/run_loop_test.go:276-304` — note it lives in `internal/study`, correcting digests that placed it in app) because commit 734eb5d replaced tier-barrier semantics with documented tier-backfill semantics (`run_loop.go:529-531`), updated the sibling test, and left this twin stale; there is no CI to catch the red baseline.
- Evidence: [FH execution] failure message "want the priority dimension" with backfill-order starts; root cause established by chair-05-F02 from commit evidence (scheduler-regression hypothesis rejected); strict ordering assertions across concurrent goroutines add brittleness [chair-05].
- Architectural reason: change-surface / drift — the suite is the only regression net, and its baseline state is unmonitored.
- Concrete consequence: every green-looking subset run hides real regressions behind a known-red test; cached CI signals built on this tree are worthless until the twin is fixed.
- Counter-evidence searched: flaky-vs-deterministic dispute resolved by measurement (5/0 runs historically, 3/3 here) [chair-01]; property itself still correctly implemented (`runnableTaskIDs` :518-549) [chair-01-F07] — the defect is the stale expectation, not the scheduler.
- Confidence: high
- Smallest useful action: update the stale twin to assert backfill semantics (mirroring its renamed sibling), then wire the suite into any CI hook available.

---

**SYNTH-02-F10**

- Priority: **P2**
- Claim: Prompt-default resolution is implemented four times with divergent failure semantics despite docs assigning ownership to the workspace; the sprint variant embeds literal error pages into runtime prompts, contrary to TRD.md:1111's fail-before-runtime requirement.
- Evidence: [chair-01-F06, strong; FH spot-check of `prompts.go:241-257` `builtin:` resolution] four resolvers with differing fallback/validation layers; sprint variant fail-open; owner assigned at ARCHITECTURE.md:471-479.
- Architectural reason: authority / drift — a documented owner exists and four consumers overrode it.
- Concrete consequence: identical inputs yield different prompts (or embedded error text) depending on which surface resolves defaults; validation guarantees differ invisibly per surface.
- Counter-evidence searched: per-surface semantic validation layers differ legitimately — only the fallback step duplicates [chair-01]; no downstream guard catches embedded error pages [chair-01].
- Confidence: medium-high
- Smallest useful action: extract the fallback-resolution step into `project` (already imported by sprint) and make the sprint variant fail closed per TRD.md:1111.

---

**SYNTH-02-F11**

- Priority: **P3**
- Claim: Small simplicity residue with cheap mechanical fixes: dead speculative alias `runcontrol.Control` (zero references [FH]), three divergent runtime-construction idioms in `app` (injected factory / package-global `var studyRuntimeFactory` test-mutated / direct construction) against the repo's own no-global-mutable-registry rule, and the atomic-write protocol copied six times (+ one variant missing fsync at `session_state.go:62-85`) although TRD §18.2 pre-authorizes extraction into the reserved-but-empty `internal/platform/filesystem` (doc.go stub [FH]).
- Evidence: [FH] `interfaces.go:67-72`; `app.go:38` vs `study_commands.go:22-24` vs `health_commands.go:112-113` [chair-06-F10]; six writer sites enumerated by chair-06-F08 with zero semantic differences found.
- Architectural reason: simplicity / change-surface — accepted debt with a pre-paid extraction path.
- Concrete consequence: a cloned third surface compiles with durability silently nil (type-assertion capability discovery); the degraded fsync-less variant can lose a summary on power loss; otherwise low.
- Counter-evidence searched: larger stage-engine registry and template generation rejected as unearned indirection [chair-06]; per-copy tests currently pin semantics, keeping this P3 [chair-01-F16].
- Confidence: high
- Smallest useful action: delete `Control`, unify the runtime-factory seam, and perform the already-sanctioned `WriteFileAtomic` extraction (retiring the fsync-less variant first).

---

### Defended architecture / rejected hypotheses

Investigated and disproved — these should not re-enter as findings:

- **"Layering violations / needs Clean Architecture split."** Rejected: import graph is a clean DAG, mechanically enforced by parser-based boundary tests in `web` and `runcontrol` [FH + scout]. All structural criticism above targets seams and vocabularies, never layer placement.
- **Wholesale merge of the two supervision engines.** Rejected by chairs 02/04: different event vocabularies and failure envelopes; classification parity is the correct-sized fix.
- **Dual JSON+SQLite persistence as split-brain.** Rejected: all three loaders deterministically prefer DB rows; JSON mirrors at terminal states; documented in migration-product-state.md [chair-01/03]. Residual gap (staleness signalling on checkpoint fallback) tracked separately in chair-03-F02.
- **Consolidating sprint/study lock/cleanup twins.** Rejected: materially different policies per TRD §18A; only the process-identity primitive crosses the duplication threshold (folded into F05).
- **Unified redaction classifier.** Rejected twice: import-boundary test pins runcontrol to stdlib; sinks legitimately filter different domains. Correct scope is value-marker superset alignment [chair-05].
- **RunAll-unlocked as ownership defect; TUI mutations as charter violation.** Both CURRENT-CONTRACT/HISTORY: lock-free ephemeral run-all is a documented Non-Goal; read-only TUI was superseded Sprint-24 history (`tui/doc.go` mutation console) [chair-01].
- **agentwrap environment leak (strong form).** Defended: harness consumes provider credentials by design; residual kernel is the empty-allowlist env inversion (chair-05-F05, P3).
- **Web security posture.** Endorsed clean by specialist-21B and chairs 04/05 (loopback binding, HttpOnly+SameSite=Strict, HMAC artifact refs, origin proofs); no finding.
- **Stage fan-out across ~20 non-Go surfaces as broken ownership.** Rejected: templates/help/config/docs are genuine boundary translation; only Go-side inventories are chargeable (F02).
- **Scheduler regression behind F09.** Disproved by commit archaeology: stale test twin of 734eb5d [chair-05, FH-executed].

### Open questions

1. **Shutdown-contract bindingness** (affects F04 consequence sizing): `docs/plans/server-shutdown-run-cancellation-contract.md` is marked "Proposed, normative addendum". If non-binding, the shutdown-attribution leg drops toward accepted debt/P3 [chair-04 open question].
2. **Was persisting failed previews during dry runs ever required?** If yes, `service.go:740/:818` becomes documented debt instead of defect [chair-06 open question 3]. The guarded siblings argue no.
3. **Is the area-reasoning exclusion from app inventories deliberate?** Would shrink F02's realized-divergence list; the missing parity mechanism stands regardless [chair-06 open question 4].
4. **External authorization artifact for productstate's fence crossing?** A workspace record would reduce F06's governance leg to process debt; the code-level ownership gaps (schema registration, sidecar handling, path-constant duplication) stand either way [chair-06 open question 1].
5. **Coverage holes from dead specialists:** SPECIALIST-20 (test architecture) died with no output and `internal/productstate` has zero tests anywhere — the arbiter may want a targeted re-run before treating "no further test-architecture findings" as settled [chair-05 provenance warning; chair-03 flagged the same for filesystem/artifact persistence].
