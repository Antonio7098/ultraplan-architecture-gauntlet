I'll start by orienting myself in both repositories, then dispatch subagents for bounded discovery.
Now I have the lay of the land. Dispatching parallel subagents for bounded discovery across the five state domains in my assignment.
Four strong discovery reports returned. Now I'll re-derive the key claims myself before reporting anything. Verifying the highest-value candidates directly:
Sanitize allowlist confirmed missing the promoted display fields. Now verifying both ends of that pipeline — producer promotion and consumer reads:
Producer side confirmed: `runtimeEventDraft` promotes `title/delta/content/...` and forces display keys (run_control.go:435,479), with a comment claiming these reach "the run timeline JS". Now checking what the durable readers expect:
Consumers confirmed reading `text/delta/detail/title/output`. Now verifying the full append path applies sanitization and searching for tests that pin either behavior:
Confirmed: `sqlite.go:617` sanitizes every append, and sanitize.go was untouched since the original commit — while the last two commits added promotion of fields the allowlist drops. Checking what payload keys the runtime layer actually produces, to gauge real-world impact:
The drift is now precisely scoped. Verifying remaining candidates — reconciler terminal write path, hardcoded target path, index-heading whitelists, and smoke section counts:
Both confirmed. `reconcileUnclaimed` is a second physical terminal writer (structurally necessary — no attempt exists to fence — but a second path nonetheless), and the execute target is a compile-time exact-match constant. Now verifying the workspace-side coupling claims and the arbitration test:
Heading drift confirmed on three fronts. Pinning down the exact section constants, smoke heading requirements, subset validation, and the docs position on dual runs:
One more coupling check — whether the workspace's git-tracked state JSONs can diverge from the DB-held authority:
The parser silently skips unrecognized sections (`section = recognizedSections[name]` → empty → rows skipped, no finding). Checking whether aren sprints actively select evidence — latent vs. active breakage:
Final verification pass on the execute-target constant usage breadth:
All verification complete. Writing the scout report:

---

# SCOUT-05 — State/lifecycle cartography: UltraPlan Go

## Scope inspected

**Implementation repo** (`/home/antonio/gauntlet-run/ultraplan-go` @ `eeaa034`, clean):
- `internal/runcontrol`: `sqlite.go` (schema, Append/Claim/ProposeTerminal/fencing), `lifecycle.go` (heartbeat/cancel/Reconcile/reconcileUnclaimed), `lifecycle_test.go`, `sanitize.go`, `migration.go`, `model.go`, `retention.go`, `local_log.go`
- `internal/app`: `run_control.go` (controlledRuntime, runtimeEventDraft, terminalOutcome), `durable_operations.go`, `run_commands.go`, `sprint_commands.go`
- `internal/study`: `state.go`, `locks.go`, `run_loop.go`, `run_state*.go`, `run_history*.go`, `cleanup_uncertain.go`, `summary.go`
- `internal/sprint`: `execute_target.go`, `service.go` mutation lease, `verification_lock.go`, `locks.go`, `validation.go`, `smoke.go`, `review.go`, `verify.go`, `state.go`, `execute_state.go`, `session_state.go`, `code_context.go`
- `internal/productstate/store.go`; `internal/project/index.go`, `validation.go`, `domain.go`; `internal/platform/runtime/{runtime.go,agentwrap.go}`; `internal/web/{server.go,run_handlers.go,operations.go,static/app.js}`; `internal/workspace/init.go`, `scaffold/templates/project-index.md`
- Commands run: targeted `git log -p` on `sanitize.go`, greps for enum writers, template consumers

**Workspace** (`/home/antonio/gauntlet-run/ultraplan-workspace` @ `368a789`): `ultraplan.yml`, `system/protocols/{plan-sprint,review-sprint,deep-smoke-sprint}-protocol.md`, `projects/ultraplan-go/project-index.md` (headings + Target line), `projects/aren-phase-01-execution-lifecycle/project-index.md` (headings), `sprints/{28,33,34}*/smoke.md` headings, git-tracked state JSON census (`git ls-files | grep -c "run-state\|flow-state"` → 107), `.ultraplan/` presence on disk (none).

Four parallel discovery subagents covered locks/leases, lifecycle traces, the event system, and workspace coupling; every reported finding below was re-derived from primary source before inclusion.

## Architecture assessment

The state cartography resolves into **five tiers with clean authority ordering**, documented in `docs/architecture.md:153-191` and honored in code:

| Tier | Medium | Owner | Lifetime |
|---|---|---|---|
| Durable run control (identity, leases, fences, events, terminal) | `.ultraplan/run-control.db` SQLite WAL/`synchronous=FULL`, immutable event journal w/ trigger | `internal/runcontrol` | Cross-process, single host |
| Product workflow state (flow/execute/study task states) | versioned JSON, temp+fsync+rename+dir-fsync; DB mirror via `productstate` at terminal boundaries | `internal/sprint`, `internal/study` | Per-sprint/per-study, guarded by O_EXCL identity lockfiles |
| Derived projections | `runs.last_sequence`, `oldest_retained_sequence`, `omission_total`, `product_state_items` hash-guarded upserts | maintained transactionally in-tx | Never recomputed from journal |
| Ephemeral | web `operationHub` ring (256 ev/256KiB), TUI buffers, `.stage-sessions.json` checkpoints (no fsync, by design), context-pack cache | `internal/web`, `internal/tui` | Process/session |
| Authored inputs | project-index/sprint-index/plan/reasoning Markdown; fingerprint-hashed into review freeze set | humans + planning agents | Round-trip through Go validators |

Sound: terminal arbitration is genuinely single-winner (fenced CAS, test-pinned idempotency); lease recovery uses exact process-birth identity (`/proc/<pid>/stat` starttime + boot_id), never PID-alone; clock authority is SQLite's own clock for expiry (`expiredLeasePredicate`); retention is inline-compacting with explicit omission accounting rather than silent truncation; three coexisting lock regimes (study loop lock, sprint mutation lease, migration lock) are each scoped to exactly one resource family.

Stressed: the **Markdown artifact contract has no closed vocabulary** — heading names are string-matched at parse time, silently skipped when unrecognized, and validated only far downstream. The **event payload contract spans three packages with no shared schema or end-to-end test**, which is where the freshest drift sits.

## Candidate findings

### SCOUT-05-F01
- **Priority**: P1
- **Claim**: The durable-event sanitize allowlist drops exactly the display fields that the two most recent observability commits (`c455510`, `eeaa034`) promote; agent stream detail never reaches the durable journal or SSE replay despite producer comments claiming it does.
- **Evidence**:
  - Producer promotes `tool/title/detail/text/delta/content/message/action/state/status/native_type/line` and force-inserts `text/delta/detail/title` display keys: `internal/app/run_control.go:435,479-486`; comment states intent "Preserve safe observable payload fields **into durable storage** … that the run timeline JS expects".
  - Storage gate allowlist omits `title/detail/text/delta/content/output/name/phase/native_type/line` and all namespaced `<field>_<sub>` keys: `internal/runcontrol/sanitize.go:10-17`; applied to every append at `internal/runcontrol/sqlite.go:617`.
  - Consumer reads the dropped keys: `internal/web/run_handlers.go:288` `firstNonEmptyPayload(payload,"text","delta","detail","message","content","title","output")` feeding `DetailText`; only `message` can ever match.
  - `git log -- sanitize.go`: last touched in `e09d394` (initial run control); both observability commits skipped it.
  - No end-to-end test pins survival of any promoted field; only the oversize path is tested (`internal/runcontrol/sanitize_test.go:50`).
- **Architectural reason**: drift + change-surface. Adding one observable field requires synchronized edits in `platform/runtime` promotion (`runtime.go:592-634`), `app/run_control.go` promote map, and `runcontrol/sanitize.go` allowlist — two modules, no shared contract artifact, no integration test.
- **Concrete consequence**: operators following `run follow` / web replay see empty or generic detail text for message/tool events and inflated omissions labelled *"unsafe event detail omitted"* (`sanitize.go:44`) — misattributing benign unallowlisted fields as safety rejections. The stated purpose of both commits is silently defeated; the next contributor will plausibly "fix" the producer again.
- **Counter-evidence searched**: browser JS reads only surviving keys (`payload.tool/action/message/state/reason/kind`, `app.js:705-711,1082-1289`) — UI degradation is partial; message-type events may carry `message`. Neither restores text/delta/title detail. Searched for aliasing/normalization downstream of Append: none exists.
- **Confidence**: high
- **Smallest useful action**: extend `allowedEventPayloadFields` with the promoted display set (or add an app↔runcontrol round-trip test asserting `runtimeEventDraft → Append → Events` payload survival), so the gate and the producers share one tested contract.

### SCOUT-05-F02
- **Priority**: P1
- **Claim**: The project-index catalog parser silently ignores unrecognized section headings, there is no authoring-time validation for them, and two real artifacts already use unrecognized headings — including the shipped scaffold template — making sprint planning fail late with remediation pointing at the wrong file.
- **Evidence**:
  - Silent skip: `internal/project/index.go:26-28` sets `section = recognizedSections[name]` (zero value for unknown headings); rows after unknown `##` headings fall through `index.go:31-33` with no finding.
  - Recognized vocabulary is six exact strings (`internal/project/domain.go:9-15`, e.g. `"Available Evidence Reports"`).
  - Shipped template contradicts parser: `internal/workspace/scaffold/templates/project-index.md:35` uses `## Available Evidence Packs And Reports` with name column `Evidence` (no recognized alias); written into every fresh workspace by `workspace init` (`internal/workspace/init.go:69,124`).
  - Live instance: `ultraplan-workspace/projects/aren-phase-01-execution-lifecycle/project-index.md` headers include `Available Agent-Harness Evidence Reports` (:48) and `Available Supporting Go CLI Reports` (:66); it has no `Available Reasoning Templates` or `Smoke Harnesses` sections at all. Its parsed catalog yields zero evidence/template entries.
  - Late failure surface: `internal/sprint/validation.go:16-17` `validateSubset` emits "selected entry not in project index" with suggestion "Select an entry listed in project-index.md" — but it *is* listed.
  - Early gate absent: `project validate` checks existence + malformed rows only (`internal/project/validation.go:46-79`); unrecognized headings produce no finding.
- **Architectural reason**: boundary + lifecycle. The Markdown catalog format is load-bearing state whose schema lives implicitly in a parser whitelist; authoring tools (template, agents), protocol docs, and the parser have drifted, and validation runs at consumption, not production.
- **Concrete consequence**: the first sprint planned against `aren-phase-01-execution-lifecycle` fails planning with a misleading error; fresh workspaces scaffolded from the embedded template reproduce the same mismatch deterministically. Fix cost lands on artifact authors debugging a parser they cannot see.
- **Counter-evidence searched**: prompts instructing exact headings (agents demonstrably deviated in aren); path normalization shims exist (`normalizeWorkspacePath`, `handbook.go:133-140`) but no section aliasing anywhere; checked whether aren sprints exist yet (none — breakage is latent, not active).
- **Confidence**: high
- **Smallest useful action**: emit a SeverityError finding in `ParseProjectIndex` for table content under unrecognized `##` headings (and/or align the scaffold template's heading/column with `domain.go`), turning silent skip into actionable diagnostics at validate time.

### SCOUT-05-F03
- **Priority**: P2
- **Claim**: Terminal state has a second physical write path — `reconcileUnclaimed`'s direct INSERT+CAS — outside the invariant its own test claims is universal ("EveryTerminalOutcomeUsesOneImmutableArbitrationPath").
- **Evidence**: direct writer at `internal/runcontrol/lifecycle.go:452-473` (own `INSERT INTO events … 'terminal'` + `UPDATE runs SET lifecycle…terminal_outcome… WHERE current_attempt_id IS NULL AND terminal_outcome IS NULL`); the pinned path is `SQLiteRepository.ProposeTerminal` (`internal/runcontrol/sqlite.go:722-807`). The test (`internal/runcontrol/lifecycle_test.go:102-117`) exercises only claimed runs via `openClaimedRepository` → `ProposeTerminal`; the unclaimed path is uncovered by it.
- **Architectural reason**: authority/lifecycle. The single-arbiter invariant is enforced by convention plus a test whose name overstates its scope; predicate semantics (`current_attempt_id IS NULL` variant) are duplicated by hand.
- **Concrete consequence**: future hardening of `ProposeTerminal` (new guard columns, changed terminal-event payload shape, retention interaction) silently diverges for never-claimed runs; reconciler-terminalized runs would grow structurally different journals from owner-terminalized ones.
- **Counter-evidence searched**: the split is structurally motivated — `ProposeTerminal` verifies a fence bound to `current_attempt_id`, which an unclaimed run cannot have; predicates are currently semantically equivalent; docs describe grace handling (`docs/architecture.md` reconciliation paragraph; `docs/recovery.md:194-208`). So this is duplication risk, not present-day incorrectness.
- **Confidence**: high
- **Smallest useful action**: extract the terminal-write body into one shared internal helper parameterized by the guard predicate, or add a parity test pinning `reconcileUnclaimed`'s writes to the same shape ProposeTerminal produces, plus cross-referencing comments.

### SCOUT-05-F04
- **Priority**: P3
- **Claim**: The approved execute target is dual-homed — compiled into the binary and restated in the authored workspace artifact — must byte-match at execute time, and nothing detects skew until then.
- **Evidence**: `const ApprovedExecuteTargetPath = "/home/antonioborgerees/coding/ultraplan/ultraplan-go"` (`internal/sprint/execute_target.go:11`), exact-match rejection at :22-24; workspace artifact repeats the same machine-specific absolute path (`projects/ultraplan-go/project-index.md:10`). On this checkout (`/home/antonio/gauntlet-run/ultraplan-go`) the pair cannot match, so `sprint … execute` fails with "unsupported execute target" until either source is edited.
- **Architectural reason**: authority/boundary. Environment identity is encoded as policy in two media (source constant + Markdown) with no startup consistency check; approval semantics and environment coupling are conflated.
- **Concrete consequence**: repository relocation or binary/workspace version skew surfaces only at execute time, with remediation text ("Update project-index.md or defer alternate target support") pointing authors at the artifact half of a two-copy constraint.
- **Counter-evidence searched**: the finding text at `execute_target.go:24` explicitly defers alternate-target support to a later sprint — the single-target policy itself is declared FUTURE-INTENT debt and is not challenged here; tests were searched for earlier skew detection (none found). Only the dual-homing/skew-latency aspect is reported.
- **Confidence**: high
- **Smallest useful action**: derive the constant from config/workspace state (or warn at `project validate`/`status` when the artifact target differs from the compiled value).

### SCOUT-05-F05
- **Priority**: P3
- **Claim**: Dead state-machine members have live operator surfaces: `Liveness` values `owner_unreachable/interrupted/cleanup_uncertain` and `CancellationState.uncertain` have no writers, yet `Health.CancellationUncertain` is queried and printed as if observable.
- **Evidence**: definitions `internal/runcontrol/model.go:60-62,82`; `IsValid` admission :68-69,:87; repo-wide grep finds zero non-test writers (Heartbeat writes `live`, reconciler writes `stalled`/`terminal`, Claim/Terminal write their own); nevertheless `SELECT COUNT(*) … cancellation_state = 'uncertain'` feeds Health (`internal/runcontrol/sqlite.go:877`) and is rendered in `run status` output (`internal/app/run_commands.go:271`).
- **Architectural reason**: lifecycle/dead-state inventory drift between the declared state space (enums + CHECK constraints, `sqlite.go:246`) and the reachable state space.
- **Concrete consequence**: operators read a health metric that can structurally never be non-zero and a liveness vocabulary advertising recoverable states (`owner_unreachable`) the system never records; diagnostics vocabulary implies recovery paths that don't exist.
- **Counter-evidence searched**: forward-compat reservation is plausible, but CHECK-constraint membership alone preserves that; the Health counter and CLI rendering add an operator-facing claim without a writer. Searched for dynamic/SQL-string construction of these values (none).
- **Confidence**: high
- **Smallest useful action**: remove `CancellationUncertain` from the Health query/display (or annotate model constants as reserved-for-future with the same wording in `run status` help).

### SCOUT-05-F06
- **Priority**: P3
- **Claim**: The deep-smoke protocol document specifies a 10-section `smoke.md`; the validator enforces 13; current artifacts satisfy 13 because Go renders them — the authoritative protocol doc lags the enforced schema.
- **Evidence**: `system/protocols/deep-smoke-sprint-protocol.md` §7 lists ten sections (missing `Harness And Protocol`, `Smoke Authoring`, `Safe Invocation`); validator requires thirteen (`internal/sprint/smoke.go:555-561`); renderer emits them (`smoke.go:465-551`); recent artifacts carry all 13 (`sprints/33-code-context-stage/smoke.md`, `34-shared-context/smoke.md` heading censuses).
- **Architectural reason**: drift between CURRENT-CONTRACT docs and enforced reality in the round-trip artifact channel; harmless today only because the renderer owns the artifact.
- **Concrete consequence**: reviewers or future re-implementations (e.g., externalizing smoke authoring per protocol §3) conforming to the documented 10-section shape will be rejected by the validator as malformed.
- **Counter-evidence searched**: confirmed the extra three headings appear in neither the protocol list nor older-artifact conventions; confirmed no schema-version field exists in any Markdown artifact that would explain the gap as versioning.
- **Confidence**: high
- **Smallest useful action**: update the protocol's §7 section list to the enforced thirteen.

## Defended architecture / rejected hypotheses

1. **Two durable runs per CLI command (operation envelope + runtime child)** — investigated as possible double-counting; defended by design: `docs/architecture.md:162-164` explicitly places "Direct CLI commands, TUI actions, web operations, and individual runtime children" inside the accept/claim boundary; each gets its own lifecycle, and correlation flows through diagnostics/support export (:186).
2. **Dual durability of product state (JSON file + `productstate` DB mirror)** — suspected split-brain; rejected: authority rule is explicit — DB wins when present (`LoadRunState`, `internal/study/state.go:27-35`), mid-run saves go DB-only with the file rewritten at completion (`state.go:59-71`); the tracked JSONs are portable/crash-recovery artifacts. In this workspace snapshot no `.ultraplan/` DB exists on disk, so files are correctly authoritative by the fallback rule. Single-host topology is documented (`architecture.md:188-191`).
3. **Absence of flock in favor of O_EXCL lockfiles** — suspected weakness; deliberate: O_EXCL payloads carry PID/acquired-at identity enabling forensic stale detection and ownership-checked release (`internal/study/locks.go:105-123` refuses release on identity change); flock auto-release would destroy the crash record. Stale takeover paths exist at every layer.
4. **Non-fsync'd ephemeral writes** (`.stage-sessions.json`, run-history JSONL, diagnostics log) — suspected durability gaps; accepted-by-design: session checkpoints are resumable caches; history appends carry dedup keys; logs are bounded drop-on-full diagnostics.
5. **`reconcileExpiredAttempts`/`saveReviewState` whole-file read-modify-write of FlowState** — suspected lost-update window; serialized by the per-sprint mutation lease plus context-marker re-entry guard (`locks.go:90-104`); bypass paths deliberately avoid rewriting live leases (`cleanup_uncertain.go:28-30`).
6. **Study retry without stored retry counter** — `Attempts` is monotonic and retries derived (`attempts-1`), preserved across restarts intentionally ("resumable rather than exactly-once", `run_loop.go:16-22`); scheduling cap via the per-pass `attempted` map prevents same-invocation loops.

## Open questions

1. F01: does `agentwrap` ever emit message events whose human-readable text arrives under the allowlisted `message` key (which would partially mask the loss)? The external module isn't vendored here; answerable only from its event fixtures.
2. F02: are the plan/review protocol docs' project-index guidance (exact headings) normative for agents, i.e., was aren's index authored off-contract, or does the contract itself permit renamed evidence sections? This changes whether the fix belongs in parser leniency vs. authoring discipline.
3. Whether `Health.CancellationUncertain` was written by an earlier schema generation (HISTORY check against pre-`e09d394` designs) — if a former writer existed, F05 becomes removal-debt rather than dead vocabulary.
