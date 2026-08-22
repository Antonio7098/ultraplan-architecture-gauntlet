Both repos verified at expected commits, clean trees. Starting broad recon of both targets.
Workspace has clear governance structure (`system/contracts`, `system/protocols`, `system/reasoning`, `projects/ultraplan-go`, `studies`). Let me read the core governance docs myself before delegating bounded discovery.
Governance model is clear: contracts with requirement IDs → contract-mapping feeds sprint planning → implementation → architecture-review protocol. Now reading the keystone architecture contract myself.
Keystone contract read. I notice two potential broken references in `architecture.md` (line 15 → `README.md`, line 285 → `release-and-versioning.md`) to verify. Now dispatching parallel bounded-discovery workers.
All four discovery workers returned. Now I'll personally verify the load-bearing claims: the embedded-vs-workspace authority question, boundary-enforcing tests, the implementation-side `docs/architecture.md`, and the possible orphaning of `contract-mapping-protocol.md`.
`rg` isn't available in shell — using the Grep tool instead. Checking how workspaces are initialized and whether the governance corpus is embedded or workspace-only authority.
Key discovery: `init` scaffolds only config/README/studies — the entire governance corpus (contracts/protocols/reasoning templates) exists only in the workspace, and nothing references `contract-mapping-protocol.md` by name. Verifying these authority questions further, plus the boundary-enforcing tests.
`contract-mapping-protocol.md` is referenced by nothing — its role is fulfilled by per-template "Contract Mapping" sections instead. Checking whether any protocol references it, verifying the web boundary test, and how the review runtime sources protocol text.
The runtime recognizes `.ultra/system/...` paths (`internal/sprint/reasoning.go:184`), but this workspace has `system/` at root with no `.ultra` directory. Checking how the workspace root and system catalogs are actually resolved.
`.ultra/` prefix is explicitly normalized legacy (prompts.go:233, reasoning_defaults.go:48) — resolved as intentional boundary translation. Now verifying the project-index contract catalog wiring, the frontend.md dangling reference, and the untested-productstate claim.
Two important leads: (1) project-index points at `/home/antonioborgerees/...` absolute paths while the gauntlet repos live under `/home/antonio/gauntlet-run/`; (2) the Active Contract Pool lists only 13 of 19 contracts — the web-surface contracts (api-contracts, frontend, accessibility) are absent despite Phase 4 being complete. Verifying both.
Confirmed: sprint 30 selected only pool-listed contracts (no API/Frontend/A11y contracts exist in the pool), and the stale absolute paths don't resolve in this environment. Checking whether the roadmap ever intended to extend the contract pool for Phase 4, and cross-checking the two architecture documents for authority conflicts.
Roadmap demands accessibility gates for Phase 4 but never extends the contract pool — the catalog is a hard gate (`internal/sprint/index.go:129` rejects unlisted entries), so API/Frontend/A11y contracts could never be selected. Now checking the two architecture documents for authority overlap.
Architecture docs are consistent across workspace and repo (same dependency graph). One last verification: `productstate` and `runcontrol` were both described as owning `.ultraplan/run-control.db` — checking for storage-authority overlap, plus corpus change history to classify the pool lag.
Both `productstate` and `runcontrol` declare the same DB path — need to see how schema ownership is divided. Also running the workspace git history separately.
Contracts corpus unchanged since initial commit; project-index updated through sprint 35 era but pool not extended. Now checking how `runcontrol` and `productstate` divide schema authority inside the same SQLite file.
All verification threads are closed. Compiling the cartography report.

---

# SCOUT-04 — Governance cartography report

### Scope inspected

**Planning workspace `/home/antonio/gauntlet-run/ultraplan-workspace` @ 368a789** (clean):
- `system/contracts/**` — all 19 contract files read/inventoried (9 core, 5 runtime, 5 surfaces; 166 requirement IDs)
- `system/protocols/` — all 6 protocols (`architecture-review-protocol.md` 472 L, `contract-mapping-protocol.md`, `plan-sprint-protocol.md`, `review-sprint-protocol.md`, `deep-smoke-sprint-protocol.md`, `sprint-review-protocol.md`)
- `system/reasoning/` — all 18 templates (structure + contract-mapping tables)
- `projects/ultraplan-go/` — `project-index.md` (read in full), `roadmap.md`, `docs/{PRD,TRD,ARCHITECTURE}.md`, sampled sprints 01/23/30/34/35 incl. `sprint-index.md`, `review.md`, `execute.md`, `flow-state.json`
- `projects/aren-phase-01-execution-lifecycle/`, `studies/{go-cli-study, agent-harness-study, ultraplan-daemon-events-study}`, `previews/`, `.agents/skills/` (10 skills)

**Implementation repo `/home/antonio/gauntlet-run/ultraplan-go` @ eeaa034** (clean):
- Import graph across `cmd/` + all 12 internal packages; composition root `cmd/ultraplan/main.go` → `internal/app`
- Architecture-encoding tests read directly: `internal/web/import_boundary_test.go`, `internal/runcontrol/import_boundary_test.go`, `internal/app/run_control_inventory_test.go`, `internal/web/api_compatibility_test.go` (via worker, spot-verified)
- Storage authority: `internal/productstate/store.go`, `internal/runcontrol/sqlite.go`, `migration_test.go`
- `docs/architecture.md` (full read), `docs/web-compatibility-baseline.md`, `internal/workspace/init.go` (embedded defaults), scaffold prompts

Commands: `git log/status` both repos, targeted greps for cross-references, link resolution checks.

### Architecture assessment

The governance system is a three-tier authority chain, and — unusually — the tiers are **machine-enforced, not just conventional**:

1. **Corpus tier** (`system/contracts|protocols|reasoning`): 19 contracts with disjoint requirement-ID namespaces (ARCH/ERR/OBS/TEST/SEC/CFG/DATA/REL/DOC/WF/PERF/PERSIST/LLM/EVAL/A11Y/API/CLI/FE/PKG — no duplicates, programmatically checked), each ending in Review Rejection Criteria. Deferral is a first-class concept ("explicit justified deferral" in testing.md:120,152; llm.md:307,315; performance.md:57).
2. **Catalog tier** (`projects/ultraplan-go/project-index.md`): declares which contracts/templates/protocols/studies are *selectable*. Sprint indexes must select from it; `internal/sprint/review.go:321` resolves every selected contract against `SectionActiveContractPool` and fails unknown entries; `internal/sprint/index.go:125-129` requires non-empty selections. The catalog is therefore a hard gate, not documentation.
3. **Sprint tier**: per-sprint artifact chain (requirements → sprint-index → technical-handbook → area-reasoning → reasoning → plan → execute → review → smoke) driven by materialized skills whose pipeline order is derivable from declared prerequisites. Reviews map conformance and deferrals to concrete requirement IDs (e.g., `sprints/11-…/review.md:80` deferral ledger keyed by ERR-RETRY-001 etc.).

The implementation repo mirrors this with **executable architecture**: import-boundary tests parse their own package's imports (`web` allows only app+stdlib; `runcontrol` allows only stdlib+sqlite+x/sys), a source-layout test pins every runtime-backed CLI entry behind the durable acceptance call (`run_control_inventory_test.go`), API/route/template-hierarchy compatibility snapshots are paired with a prose baseline doc declaring "snapshot regeneration alone is not approval". Composition is dependency-inverted (`tui→app`, `web→app`; runners injected as closures, `main.go:27-41`). Platform never imports product packages (verified full import graph; no cycles).

Sound: module ownership statements live in `doc.go` files next to the code; lifecycle invariants are heavily tested (terminal arbitration, fencing, fail-closed persistence); history is handled explicitly (`.ultra/` prefix normalization, schema-v0 compat shims, `sprint-review-protocol.md` alias). Stressed: the corpus↔catalog wiring (F01), environment portability of catalog paths (F02), and superseded-document hygiene (F03/F04).

### Candidate findings

---

**SCOUT-04-F01**
- **Priority:** P2
- **Claim:** Only 13 of 19 contracts are wired into the sole enforcement gate; the six unwired ones include exactly the surface contracts governing the Phase 4 web UI, so completed web sprints passed review without ever being checked against them.
- **Evidence:** `project-index.md:31-45` (Active Contract Pool = 13; missing `api-contracts.md`, `frontend.md`, `accessibility.md`, `privacy-and-data.md`, `release-versioning.md`, `package-public-api.md`); `internal/sprint/review.go:321` resolves selections against the pool; `sprints/30-web-foundations/sprint-index.md:23` states "All paths must appear in the project index's 'Active Contract Pool' table" and selects only the 9 pool contracts. Meanwhile `roadmap.md:1179,1204,1207` set accessibility acceptance criteria for Phase 4, and `sprints/32-hardening-and-release/review.md:68` records the interactive browser accessibility audit as deferred/blocked.
- **Architectural reason:** drift / authority — the corpus evolved to 19 contracts (frozen since initial commit `94fa263`) while the catalog gained Phase 4 reasoning templates (`project-index.md:128-129`) but never Phase 4 contracts.
- **Concrete consequence:** review verdicts of `pass`/`pass_with_findings` on sprints 30–34 assert conformance only against contracts that were selectable; API-* / FE-* / A11Y-* requirements (166-ID corpus) are structurally unenforceable for this project until the pool is edited. A future audit reading "review passed against selected contracts" will overestimate coverage.
- **Counter-evidence searched:** Roadmap-level phase gates do state a11y/security outcomes (partial mitigation at a different layer); Security/Testing selections overlap some API concerns; privacy/release/package contracts may be genuinely N/A for a local single-user tool; project-index.md:29 warns pool membership ≠ blocking. None of these restore enforceability of the three surface contracts for a shipped browser surface.
- **Confidence:** high (wiring mechanics verified end-to-end); medium on materiality.
- **Smallest useful action:** add the Phase-4-relevant contracts (`api-contracts.md`, `frontend.md`, `accessibility.md`) to the Active Contract Pool before Sprint 35's planning chain selects context; record the other three as intentionally out-of-pool with a reason row.

---

**SCOUT-04-F02**
- **Priority:** P2
- **Claim:** The authoritative catalog pins machine-specific absolute paths that do not exist in the current environment, and the skill contract prefers them over the correct relative path.
- **Evidence:** `project-index.md:10` `Target Implementation Directory: /home/antonioborgerees/coding/ultraplan/ultraplan-go` and `:135` smoke harness root — `/home/antonioborgerees` does not exist (verified); the actual sibling repo resolves correctly via `Repository: ../ultraplan-go/` (`project-index.md:9`). Skill instruction (`skills.go:400`, mirrored in materialized SKILL.md): resolve repository from *Target Implementation Directory*, falling back to *Repository* "only when the target field is absent".
- **Architectural reason:** boundary / failure-semantics — governance inputs embed environment identity instead of workspace-relative identity; the documented fallback triggers on field absence, not resolution failure.
- **Concrete consequence:** execute/review/smoke agents in this environment attempt a nonexistent root first; smoke evidence resolution against the harness path fails; behavior then depends on undocumented agent improvisation rather than the governed rule — exactly what the fingerprinted-input regime tries to eliminate.
- **Counter-evidence searched:** Relative `../ultraplan-go/` works, so human-driven sessions likely self-correct; this may be plain HISTORY (workspace authored elsewhere, e.g., Aren project-index shows the same pattern). But nothing marks the paths as stale, and the skill text gives no unresolvable-path rule.
- **Confidence:** high (existence), medium (consequence).
- **Smallest useful action:** change the two absolute paths to workspace-relative ones (or add an explicit "if target path does not resolve, fall back to Repository" clause to the skill contract).

---

**SCOUT-04-F03**
- **Priority:** P3
- **Claim:** Two of six protocols are orphaned predecessors with no supersession marker: nothing in either repository references `contract-mapping-protocol.md` or `plan-sprint-protocol.md`, and their roles are now performed by other mechanisms.
- **Evidence:** grep for "contract-mapping"/"Contract Mapping" across the workspace hits only the protocol file itself plus 14 reasoning templates' own "Contract Mapping" sections; zero hits in ultraplan-go. `plan-sprint-protocol.md:3-9` speaks of "ultraplan-go-workspace/..." paths and pre-Go phrasing; its function lives in `internal/workspace/scaffold/prompts/plan-sprint.md` + the `ultraplan-plan` skill. Contrast: `sprint-review-protocol.md:1-15` is a properly marked compatibility alias pointing to its successor, and `project-index.md:141-147` wires only the other three protocols.
- **Architectural reason:** drift — the protocol directory mixes current authority, marked aliases, and unmaintained dead documents without distinguishing them.
- **Concrete consequence:** an agent or contributor consulting `system/protocols/` (the natural place) can adopt a workflow no stage consumes — e.g., producing the contract-mapping document that no skill reads, or planning per the old manual protocol instead of the governed flow.
- **Counter-evidence searched:** Checked whether reviews/roadmap/skills reference them indirectly — none do; checked whether the CLI embeds them — it embeds prompts/templates/skills, not protocols. Their "Ultra workflow" lineage marks them HISTORY, but they are stored unmarked among CURRENT-CONTRACT documents.
- **Confidence:** high.
- **Smallest useful action:** add one-line status headers ("superseded by X") to both files, mirroring `sprint-review-protocol.md`.

---

**SCOUT-04-F04**
- **Priority:** P3
- **Claim:** The keystone contract contains broken references, including a pointer to a corpus index that does not exist.
- **Evidence:** `core/architecture.md:15` "specialized contracts listed in [README.md](README.md)" — no README exists anywhere under `system/contracts/` (verified by listing; the promised corpus index is absent, so the corpus has no canonical top-level inventory); `core/architecture.md:286` links `../core/release-and-versioning.md`, actual file is `release-versioning.md`; `surfaces/frontend.md:14` references `ops/operational-contract/` — no `ops/` tree exists anywhere in the workspace.
- **Architectural reason:** drift inside the authority layer itself.
- **Concrete consequence:** contract-resolution agents (and the gauntlet's own reviewers) following links from the most-cited document hit dead ends; the missing index weakens the "which contracts exist" answer that contract-mapping used to provide.
- **Counter-evidence searched:** architecture.md's Related Contracts footer partially substitutes for the missing README but is itself broken on the last row; no other index location found.
- **Confidence:** high.
- **Smallest useful action:** fix the two links, add the promised `system/contracts/README.md` index (13-line table also solving F01 visibility).

---

**SCOUT-04-F05**
- **Priority:** P3
- **Claim:** Two packages co-own one SQLite file with an implicit, undocumented division of authority: `runcontrol` owns the file-level schema gate, `productstate` owns tables inside the same file.
- **Evidence:** both declare the same path — `internal/productstate/store.go:19` and `internal/runcontrol/sqlite.go:22` (`DatabaseRelativePath = ".ultraplan/run-control.db"`). runcontrol sets `PRAGMA user_version = 1` (`sqlite.go:359`) and rejects/migrates on mismatch with file-level backup (`migration_test.go:67-79`); productstate independently `CREATE TABLE IF NOT EXISTS product_states…` in that file (`store.go:92-103`) with no version marker. Connection pragma policy (WAL/FULL/busy-timeout/fk) is duplicated as literals (`store.go:67-72` vs `docs/architecture.md:166-167`). `docs/architecture.md:153-160` names runcontrol as owner of `.ultraplan/run-control.db` without acknowledging coexistence. Additionally `internal/productstate` has zero test files (verified).
- **Architectural reason:** lifecycle / ownership — a shared durable resource has two independent schema owners and no pinning test or stated convention for who owns `user_version`.
- **Concrete consequence:** a future runcontrol schema bump that backs up or rejects "its" database silently sweeps away product-state tables; divergent pragma evolution between the two open() paths becomes order-dependent; nothing fails today, which is why it is invisible.
- **Counter-evidence searched:** Current settings are identical; recovery.md:218 documents run-control backups; Sprint 35 requirements (`sprints/35-durable-run-observability/requirements.md:47-66`) deliberately leave storage representation as an open question, so this may be intentionally transitional debt pending Sprint 35 reasoning. That mitigates urgency but the invariant is currently enforced nowhere.
- **Confidence:** medium.
- **Smallest useful action:** one sentence in `docs/architecture.md` stating the coexistence contract (runcontrol owns file/user_version/migration; productstate owns its tables and must never touch pragmas/version), or a test opening both stores against one file.

### Defended architecture / rejected hypotheses

1. **`.ultra/` prefix vs root-relative layout is a defect?** Rejected. Explicitly normalized at the boundary: `prompts.go:233` instructs dropping the prefix; `reasoning_defaults.go:48`, `handbook.go:139` strip it; fixtures test both forms; `sprint-review-protocol.md` exists precisely to document the alias. Necessary migration translation.
2. **`tui→app` / `web→app` inversion smells like a service locator?** Rejected. Runners are plain injected closures (`main.go:27-41`, `app.Config` fields `app.go:32-33`); `docs/architecture.md:21-27` names the prohibited cycle; the web allowlist test makes regression impossible.
3. **Two architecture documents (workspace `ARCHITECTURE.md` 733 L vs repo `docs/architecture.md` 191 L) violate DOC single-source-of-truth?** Rejected as conflict: dependency graphs agree verbatim (`ARCHITECTURE.md:529-536` ≡ `docs/architecture.md:11-19`), audiences differ (planning vs implementation), and authority is explicitly partitioned (`project-index.md:17`). Residual dual-maintenance risk noted, not a finding.
4. **`platform/config` imported by `platform/logging|runtime` violates platform neutrality?** Rejected — config is a platform-internal leaf; the contract's rule (platform ⊄ product) holds across the entire graph.
5. **Embedded prompts duplicating materialized skills drift?** Rejected as designed: override model is explicit (`init.go:107-135`, workspace README "Materialize editable copies only when you need local overrides").
6. **Sprint 28 `Verdict: fail` / blocked dogfood in sprint 34 indicates broken governance?** Opposite — verdict computed by product code, not the LLM (`review-sprint-protocol.md:65-72`); failures and blocked real-runtime dogfood are recorded truthfully (`sprints/34-shared-context/execute.md:89-93`). The control loop is working.
7. **daemon-events study contradicts implemented runcontrol design?** Rejected — it is an uninitialized research scaffold (questionnaires only; zero claims about ultraplan-go; unreferenced anywhere), correctly positioned as input to Sprint 35 reasoning; `roadmap.md:1398` explicitly commits behavior, not topology.

### Open questions

1. Is implementation of durable-run observability proceeding ahead of Sprint 35's governed chain (repo HEAD commits touch exactly that scope while sprint 35 flow-state shows sprint-index/handbook/reasoning/plan all `missing`)? A snapshot cannot establish authorship order; if yes, the corpus→catalog→sprint enforcement loop is being bypassed for current work, which would upgrade F01's materiality.
2. Is the 13-contract pool considered final for this project (with the other six deliberately local-tool-N/A), or is extension planned with Sprint 35+? This determines whether F01 is drift or accepted scope.
3. Were the `/home/antonioborgerees/...` paths valid in the workspace's origin environment (pure HISTORY), or has the workspace been relocated without re-basing its catalogs (live defect for any stage agent)?
