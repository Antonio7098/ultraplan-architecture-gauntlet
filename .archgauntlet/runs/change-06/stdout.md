### Scope inspected

Implementation repo `/home/antonioborgerees/coding/ultraplan/gauntlet-run/ultraplan-go` @ eeaa034:

- `internal/sprint/review.go` (PrepareReview, Review, render/validate, `reviewRelevantProjectIndexContent`, `loadReviewAsset`), `internal/sprint/smoke.go`, `internal/sprint/smoke_protocol.go`, `internal/sprint/smoke_author.go`, `internal/sprint/smoke_types.go`, `internal/sprint/index.go`, `internal/sprint/validation.go`, `internal/sprint/flow.go`, `internal/sprint/prompts.go`, `internal/sprint/review_runtime_validation.go`
- `internal/project/domain.go`, `index.go`, `validation.go`
- Consumer layers: `internal/app/sprint_commands.go`, `internal/web/operation_handlers.go`, `internal/app/web_usecases.go`, `internal/tui/doc.go`, `internal/app/run_control_inventory_test.go`
- Workspace scaffold: `internal/workspace/scaffold/{templates,prompts}/…` (project-index, sprint-index, review/smoke prompts/templates)
- Authoritative workspace docs: `system/protocols/review-sprint-protocol.md`, `system/protocols/deep-smoke-sprint-protocol.md` (+ `sprint-review-protocol.md` alias), `projects/ultraplan-go/project-index.md`
- Verification: `go test ./internal/sprint ./internal/project` — green. Read-only; no target files modified.

### Architecture assessment

The probe decomposes into five concrete change scenarios, each with a distinct owner and edit surface:

1. **New review protocol document** (content-level strategy): pure data. Add a row under `## Review Protocols` in project-index.md, select it under `## Required Review Protocols` in sprint-index.md (`internal/project/domain.go:15`, `internal/sprint/index.go:31`). Zero Go changes. Membership is enforced once (`validateSubset`, `internal/sprint/validation.go:14`) and re-resolved against the catalog during manifest preparation (`internal/sprint/review.go:292–317`); content is hashed into the review fingerprint (`review.go:365–367`).
2. **Strategy emphasis via prompt/template**: workspace overrides `prompts/review.md` / `templates/review.md` are loaded and marker-checked (`review.go:192–199`, required markers "Automated Sprint Review", "Review Context"/"Final Assessment"), with embedded defaults in scaffold. Data-only; deterministic verdict computation stays product-owned, matching the protocol doc ("An LLM may summarize but cannot choose or override the verdict").
3. **Add/replace smoke harness**: the catalog requires exactly one `## Smoke Harnesses` row (`internal/sprint/smoke_protocol.go:116–124`); swapping it plus providing a protocol-v1 manifest (capabilities `discovery/run/evidence-v1/scope-mapping/authoring-v1`, `smoke_protocol.go:213–258`; entry validation `internal/project/validation.go:103–131`) is zero-Go-change.
4. **New suites/tests/levels within the harness**: external discovery data only; relationships are validated generically (`validateSmokeDiscovery`, `smoke_protocol.go:260–373`), selection generically (`selectSmoke`, `:441–540`). Zero Go changes.
5. **Structural extension** (new scope-kind axis, new reviewer coverage class): confined to `internal/sprint` — request fields, `selectSmoke` precedence, `expectedSmokeTests` switch, or `PrepareReview`/`RenderReviewMarkdown`/`ValidateReviewContent`. App/web/TUI only plumb typed requests; they are declared non-authorities (`internal/tui/doc.go:12–17`; both workspace protocol docs' "TUI Parity" sections), and the durable-boundary inventory test (`run_control_inventory_test.go:11–54`) prevents bypasses.

What makes this sound is the deliberate authority topology around fingerprints: `reviewRelevantProjectIndexContent` (`review.go:1341–1369`) strips the smoke-harness section from the review's frozen view so replacing a smoke strategy never invalidates implementation review evidence, while smoke's preflight independently re-validates review freshness and digest (`smoke_protocol.go:177–209`). Coupling is directional (smoke gates on review; review ignores smoke identity), which prevents mutual-invalidation churn when either strategy changes.

Ambiguity worth recording: review protocols are fingerprinted *inputs*, never *reviewer coverage* (`resolve(..., reviewer=false)`, `review.go:321–322`; only contracts and handbook get reviewers, `:318–331`). A newcomer might expect a "required protocol" to be enforced by its own reviewer. This matches CURRENT-CONTRACT (review-sprint-protocol.md §4: "one structured reviewer per selected contract and one for the technical handbook") — see rejected hypotheses.

### Candidate findings

**CHANGE-06-F1**
- Priority: P3
- Claim: The artifact-section contract exists twice as parallel literal lists — renderer vs validator — for both stages: `RenderReviewMarkdown` sections (`internal/sprint/review.go:1050`) vs `ValidateReviewContent` headings (`review.go:1102`); `RenderSmoke` emitted headings (`internal/sprint/smoke.go:464–551`) vs `ValidateSmokeContent` heading list (`smoke.go:560`).
- Evidence: file:line above; also `"## Smoke Harnesses"` literal vs `project.SectionSmokeHarnesses` constant at `review.go:1350` despite `internal/sprint` already importing `internal/project`.
- Architectural reason: drift / duplicated mapping (change-surface)
- Concrete consequence: Adding a structural section/kind and updating only the renderer quietly weakens enforcement (validator stops short); updating only the validator fails every run loudly. The quiet direction is the risky one when the two lists diverge over time.
- Counter-evidence searched: Same-file adjacency; runtime self-reconciliation (`Review` validates its own render output, `review.go:608`; `commitSmoke` validates before write, `smoke.go:419`); end-to-end pinning via `service.ValidateReview` after `service.Review` (`review_test.go:142–154`) and smoke gate reuse (`smoke_protocol.go:193`, `verify.go:197,229`, `service.go:178,184`). Drift in existing sections therefore fails CI immediately; only additive omission is silent. Consequence is real but small.
- Confidence: high (facts), low-to-medium (materiality)
- Smallest useful action: Define each required-section list once as a package-level slice consumed by both functions (or add one round-trip test asserting `RenderX` output passes `ValidateX`); use `project.SectionSmokeHarnesses` at `review.go:1350`.

That is the complete finding list. No P0–P2 findings were sustained.

### Defended architecture / rejected hypotheses

1. **"Review protocols without their own reviewer is an enforcement gap."** Rejected. CURRENT-CONTRACT explicitly scopes reviewers to contracts + handbook (review-sprint-protocol.md §4); code implements exactly that (`review.go:318–331`, comment at `:327` "independent reviewer even though it is also a governed input"). Protocols shape reviewer context and the fingerprint; making them coverage classes would be new intent, not a defect.
2. **"Exactly-one-smoke-harness blocks adding smoke strategies."** Rejected. Strategy variety lives inside the harness (suites/tests/levels/mappings via discovery) and the single catalog row *is* the anti-parallel-authority mechanism. HISTORY confirms intent: the legacy `Smoke Harness Directory:` Project Scope field is actively rejected as a "duplicate smoke harness source … the Smoke Harnesses catalog is authoritative" (`internal/project/validation.go:14,43–45`).
3. **"Two harness validations (`project/validation.go:103–131` and `prepareSmokeStatic`) are parallel authorities."** Rejected. They execute at different lifecycle points (catalog authorship vs stage preflight), re-resolve symlinks canonically because filesystem state can change in between, and the sprint package cannot trust a prior project-package pass. Necessary boundary translation, not duplication of decision-making.
4. **"UI layers duplicate review/smoke workflow policy."** Rejected. TUI/Web emit typed `OperationRequest`s into app use cases; `tui/doc.go:12–17` forbids owning execution; `run_control_inventory_test.go` enforces the durable boundary; scope-kind semantics exist only in `selectSmoke`/`expectedSmokeTests`.
5. **"`smoke_author.go:221` hardcoded governed-input list duplicates flow authority."** Rejected for this probe. It is an agent-facing presentation list inside the author prompt, not a resolution path; actual reads go through the store/artifact APIs. It would matter only for probe 1 (new governed stage), not strategy addition.
6. **"Adding a strategy requires touching config authorities."** Partially rejected: `smoke.*` config keys (`internal/platform/config/config.go:125–144`) cover timeouts/limits/environment only; strategy identity (harness, suites, protocols) is entirely workspace-catalog-owned. Config is not a second registry of strategies.

**Locality verdict:** authoritative owner for review strategy = project catalog + sprint-index selection (data) with `internal/sprint/review.go` as the sole structural owner; for smoke strategy = the single cataloged harness (data/external) with `internal/sprint/smoke_protocol.go|smoke.go` as sole structural owners. Actual edit surface matches expected surface in all five scenarios; no unrelated modules touched for data-level changes; UI plumbing changes only when a new request axis is introduced, and then only additively. Architecture defended.

### Open questions

- The workspace project-index marks "versioned Phase 3 manifest planned in Sprint 27" for the smoke harness. If a protocol-v2 lands, `SmokeProtocolMajor`/capability list (`smoke_protocol.go:213–231`) is the single gate — no action needed now, but whether old harnesses must remain runnable concurrently is unstated (FUTURE-INTENT, not a current defect).
- Whether review protocols are ever intended to become mandatory reviewer coverage classes (they currently are context+fingerprint only). If yes, that intent should eventually be recorded in the protocol doc; today's behavior is contract-conformant.
