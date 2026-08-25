# Chair-05 — Quality tribunal synthesis (security, errors, observability, testing, performance, operational claims)

### Scope inspected

**Primary input:** challenge-05 (10 candidate findings F01–F10), re-derived against the target rather than accepted.

**Implementation repo** (`ultraplan-go` @ eeaa034c, clean, read-only). Read personally:
- `internal/runcontrol/sanitize.go` (full file — allowlist, gate, deny-lists), `git log --follow internal/runcontrol/sanitize.go` (untouched since `e09d394`)
- `internal/app/run_control.go` promotCHAIR-05 synthesis written to `.archgauntlet/runs/chair-05/stdout.md` (37 KB). Both target repos untouched (verified clean).

**Outcome:** 11 findings — F01–F10 from challenge-05 adjudicated, F11 added.

- **Confirmed P1** — sanitize-gate/promotion drift: producers promote keys the storage allowlist drops, consumers read exactly those keys, and the pre-sanitization coalescing hash inflates rows while discarding content (re-derived end-to-end personally; 6 prior observers corroborate).
- **Root-caused** — the red test (`TestRunLoopStartsPriorityTierBeforeLaterTiers`): commit `734eb5d` deliberately switched to documented tier-backfill and inverted its sibling test, leaving this one stale. Regression hypothesis rejected.
- **Corrected before promotion** — F03 ("operations copy can't represent timed_out" is wrong; real divergence is persistence-loss asymmetry), F08 (parsed uint64 passed, not raw header; sharpened consequences: spurious 409 / mixed-schema duplication).
- **New single-observer finding** — `internal/productstate` (second schema owner of the shared DB) has zero tests anywhere.
- **Rejected/defended** — unified redaction classifier, agentwrap env inheritance, metrics-dead-code, unwired Notifier, quota WAL coupling, web security posture.

Compensating note recorded for downstream stages: SPECIALIST-20 (test architecture) died without output; this chair's independent sweep (full suite, `-race`, fake/inventory census) fills that gap.
er cadence, hub bounds, poll intervals)
- `study/init_clone.go` call chain vs `platform/process` contract; `smokeEnvironment` nil-env propagation through config `clearListField` to `cmd.Env`

**Prior outputs read for provenance/adjudication:** SPECIALIST-17B, 18A/B, 19A/B, 20A/B (empty — stage died without output), 21A/B; GENERALIST-02, GENERALIST-03; CHAIR-04 (overlapping interface findings); CHALLENGE-05. Workspace contracts consulted: `system/contracts/core/observability.md` (OBS-CORE-001, OBS-PII-001 — no allowlist ratification found).

**Commands:** `go build ./... && go vet ./... && go test ./... -count=1` (one failure, see F02); targeted `-count=3` rerun; `go test -race` on app/web/runcontrol (clean); git log/show on `sanitize.go`, `run_loop*.go`, `734eb5d`.

### Architecture assessment

The quality-bearing core is sound. Dependency boundaries are mechanically enforced by two parser-based import tests (`internal/runcontrol/import_boundary_test.go`, `internal/web/import_boundary_test.go`); `go vet` is clean; `-race` is clean on the three most concurrent packages; runcontrol has genuine fault injection against real SQLite (quota-full via `PRAGMA max_page_count`, closed-store, read-only loss — `fault_test.go:12-129`) with "no stale success" semantics pinned. Sanitization defense-in-depth is real: the storage gate cannot be bypassed upstream because `Append` is the sole write path and applies `sanitizeEventDraft` unconditionally (`sqlite.go:617`). Metrics are wired to three real consumers; the web security posture matches its documented threat model.

Stress concentrates in three patterns, all at seams rather than inside modules:

1. **Policy drift between producer and final authority.** The durable-event payload policy lives in two unsynchronized lists (producer promotion in `app/run_control.go`, storage allowlist in `runcontrol/sanitize.go`), plus value-marker deny-lists that diverge per sink. The two most recent commits (`c455510`, `eeaa034`) changed only producers.
2. **Concepts owned N times with divergent behavior.** Terminal-outcome classification (≥4 sites), supervision failure handling (two engines with different persistence-loss semantics), SSE id allocation (two namespaces on one endpoint).
3. **Testing gaps exactly where composition happens.** Fault coverage ends below the `Repository` interface; nothing injects mid-run store failures into the composition layer, one substantive package (`productstate`) has zero tests, and one stale invariant test keeps main red.

Note for downstream stages: SPECIALIST-20 (test architecture, both twins) produced no report — this chair compensated with an independent sweep; testing-domain coverage rests on challenge-05 F02/F07/F08 plus this synthesis.

### Candidate findings

---

**ID: CHAIR-05-F01**
**Priority:** P1
**Claim:** The durable storage allowlist silently discards exactly the agent-stream detail the two most recent observability commits promote; durable consumers read precisely those doomed keys; zero tests pin survival of any promoted key; and the pre-sanitization coalescing hash turns the mismatch into row inflation. The allowlist is the documented final redaction authority but nothing connects it to producer promotion lists.
**Evidence:** REALITY, re-derived personally. Producers promote `tool,title,detail,text,delta,content,message,action,state,status,native_type,line` plus namespaced `<field>_<sub>` keys (`app/run_control.go:435` promote map; namespaced fallback ~:437-447) and force-add `tool/action/title/detail/text/delta` from nesting (~:479-486). Gate permits none of `title/detail/text/delta/content/output/native_type/line` or any namespaced key (`runcontrol/sanitize.go:10-17`, applied at `sqlite.go:617`), incrementing `"unsafe event detail omitted"` (`sanitize.go:29-45`). Durable web timeline reads exactly those keys: `firstNonEmptyPayload(event.Payload,"text","delta","detail","message","content","title","output")` (`web/run_handlers.go:288`), of which only `message` survives post-gate. `sanitize.go` untouched since initial commit `e09d394` while `c455510`/`eeaa034` touched only producers (verified via `git log --follow`). Coalescing hash computed over draft payload *pre*-sanitization (`run_control.go:177-178`: `hash := payloadHash(draft.Payload)`): events differing only in gate-doomed keys never coalesce yet store identical sanitized rows — inflating durable rows while discarding their distinguishing content, defeating the stated coalescing intent ("text deltas … have distinct payload hashes", `run_control.go:175-176`). The sole app-level sanitize test asserts a hostile `secret` key is dropped (`run_control_test.go:82`), not survival of promoted keys. Namespaced keys from `eeaa034` are structurally unpinnable in a fixed allowlist.
**Architectural reason:** drift / change-surface — adding one observable field requires synchronized edits across `platform/runtime`, `app/run_control.go`, `runcontrol/sanitize.go`, and the web/TUI consumers with no shared contract or integration test; `eeaa034` repeated the exact miss of `c455510`.
**Concrete consequence:** `run show`, `/runs/<id>`, SSE replay, TUI run view, and support exports render message-only detail while omission counters inflate with reason "unsafe event detail omitted" — operators misread healthy runs as redacted, and `OmissionTotal` loses its value as a trust signal for genuine redactions.
**Counter-evidence searched:** alternative unsanitized read path (none — Append is sole write path); live CLI progress compensating (true, `study_commands.go:499-509` reads pre-sanitization memory — confirms this is specifically the durable channel); tests asserting intentional drop (none); workspace contract ratifying strictness (observability.md mandates no secrets (OBS-PII-001) but ratifies no field allowlist). Corroborates SPECIALIST-18B-F01, CHALLENGE-05-F01, SCOUT-05/06, SPECIALIST-07A/B, GENERALIST-04-F01, CHALLENGE-02 F05 — independently re-derived here end-to-end.
**Confidence:** high
**Smallest useful action:** Pick one owner for the decision: extend `allowedEventPayloadFields` with the promoted display keys (plus a rule for namespaced keys) and add one end-to-end test asserting a representative runtime event's keys survive Append→Events→view; or revert producer/consumer expectations if strictness is ratified policy. Either way, move the coalescing hash after sanitization or include only stored keys.

---

**ID: CHAIR-05-F02**
**Priority:** P2
**Claim:** Main is red at the reviewed commit because two study run-loop tests encode contradictory scheduling contracts: `TestRunLoopStartsPriorityTierBeforeLaterTiers` pins pre-`734eb5d` tier-barrier semantics that the implementation deliberately replaced with documented tier-backfill semantics; its sibling was renamed and inverted in the same commit but this one was left stale. Root cause is now established: stale expectation, not scheduler regression.
**Evidence:** REALITY. Failure deterministic 3/3 (`-count=3`): `first starts = [analysis:01-structure analysis:02-runtime], want the priority dimension` (`run_loop_test.go:303`). Commit `734eb5d` ("Keep study run loop at requested parallelism") rewrote the scheduler to slot-refill with backfill, documented in-source: "Dimension order remains a priority, but not a barrier: lower-priority dimensions backfill any workers the earlier tiers cannot occupy" (`run_loop.go:529-531`), renamed `TestRunLoopDoesNotFillParallelSlotsFromLaterPriorityTiers` → `TestRunLoopFillsParallelSlotsFromLaterPriorityTiers` asserting backfill ("want two priority tasks plus one backfill"), and left `TestRunLoopStartsPriorityTierBeforeLaterTiers` asserting both first starts come from the priority dimension. Additional brittleness: it asserts strict ordering of `Started` progress events emitted from concurrently launched goroutines — not schedulable even under intended semantics; its passing sibling asserts a multiset instead.
**Architectural reason:** lifecycle / testing-discipline — a product-visible guarantee ("priority dimensions start first") is asserted only by a test that now contradicts the code's own documentation and its sibling; red main breaks the trust basis of every other "verified by tests" claim in this review.
**Concrete consequence:** CI cannot distinguish future regressions from known-red; contributors must either distrust the suite or learn to ignore failures — both corrode the test discipline the rest of the repo genuinely maintains.
**Counter-evidence searched:** flake hypothesis (refuted: deterministic 3/3); fixture insufficiency (fixture yields ≥2 priority-tier tasks, expectation satisfiable under old semantics); scheduler regression hypothesis (refuted by commit evidence above — supersedes CHALLENGE-05-F02's unresolved regression-vs-stale question).
**Confidence:** high
**Smallest useful action:** Align the test with backfill semantics (multiset-style assertion like its sibling) or delete it as superseded; restore green before further review cycles rely on the suite.

---

**ID: CHAIR-05-F03**
**Priority:** P2
**Claim:** Terminal-outcome classification exists as ≥3 behavioral copies with divergent inputs, vocabularies, and persistence-loss semantics; cancel origin (user vs Ctrl-C vs store-error vs shutdown) collapses to one indistinguishable durable record for app operations; and the live web projection omits `timed_out`/`persistence_degraded` from its terminal set. One correction to the tribunal: the operations copy *does* produce `timed_out` when `runErr` wraps `DeadlineExceeded` — the divergence is elsewhere.
**Evidence:** REALITY, all sites verified. Copy 1 `terminalOutcome` (`app/run_control.go:625-637`) inspects parent ctx, `result.Status`, `result.Error.Category`. Copy 2 `FinishOperation` (`app/durable_operations.go:241-252`) uses only `errors.Is(runErr, context.DeadlineExceeded/Canceled)` plus wrapper state from bare `ctx.Err()` (`:56-62`). Copy 3 hub projection (`web/operations.go:284-292` finish mapping; `terminalOperationState` :614-621 covers succeeded/failed/cancelled/interrupted/cleanup_uncertain only — `timed_out`/`persistence_degraded` treated as non-terminal). Persistence-loss asymmetry (the strongest kernel): runtime path routes mid-run store errors through `setPersistenceErr` → proposes `TerminalPersistenceLost` (`run_control.go:156-163,198,282-291`); the operations control loop cancels silently on identical errors (`durable_operations.go:190-216`), so the journal records `TerminalCancelled/"operation cancelled"` with the root cause unrecoverable (`persistence_degraded` proposed only at start time, `:110-113`). Origin conflation: web button reason `"user_request"` lives only in the ephemeral doc/SSE (`operation_handlers.go:195` → `operations.go:360-369`), never in the terminal row; Ctrl-C, store-requested cancellation, and internal store-error cancels all land identically. `TerminalProposal` carries no structured cause (`runcontrol/model.go:470-474`); neither classifier reads `snapshot.Cancellation.Reason`. Web server shutdown adds a fifth writer recording `cleanup_uncertain` (`operations.go:495-543`).
**Architectural reason:** drift / failure-semantics — "why did this operation end" is owned per-copy with different evidentiary standards; the durable journal (audit authority) records the least differentiated version for app operations, and the live/durable projections can disagree until refresh.
**Concrete consequence:** incident forensics cannot distinguish operator-cancelled from store-interrupted operations in the durable record; a deadline-exceeded operation shows live state `cancelled/failed` while refresh reveals `timed_out`; each new terminal nuance must be added in 3+ places with nothing enforcing convergence (divergence already materialized).
**Counter-evidence searched:** richer enum states available to copy 2 (none — no Timeout in wrapper state enum, but sentinel check compensates, correcting the tribunal's overstatement); reconciler disambiguation later (independent copies #4/#5, `lifecycle.go:446-495`); intentional conflation rationale (none found). Corroborates SPECIALIST-18B-F03, SPECIALIST-07B F02; refines CHALLENGE-05-F03.
**Confidence:** high
**Smallest useful action:** Make FinishOperation accept an explicit cause (call sites know user/internal/store/shutdown origin at cancel time), map through one shared classifier for copies 1-2, let copy 3 project terminality from the durable snapshot instead of a hand-maintained set.

---

**ID: CHAIR-05-F04**
**Priority:** P2
**Claim:** `study init`'s git clone is the one external-process boundary bypassing the platform/process contract: `study.Init` takes no context at all; the clone uses raw `exec.Command` + `CombinedOutput()` with no timeout, group kill, or output cap — reachable unbounded from scripts/CI.
**Evidence:** REALITY, call chain verified hop-by-hop. `study/init_clone.go:44-50` (`exec.Command("git","clone","--depth","1",url,dest)` + `CombinedOutput()`); `Init(req InitRequest)` has no ctx parameter (`init.go:55`), clones sequential at `:84`; production caller passes no runner override (`app/study_commands.go:1390-1397`). Contrast `platform/process/process.go:60-123`: mandatory positive timeout (`:71-73`), stdout/stderr caps (`DefaultStdoutLimit 4 MiB`), setpgid + group TERM/KILL (`process_unix.go:14-35`) — used by sprint smoke (`smoke.go:75,128`) but not here. Display-time truncation/redaction exist (`init_clone.go:52-65`) but full output still buffers in memory.
**Architectural reason:** boundary / failure-semantics — inconsistent subprocess governance for the same operation class (bounded network subprocess); SEC-NET-001-style bounding satisfied everywhere else.
**Concrete consequence:** unreachable remote, huge repo, or credential/host-key prompt edge cases block `ultraplan study init` indefinitely in non-interactive automation (JSON-output mode makes CI first-class); Ctrl-C mitigates interactive use only.
**Counter-evidence searched:** timeout wrapper anywhere up-chain (none); stdin connected enabling prompts (no — reads /dev/null, partial mitigation); docs promising a bound (none — `user-guide.md:112-119` documents `--dry-run`/`--no-clone` escapes instead); timing-pinned tests (fake runner only, `init_test.go:70-89`). Corroborates SPECIALIST-21B-F01, CHALLENGE-05-F04 — two independent observers, falsification failed in both.
**Confidence:** high (mechanism), medium (operational frequency)
**Smallest useful action:** Wrap in `exec.CommandContext` with a configurable default timeout (or route through `process.Runner`), preserving existing redaction.

---

**ID: CHAIR-05-F05**
**Priority:** P3
**Claim:** Deny-by-default smoke env silently inverts to inherit-all when the effective allowlist yields nothing: `smokeEnvironment` returns nil, and `DirectRunner`'s `append([]string(nil), req.Env...)` produces nil `cmd.Env` — which Go os/exec defines as inherit-parent-environment. Reachable deliberately: config validation accepts an empty list and `clearListField("smoke.environment")` sets nil without restoring defaults.
**Evidence:** REALITY. `smoke_protocol.go:623-642`: `var env []string`, appended only for non-empty getenv hits → nil when nothing resolves. `process/process.go:86`: `cmd.Env = append([]string(nil), req.Env...)`. Default five-name list (`smoke_types.go:53`; config default `config.go:174`), but empty passes validation (`config.go:458-461`) and reset sets nil (`config.go:516-517`). No code distinguishes "empty because unmatched" from "empty on purpose"; no test covers empty-allowlist behavior (only `TestDefaultSmokeEnvironmentPreservesInterpreterPath`, `smoke_test.go:108-128`, under defaults). Aggravating: the artifact text claims "Environment: bounded allowlist" unconditionally (`smoke.go:489`).
**Architectural reason:** failure-semantics at the trust boundary — the API cannot express "empty env" distinctly from "inherit everything"; the hardened configuration is exactly the degenerate case that fails open.
**Concrete consequence:** an operator who clears `smoke.environment` intending zero inheritance hands smoke commands the full parent environment (including provider credentials held by the harness parent) with no signal and a misleading artifact claim; today's default-path behavior is correct, so the defect hides until someone hardens deliberately.
**Counter-evidence searched:** callers always passing non-empty env under defaults (true — no live leak now); any consumer treating empty as intentional (none); exec documentation confirming nil-means-inherit (Go os/exec). Confirms the narrowed kernel of SPECIALIST-14A defended by CHALLENGE-05.
**Confidence:** high (mechanism), low-medium (exposure requires deliberate clearing)
**Smallest useful action:** Initialize `env := []string{}` in `smokeEnvironment` (or have DirectRunner set `cmd.Env = req.Env` unconditionally with a documented empty-means-empty contract) plus one empty-allowlist test.

---

**ID: CHAIR-05-F06**
**Priority:** P3
**Claim:** `jsonMarshalTruncated` neither marshals JSON nor truncates; nested structured payload values persist as Go `%v` syntax under a name and comments that promise otherwise, citing an import-cycle concern that does not exist.
**Evidence:** REALITY, verbatim. `app/run_control.go:570-575` returns `fmt.Sprintf("%v", v), nil` unconditionally; comment claims "marshal then truncate to safe limit" and cites a cycle concern — `encoding/json` is imported in eight sibling files in the same package and by `runcontrol/sanitize.go:4`. Sole caller `payloadValueString` handles `map[string]any/map[string]string/[]any/[]string` values (:558-564). Downstream `boundedPayloadValue` does bound size (2048), so the practical defect is format fidelity and naming, not unboundedness.
**Architectural reason:** drift — the helper's contract lies to maintainers; anyone extending payload encoding will assume JSON.
**Concrete consequence:** allowlisted fields carrying nested values store `map[foo:bar]` artifacts; consumers parsing or diffing durable payloads get misleading text. Mitigated: no current consumer parses payload values as JSON (grep-verified across web/app/browser JS — JSON.parse appears only on SSE envelopes), so this is diagnostic-fidelity debt, not breakage.
**Counter-evidence searched:** consumers depending on `%v` format (none); whether the gate rejects such strings (it doesn't — plain text passes `unsafeEventValue`). Confirms GENERALIST-03-F05 / CHALLENGE-05-F06; my nondeterminism sub-hypothesis remains rejected (fmt sorts map keys since Go 1.12).
**Confidence:** high
**Smallest useful action:** Implement actual `json.Marshal` + truncate (three lines, matching sanitize's encodability assumption) or rename to `goFormatValue` and fix comments.

---

**ID: CHAIR-05-F07**
**Priority:** P3
**Claim:** Mid-run persistence-loss handling — the flagship fail-closed invariant — is untested above the repository interface: no test anywhere fails `Append`/`Snapshot`/`Heartbeat` mid-run and asserts cancellation plus correct terminalization for either runtime children or app operations, and no injectable `Repository` error seam exists outside `runcontrol`.
**Evidence:** REALITY, exhaustive inventory. App-level fault coverage ends at start-of-run: `run_control_test.go:87-105` closes the repo *before* StartRun and asserts child-not-started (`:101-104`); `durable_operations_test.go` holds only happy path + closed-store accept dedup (`:11-97`). Zero occurrences of `setPersistenceErr`, `TerminalPersistenceLost`, or quota/store error injection in any `internal/app` or `internal/web` test. `runcontrol/fault_test.go:12-129` injects below the interface (PRAGMA max_page_count/query_only, Close) against the repository itself; `lifecycle_test.go:103-111` tests storage of all seven outcomes, not callers' detection wiring. No fake implements `runcontrol.Repository` outside runcontrol (web wraps a real SQLite repo; tui fakes are happy-path use-case doubles). The untested wiring: OnEvent append failure → `persistenceErr` → cancel → `TerminalPersistenceLost` proposal (`run_control.go:197-201,282-292`) and the operations-loop counterpart that silently degrades to `cancelled` (see F03).
**Architectural reason:** lifecycle / testing — the composition seam where persistence failure meets run liveness is exactly what fault injection below the interface cannot reach; regressions here (swallowing `setPersistenceErr`, proposing wrong outcome) would pass the whole suite.
**Concrete consequence:** a refactor could silently convert fail-closed persistence loss into continue-on-lost-events (silent journal gaps) — the inverse of the property docs advertise — with no failing test.
**Counter-evidence searched:** integration tests injecting Repository failures (none in app/web); transitive coverage via fault_test (impossible — different layer); lifecycle arbitration tests covering proposal provenance (arbitration yes, detection wiring no). Confirms CHALLENGE-05-F07 with fuller inventory; corroborated by independent sweep.
**Confidence:** high
**Smallest useful action:** One table-driven test with a fail-after-N-appends `Repository` wrapper driving both `controlledRuntime.StartRun` and the durable-operation manager, asserting cancel propagation and proposed outcome.

---

**ID: CHAIR-05-F08**
**Priority:** P3
**Claim:** Browser SSE resume mixes two event-id namespaces on one endpoint — per-record hub counters and durable journal sequences — with reconciliation left to generic HTTP status handling; consequences refined: a fully caught-up browser after restart typically receives a spurious `409 cursor_ahead` (hub ids run ahead of journal sequences), mid-stream browsers receive silent partial duplication rendered in a *different payload schema* (nested `{stage,task,payload}` vs flat hub keys), and mixing can occur live without restart via capacity/session subscribe fallback. The reconnect path is untested.
**Evidence:** REALITY, mechanism verified with two corrections to the tribunal. Hub ids from per-record counter starting at 1 (`operations.go:97,426-434`, first event `snapshot` at :215); durable events carry `ID: event.Sequence` (`operation_handlers.go:518`). Correction 1: the handler passes the *parsed* `uint64` to the durable fallback, not the raw header; malformed headers 400 out first (`operation_handlers.go:218-235`, `parseEventID` `operations.go:658-667`). Correction 2: journal rows for a web op are lifecycle(1)+progress/omission/cancellation+terminal — `accepted`/`claimed` mappings at `operation_handlers.go:504` are dead code (never written), so namespace drift is structural, not incidental. Hub-first gating confirmed: `publishAppEvent` records durably before hub append (`operations.go:245-255`), with coalesced events dropped from *both* projections. Guards: `cursor_ahead`/`replay_gap` at `operation_handlers.go:440-447`. Tests: no test sends `Last-Event-ID` to the operation events endpoint (`sse_test.go` covers frame format only; hub tests use lastID=0); the sole header test targets the runs endpoint.
**Architectural reason:** boundary / testing — one wire contract served by two numbering authorities and two payload schemas; nothing marks which namespace produced a frame.
**Concrete consequence:** acceptable UX today (browser refresh recovers), but any change to which rows are journaled vs published silently changes what clients miss, re-read, or mis-parse; API consumers polling the fallback endpoint see an undocumented schema switch mid-stream.
**Counter-evidence searched:** silent-loss window (none — gap guard catches below-oldest cursors); deliberate namespace marker (absent); severity inflation (rejected — bounded duplicates + visible 409). Refines SPECIALIST-12A and CHALLENGE-05-F08 with corrected mechanics.
**Confidence:** medium-high
**Smallest useful action:** One reconnect test asserting restart semantics (expect the 409-or-duplicate behavior explicitly); stamp durable-origin frames with `durable:true` so clients can dedupe without numeric assumptions.

---

**ID: CHAIR-05-F09**
**Priority:** P3
**Claim:** `ultraplan health` reports a `filesystem.read` check statically hardcoded to ok; aggregate status ignores it; and the only stat-based validation beside it cannot detect unreadable files — a diagnostics surface asserting a probe that never ran.
**Evidence:** REALITY. `health_commands.go:65`: unconditional `Status:"ok"` entry; grep finds no other reference to `filesystem.read` in the tree; aggregation considers only workspace validity, config error, runtime failure (`:82-85`). `workspace.Validate` probes via `os.Stat` only (`validation.go:14-28`) — mode-000 files pass. The only genuine read during health is config load, surfaced under `config.validation`, not `filesystem.read`.
**Architectural reason:** drift / observability honesty — fabricated checks erode trust in the other seven and misdirect incident triage (EIO/NFS rot/permission cases report healthy).
**Concrete consequence:** operators diagnosing unreadable-file incidents see `filesystem.read: ok` from the tool whose purpose is triage.
**Counter-evidence searched:** discovery/validate subsuming readability (stat ≠ read-open; gross failures surface under other checks but the labeled check remains false); tests pinning execution (health_commands_test asserts presence only). Confirms CHALLENGE-05-F09 firsthand.
**Confidence:** high
**Smallest useful action:** Actually open-and-read the marker file (few lines) and reflect failure in aggregate status.

---

**ID: CHAIR-05-F10**
**Priority:** P3
**Claim:** Secret VALUE markers diverge across sinks with concrete escape paths: `xoxb-…` and `aws_secret_access_key=…` are caught only by the config-display classifier — they pass the durable storage gate, land in SQLite/browser/support export, and the web timeline renders them client-side; Basic-scheme credential strings escape every sink including config display.
**Evidence:** REALITY, four-sink comparison. Storage gate value markers: `bearer |sk-|ghp_|github_pat_|-----begin private key` (`sanitize.go:92`). Config display adds `xoxb-|aws_secret_access_key` over key+value (`config/redaction.go:26-31`) — sole occurrence of both. Web inline markers: `token=|secret=|authorization:|cookie:` (`web/operations.go:636-649`). Producer key fragments add `authorization|cookie|auth` but inspect keys only (`run_control.go:501-509`). Free-form allowlisted fields (`message`, `reason`, quasi-free `tool/action/code`) can carry arbitrary values ≤2048 bytes to disk; `newRunEventView` renders `payload.message` to the browser without `safeProjectedText` screening on that path. Delta matrix verified: `"xoxb-1234"` in `message` → stored + displayed; `"Bearer x"` → dropped at storage; `"Basic dXNlcjpwYXNz"` → stored, displayed, passes config too. (Provenance fix: allowlist is 27 fixed keys, not 28.)
**Architectural reason:** drift across independently maintained deny-lists — value-level marker sets have no superset owner; each sink re-decides "what looks like a secret".
**Concrete consequence:** credential-shaped strings echoed by agents into allowed free-form fields survive all layers into durable storage and browser display; each new credential format must be added in N places with omissions undetectable locally.
**Counter-evidence searched:** field allowlist domination (largely true for KEY names — dissolves the broader five-engine claim to this value-level kernel, concurring with the tribunal's narrowing of GENERALIST-02-F05); unify-into-one-classifier viability (no — see rejected hypotheses); upstream truncation limiting exposure (bounds size, not content). Confirms CHALLENGE-05-F10 with sharpened deltas.
**Confidence:** high (divergence), medium (real-world occurrence)
**Smallest useful action:** Extend `unsafeEventValue` value markers with `xoxb-`/`aws_secret_access_key` (superset alignment, one line + test); consider a basic-auth pattern; keep layers independent.

---

**ID: CHAIR-05-F11** *(single-observer — new, from this chair's sweep)*
**Priority:** P3
**Claim:** `internal/productstate` — the second schema owner sharing `.ultraplan/run-control.db`, authoritative store for DB-mode sprint/study state — has zero tests: no test file in the package and no reference from any test in the repo.
**Evidence:** REALITY. `ls internal/productstate` → `store.go` only (206 lines); `grep -rln 'productstate\.' --include='*_test.go' internal/` → empty. Meanwhile runcontrol's half of the same file has 42 test functions including dedicated fault tests, and GENERALIST-02-F02/GENERALIST-03-F02 document cross-module coupling (shared file, quota accounting spanning product tables, divergent DSN flags, lockless schema setup).
**Architectural reason:** testing / ownership asymmetry — one physical store with two owners of radically different verification rigor; the owner with *fewer* guarantees (no migrate lock, no ping/integrity checks) is also the untested one.
**Concrete consequence:** schema or write-behavior changes to product-state rows (which feed quota accounting and restore semantics) ship with no regression signal; combined with the shared-file coupling this is where silent cross-module breakage would land first.
**Counter-evidence searched:** indirect exercise via sprint/study DB-mode flows (possible in principle, but no test file references the package, so any coverage is incidental and unpinned); thin-wrapper argument rejected (206 lines with own DSN/cache/schema DDL). Single strong observer (this chair's sweep + personal verification); no second independent observer exists because SPECIALIST-20 died — hence single-observer status, not dismissal.
**Confidence:** high (facts), medium (weight)
**Smallest useful action:** One round-trip test per public store method (save/load/list/delete + schema creation on fresh file) mirroring `openTestRepository` conventions from runcontrol.

### Defended architecture / rejected hypotheses

- **"Unify the five redaction engines into one shared classifier" (GENERALIST-02 remedy). Rejected again after re-examination.** `runcontrol` is test-enforced to import only stdlib + sqlite driver + x/sys (`import_boundary_test.go:33`), and each sink filters a different domain (storage keys/values, producer keys, config key+value, projected display text, argv fragments). A shared classifier couples the durable leaf to `platform/config` and removes no list. Correct scope is value-marker superset alignment (F10) plus allowlist ownership clarity (F01).
- **"Scheduler regression explains the red test." Rejected with root cause.** Commit `734eb5d` deliberately introduced tier-backfill (documented `run_loop.go:529-531`) and updated its sibling test to assert the new semantics; only the stale twin was left behind (F02). Supersedes the open regression-vs-stale question in CHALLENGE-05-F02.
- **"The app-operation copy cannot represent `timed_out`." Corrected.** It does when `runErr` wraps `context.DeadlineExceeded` (`durable_operations.go:244-245`); the real divergences are inputs inspected, reason vocabulary, persistence-loss handling, and the web terminal-set omission (F03).
- **"Raw Last-Event-ID header is fed across namespaces." Corrected.** Header is parsed to uint64 with malformed→400 (`operation_handlers.go:218-226`); the defect is semantic namespace mixing, not raw passthrough (F08).
- **"agentwrap inheriting `os.Environ()` leaks secrets into an untrusted process." Defended (residual kernel = F05).** The opencode harness consumes provider credentials by design; secrets are directed to the runtime-native environment (`user-guide.md:94`); `WithEnv` intentionally additive; the actually untrusted smoke boundary is deny-by-default with explicit allowlist.
- **"Metrics are write-only dead code." Rejected.** Verified exposure via `/api_health` (`handlers.go:500-508` → `RunHealth`), `run diagnostics --json` Health payload (`run_commands.go:264-267`), and support export (`run_commands.go:280,308-322`).
- **"Unwired `Notifier` is a defect." Rejected.** Documented optional best-effort optimization (`interfaces.go:41-45`); adaptive polling (250ms–1s, `run_handlers.go:512-516`) is the designed primary path.
- **"Quota measures bytes it cannot govern (WAL/product-table coupling)." Accepted debt, conservative-by-design.** Enforcement synchronous/physical/fail-closed with reserved lifecycle writes; health reports degraded status; spurious rejects need ≥~97% occupancy of the 512 MiB default. Cost per append is bounded (~1 ReadDir + ~3 stats + one COUNT/SUM aggregate — measured code path, `sqlite.go:617-622`, `retention.go:35-62`); fine for local-tool scale, retained as open question under production-sized journals.
- **Web security posture concerns. Rejected (endorse SPECIALIST-21B + CHALLENGE-05).** Loopback-only pre/post-bind, HttpOnly+SameSite=Strict session cookie, host pinning, origin proofs incl. port-stripped tolerance requiring `Sec-Fetch-Site` + exact Referer, CSRF proofs, 64 KiB body cap, opaque HMAC artifact refs with symlink+containment checks; trust-model honesty documented (`local-web.md:84-113`); CSPRNG-fallback rejection impractical.
- **Import-boundary tests exempt `_test.go` files** (`web:18-19`, `runcontrol:19-20`): noted; below finding threshold (test-only imports cannot ship), recorded for the evolution chair.

### Open questions

1. **F01 remedy direction:** was the strict allowlist ratified as policy after `c455510` (flips remedy from "extend allowlist" to "retract producer/consumer expectations")? Neither repo docs nor workspace `observability.md` ratifies either side; author answer decides. Same open question CHALLENGE-02 posed.
2. **Coalesced-progress invisibility:** `publishAppEvent` drops coalesced events from *both* hub and browser (`operations.go:246-255` + `durable_operations.go:140-147` returning `committed=false`). Intended UX (suppress duplicate spam) or oversight? Affects F01/F08 severity assessments.
3. **SSE semaphore coupling:** every SSE stream pins one of 32 global request slots (`MaxInFlight=32`, acquired per request at `security.go:121-127`) for up to 30 min, equal to `MaxConcurrentStreams=32` — 32 concurrent streams would starve ordinary requests. Only observable with many simultaneously-followed runs; needs a capacity decision, not evidence of current harm.
4. **Reconciler scaling:** each active operation runs its own 1 s tick with 10 s reconcile gates and no cross-goroutine singleflight (`durable_operations.go:180,210-216`; `run_control.go:214,249-258`) — bounded by MaxActiveOperations=8 plus CLI runs today; contention profile with production-sized journals unknown.
5. **productstate testlessness (F11):** intentional (thin wrapper awaiting daemon-events redesign) or gap? If a redesign will replace the package, the smallest action is documenting that instead of writing tests against soon-to-change schema.
