I have the roadmap mapped. Now let me verify repo state and read the remaining authoritative docs.
Commits match the assignment and both trees are clean. The workspace roadmap shows Phases 1–5 delivered and Phase 6 (Sprint 35 durable run control) as current work — and the implementation repo has an `internal/runcontrol` package plus recent observability commits. Let me delegate bounded discovery to subagents in parallel.
The subagents returned substantial evidence. Before classifying, I need to re-derive the highest-stakes claims myself: (1) the productstate/SQLite move vs the roadmap's Phase 6 fence, (2) the Sprint 35 planning-chain gap, and (3) the TRD stage-key contradiction vs config reality.
Timeline confirmed: implementation raced ahead of the workspace planning chain. Now verifying the productstate/SQLite question directly against the roadmap fence.
SQLite is indeed authoritative post-migration, with JSON demoted to terminal-stage checkpoints. Now checking whether Sprint 35's own requirements authorize this, plus the TRD stage-key contradiction against real config validation.
Sprint 35's own non-goals prohibit replacing canonical JSON flow state — yet the implementation made SQLite authoritative. One counter-evidence source remains: the impl-repo experiment plan mentions `storage migrate`. Checking its context, plus the real config stage-key set.
The experiment plan itself gates SQLite product persistence behind "Phase D" with an explicit storage-mode config and contract-suite requirement — none of which exists in the code. Now checking config stage keys and doing a build/test sanity pass.
All verification passes are complete. Writing the cartography report.

---

# SCOUT-03 — Product-contract cartography

### Scope inspected

**Authoritative planning workspace** (`ultraplan-workspace` @ `368a789`, clean):
- `projects/ultraplan-go/roadmap.md` (full, 1564 lines), `project-index.md` (full), `docs/PRD.md` v1.7.0, `docs/TRD.md` v1.8.0, `docs/ARCHITECTURE.md` (via subagent, spot-verified)
- `projects/ultraplan-go/sprints/35-durable-run-observability/{requirements.md,flow-state.json}` (read directly); flow-states for sprints 30/32/33/34 (via subagent)
- `system/contracts/` inventory, `studies/` inventory (`go-cli-study`, `agent-harness-study`, `ultraplan-daemon-events-study`), `projects/aren-phase-01-execution-lifecycle/` presence

**Implementation repository** (`ultraplan-go` @ `eeaa034`, clean):
- Full package tree; deep-dive `internal/runcontrol/*` (27 files) and `internal/productstate/store.go`
- Boundary verification: `internal/web/import_boundary_test.go`, `internal/runcontrol/import_boundary_test.go` (both executed: PASS), imports of `web/tui/app/cmd/platform-runtime` (via subagent, spot-verified)
- Storage-authority trace: `internal/sprint/state.go:200-237`, `state_database.go`, `execute_state.go`, `internal/study/state_database.go`, `internal/app/storage_commands.go`
- Docs: `docs/architecture.md`, `docs/migration-product-state.md`, `docs/plans/integrated-roadmap.md`, `docs/plans/ultraplan-local-server-experiment-plan.md` §D
- Commands: `go build -o /tmp/opencode/ultraplan-scout-build ./cmd/ultraplan` (OK), targeted boundary tests (PASS), git log timeline reconstruction

**Subagents used:** 5 independent bounded investigations (runcontrol/productstate map; boundary claims; PRD/TRD/ARCHITECTURE summary; impl-docs vs authority; Sprint 35 status mapping). All conclusions re-derived above from returned evidence plus my own reads.

---

### Architecture assessment

**Evidence classification**

| Class | Content |
|---|---|
| REALITY | Impl @ `eeaa034`: 12 packages incl. new `internal/runcontrol` (~5.5k lines, SQLite, leases/fencing/reconciliation, introduced whole in `e09d394` 2026-08-21 17:26) and `internal/productstate` (`02e2ec4` 2026-08-21 17:57). Build passes; both import-boundary tests pass. |
| CURRENT-CONTRACT | Workspace roadmap Phases 1–6 committed scope (Sprint 35 = current committed work, not completed); PRD v1.7.0 / TRD v1.8.0 (both dated 2026-08-20); `ARCHITECTURE.md`; `project-index.md`; `system/contracts/*`; sprint-35 `requirements.md`. |
| FUTURE-INTENT | Roadmap Gates A–E (content identity, QA/repair, retrieval, persistence/SQLite, graph/cloud/Aren); `docs/plans/` Sprints 36–39 / Phases 10–15 incl. experiment-plan "Phase D" SQLite product persistence; `aren-phase-01` project; `ultraplan-daemon-events-study` (added 2026-08-22 11:31, evidently to feed Sprint 35 reasoning). |
| HISTORY | `flow-state.legacy-v1.json` files; `scripts/migrate-legacy-sprint-state.sh`; `docs/migration-from-ultra-cli.md`; `docs/phase3-migration.md`; manual `review.md`/`deep-smoke.md` superseded by generated artifacts; TRD duplicated §8–§10 block (older domain model). |

**What is sound.** For Phases 1–5 the three layers (roadmap ↔ index ↔ reality) align well. Enforced boundaries match contracts: `internal/web` non-test code imports only stdlib + `internal/app` (verified by AST test); bubbletea/glamour/lipgloss appear nowhere outside `internal/tui`; `cmd/ultraplan/main.go:21-42` does explicit closure composition with no global runner registry; `internal/platform/runtime` imports no product packages; CLI/TUI/Web converge on the same typed app use cases (spot-checked review/verify and study run-loop operations). Delivered sprints 30/32/33 have complete governed chains through review+smoke. The new `runcontrol` module is internally coherent, dependency-minimal (stdlib + sqlite + x/sys, test-enforced), and covers most Sprint 35 requirement areas (identity, fencing, reconciliation matrix, redaction gate, retention/quota).

**What is stressed.** All stress concentrates at the Phase 6 frontier, where REALITY (committed 2026-08-21/22) has outrun the governance chain (last workspace planning snapshot for Sprint 35: 2026-08-20T12:55Z, requirements-only). Three seams follow, plus two latent contract-layer inconsistencies that become load-bearing exactly now, since downstream gauntlet stages must adjudicate Sprint 35 against these documents.

---

### Candidate findings

#### SCOUT-03-F01
- **ID:** SCOUT-03-F01
- **Priority:** P1
- **Claim:** Implementation made SQLite authoritative for mutable *product* state (sprint flow outcomes, execute tasks, study run state) without the storage-mode gate that every contract layer requires for that move, contradicting the explicit Phase 6 fence and realizing gated FUTURE-INTENT early.
- **Evidence:**
  - CURRENT-CONTRACT: WS `roadmap.md:1368` — "Sprint 35 … must not move Markdown, **flow outcomes**, Git/source state, or smoke evidence into a new canonical store"; `TRD.md:57` (SQLite product persistence before its gate is out of scope), `TRD.md:126`, `TRD.md:2235` ("Sprint 35 operational run storage does not select a new authored-artifact authority"); `ARCHITECTURE.md:716` ("Persistence selection occurs once at composition… No silent dual writes"); sprint-35 `requirements.md:42,72`.
  - REALITY: IMPL `02e2ec4`; `internal/sprint/state_database.go:13-14` (kinds `sprint_flow`, `sprint_execute`), `internal/study/state_database.go` (kind `study_run`) all stored in `.ultraplan/run-control.db`; `internal/sprint/state.go:216-227` — once migrated, saves go to SQLite and the JSON file is written only when **all** stages are terminal (`flowStateCheckpoint`, state.go:230-237); `docs/migration-product-state.md:24` — "After import, SQLite is authoritative for that record."
  - FUTURE-INTENT pulled forward: IMPL `docs/plans/ultraplan-local-server-experiment-plan.md:699-763` defines this exact capability as gated Phase D requiring an explicit `storage.mode` config (D4), per-mode contract suites (D3), and validate-before-switch importer semantics (D5) — none present; WS roadmap Gate D (`roadmap.md:1462-1469`) sequences it after durable-run dogfood.
- **Architectural reason:** authority + drift. Mid-flow, `flow-state.json` on disk is stale by design; SQLite is the only current truth, while every contract document names the JSON file canonical. Authority also becomes per-record mixed (migrated records in DB, others file-backed) with no composition-time mode selection.
- **Concrete consequence:** Any consumer that reads governed artifacts as files — humans, external smoke-harness checks ("existing artifact and flow-state behavior remains compatible and authoritative", `requirements.md:114`), backup/sync tooling, or the workspace-side review protocol itself — observes stale intermediate stage states after migration; and the Gate D comparison ("Compare filesystem and SQLite modes on real work") is pre-decided before its evidence existed.
- **Counter-evidence searched:** Searched both repos for any post-hoc acceptance of this move: workspace HEAD (today 11:31) still contains the fence; no `storage.mode` config exists; no reasoning/decision artifact mentions `productstate`. Timeline check: fence entered the workspace at `2dc3b74` (2026-08-21 09:15), ~8.5 h before `02e2ec4` (17:57). Found no justification doc. The strongest available defense is `docs/migration-product-state.md` itself, which documents intent but carries no contract authority.
- **Confidence:** high (facts); the normative judgment (defect vs unrecorded accepted amendment) belongs to later stages.
- **Smallest useful action:** Either restore file-canonical reads/writes for product state until Gate D, or record an explicit contract amendment (roadmap/TRD version bump) that legalizes DB authority with the Phase D safeguards (mode config, suites, switch-on-validation).

#### SCOUT-03-F02
- **Priority:** P1
- **Claim:** Sprint 35's central design decisions (storage representation, topology, lease/fencing model) were decided in committed code before the sprint's own required reasoning/plan artifacts exist, breaking the workspace's decision-record authority chain.
- **Evidence:** WS `sprints/35.../flow-state.json` (updatedAt 2026-08-20T12:55Z): `requirements` complete; `code-context` ready; `sprint-index`, `technical-handbook`, `area-reasoning`, `reasoning`, `plan` all `missing`. IMPL `e09d394` (2026-08-21 17:26) lands the full mechanism: SQLite choice (`sqlite.go:22`), multi-process direct-access topology (`sqlite.go:41-42`), 1s/5s/15s lease constants (`model.go:476-484`), birth-token fencing (`process_linux.go:16-45`) — answers to open questions 3, 4, 7 of `requirements.md:53-57`. Contract: `requirements.md:66` — "Decisions that materially affect public API compatibility, authority, topology, security, or recoverability **must be recorded in reasoning before implementation planning**"; WS `roadmap.md:1398`.
- **Architectural reason:** authority + lifecycle. The workspace chain is the designated owner of design decisions; code became the decision record de facto, with no artifact against which later review can check the chosen trade-offs.
- **Concrete consequence:** Sprint 35 review (`requirements.md:104-116`) cannot verify "the chosen model" against recorded reasoning; later re-litigation or rollback has no baseline; downstream sprints inherit undocumented invariants.
- **Counter-evidence searched:** Looked for reasoning artifacts anywhere: impl repo has no reasoning dirs for sprint 35; workspace flow-state lists them missing; searched impl `docs/plans/` for topology/storage decision text — the experiment plan discusses options but predates and doesn't select. No counter-evidence found. Note: choosing SQLite *for operational run records* is itself allowed (`requirements.md:20` defers mechanism to reasoning); the defect is the missing record, not the choice.
- **Confidence:** high.
- **Smallest useful action:** Materialize the sprint-35 reasoning/plan artifacts retroactively from the implemented decisions (with explicit "decided during implementation" provenance), or halt further Sprint 35 scope until the chain catches up.

#### SCOUT-03-F03
- **Priority:** P2
- **Claim:** `project-index.md` catalog lags workspace content: two studies (including final reports) are absent from "Available Studies"/"Available Evidence Reports", and the smoke-harness manifest status cell is stale — a latent failure for the next governed selection.
- **Evidence:** WS `project-index.md:97-101` lists only `go-cli-study`; workspace contains `studies/agent-harness-study/reports/final/*` (many final reports) and `studies/ultraplan-daemon-events-study/` (added in HEAD commit `368a789`). Selection enforcement is real: IMPL `internal/sprint/validation.go:30,44` fails selections not present in the project index. Also `project-index.md:135` still says versioned harness manifest "planned in Sprint 27", which flow-state shows completed (verdict pass).
- **Architectural reason:** drift + change-surface. The index is the subset-validation authority; uncataloged evidence makes the upcoming sprint-index stage fail or pressure an unplanned index edit mid-planning.
- **Concrete consequence:** When Sprint 35 planning materializes (its evident purpose — the daemon/events study maps onto open questions 4–8), `sprint ... flow --to sprint-index` fails validation until someone updates the catalog, coupling a mechanical fix into a design stage.
- **Counter-evidence searched:** Checked whether studies are discovered dynamically rather than cataloged: study *runtime* discovery is dynamic, but sprint-index *selection* validation is strictly catalog-bound (validation.go above). No alternate path found.
- **Confidence:** high.
- **Smallest useful action:** Add the two studies (and their report rows) to `project-index.md` before materializing Sprint 35's sprint-index; refresh the Sprint 27 manifest cell.

#### SCOUT-03-F04
- **Priority:** P3
- **Claim:** TRD v1.8.0 contains internal contradictions that weaken it as adjudication authority for Sprint 35 review.
- **Evidence:** WS `docs/TRD.md`: §8.1–§10.1 duplicated verbatim (first ~lines 453-778, second mis-leveled inside section 10, ending identically "…defined in section 18."); conflicting stage-model key lists — §6.1 includes `code_context` but not `review`; §18.7 includes `review` but not `code-context`; two incompatible `Project` structs (§8.5 vs §18.3) and `Sprint` structs (§8.6 vs §18.4). Reality check: IMPL config validation accepts the superset (`planning.code_context_model` … `planning.review_model` … `planning.smoke_model`, `internal/platform/config/config.go` env-field table), so §18.7 is the stale section; no runtime defect.
- **Architectural reason:** drift (contract layer).
- **Concrete consequence:** Reviewers and the automated review stage citing TRD sections can pick either side of a contradiction; the duplicated domain models invite citing superseded shapes.
- **Counter-evidence searched:** Verified against code that reality is a consistent superset, so this is documentation debt only.
- **Confidence:** high (existence), low impact.
- **Smallest useful action:** One editorial TRD pass: delete the duplicated §8–10 copy, reconcile §18.7 with §6.1, mark §8.x domain structs as historical.

#### SCOUT-03-F05
- **Priority:** P3
- **Claim:** `ARCHITECTURE.md` is unversioned and self-contradictory in its Core Rule.
- **Evidence:** No version/status/changelog header (contrast PRD/TRD). "Core Rule → Avoid this as the default shape" lists `internal/study` while the same section's Prefer-list, Target Layout, and Module Ownership mandate `internal/study` (WS `docs/ARCHITECTURE.md`, Core Rule section).
- **Architectural reason:** drift (authority pinning).
- **Concrete consequence:** Findings in this gauntlet cannot cite a pinned architecture revision; the avoid-list contradiction can be quoted against the mandated layout.
- **Counter-evidence searched:** Confirmed via subagent read of the full document; the contradiction is textual, likely an editing leftover.
- **Confidence:** high.
- **Smallest useful action:** Remove `internal/study` from the avoid-list; add version/date metadata matching PRD/TRD convention.

---

### Defended architecture / rejected hypotheses

1. **"Impl-repo `docs/architecture.md` duplicates the authoritative workspace ARCHITECTURE.md, violating `project-index.md:17`."** Rejected: zero heading overlap; the impl doc is descriptive of shipped code (real paths, concrete parameters), claims no document authority, and workspace roadmap itself designates `docs/plans/` as mandatory design inputs (`roadmap.md:1235-1250`). Different document kinds, not mirrors.
2. **"`internal/web` importing `internal/sprint`/`internal/runcontrol` breaks the web boundary."** Rejected for production code: non-test web files import only stdlib + `internal/app` (verified per-file); the two product imports exist solely in `_test.go` fakes/fixtures, and the enforcing test (`import_boundary_test.go:18-20`) deliberately exempts tests — the exemption is codified local invariant, not a gap.
3. **"`platform/runtime` leaks study semantics."** Rejected as architectural issue: no product imports; only unused metadata-key constants named in study vocabulary (`policy.go:7-10`, referenced nowhere) — dead naming, not a dependency-direction violation.
4. **"`runcontrol` chose SQLite, which the roadmap forbids."** Wrong reading: the roadmap refuses to *pre-commit* SQLite for operational run records but allows reasoning-selected SQLite (`roadmap.md:1398`, `requirements.md:20,53`). The actual violations are separated into F01 (authored product state) and F02 (unrecorded decision), not the runcontrol storage choice itself.
5. **"Deep cohesive `runcontrol` + `productstate` sharing one DB file is a layering smell."** Investigated and set aside: they share only the database *file* path, have disjoint schemas (`runs/attempts/events` vs `product_states/product_state_items`), disjoint ownership statements (`runcontrol/doc.go:1-6`), and both are boundary-test-fenced. File-sharing is a deliberate single-workspace-store choice; whether wise is moot until F01's authority question is settled.

---

### Open questions

1. Does any accepted decision record for the Sprint 35 design choices exist outside both repositories (e.g., a branch, an out-of-band approval)? If yes, F02 downgrades to a synchronization defect.
2. Was the productstate authority move consciously framed as an amendment to Gate D sequencing somewhere invisible to both repos? If yes, F01 becomes a doc-sync finding instead of a fence breach.
3. Is the JSON-checkpoint-only-at-terminal-stages write policy (`state.go:230-237`) documented as intentional compatibility behavior anywhere authoritative? I found no such statement; if it exists, F01's consequence narrows considerably.
4. Sprint 35's dependencies cite "Sprint 34 shared-context real-runtime dogfood, which supplied the cross-surface operational evidence" (`requirements.md:96`), but sprint 34's smoke flow-state records verdict **blocked** with a null `lastComplete.runId` (no harness run evidence). Did real dogfood evidence exist outside the harness? This affects how much weight the Phase 6 motivation narrative ("real use exposed…") can carry in review.
