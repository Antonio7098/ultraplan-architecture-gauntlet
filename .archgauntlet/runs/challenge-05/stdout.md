# CHALLENGE-05 — Security, errors, observability, testing, performance, operational claims

### Scope inspected

**Implementation repo** (`ultraplan-go` @ `eeaa034`, clean), read first-hand: `internal/runcontrol/{sanitize.go,import_boundary_test.go,interfaces.go (Notifier)}`, `internal/app/{run_control.go (full), health_commands.go, durable_operations.go, operation_runner.go}`, `internal/web/{operations.go (safeProjectedText/hub publish/finish), operation_handlers.go (SSE reconnect/durable fallback), run_handlers.go, templates/run.html}`, `internal/platform/config/redaction.go`, `internal/sprint/{smoke_protocol.go (smokeEnvironment), smoke_types.go}`, `internal/study/init_clone.go`, `internal/workspace/validation.go`, `internal/tui/views.go` (omission rendering).

**Commands**: `go test ./... -count=1`; `go test ./internal/study -run TestRunLoopStartsPriorityTierBeforeLaterTiers -count=3` (fails deterministically); `git log --follow -- internal/runcontrol/sanitize.go`; targeted `rg` for payload-key consumers (`payload.`/`Payload[` in web/TUI/CLI), `Notifier`, `OpenSQLite` call sites, `Terminal*` classification sites.

**Subagent evidence** (bounded, re-derived by me before use): error-taxonomy/lifecycle trace (FinishOperation, classifyStoreError escapes, retry-helper inventory, metrics/local-log exposure); testing/perf/ops verification (test inventory, quota enforcement site, hub bounds, shutdown drain, smoke timeoutschallenge-05 complete — report written to `.archgauntlet/runs/challenge-05/stdout.md` (154 lines).

**Outcome:**

**Candidate findings (10):**
- **F01 (P1)**: Sanitize-gate/promotion drift re-derived end-to-end — producers promote `title/detail/text/delta/…`, gate drops them, durable consumers read exactly those keys (`run_handlers.go:288`), zero pinning tests; plus new nuance: eeaa034's namespaced keys are structurally impossible to pass the fixed allowlist.
- **F02 (P2)**: **Deterministic red test at reviewed commit** — `TestRunLoopStartsPriorityTierBeforeLaterTiers` fails 4/4 runs; no prior review stage caught it.
- **F03 (P2)**: Outcome classification exists as three divergent copies; cancel-origin conflated in durable record; live web can't represent `timed_out`.
- **F04 (P2)**: `study init` git clone bypasses platform/process contract (no ctx/timeout).
- **F05–F10 (P3)**: smoke-env nil→inherit-all inversion; `jsonMarshalTruncated` lies about its contract; mid-run persistence-loss wiring untested at app level; SSE dual id-series untested (severity reduced from SPECIALIST-12A); fabricated `filesystem.read` health check; secret value-marker divergence.

**Rejected hypotheses (8)** — notably: unifying redaction classifiers would violate runcontrol's test-enforced leaf status; my own `%v`-nondeterminism hypothesis failed (fmt sorts map keys since Go 1.12); agentwrap env inheritance defended as by-design with F05 as the residual kernel.

**Open questions (3)**: allowlist policy ratification, F02 root cause, near-quota WAL transience.
payload fields both observability commits promote; every durable consumer reads those dropped keys, no test pins their survival, and the commit messages claim the opposite of runtime behavior.
**Evidence:** REALITY. Producer promotes `tool,title,detail,text,delta,content,message,action,state,status,native_type,line` plus namespaced `<field>_<sub>` keys (`internal/app/run_control.go:435,438,479-486`); allowlist permits none of `title/detail/text/delta/content/output/native_type/line` or any namespaced key (`internal/runcontrol/sanitize.go:10-17`), applied on every append (`sqlite.go:617`), incrementing `"unsafe event detail omitted"` (`sanitize.go:30-44`). Durable web timeline reads precisely the doomed keys: `firstNonEmptyPayload(event.Payload, "text","delta","detail","message","content","title","output")` (`web/run_handlers.go:288`) rendered at `templates/run.html:18` (`data-run-text`, `.run-event-detail`), omission total shown (`run.html:11`); TUI renders omission counts (`tui/views.go:338-339`). `sanitize.go` untouched since initial commit `e09d394` while `c455510` ("surface agent stream details in durable run events") and `eeaa034` ("promote nested runtime payload facts to top level") touched only producers. No test in `runcontrol/*_test.go`, `app/run_control_test.go`, `durable_operations_test.go`, or `web/run_handlers_test.go` asserts survival of any promoted key through Append→Events (grep verified empty). New nuance beyond prior outputs: eeaa034's namespaced keys are *structurally impossible* to pass the gate (not enumerable in a fixed allowlist), and the coalescing hash is computed pre-sanitization over draft payload (`run_control.go:177-178`), so events differing only in gate-doomed keys never coalesce — inflating durable rows while their content is discarded.
**Architectural reason:** drift / change-surface — the allowlist is the documented final redaction authority but nothing connects it to producer promotion lists; adding one observable field requires synchronized edits across `platform/runtime`, `app/run_control.go`, and `runcontrol/sanitize.go` with no shared contract or integration test.
**Concrete consequence:** `run show`, `/runs/<id>`, SSE replay, TUI run view, and support exports render message-only fallback text (or blank detail) for tool/reasoning/message events while omission counters inflate with reason "unsafe event detail omitted" — operators misread healthy runs as redacted; the next contributor will edit the producer again and be re-bitten, exactly as `c455510` was.
**Counter-evidence searched:** alternative unsanitized read path (none — Append is sole write path); live-SSE bypass making durable loss irrelevant (production passes no Notifier — `OpenSQLite` call sites `run_control.go:54`, `storage_commands.go:60`; consumers poll repository events); tests asserting intentional drop (none); docs ratifying strictness post-c455510 (none found). Corroborates SCOUT-05-F01, SCOUT-06-F01, SPECIALIST-07A/B, GENERALIST-04-F01, CHALLENGE-02 F05 — independently re-derived here.
**Confidence:** high
**Smallest useful action:** Either extend `allowedEventPayloadFields` with the promoted display keys and add one end-to-end test asserting their survival through Append→Events→view, or (if strictness is policy) revert the promotion lists and fix consumer fallback expectations — but pick one owner for the decision.

---

**ID: CHALLENGE-05-F02**
**Priority:** P2
**Claim:** The reviewed commit has a deterministically failing test (`TestRunLoopStartsPriorityTierBeforeLaterTiers`), contradicting the repo's advertised test discipline; either priority-tier scheduling regressed or the guarantee's test is stale.
**Evidence:** REALITY. `go test ./... -count=1`: all packages pass except `--- FAIL: TestRunLoopStartsPriorityTierBeforeLaterTiers (internal/study, run_loop_test.go:303)`; reproduced 4/4 runs including `-count=3`. Failure output: `first starts = [analysis:01-structure analysis:02-runtime], want the priority dimension` — with `dimension_order ["02"]` and Parallelism 3 (`run_loop_test.go:276-305`), the second started task came from a later tier. Recent commits touching these files: `f067727`, `0f99eb7`, `5839f9a`.
**Architectural reason:** lifecycle / testing-discipline — study run-loop scheduling order is a product-visible guarantee asserted nowhere else; a red main at the reviewed commit breaks the trust basis every other "verified by tests" claim in this gauntlet rests on.
**Concrete consequence:** CI cannot distinguish future regressions from this known-red state; if the scheduler regression is real, priority dimensions run late for users (correctness of completion unaffected — status still Completed).
**Counter-evidence searched:** reruns with `-count=3` (deterministic, not flake-order noise); fixture/task-set misread (fixture yields ≥2 priority-tier tasks, so expectation is satisfiable); skipped-test gating (unrelated). No prior review output noticed this failure (generalist-04 and others ran targeted packages only).
**Confidence:** high (red state); medium (which side — code or expectation — is wrong)
**Smallest useful action:** Owner triage: bisect against `5839f9a..eeaa034` to decide regression-vs-stale-expectation and restore green before further review cycles rely on the suite.

---

**ID: CHALLENGE-05-F03**
**Priority:** P2
**Claim:** Terminal-outcome classification exists as three divergent behavioral copies; the app-operation copy cannot distinguish user cancellation from store-driven/internal cancellation (all collapse to `ctx.Err()`), and the live web projection cannot represent `timed_out` at all.
**Evidence:** REALITY. Copy 1 (runtime): `app/run_control.go:625-637` inspects parent ctx, `result.Status=="cancelled"`, and `result.Error.Category`. Copy 2 (operations): `app/durable_operations.go:241-252` classifies only from `errors.Is(runErr, context.DeadlineExceeded/Canceled)` and wrapper-supplied `state`; wrappers derive state from bare `ctx.Err()` (`:56-62`) or `failedOperation` (`operations.go:619-624`). Four distinct cancel origins funnel identically: user button (`web/operation_handlers.go:195` → `operations.go:345-372`), Ctrl-C (`cmd/ultraplan/main.go:19` NotifyContext), internal control-loop cancel on store errors (`durable_operations.go:190-216`), store-requested cancellation (`:195-201`). Copy 3 (live web): `web/operations.go:284-292` maps only succeeded/cancelled/failed/interrupted — no `timed_out`, no `persistence_degraded` — so a deadline-exceeded operation shows live state `failed` while its durable row says `timed_out` (visible after refresh via `DurableStatus.RefreshPath`). Cancellation origin survives only in side channels (`lifecycle.go:98-108` DB event; hub doc reason `operations.go:361`), never in the `TerminalProposal`.
**Architectural reason:** drift / failure-semantics — same concept (why did this run end) owned three times with different vocabularies and different evidentiary standards; the durable journal, which is the audit authority, records the least-differentiated version for app operations.
**Concrete consequence:** incident forensics cannot distinguish operator-cancelled from store-interrupted operations in the durable record; support exports under-report timeout-class failures for operations; each new terminal nuance must be added in three places with nothing enforcing convergence (already diverged).
**Counter-evidence searched:** richer inputs available to copy 2 (result.State carries only Cancelled/Failed/Partial/Succeeded — no Timeout state exists in the enum path); intentional conflation rationale in docs/comments (none found); whether reconciler disambiguates later (no — `reconcileUnclaimed`/probe decisions are independent copies #4/#5, `lifecycle.go:446-495`). Refines SPECIALIST-07B F02 with full site enumeration.
**Confidence:** high
**Smallest useful action:** Make FinishOperation accept an explicit cause (the call sites already know user vs internal vs store origin at cancel time) and map it once through a shared outcome-classifier used by both copies 1 and 2; let copy 3 project from the durable snapshot instead of re-deriving.

---

**ID: CHALLENGE-05-F04**
**Priority:** P2
**Claim:** `study init`'s git clone is the one external-process boundary that bypasses the platform/process contract: no context, no timeout, unbounded blocking on `CombinedOutput`, reachable from scripts/CI.
**Evidence:** REALITY. `internal/study/init_clone.go:44-50`: `exec.Command("git", "clone", "--depth", "1", url, dest)` + `cmd.CombinedOutput()` — no `CommandContext`, vs `platform/process/process.go:60-152` where timeout/group-kill/cleanup are mandatory (used by sprint smoke, `smoke.go:75,128`). Caller chain: `study_commands.go:1390` → `study.Init` (`init.go:84`) → `runCloneActions` (`init_clone.go:72-78`). Output redaction/truncation present (`init_clone.go:52-65`).
**Architectural reason:** boundary / failure-semantics — an inconsistent subprocess governance model; every other child process inherits bounded cleanup semantics, this one can hang forever.
**Concrete consequence:** unreachable remote, huge repo, or credential/host-key prompt edge cases block `ultraplan study init` indefinitely in non-interactive automation (JSON-output mode implies CI usage is first-class); Ctrl-C mitigates interactive use only.
**Counter-evidence searched:** timeout wrapper anywhere up the chain (none); docs promising a bound (none — `user-guide.md:112-119` documents `--dry-run`/`--no-clone` escape hatches instead); stdin connected enabling prompts (no — partial mitigation); tests pinning timing (fake runner, `init_test.go:70-89`). Corroborates SPECIALIST-21B; re-derived personally.
**Confidence:** high (mechanism), medium (operational frequency)
**Smallest useful action:** Wrap in `exec.CommandContext` with a configurable default timeout (or route through `process.Runner`), preserving existing redaction.

---

**ID: CHALLENGE-05-F05**
**Priority:** P3
**Claim:** The smoke environment builder returns nil when its allowlist yields nothing, and `process.DirectRunner` treats nil env as inherit-parent — so deny-by-default silently inverts to allow-all exactly in the hardened-environment case the allowlist exists for.
**Evidence:** REALITY. `smoke_protocol.go:635-641`: `var env []string` appended only for non-empty getenv hits → nil when all lookups fail (e.g., cleared-env sandbox); default allowlist PATH/HOME/TMPDIR/LANG/LC_ALL (`smoke_types.go:53`) supplies names but values come from the parent environment. `process/process.go:84-86`: `cmd.Env = append([]string(nil), req.Env...)` → nil → Go exec semantics inherit `os.Environ()` wholesale.
**Architectural reason:** failure-semantics at the trust boundary — the API cannot express "empty env" distinctly from "inherit everything", so the safe interpretation is unreachable in the degenerate case.
**Concrete consequence:** a CI runner with a scrubbed environment intending zero env inheritance for smoke commands instead hands them the full parent env (whatever secrets exist there), with no signal; current happy-path behavior is correct, so the defect hides until someone hardens deliberately.
**Counter-evidence searched:** callers always passing non-empty env today (true — smoke builds from settings+manifest, so no live leak now); agentwrap path needing different treatment (separate, see defended section); exec documentation confirming nil-means-inherit (Go os/exec). Narrows SPECIALIST-14A's broader claim to this precise kernel.
**Confidence:** high (mechanism), low-medium (exposure)
**Smallest useful action:** Initialize `env := []string{}` in `smokeEnvironment` (or make DirectRunner set `cmd.Env = req.Env` unconditionally with a documented empty-means-empty contract plus one test).

---

**ID: CHALLENGE-05-F06**
**Priority:** P3
**Claim:** `jsonMarshalTruncated` neither marshals JSON nor truncates; nested structured payload values persist as Go-syntax `%v` strings under a name and comments that promise otherwise, corrupting diagnostic fidelity for the fields that do survive the gate.
**Evidence:** REALITY. `app/run_control.go:570-575` returns `fmt.Sprintf("%v"), nil` unconditionally; comments claim "marshal then truncate to safe limit" and cite an import-cycle concern that does not exist (`encoding/json` is stdlib, already used in `runcontrol/sanitize.go:46`). Consumed at `:560` for `map[string]any/map[string]string/[]any/[]string` payload values — exercised in practice because runtime payloads are decoded JSON objects (`platform/runtime/runtime.go:108,568-592` promotion over `map[string]any`).
**Architectural reason:** drift — the helper's contract lies to maintainers; anyone extending payload encoding will assume JSON output.
**Concrete consequence:** allowlisted fields carrying nested values (e.g., `message` built from a map) store `map[foo:bar]` Go syntax instead of JSON; downstream consumers parsing or diffing durable payloads get misleading artifacts; oversize nested values bypass the promised truncation inside the helper (outer bounds eventually catch them).
**Counter-evidence searched:** consumers depending on `%v` format (none — JS/template treat as text, GENERALIST-04 concurs); whether the gate would reject such strings (it doesn't — plain text passes `unsafeEventValue`). Confirms GENERALIST-03 finding; my additional nondeterminism hypothesis was **rejected** (see below).
**Confidence:** high
**Smallest useful action:** Rename to `goFormatValue` and fix the comments, or implement actual `json.Marshal` + truncate (three lines) — either restores honesty; JSON is preferable since sanitize already assumes encodable shapes.

---

**ID: CHALLENGE-05-F07**
**Priority:** P3
**Claim:** Mid-run persistence-loss handling — the flagship fail-closed invariant — is untested above the repository interface: no test makes `Append` fail mid-run and asserts cancellation + `TerminalPersistenceLost` terminalization for either runtime children or app operations.
**Evidence:** REALITY. App-level fault coverage ends at start-of-run: `run_control_test.go:103` asserts accept-time persistence error blocks child start; `durable_operations_test.go` contains only the happy path and a closed-store dedup assertion (`:11,:65`). The untested wiring: mid-run append failure → `setPersistenceErr` → cancel → `TerminalPersistenceLost` proposal (`run_control.go:197-201,282-292`; `durable_operations.go:165-171,110-113,241-253`). `runcontrol/fault_test.go` covers full/read-only/closed stores only below the Repository interface; `lifecycle_test.go:102-103` touches `TerminalPersistenceLost` only in arbitration.
**Architectural reason:** lifecycle / testing — the composition layer where persistence failure meets run liveness is exactly the seam fault injection doesn't reach; regressions here (e.g., swallowing `setPersistenceErr`, proposing wrong outcome) would pass the whole suite.
**Concrete consequence:** a refactor could silently convert fail-closed persistence loss into continue-on-lost-events (silent journal gaps) without any test failing — the inverse of the property docs advertise.
**Counter-evidence searched:** integration-style tests elsewhere in app (none inject Repository failures); whether runcontrol fault tests transitively cover the wiring (they cannot — they test the repository, not callers). Narrower than initially hypothesized thanks to `run_control_test.go:103`.
**Confidence:** high
**Smallest useful action:** One table-driven test with a failing-after-N-appends Repository fake driving both `controlledRuntime.StartRun` and the durable-operation manager, asserting cancel propagation and proposed outcome.

---

**ID: CHALLENGE-05-F08**
**Priority:** P3
**Claim:** Browser SSE resume mixes two id namespaces on one endpoint — transient hub counters (per-record, from 1) and durable journal sequences — and the reconnect path feeds the raw header between them; consequences are bounded (duplicate delivery or explicit 409 refresh) but the behavior is untested and relies on incidental numeric near-alignment.
**Evidence:** REALITY. Hub ids: `record.nextEventID` per-record counter (`web/operations.go:97,428,447`). Durable ids: `operationEvent{ID: event.Sequence}` (`operation_handlers.go:518`). Reconnect: `handleOperationEvents` parses `Last-Event-ID` then on subscribe failure passes it straight to `followDurableOperationEvents(after)` (`operation_handlers.go:218-235`), which replays journal rows after that number (:464-487) with `cursor_ahead`/`replay_gap` guards (:440-446). Alignment is preserved per-event by design (`publishAppEvent` appends to hub only when the durable append committed, `operations.go:246-255`), but journal-only rows (accept lifecycle, terminal, omission) and hub-only rows (snapshot/started, `operations.go:215,227`) shift the series, so resumed streams duplicate a few events or end in `cursor_ahead`. No test sends `Last-Event-ID` to this endpoint (`sse_test.go` covers frame format only; hub test uses lastID=0).
**Architectural reason:** boundary / testing — one wire contract, two numbering authorities, reconciliation left to generic HTTP status handling.
**Concrete consequence:** after server restart (session regenerated, hub empty) browsers re-receive a small tail of duplicates or must full-refresh via `DurableStatus.RefreshPath`; acceptable UX today, but any future change to which rows are journaled vs published silently changes what clients miss or re-read, undetected.
**Counter-evidence searched:** silent-loss window (none found — gap guard :444-446 catches below-oldest cursors with visible 409); deliberate namespace marker in event payloads (absent). Refines SPECIALIST-12A: mechanism confirmed, severity lower than implied.
**Confidence:** medium-high
**Smallest useful action:** One reconnect test asserting no-loss/no-dup semantics for the restart case; optionally stamp durable-origin events with a `durable:true` field so clients can dedupe without numeric assumptions.

---

**ID: CHALLENGE-05-F09**
**Priority:** P3
**Claim:** `ultraplan health` reports a `filesystem.read` check that executes no read — it is statically hardcoded to ok, and the indirect stat-based validation it sits beside cannot detect unreadable files.
**Evidence:** REALITY. `app/health_commands.go:65`: `checks = append(checks, healthCheck{ID: "filesystem.read", ..., Status: "ok", Message: workspace.MarkerFile})` unconditional; aggregate status ignores it (`:82-85`). `workspace.Validate` probes existence via `os.Stat` only (`workspace/validation.go:14-31`), which succeeds for mode-000 files.
**Architectural reason:** drift / observability honesty — a diagnostics surface asserting a probe that never ran erodes trust in the other seven checks.
**Concrete consequence:** an operator diagnosing EIO/permission incidents (NFS rot, sealed dirs) sees `filesystem.read: ok` from a tool whose whole purpose is triage.
**Counter-evidence searched:** whether discovery/validate subsume readability (stat ≠ read-open; discovery reads some files, so gross failures surface elsewhere — but the labeled check remains fabricated); tests pinning the check (health_commands_test asserts presence, not execution).
**Confidence:** high
**Smallest useful action:** Actually open-and-read the marker file (few lines) and reflect failure in aggregate status.

---

**ID: CHALLENGE-05-F10**
**Priority:** P3
**Claim:** Secret VALUE markers diverge between the durable storage gate and the config-display classifier: tokens like `xoxb-…`/AWS secret keys are caught for config display but not for durable event payloads, so a secret embedded in an allowlisted field value persists everywhere.
**Evidence:** REALITY. Storage-gate value markers: `bearer |sk-|ghp_|github_pat_|-----begin private key` (`runcontrol/sanitize.go:92`); key-fragment list lacks `authorization|cookie|auth` (`sanitize.go:76`). Config display: adds `xoxb-|aws_secret_access_key` and `apikey/api_key/--key` markers over key+value (`config/redaction.go:26-35`). Web projection value markers: inline `token=|secret=|authorization:|cookie:` only (`web/operations.go:639`). Concrete delta: a `message` field containing `xoxb-1234` passes the storage gate (no marker match), passes the web projection (no inline marker), lands in SQLite, browser, and support export.
**Architectural reason:** drift across independently maintained deny-lists — each sink re-decides "what looks like a secret", and value-level sets have no superset owner.
**Concrete consequence:** credential-shaped strings echoed by agents into allowed fields survive all layers; each new credential format must be added in N places with omissions undetectable locally.
**Counter-evidence searched:** whether the field ALLOWLIST already dominates (largely yes for KEY names — only 28 fixed names pass, none credential-like, so the fragmented KEY-lists concern mostly dissolves; the residual risk is precisely VALUES of allowlisted fields, where the divergence is real); whether unify-into-one-classifier is viable (no — see rejected hypotheses). Narrows GENERALIST-02's five-engine claim to the defensible kernel.
**Confidence:** high (divergence), medium (real-world occurrence)
**Smallest useful action:** Extend `unsafeEventValue` value-marker list to include `xoxb-` and `aws_secret_access_key` (superset alignment, one line + test), keeping layers independent.

### Defended architecture / rejected hypotheses

- **"Unify the five redaction engines into one shared classifier" (GENERALIST-02 remedy). Rejected.** `runcontrol` is test-enforced to import only stdlib + sqlite + x/sys (`import_boundary_test.go:12-39`), and each layer filters a different domain (storage keys/values, producer keys, config key+value, projected display text, argv fragments). A shared classifier couples the durable leaf to `platform/config` and adds translation indirection without removing any list. Correct scope is marker-set alignment at the value level (F10) and allowlist ownership clarity (F01).
- **"%v map formatting is nondeterministic, breaking coalescing hashes." My own hypothesis, rejected.** `fmt` prints maps in sorted key order (stable since Go 1.12), so `jsonMarshalTruncated`'s output is deterministic; the defect is fidelity/naming (F06), not stability.
- **"agentwrap inheriting `os.Environ()` leaks secrets into an untrusted process" (SPECIALIST-14A strong form). Defended.** The opencode harness is the component that consumes provider credentials by design; docs direct secrets to the runtime-native environment (`user-guide.md:94`); `WithEnv` is intentionally additive (`opencode.go:21`), and the smoke path — the actually untrusted boundary — is deny-by-default via explicit minimal allowlist (`smoke_protocol.go:623-642`). Residual kernel captured as F05.
- **"Metrics are write-only dead code." Rejected.** Counters surface through `api_health` JSON (`handlers.go:500-508` via `Health.Metrics`, `model.go:408`), `run diagnostics --json` (`run_commands.go:264-267`), and the support export (`run_commands.go:280,311-320`).
- **"Unwired `Notifier` is a defect." Rejected.** Documented as optional best-effort latency optimization (`interfaces.go:41-43`); polling is the designed primary consumption path; no production caller promises push.
- **"Shutdown-time cleanup uncertainty refuses non-product operations — repair gap" (`web_usecases.go:351`). Latent only.** Every current operation mutates a sprint/study target that startup reconciliation sweeps regardless of originating target kind (`server.go:76-81` → `ReconcileOperations`, `web_usecases.go:354-382`); the refusal path becomes a defect only when new operation kinds with non-product side effects appear (future-risk, not reported as current).
- **"Quota measures bytes it cannot govern (WAL coupling)". Defended as conservative-by-design.** Enforcement is synchronous, physical (`retention.go:35-56`), fail-closed with reserved lifecycle writes, and health reports degraded status (`sqlite.go:901-906`); transient WAL spikes causing spurious rejects require ≥~97% occupancy of the default 512 MiB quota. Left as open question, not a finding.
- **Web security posture concerns. Rejected.** Loopback-only enforced pre-bind and post-bind (`serve_commands.go:92-112`, `server.go:167-177`), HttpOnly+SameSite=Strict cookies, host pinning, origin proofs, CSRF header on API mutations with one-time preparation store for form mutations, 64 KiB body cap (`security.go:134-169`); static assets from embed allowlist; artifact refs are opaque HMAC-mapped with symlink+containment checks (`web_usecases.go:892-910`). CSPRNG-failure fallbacks are impractical to trigger and validity doesn't hinge on them (concur with SPECIALIST-21B).

### Open questions

1. **F01 remedy direction:** was the strict allowlist ratified as policy after c455510 (flips remedy from "extend list" to "retract producer/consumer expectations")? Same open question CHALLENGE-02 posed; author answer decides.
2. **F02 root cause:** scheduler regression vs stale expectation in `TestRunLoopStartsPriorityTierBeforeLaterTiers`; bisection across `5839f9a..eeaa034` determines whether this is functional or test debt.
3. **Near-quota WAL transience:** do real workloads observe spurious `CodeQuota` rejects from checkpoint-timing WAL growth? Only observable with production-sized journals; would justify measuring logical bytes instead of file size.
