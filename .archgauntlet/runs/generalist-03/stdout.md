I'll orchestrate this broad review. First, let me build my own foundation: read the authoritative docs, map the package graph, then delegate bounded investigations to subagents.
Dependency graph mapped. Now reading authoritative planning docs before delegating.
Now the workflow and persistence contracts:
Contracts absorbed. Now delegating bounded discovery to four independent scouts/workers in parallel while I plan my own verification reads.
First scout returned strong evidence. Dispatching the remaining three investigations in parallel now.
Empty result — retrying that investigation, split into two parallel tasks to be safe.
Empty again — splitting that scope and switching agent types.
Two scopes remain. Dispatching both in parallel with the agent type that works.
All investigations returned. Now I verify the load-bearing claims myself before forming findings.
Recovery runbook gives strong counter-evidence for several candidates. Now verifying the remaining load-bearing claims in source:
The event-callback threading question hinges on whether `Adapter.StartRun` drains the events goroutine before returning, especially on cancellation. Reading the remainder:
Two real defects surfaced here. Verifying the blast radius across other OnEvent consumers and main.go exit behavior:
Evidence base solid. Now the adversarial falsification pass on my top candidates before reporting:
That agent type is failing — rerunning falsification with the reliable type:
F01 survives with a precise correction. Second falsification round for the storage-ownership and lock candidates:
All falsification rounds complete. Compiling the final report.

### Scope inspected

**Authoritative planning workspace** (`ultraplan-workspace` @ 368a789): `projects/ultraplan-go/docs/ARCHITECTURE.md`, `docs/PRD.md` structure + sprint catalogue (35 sprints), `system/contracts/core/architecture.md`, `system/contracts/runtime/workflows.md`, `system/contracts/runtime/persistence-and-migrations.md` (index), `system/reasoning/*` inventory.

**Implementation repo** (`ultraplan-go` @ eeaa034, dirty=false):
- Package/import graph via `go list -f` over all packages; `go vet ./...` clean.
- Read directly: `internal/platform/runtime/runtime.go` (Adapter.StartRun, events goroutine), `internal/sprint/{verification_lock.go, execute_state.go, execute.go (task loop), review.go (resume checkpoints), service.go (Status/mutations), verify.go}`, `internal/study/{state.go, run_loop.go (scheduler/persist/session checkpoint)}`, `internal/web/server.go`, `internal/app/{run_control.go (event payload path), storage_commands.go}`, `cmd/ultraplan/main.go`.
- Delegated bounded investigations (evidence returned, conclusions re-derived by me): `internal/runcontrol` full subsystem; `internal/web` operation hub/SSE/shutdown; `internal/app` durable-operation wiring; `internal/study` run-loop/history/productstate usage; `internal/productstate/store.go`; sprint execute/review/smoke durability; agentwrap v0.0.0-20260821190033 library source (module cache).
- Docs read as CURRENT-CONTRACT counter-evidence: `docs/recovery.md`, `README.md`, CLI help text in command files.
- Read-only throughout; target repos unmodified.

### Architecture assessment

The module-driven topology is real and mostly disciplined: dependency direction matches the authoritative ARCHITECTURE.md (`web/tui -> app -> modules`; platform packages import no product code — enforced by `runcontrol/import_boundary_test.go:12-38`). Durable-write discipline is consistently high (temp+fsync+rename+dir-sync in study `state.go:73-119`, sprint `execute_state.go:132-182`, smoke `atomicWriteFile`). runcontrol itself is unusually rigorous: single-tx acceptance, CAS-guarded transitions with `RowsAffected` checks, fence-verified owner writes, immutable event journal with PK `(run_id,sequence)` and immutability trigger, birth-token PID-reuse protection, conservative never-infer-success reconciliation, tested clock-jump safety. Web SSE isolation (drop+disconnect slow subscribers, tested) and browser-disconnect≠cancellation (dedicated `operationRoot`) match contract exactly.

Stress concentrates at four seams: (1) the platform/runtime event-callback lifecycle contract, which no composition honors; (2) one SQLite file hosting two logical stores with disjoint operational rigor (runcontrol vs productstate); (3) mutation leases built on O_EXCL+PID without identity recheck at reclaim; (4) `sprint status` acting as an unsynchronized writer alongside the lease it coexists with. Each is a lifecycle/authority problem, not a layering problem.

### Candidate findings

---

**ID: GENERALIST-03-F01**
**Priority: P1**
**Claim:** `platform/runtime.Adapter.StartRun` provides no OnEvent-quiescence guarantee, and sprint execute mutates/persistently writes shared execute state from the event-callback goroutine without mutual exclusion against the main loop — a data race with lost-update potential precisely during cancellation-racing-completion.

**Evidence:**
- `internal/platform/runtime/runtime.go:308-321`: if `run.Wait(ctx)` exceeds 5s after ctx cancellation, StartRun returns immediately, abandoning the live events goroutine; `runtime.go:328-340`: success path waits ≤1s for the events channel, else abandons it.
- agentwrap `opencode/runtime.go` (module cache, pinned `go.mod:8`): `Wait()` returns on `done` close while up to 32 buffered events remain undelivered; defers run LIFO (`cancel()` → close(done) → close(events)); `Cancel()` waits only 100ms; producer finalization (proc wait, stderr drain, 2s cleanup, 5s DB reconcile) can outlive cancellation by seconds.
- `internal/sprint/execute.go:244-259`: OnEvent closure mutates `task.Runtime`, `task.UpdatedAt`, `state.UpdatedAt` and calls `SaveExecuteRunState` under `sessionMu`; the main loop mutates the same fields and saves (`execute.go:266-306`) taking `sessionMu` only to read `checkpointErr` — not during its own writes.
- `internal/sprint/service.go:21-36`: Service has no mutex guarding `SaveExecuteRunState`/`SaveFlowState`; `startSprintRuntime` (`runtime_metrics.go:115-121`) adds none.
- No test exercises OnEvent-after-return; all sprint fakes fire OnEvent synchronously inside StartRun (`execute_state_test.go:35-37`, `review_test.go:55-57`); `runtime_test.go:472-488` fake pre-closes its events channel.

**Architectural reason:** lifecycle / failure-semantics — the volatile-boundary seam (platform/runtime) exports an implicit concurrency contract ("no callbacks after return") that its implementation does not provide and consumers assume.

**Concrete consequence:** Operator cancels an execute whose opencode run takes >5s to finalize (realistic per cleanup timings above): the abandoned goroutine keeps firing OnEvent saves while the main loop marks the task cancelled and saves terminal state. Interleaved whole-file renames of `.run-state.json` lose one writer's update (e.g., terminal diagnostic overwritten by a stale session checkpoint, or vice versa); concurrent field mutation is a schedule-dependent data race (`go test -race` blind here). Corrupted resume state then feeds `review` fingerprints and `verify` gates.

**Counter-evidence searched:** agentwrap source (Wait/Cancel/close semantics — confirms gap); sprint Service fields (no serializer); test suite (no coverage of overlap); review.go initially suspected identically but **refuted** — its flow-state RMW is funneled to a single collector goroutine with late-event regression guards (`review.go:504-551, 792-803`), proving the team knows late delivery happens and defends it elsewhere; study run-loop serializes via versioned `persist`/`saveMu` (`run_loop.go:139-175, 282-291`) and is safe. Only the sprint-execute path is exposed, which is where the risk lives.

**Confidence:** high (mechanism structurally verified through library source; no mitigating synchronization found).

**Smallest useful action:** Extend `sessionMu` in `execute.go` to cover all main-loop task mutations and saves (making callback/main-loop access mutually exclusive), and state the OnEvent postcondition explicitly in `Adapter.StartRun` docs; optionally bound-drain eventsCh unconditionally after Wait returns since channels close LIFO.

---

**ID: GENERALIST-03-F02**
**Priority: P2**
**Claim:** `.ultraplan/run-control.db` is one physical SQLite file owned by two packages with disjoint operational rigor; run control's quota, backup/restore, and schema governance silently span product-state data that productstate owns, and study-side migration/reset flows treat the two authorities inconsistently.

**Evidence:**
- Same path constant: `productstate/store.go:19` == `runcontrol/sqlite.go:22` (`.ultraplan/run-control.db`); separate `sql.DB` pools, separate option sets; productstate skips ping/pragma/integrity verification (`store.go:67-90` vs `runcontrol/sqlite.go:207-227`).
- Quota: `runcontrol/retention.go:35-56` counts every `run-control.db*` byte (including WAL frames carrying product rows); gates at `sqlite.go:384-395` (soft: reject Accept), `lifecycle.go:24-28` (hard: heartbeat fails → owners must stop); `productstate.Save` (`store.go:150-206`) has no quota gate and compaction (`retention.go:97-160`) never reclaims product tables.
- Schema: productstate sets no `user_version`/schema row (`store.go:92-116`); runcontrol treats any pre-existing non-sqlite schema as legacy → wholesale backup + stamp v1 (`migration.go:58-73, 167-173`).
- Reset: `study_commands.go:266-305` gates the destructive-reset confirmation on `os.Stat(RunStatePath)` only; `run_loop.go:454-491` archives only the JSON file — DB-authoritative progress is discarded unprompted, though help text promises "archive and rebuild."
- Silent authority flip: `LoadRunState` falls back to possibly-stale JSON whenever the DB record is absent (`study/state.go:27-36`); restore/delete of the shared file flips authority invisibly.

**Architectural reason:** authority / ownership — execution facts and product state share a store without a single owner of the file's lifecycle policies.

**Concrete consequence:** A workspace with many migrated sprints/studies accumulates product rows until run-control soft quota blocks new run acceptance and hard quota kills heartbeats of *live* operations — with no compaction path and no doc connecting the cause (`recovery.md:211-215` advises freeing space "outside the active database," which cannot help here). An offline restore following `recovery.md:217-224` rewinds flow/execute/study records with no warning; `RestoreBackup` (`migration.go:296-352`) swaps the whole file and even leaves stale `-wal/-shm` siblings.

**Counter-evidence searched:** `retention_test.go:100-128` proves prefix-wide counting is intentional for foreign files — but no test/doc acknowledges productstate data living inside that count; `storage migrate` help says files "remain as checkpoints" (documents mirroring, not the quota/restore coupling); `RestoreBackup` is referenced only by `migration_test.go:137` (manual procedure, downgrading reachability of the rewind sub-claim); productstate-first creation requires a crash window between empty-file creation and migration stamping (`sqlite.go:171-181`) — narrowed trigger.

**Confidence:** high on mechanism; medium on operational frequency.

**Smallest useful action:** Give productstate either its own database file or first-class citizenship in runcontrol's quota/backup/schema regime (exempt product tables from `storageBytes`, include them in restore warnings); fix the reset confirmation/archive to consult `RunStateInDatabase`.

---

**ID: GENERALIST-03-F03**
**Priority: P2**
**Claim:** Stale-lock reclaim in both mutation-lease implementations has a remove/create TOCTOU: two processes reclaiming a dead holder's lock simultaneously end up both holding it.

**Evidence:**
- `internal/sprint/verification_lock.go:49-58`: liveness via `syscall.Kill(pid,0)` only (no birth token, unlike `runcontrol/migration.go:140-165`), then unconditional `os.Remove(path)` before O_EXCL retry. Fatal interleave: A removes stale L0 and creates L_A; B (read L0 earlier) removes L_A and creates L_B — both proceed.
- `internal/study/locks.go:59-86`: identical pattern; second `EEXIST` fails safely only if ordering cooperates.
- `ForceUnlockRunLoop` (`study/locks.go:161-167`) removes with no liveness check, widening exposure.
- Release-side identity checks (`verification_lock.go:89-91`) detect theft only after concurrent mutation already occurred.
- No concurrent-reclaim coverage in `locks_test.go` (both packages).

**Architectural reason:** failure-semantics — the lease is the sole serialization point for governed sprint/study mutation, so its acquire protocol must be race-free, not merely usually-race-free.

**Concrete consequence:** After a crash, supervisor auto-restart or scripted parallel launches of two `run-loop`s (or two sprint mutations) interleave last-writer-wins atomic renames of run-state/flow-state — duplicate agent spend, lost stage transitions, divergent artifacts — with conflict surfaced only at release time as an error nobody is waiting on.

**Counter-evidence searched:** flock/inode-comparison alternatives anywhere in repo (none); PID-reuse direction (false-alive is safe-direction, noted); tests covering simultaneous reclaim (absent); probability honestly narrow (dead-holder lock + microsecond-scale interleave), but the system's own recovery docs direct operators to relaunch after crashes, i.e., the precondition is the documented recovery path itself.

**Confidence:** high on mechanism, medium on practical incidence.

**Smallest useful action:** Before `os.Remove`, stat-and-compare the lock file inode against the one just parsed (retry O_EXCL loop on mismatch); or record process birth identity in the lock and re-verify post-remove, matching the rigor runcontrol already applies to its migration lock.

---

**ID: GENERALIST-03-F04**
**Priority: P2**
**Claim:** CLI/TUI `sprint status` persists `flow-state.json` (and DB copy) derived from its own load-time snapshot without taking the mutation lease, so it can clobber state written by an execute/review operation that legitimately holds the lease.

**Evidence:**
- `internal/sprint/service.go:64` (`statusWrites: true` default), `service.go:191-195` (unconditional save in Status), Status never touches `acquireMutation`/lock inspection.
- Save merges prior Review/Smoke only when nil (`state.go:201-228`) using copies captured at Status's own earlier load — review-resume checkpoints, ActiveAttempt, and stage transitions committed mid-operation are overwritten; DB-authoritative mode loses identically (`productstate/store.go:160-178` hash-guard prevents only identical writes).
- Web disables it via `WithoutStatusWrites()` (`web_usecases.go:245-252`, `usecases.go:121-130`); TUI (`tui_commands.go:37-41`) and CLI (`sprint_commands.go:81`) do not.
- `docs/recovery.md:51, 64-66, 72` actively instruct running `sprint status` during/after interrupted operations; no doc warns about running it *during* one.

**Architectural reason:** authority / boundary — a presentation-refresh write shares write authority with the lease designed to serialize mutations, without coordination.

**Concrete consequence:** During a long review with per-reviewer resume checkpoints, an operator (or script following the runbook) runs `sprint status`; its save reverts coverage checkpoints/session continuity written since its load — reviewers re-run (duplicate provider spend) or resume validation sees inconsistent attempt metadata and forces a fresh review.

**Counter-evidence searched:** Intentionality is documented everywhere except the race itself (`cli-reference.md:255`, `stage-skills.md:131`); the read-only variant exists and is used only by web, demonstrating the hazard is known in principle; no CAS, lease probe, or mtime check guards Status saves.

**Confidence:** high.

**Smallest useful action:** Make Status's persist conditional (skip when a valid lease exists for the sprint, or re-load-and-merge immediately before rename), or route CLI/TUI status through `WithoutStatusWrites` plus an explicit `--refresh-state` flag.

---

**ID: GENERALIST-03-F05**
**Priority: P3**
**Claim:** Structured runtime-event payload values in the durable run-control journal are serialized with `fmt.Sprintf("%v")` despite being labeled compact encodings, so map/slice payloads are stored as Go syntax, not JSON.

**Evidence:** `internal/app/run_control.go:570-575`: `jsonMarshalTruncated` returns `fmt.Sprintf("%v", v), nil` (comment even admits avoiding `encoding/json`); consumed at `:560` for `map[string]any…[]string` payload values feeding runcontrol events (sanitized downstream by `runcontrol/sanitize.go`, which correctly assumes encodable values).

**Architectural reason:** drift — the journal is the system's audit/replay authority (`recovery.md:186-204` directs operators to `run follow`), and this path quietly degrades its machine-readability.

**Concrete consequence:** Replay/diagnostics consumers parsing payload values as JSON fail or mis-render (e.g., `map[tool:bash]`), undermining the exact evidence trail the recovery runbook relies on.

**Counter-evidence searched:** Whether any consumer depends on the `%v` format (none found); whether runcontrol rejects such values (it sanitizes but doesn't validate JSON-ness); frequency (structured payloads occur whenever runtime events carry nested maps — common for opencode session/tool events).

**Confidence:** high.

**Smallest useful action:** Use `encoding/json.Marshal` with size truncation (the package already imports it elsewhere).

---

**ID: GENERALIST-03-F06**
**Priority: P3**
**Claim:** Study run history (`tasks.jsonl`) is appended without fsync and read strictly, so a torn final line wedges run-loop startup and completion until manual file surgery.

**Evidence:** Append path `study/run_history.go:94-107` (`O_APPEND`, single Write, no `file.Sync()`, no dir sync — unlike run-state `state.go:101,117`); reader `run_history.go:198-217` errors on any malformed line; `SyncRunHistory` failures abort RunLoop start/end (`run_loop.go:70-76, 389-391`). Contrast: diagnostics JSONL reader tolerates malformed lines (`run_loop_diagnostics.go:385-390`) and is size-capped; tasks.jsonl is unbounded.

**Architectural reason:** failure-semantics — an auxiliary evidence log can block resumption of the durable workflow it merely observes.

**Concrete consequence:** Power loss mid-study leaves a partial final line; every subsequent `run-loop` invocation fails at sync/read until an operator edits the JSONL by hand — a recovery path `docs/recovery.md` never mentions.

**Counter-evidence searched:** Dedupe-on-append design shows resume tolerance was a design goal; fail-closed on history IO errors is defensible for scheduling decisions (`recordErr(recordHistory(id))`) but startup-wedge from a torn tail is stricter than any documented stance.

**Confidence:** medium-high (mechanism certain; OS-crash-only trigger).

**Smallest useful action:** Tolerate a malformed *final* line (truncate-with-warning) while keeping strictness for interior lines, or fsync appends.

---

**ID: GENERALIST-03-F07**
**Priority: P3**
**Claim:** The web server's abnormal-exit path (Serve error) skips the entire drain/persist-shutdown sequence that the ctx-cancellation path performs, leaving workers unsupervised while the process exits.

**Evidence:** `internal/web/server.go:125-130`: `serveErr` branch returns without `hub.drainAndWait` or cleanup-uncertain persistence (only deferred `cancelOperations()` at `:84` fires); compare the full sequence at `:131-153`. `main.go:21` exits immediately on return. Contract (`ARCHITECTURE.md`, web section): shutdown must "persist truthful terminal or uncertain outcomes."

**Architectural reason:** lifecycle — asymmetric termination semantics for the same worker set.

**Concrete consequence:** Listener failure under load cancels workers but persists nothing: durable runs sit `running` until lease expiry/probe reconciliation on next run-control use; sprint/study running markers await the next web startup reconcile or manual resume; `.cleanup-uncertain.json` is never written, so nothing flags the uncertainty the graceful path would have recorded.

**Counter-evidence searched:** Dead-PID lock auto-reclaim and reconcile-on-open (`app/run_control.go:64`), reconcile-at-web-startup (`web_usecases.go:354-382`), and resume's own stale-running conversion (`execute.go:195-199`) substantially heal the aftermath; hence P3, not P2.

**Confidence:** high on mechanism, low-medium on impact.

**Smallest useful action:** Route the serveErr branch through the same `drainAndWait(shutdownCtx)` + uncertainty-persistence path with a short timeout.

---

### Defended architecture / rejected hypotheses

1. **Review flow-state race via OnEvent (part of F01's original claim) — refuted.** Session events funnel through a buffered channel to a single collector goroutine; `updateReviewResume` RMWs run serialized there, with an explicit late-event regression guard (`review.go:792-803`). The surviving review-side exposure is narrower (abandoned goroutine send-on-closed-channel panic, `lastSessionID` races) and folded into F01.
2. **"Accept→Claim orphan is a defect."** Documented intentional: 45s grace then `interrupted`/`owner_never_claimed_after_grace`, explicitly refusing to invent attempts (`recovery.md:206-209`; `lifecycle.go:427-479`).
3. **"Fencing generations frozen at 1 = dead machinery."** Defended: fencing still fences zombie writers of the same attempt (`verifyFence` applied at heartbeat/append/terminal); takeover-after-death deliberately routes through new-run retry, not attempt reuse — consistent with retry=new-run semantics.
4. **SSE slow-subscriber backpressure blocking operations** — refuted by construction and test (`operations.go:455-465` non-blocking drop+disconnect; `operations_test.go:249-267`).
5. **Browser disconnect cancelling work** — refuted: workers derive from `operationRoot`, request contexts never reach `record.cancel` (`server.go:83,106`).
6. **Cancellation-vs-terminal arbitration ambiguity** — refuted: CAS predicates + immediate txs make completion win cleanly; tested including multi-process races (`lifecycle_test.go:80-174`).
7. **JSON↔DB divergence corrupting review fingerprints in normal flows** — largely refuted: DB-mirror-to-file occurs exactly at all-terminal checkpoint (`execute_state.go:105-130`), which is also the gate `requireCompleteExecute` (`verify.go:98-133`) enforces before review; raw-file readers in review see post-checkpoint content. Residual divergence windows are covered under F02's reset/fallback items.
8. **Study run-loop internal concurrency** — refuted as a defect: versioned clone-under-lock `persist` with debounced saves (`run_loop.go:139-175`) is a clean in-process design.

### Open questions

1. agentwrap producer behavior when its 32-slot event buffer fills (block vs drop) would sharpen F01's success-path likelihood; the pinned module's producer goroutine appeared unbounded-send but needs confirmation.
2. Is the productstate-first DB-creation crash window (empty file created before runcontrol stamps `user_version=1`) reachable through any current entrypoint, or only via out-of-band file creation? Determines F02(c)'s severity.
3. Should `RestoreBackup` ever be wired into product commands, its product-state rewind becomes a P1 data-loss path; today it is a documented manual procedure only — confirming intended permanence would settle F02(b).
