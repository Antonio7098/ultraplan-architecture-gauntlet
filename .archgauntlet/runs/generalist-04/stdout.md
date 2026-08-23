I'm orchestrating a broad whole-system review. Let me orient with quick reads while delegating bounded discovery to independent subagents.
Strong evidence returned. Now I'll re-derive the key findings myself — verifying the highest-value hypotheses directly before reporting anything.
Storage-gate allowlist confirmed missing the promoted keys. Now verifying the producer side (app flattening) and consumer side (web view):
Producer and consumer confirmed misaligned with the storage gate. Counter-evidence checks now: what keys the runtime actually emits, whether any test pins this behavior, and the roadmap/SQLite authority question:
Roadmap correction captured: line 1398 commits behavior, not the storage mechanism — so no direct contradiction, but I must check whether product-state→SQLite was authorized anywhere in the workspace, since roadmap:1368 scopes a constraint here. Verifying counter-evidence on several fronts:
The workspace contract picture is significant: Gate D reserves the product-persistence/SQLite authority decision, and multiple sprint reasonings reject SQLite until measured. Verifying whether any artifact authorized the early move:
No authorizing artifact exists — Gate D remains unexecuted while the implementation already moved product state authority. Now checking how authority flips in practice (who creates the shared DB):
All key findings verified firsthand. Quick baseline health check before writing the report:
Deterministic red test at HEAD (3/3 failures). Investigating before characterizing it:
One last verification for the agent-legibility item:
# UltraPlan Architecture Gauntlet — GENERALIST-04 (whole-system evolution review)

### Scope inspected

**Implementation repo** (`ultraplan-go` @ `eeaa034`, 195 commits since 2026-05-30, 291 Go files / 64.6k lines, 112 test files):

- Read in full or in relevant part: `internal/runcontrol/{sanitize.go,model.go,sqlite.go,lifecycle.go,migration.go,retention.go}` (+ all its test files), `internal/productstate/store.go` (full), `internal/app/{run_control.go,run_commands.go,storage_commands.go,web_usecases.go}`, `internal/study/{run_loop.go,run_state.go,state.go,state_database.go,cleanup_uncertain.go,locks.go,run.go,run_loop_test.go}`, `internal/sprint/{state.go,state_database.go,verify.go,domain.go,cleanup_uncertain.go}`, `internal/platform/runtime/{runtime.go,events.go}`, `internal/web/{run_handlers.go,operation_handlers.go,operations.go,server.go}`, `internal/web/static/app.js`, `internal/web/templates/*`.
- Docs: `README.md`, `docs/architecture.md` (spot-checked claims), `docs/recovery.md`, `docs/migration-product-state.md`, `docs/cli-reference.md`, `docs/plans/*` (grep).
- Commands: `go build ./...` (clean), `go vet ./internal/{app,runcontrol,productstate,web}` (clean), `go test ./internal/{runcontrol,sprint,study}` → **study package fails deterministically**; full git churn/co-change analysis; workspace greps for authorization artifacts.

**Authoritative planning workspace** (`ultraplan-workspace` @ `368a789`): `system/contracts/core/architecture.md`, `system/contracts/runtime/persistence-and-migrations.md`, `projects/ultraplan-go/roadmap.md` (:1340–1468 read directly), `projects/ultraplan-go/docs/ARCHITECTURE.md`, `docs/TRD.md`, sprint `30/31/32/33/34/35` requirements/reasoning/flow-states, `.agents/skills/` inventory, `ultraplan.yml`.

Five independent subagents gathered evidence (lifecycle trace, event-pipeline trace, doc-drift, git churn); every headline finding below was re-derived and personally verified against source before inclusion.

---

### Architecture assessment

**Sound.** The module-driven structure is real, not cosmetic: dependency direction holds inward (no product module imports `internal/app`; `internal/platform` imports only `platform/config`; verified by import scan), composition is constructor-injected with no globals or `init()` magic in web/tui, and the run-control subsystem is the strongest part of the repo — fenced compare-and-set writes, single-winner terminal arbitration, process-birth identity probes, ordered migrations with bounded backups, and genuinely adversarial tests (multi-process shared sequences, clock jumps, PID reuse, fault injection). Atomic temp+fsync+rename persistence is consistent across study/sprint state writers. `docs/architecture.md` is unusually accurate: every numeric claim I spot-checked (hub bounds, lease intervals, prompt prefix caps, DB pragmas) matched code.

**Stressed.** Three stress points recur across findings:

1. **Schema contracts without a single owner.** The runtime-event payload has three layers (runtime promotion in `platform/runtime`, app flattening in `app/run_control.go`, storage allowlist in `runcontrol/sanitize.go`) plus two consumers (web handler view, browser JS) — and they have drifted apart within one week of shipping (F01). Similarly, status vocabularies are re-declared per layer instead of flowing from their owning modules (F06).
2. **Implementation outrunning its governance.** The authoritative planning workspace reserves the product-persistence authority decision for Gate D and sprint-35 reasoning; both were pre-empted in code during the last week of an accelerating commit cadence (84 commits in Aug) (F02), while the newest authority layer ships without direct tests (F05) and `go test ./...` — the roadmap's own release gate — is red at HEAD with no CI to catch it (F03).
3. **Undocumented physical coupling.** Two packages independently own schema in one SQLite file, with asymmetric migration discipline, and no doc names the coupling (F04).

Change-locality data supports this reading: co-change analysis shows tight healthy coupling (sprint/service.go ↔ app commands ↔ docs; UI moves as one css+js+html+templates_test unit), zero package renames ever (stable module boundaries), and test churn tracking code churn ~100% — yet the one deterministic failure that exists shipped anyway, which is a discipline signal rather than a design signal.

---

### Candidate findings

---

#### ID: GENERALIST-04-F01
**Priority:** P1

**Claim:** The observability feature shipped in commits `c455510`/`eeaa034` ("surface agent stream details in durable run events", "promote nested runtime payload facts to top level") is silently defeated by the runcontrol storage allowlist: most keys the producer deliberately promotes and the web consumer explicitly reads are dropped at `Append` time and misreported as *unsafe* omissions.

**Evidence:**
- Producer intent: `internal/app/run_control.go:407-410` — comment: "Preserve safe observable payload fields into durable storage… top-level payload keys **that the run timeline JS expects** (payload.tool, payload.action, payload.title, etc.)"; promote list at `:435` includes `title,detail,text,delta,content,native_type,line`; guaranteed presence via `findNestedString` at `:479-486` for `tool,action,title,detail,text,delta`.
- Storage gate: `internal/runcontrol/sanitize.go:10-17` — `allowedEventPayloadFields` contains only `tool,action,state,status,message,…`; `title,detail,text,delta,content,output,native_type,line` are absent, so `sanitizeEventDraft` drops them and increments the `"unsafe event detail omitted"` omission (`sanitize.go:29-45`).
- Consumer: `internal/web/run_handlers.go:288` — `firstNonEmptyPayload(event.Payload, "text","delta","detail","message","content","title","output")`: only `message` can ever survive the gate; `DetailText` is therefore near-always empty on durable replay. Browser JS reads top-level `payload.tool/kind/action` (`app.js:1082,1085,1283,1289`) — those survive, which is why tests pass.
- Runtime layer promotes the same keys independently (`platform/runtime/runtime.go:565-634`), compounding the three-layer drift.

**Architectural reason:** drift | boundary — a cross-layer schema contract (producer→storage→consumer) with no single owner and no contract test pinning it.

**Concrete consequence:** The exact capability the two most recent commits advertise — agent stream detail visible on run pages after restart/replay — does not work from the durable journal (live hub streams are unaffected; replay/history is what's lossy). Additionally, every dropped safe field inflates the omission counter, so operators see misleading "unsafe detail omitted" warnings on ordinary events, eroding trust in the omission channel that is otherwise carefully designed. Any future journal consumer (roadmap telemetry/export, support bundles per `docs/recovery.md`) inherits silently lossy data.

**Counter-evidence searched:** `runcontrol/sanitize_test.go` asserts only hostile/oversize handling (predates promotion; nothing pins promoted-key survival). `run_handlers_test.go:268-295` asserts `data-run-tool="bash"` — `tool` *is* allowlisted, so green tests mask the gap. Checked whether live SSE bypasses the gate (it does, but run timeline renders from durable `RunEvents`; production wiring passes no `Notifier`, `app/run_control.go:54`). Checked whether `message` subsumes the content: agentwrap payloads carry tool titles/details/text deltas under distinct keys — that's precisely why the promote lists exist.

**Confidence:** high.

**Smallest useful action:** Add the promoted display keys to `allowedEventPayloadFields` (the existing `sensitiveEventField`/`unsafeEventValue` filters still apply to them), and add one contract test asserting producer-promoted keys survive `Append`. Longer term, derive the allowlist and the promote list from one shared declaration so the layers cannot diverge again.

---

#### ID: GENERALIST-04-F02
**Priority:** P1

**Claim:** Mutable product state (study run state, sprint flow outcomes including review/smoke evidence) was moved to SQLite as canonical authority ahead of the governance decision that the authoritative planning workspace explicitly reserves for Gate D — and the authority flip triggers implicitly as a side effect of any run-control use, not through an explicit decision point.

**Evidence:**
- CURRENT-CONTRACT (workspace): `roadmap.md:1462-1468` — Gate D: "Keep Sprint 35 operational run persistence distinct from the authority decision for authored product artifacts"; "Select one authoritative storage mode… prohibit silent dual writes". `roadmap.md:1435` sequences "proven product persistence boundary and optional SQLite" *after* Gates A–C. `projects/ultraplan-go/docs/TRD.md:57` forbids introducing "SQLite product persistence… before their explicit roadmap evidence gates". `sprints/34-shared-context/reasoning.md:115` rejects alternate persistence pending "a new authority decision".
- REALITY (implementation): commit `02e2ec4` (2026-08-21) created `internal/productstate`; load/save prefer DB whenever `.ultraplan/run-control.db` exists (`internal/study/state_database.go:18,63`; `internal/sprint/state_database.go:58,92`). FlowState embeds review/smoke execution evidence (`internal/sprint/domain.go:135-143`) and persists through `saveFlowStateDatabase` (`state_database.go:69-97`) — i.e., flow outcomes and smoke evidence did move into the new store.
- Implicit flip: any run-control use lazily creates the DB file (`internal/app/run_control.go:35-73` → `runcontrol.OpenSQLite` creates schema, `sqlite.go:154-191`), after which `productstate.Existing` reports enabled and product loads become DB-authoritative. No config flag, migration gate, or operator-visible decision mediates this.
- Sprint-35 scope itself also ran ahead: the full durable-run subsystem is built and documented (`docs/architecture.md:153-191`) while `sprints/35-durable-run-observability/` has no `reasoning.md` — yet `roadmap.md:1398` requires reasoning to select topology/storage/lease model. (The choice of SQLite for *operational* records is permitted by `roadmap.md:1368`; shipping it before reasoning is process lag, not mechanism violation.)

**Architectural reason:** authority | drift — the plan-authority inversion means the workspace can no longer steer decisions it was designed to govern, and a runtime behavior change (which store wins) activates invisibly.

**Concrete consequence:** A user who never opted into SQLite gets DB-authoritative product state the first time any run-control-backed command touches the workspace; JSON checkpoints are written only at terminal states (`study/state.go:59-71`, `sprint/state.go:201-237`), so mid-run crash recovery now depends on the new store without the user knowing authority changed. If Gate D later selects differently, a second migration becomes mandatory. Workspace review gates continue evaluating file-based assumptions while runtime truth lives elsewhere.

**Counter-evidence searched:** Searched workspace `system/`, `projects/`, implementation `docs/plans/` for any artifact authorizing early adoption (deviation record, ADR, updated roadmap) — none found; `docs/migration-product-state.md` documents mechanics but claims no mandate. Verified `roadmap.md:1368,1398` directly rather than trusting paraphrase: sprint-35 SQLite for operational records is allowed; moving flow outcomes/smoke evidence into a canonical store is not. Considered whether `Existing()` gating counts as opt-in — it keys on mere file existence created by unrelated subsystems, so no.

**Confidence:** high on facts; medium overall only insofar as an out-of-band approval outside both repositories cannot be ruled out.

**Smallest useful action:** Record an explicit authority-decision/deviation artifact in the workspace for product-state SQLite (or revert product tables to non-authoritative until Gate D), and make the authority flip explicit — e.g., require `storage migrate` (or a config key) rather than implicit DB-file existence to enable the DB path.

---

#### ID: GENERALIST-04-F03
**Priority:** P1

**Claim:** `go test ./...` — the repository's own release gate — is deterministically red at HEAD: `TestRunLoopStartsPriorityTierBeforeLaterTiers` asserts scheduler-dependent goroutine emission ordering, and there is no CI to catch it.

**Evidence:**
- Failure reproduced 5/5 runs (including 3× isolated rerun): first started tasks `["analysis:01-structure","analysis:02-runtime"]`, want both to be `analysis:02-runtime` (`internal/study/run_loop_test.go:299-304`).
- Mechanism: tasks spawn as goroutines (`internal/study/run_loop.go:357-364`); Started progress events are emitted inside `runTask` on those goroutines, so cross-task emission order follows Go's LIFO runqueue behavior, not schedule order. The scheduler's priority ranking itself is correct (`runnableTaskIDs` ranks by `dimensionOrder` at `run_loop.go:527-549` — verified).
- History: commit `5839f9a` ("resume interrupted study sessions") rewrote the previous race-tolerant test `TestRunLoopFillsParallelSlotsFromLaterPriorityTiers` (set-based assertion over `orderedRuntime` call order) into this position-sensitive progress-callback assertion. No CI configuration exists (no `.github/workflows`).

**Architectural reason:** change-surface | failure-semantics — the verification architecture cannot enforce the roadmap's release gate (`roadmap.md:1396` requires `go test ./...` pass), so regression protection is nominal exactly where iteration is fastest.

**Concrete consequence:** Any of the last several commits could have carried silent regressions undetected; today's red suite also masks future genuine failures in the same package (signal-to-noise collapse), and contributors lose trust in the gate. The priority-tier property is real but now untested — its only guard is broken.

**Counter-evidence searched:** Confirmed the scheduling function still orders correctly (so this is test over-specification, not a scheduler regression). Checked machine-dependence: deterministic across repeated runs and package-wide runs on this host; parallelism throttling (`effectiveParallelism`, `run_loop.go:320-343`) doesn't explain reordering. Checked whether a CI file exists elsewhere (Makefile targets/scripts exist but nothing enforces them).

**Confidence:** high.

**Smallest useful action:** Restore a race-tolerant assertion (assert the first *scheduled wave* contains the priority dimension's tasks before backfill, via the existing `orderedRuntime` seam) or serialize Started emission ordering if positional guarantees are intended; then wire `go test ./...` into any pre-commit/pre-push hook or minimal CI so the documented gate is enforced.

---

#### ID: GENERALIST-04-F04
**Priority:** P2

**Claim:** Two packages independently own schema in the same SQLite file with asymmetric migration discipline, and the coupling is named nowhere in either repo's documentation.

**Evidence:**
- Same file: `internal/productstate/store.go:19` and `internal/runcontrol/sqlite.go:22` both target `.ultraplan/run-control.db`.
- Asymmetric ownership: runcontrol brings ordered migrations keyed on `PRAGMA user_version`, timestamped backups (≤3 × 512 MiB), integrity checks, exclusive `<db>.migrate.lock` (`runcontrol/migration.go:25-82,186-234`); productstate creates its tables with bare `CREATE TABLE IF NOT EXISTS` on every open (`store.go:92-116`), no version tracking, no backup discipline.
- Undocumented: neither `docs/architecture.md` nor `docs/migration-product-state.md` states that product tables share the run-control database ("the workspace database" is unnamed).

**Architectural reason:** ownership | lifecycle — file-level lifecycle (backup, migration, retention quotas) is governed by one owner while another owner's tables ride along unmanaged; future productstate schema evolution currently has no defined path that preserves runcontrol's safety properties.

**Concrete consequence:** The first product-state schema change must invent ad-hoc ALTER logic outside the established migration machinery — likely skipping backups/integrity checks and risking divergence between processes running different binary versions against one file. Conversely, runcontrol retention/quota decisions manage file growth without visibility into product-table contribution.

**Counter-evidence searched:** Deliberate-simplicity defense checked and partially accepted: DSN pragmas match exactly across owners (`store.go:67-72` vs `sqlite.go:73-80`), showing coordinated intent; WAL + busy_timeout make coexistence safe today; runcontrol's migration backups do incidentally cover product tables. Only user_version ownership keeps versioning coherent — by accident of one owner touching it, not by stated rule.

**Confidence:** high on facts; medium on severity (today it works).

**Smallest useful action:** One paragraph in both doc files naming the shared file and stating the evolution rule (e.g., "product-state schema changes enter the runcontrol ordered-migration list"), before the first real product-state schema change forces the issue.

---

#### ID: GENERALIST-04-F05
**Priority:** P2

**Claim:** The entire new persistence-authority layer — `internal/productstate`, the `storage migrate` command, and the `Migrate*ToDatabase` functions — has zero direct test coverage, one week after becoming canonical.

**Evidence:**
- Grep across all `*_test.go`: no references to `productstate.`, `runStorage`, or `storage migrate`.
- `git log` shows `internal/productstate/store.go` touched by exactly one commit ever (`02e2ec4`).
- Untested critical paths: `Store.Save` stale-item deletion loop (`store.go:182-204`), header/hash-guarded upserts (`store.go:160-178`), `MigrateRunStateToDatabase` (`study/state_database.go:43-80`), legacy-terminal-execute skip on the *migration* side (`app/storage_commands.go:166-169`; only the reconcile-side skip is tested, `sprint/locks_test.go:46`).

**Architectural reason:** change-surface | failure-semantics — this layer arbitrates which state wins on every load/save for all product workflows; bugs here corrupt canonical state with no fallback signal.

**Concrete consequence:** A Save/migration bug (e.g., wrong stale-item deletion dropping a stage record) would silently rewrite authoritative history; the surrounding packages demonstrate the expected rigor (strict-load validation, atomic-write tests), making this layer the least-defended critical component in the repo precisely while it is youngest.

**Counter-evidence searched:** Looked for indirect coverage via web/app integration tests — none exercise the DB-backed branches (they use file-based fixtures/temp dirs without the DB present, or `OpenSQLite` only for run-control concerns).

**Confidence:** high.

**Smallest useful action:** Add round-trip tests for `Store.Save/Load` (upsert, stale deletion, hash-guard skip) and migration tests mirroring the existing `state_test.go` patterns, including the legacy-terminal skip and already-imported skip paths.

---

#### ID: GENERALIST-04-F06
**Priority:** P2

**Claim:** Status/lifecycle vocabularies owned by product and runcontrol modules are re-declared as string literals across web handlers, templates, TUI, and usecases — including inconsistent spellings the presentation layer must absorb.

**Evidence:**
- Lifecycle sets redeclared despite exported constants (`runcontrol/model.go:18-30`): `internal/web/run_handlers.go:315-319`, `internal/web/operation_handlers.go:176`, `internal/web/operations.go:616` (+ contract-test copy `operations_contract_test.go:83`).
- Template string compares: `templates/sprint.html:65,75,83-92`, `project.html:44`, `runs.html:5`, `operation.html:14`; `internal/app/web_usecases.go:482` compares stage status literals.
- Study active-task predicate triplicated: `tui/views.go:245-249`, `app/study_commands.go:1129`, `app/study_usecases.go:206`.
- Vocabulary split inside the product module itself: StageStatus/ExecuteTaskStatus use `"complete"` (`sprint/domain.go:47,66`) while Review/Smoke use `"completed"` (`review.go:31`, `smoke_types.go:22`), forcing dual-spelling handling downstream.

**Architectural reason:** change-surface | drift.

**Concrete consequence:** Adding one lifecycle outcome or renaming one status requires synchronized edits in ≥6 places, none compile-checked (templates), with failure mode being silently mis-cued UI (a new terminal value renders "Unknown"/"Attention"-less). Churn data shows web templates/handlers are the hottest files in the repo — this duplication sits directly on the highest-frequency change axis.

**Counter-evidence searched:** The codebase already demonstrates the fix pattern (`runLifecycleView` computes cue strings in the handler from typed values) — so this is partial adoption, not an architectural constraint; html/template receives data and needs no Go imports, so exporting predicates/constants from product modules respects the verified dependency direction.

**Confidence:** high on duplication; medium on consequence severity.

**Smallest useful action:** Extend the `runLifecycleView` pattern: handlers/usecases compute status cues from exported module constants/predicates; delete template-side string compares. Optionally normalize `"complete"` vs `"completed"` at the next flow-state schema bump.

---

#### ID: GENERALIST-04-F07
**Priority:** P3

**Claim:** `jsonMarshalTruncated` does not marshal JSON — structured payload values are persisted to the durable journal as Go `%v` syntax under a name and comment claiming JSON encoding.

**Evidence:** `internal/app/run_control.go:558-564` routes map/slice values through `jsonMarshalTruncated`; `:570-575` returns `fmt.Sprintf("%v", v)` with comments claiming "marshal then truncate" and an impossible stdlib "import cycle" rationale.

**Architectural reason:** drift | legibility — the journal is positioned as replayable/inspectable support-bundle material; its structured fields aren't machine-parseable as advertised.

**Concrete consequence:** Namespaced sub-key values and stringified nested maps land in the journal as `map[k:v]` strings; any export/telemetry consumer parsing JSON fails silently. Today impact is display-only, but it contaminates exactly the observability surface F01 shows is already fragile.

**Counter-evidence searched:** No current consumer parses these values as JSON (web JS treats them as text), so no live breakage; sanitize still applies afterward.

**Confidence:** high.

**Smallest useful action:** Actually `json.Marshal` + truncate (encoding/json is already imported elsewhere in runcontrol); delete the misleading comments.

---

#### ID: GENERALIST-04-F08
**Priority:** P3

**Claim:** Recovery-critical documentation contradicts code: the documented review-attempt expiry is 24 hours where code uses 2 hours, and the cleanup-uncertain marker filename split (dotted vs undotted) is half-documented.

**Evidence:**
- `docs/recovery.md:75`: "more than **24 hours**" vs `internal/sprint/verify.go:455-467`: `now.Sub(lastSeen) > 2*time.Hour` plus immediate dead-PID expiry (`:459-461`). (The 24h constants found elsewhere are smoke-manifest timeout bounds and CLI flags — different things.)
- Marker split: `internal/sprint/cleanup_uncertain.go:15` = `.cleanup-uncertain.json`; `internal/study/cleanup_uncertain.go:15` = `cleanup-uncertain.json`; `recovery.md:112` documents only the dotted form.

**Architectural reason:** drift — recovery paths are exactly where doc-contract fidelity matters most; an agent or operator following the authoritative doc misjudges stall detection and misses the study-side uncertainty marker.

**Concrete consequence:** Operators wait a day for timeouts that fire in two hours (or wrongly diagnose early timeouts as bugs); study cleanup uncertainty is undiscoverable via docs, weakening the fail-closed reconciliation story (`ReconcileInterruptedRun` depends on that marker, `study/cleanup_uncertain.go:64-150`).

**Counter-evidence searched:** Verified no third constant reconciles the discrepancy; confirmed both marker constants are used in write/read paths respectively (not dead code).

**Confidence:** high.

**Smallest useful action:** Correct `recovery.md:75` to 2 hours (mentioning dead-PID immediate expiry) and document both marker filenames — or unify the names in code.

---

#### ID: GENERALIST-04-F09
**Priority:** P3

**Claim:** The agent-facing skill set is out of sync with code and docs: 11 stage skills are defined in code and documented, but only 10 are materialized in the authoritative workspace — `ultraplan-code-context` is absent.

**Evidence:**
- Code defines exactly 11 skills incl. `ultraplan-code-context` (`internal/workspace/skills.go:38-228`, :102-118); documented in `README.md:199` and `docs/stage-skills.md:5-17`.
- Workspace `.agents/skills/` contains 10 directories; `ultraplan-code-context` is missing (verified by directory listing). The implementation repo itself has no `.agents` materialization at all.

**Architectural reason:** drift | change-surface — the code-context stage was added to the skill registry (sprint 33 era) but the workspace's materialized skills were never refreshed; agents operating under the documented workflow cannot invoke `$ultraplan-code-context`.

**Concrete consequence:** An agent following `stage-skills.md` in the workspace hits a missing skill for a stage the implementation fully supports; the discrepancy also hides whether other materialized skills lag their embedded definitions (spot-check showed the existing 10 are current).

**Counter-evidence searched:** Confirmed the skill isn't intentionally deprecated (code, README, and stage-skills all present it as current with a canonical-delegation exception).

**Confidence:** high.

**Smallest useful action:** Rerun `ultraplan skills materialise` against the workspace and add a doc note that skill materialization must follow stage additions.

---

### Defended architecture / rejected hypotheses

1. **"Dual-backed state (SQLite + JSON checkpoints) risks lost updates or split-brain."** Rejected as a defect: load deterministically prefers DB when present (`study/state.go:27-57`, `sprint/state.go:20-32`); writes are atomic temp+rename+fsync; JSON mirrors only at terminal states by design ("checkpoints", matching `docs/migration-product-state.md`); crash windows are covered by `ResumeValidateRunState`/`ReconcileRunState` (`run_state.go:277-361`) and reconcile never promotes stale running state. Legacy files are intentionally preserved as compatibility checkpoints, and legacy-shape parsers (`execute_state.go:81-103`, `state.go:131-148`) are guarded by tests. The residual concern (authority flips implicitly) is captured in F02, not here.

2. **"Sprint stage status has two sources of truth (declared builders vs artifact-derived `DeriveStages`) that can diverge."** Investigated and largely defended: `DeriveStages` (`service.go:1340-1402`) recomputes from artifacts on every `Status` read, making declared-vs-derived divergence self-healing; legacy/pre-code-context states are explicitly exempted (`service.go:150,191-195`). Risk is bounded to the window between mutations; no observed inconsistency in workspace artifacts.

3. **"Inline synchronous SQLite append under `eventMu` back-pressures the agent event stream."** Rejected at current scale: appends are local WAL/immediate-tx with bounded values and a 250 ms coalescing window; persistence failure is recorded first-error-wins without killing the run (`app/run_control.go:149-209`); the runtime ring buffer absorbs bursts (`runtime.go:139`). Slow-disk pathology remains theoretical; the durability-first trade-off is coherent with the subsystem's contract.

4. **"Two memory-pressure fixes in six days (July) indicate structural flaw in the run loop."** Rejected as current defect: the fixes (`7340090`, `b332838`) introduced the adaptive `effectiveParallelism` throttle and bounded collection now visible at `run_loop.go:320-343` and the 200-event ring at `runtime.go:139`; no recurrence in the subsequent six weeks of heavy churn.

5. **"Roadmap forbids SQLite, so runcontrol violates the plan."** Disproved by direct reading: `roadmap.md:1368,1398` delegate the operational-storage mechanism to sprint-35 reasoning and restrict only product-artifact authority (covered correctly in F02). The implementation's runcontrol-on-SQLite is mechanism-compliant; its timing precedes required reasoning (noted in F02).

6. **"TUI/web divergence is parity drift."** Rejected: the TUI was chartered as a read-only foundation owning no product state (`sprints/24-tui-foundations/requirements.md:62`); its 9-commit history vs web's 45 reflects staged surface evolution, not abandonment of a parity contract.

7. **"The nil `Notifier` seam is dead abstraction."** Rejected as defect: documented as best-effort latency optimization (`runcontrol/interfaces.go:41-45`); poll-based SSE with cursor/gap semantics is implemented and tested. Whether the seam is *earned* yet is debatable, but removing it would save little and the poll path is correct.

---

### Open questions

1. **Out-of-band authorization for F02:** Does an approval record for product-state SQLite adoption exist outside both repositories (chat, PR discussion, local notes)? If yes, F02 downgrades from authority violation to documentation debt; if no, it stands as written.
2. **Intended durability posture for promoted payload keys (F01):** Was excluding free-text-ish keys (`text/delta/detail/title`) from `allowedEventPayloadFields` a deliberate redaction posture that the promotion commits mistakenly assumed away? If deliberate, the fix is to retract the consumer/promoter expectations instead of widening the allowlist — the answer changes the remedy, not the finding.
3. **Run-page SSE lifetime:** `followRunSSE` (`run_handlers.go:461-527`) has no max-stream-lifetime cap, unlike operations SSE (30 min caps at `operation_handlers.go:255,460`). Long-running studies may legitimately stream for hours, so this may be intentional; if not, it is an unbounded-resource inconsistency worth one line of policy.
