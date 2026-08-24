### Scope inspected

**Implementation repo** (`ultraplan-go` @ eeaa034):
- `internal/platform/runtime/`: `runtime.go`, `opencode.go`, `health.go`, `agentwrap.go`, `events.go`, `policy.go`, plus `runtime_test.go`, `opencode_test.go`, `cache_test.go`
- Consumers: `internal/sprint/` (`flow.go`, `service.go`, `session_state.go`, `execute.go`, `runtime_metrics.go`, `review_runtime_validation.go`, `smoke_author.go`, `code_context.go`), `internal/study/` (`run.go`, `service.go`), `internal/app/` (`sprint_commands.go`, `study_commands.go`, `health_commands.go`, `run_control.go`, `app.go`, `operations.go`)
- Config/scaffold: `internal/platform/config/config.go`, `internal/workspace/init.go`
- Docs: `docs/architecture.md`

**Authoritative workspace**: `projects/ultraplan-go/docs/TRD.md` §11–12, §13 (change log)

**External dependency**: `/home/agentwrap` source (`runtime.go`, `events.go`, `config.go`, `health.go`, `observability.go`, `opencode/projector.go`, `opencode/health.go`) to verify how `RuntimeContext.RuntimeKind` is actually consumed

### Architecture assessment

The change is unusually well-localized; the seam is real and already load-bearing:

- **Generic contract lives where it should.** `Runtime` (runtime.go:245-249) wraps `agentwrap.Runtime`; all request/result/event/error translation (`toAgentwrapRequest`, `mapResult`, `mapEvent`, `mapError`, payload sanitization in `agentwrap.go`) is runtime-neutral and tested against a synthetic fake via `NewAdapter(fakeRuntime{...})` in `runtime_test.go`. A second adapter reuses all of it.
- **Product modules depend on minimal interfaces, not the adapter.** `sprint.Runtime` is `StartRun`-only (flow.go:13-15); `study.Runtime` likewise (service.go:25-26). Both are exercised almost exclusively through hand-written fakes (`WithRuntime(...)` across ~20 test files), proving the coupling surface is exactly one method.
- **Adapter-specific knowledge is quarantined in one file.** `opencode.go` is the only opencode-importing file; even reasoning-effort translation (`requestVariantRuntime`, opencode.go:65-83) lives here, fed by runtime-neutral metadata keys `variant`/`reasoning_effort` set by product code (service.go:1039-1041).
- **Composition matches the mandated stack** (TRD §11.2): `Observing(Validating(Policy(concrete)))` at opencode.go:53-61.
- **Selection authority exists but is pinned.** `runtime.default` config field (config.go:170) is validated to `"opencode"` (config.go:409-411). Per TRD §11.1 ("first concrete runtime adapter") this pinning is CURRENT-CONTRACT, not a defect.
- **Stressed points** are at the state/wiring edges, detailed below: persisted session compatibility keys, health identity stamping inside the generic `Adapter`, triplicated construction seams, and substring-based tool attribution.

### Candidate findings

#### CHANGE-03-F01
- **Priority:** P2
- **Claim:** Adding a second adapter silently enables mid-sprint `runtime.default` switches, but every persisted session-reuse path identifies sessions by provider/model/workdir only — no runtime identity — and the sprint planning-stage continuation path has no fresh-session fallback, so a foreign session ID becomes an opaque stage failure.
- **Evidence:**
  - `session_state.go:21-28` — `stageSessionRecord{SessionID, Provider, Model, WorkDir}`; `:87-92` — `stageSessionCompatible` checks exactly those fields; `:97-101` resumes with `SessionAction="continue"` unconditionally; `:48,56` — `DisallowUnknownFields` + schema v1 means adding a field requires a deliberate bump.
  - `execute.go:539-549` — `reusableExecuteSession` matches model + non-empty session ID only; architecture.md:151 documents reuse as "model and target compatibility".
  - `study/run.go:115,175` — fingerprint includes provider/model/workdir but not runtime kind; contrast study's graceful `studyContinuationNeedsFreshFallback` (run.go:157-171) and review repair's `AllowFreshSessionFallback` (review_runtime_validation.go:77-78). Planning stages (session_state.go:94-131) have neither.
  - Today unreachable because config.go:409 rejects non-opencode — the gate is removed by precisely this change.
- **Architectural reason:** lifecycle / failure-semantics (product-owned durable state vs. adapter-owned session namespaces)
- **Concrete consequence:** operator flips `runtime.default` between interrupted stage runs; resume sends an OpenCode session ID into a different CLI's continue semantics; planning stage fails with a runtime-classified error attributed to the stage, retryable loop persists the same incompatible session until manual `.stage-sessions.json` surgery.
- **Counter-evidence searched:** study/review paths do fall back (so the pattern exists and its absence in `session_state.go` is not uniform carelessness); TRD §11.3 permits sessions "only when UltraPlan intentionally continues ... a related run" — a cross-runtime continue is not that, so no current contract is violated; runtime metrics (runtime_metrics.go:135-137) also omit runtime kind, confirming this is systemic, not a one-off.
- **Confidence:** high on mechanics, medium on severity (requires operator action post-change)
- **Smallest useful action:** when the second constructor lands, extend session-compat predicates (stage sessions, execute records, study fingerprints) with resolved runtime identity — or clear persisted sessions on runtime switch — as part of the same change, with the schema-v2 bump `DisallowUnknownFields` forces anyway.

#### CHANGE-03-F02
- **Priority:** P3
- **Claim:** The generic `Adapter` stamps `RuntimeKind/RuntimeName: "opencode"` in `Health()` regardless of which runtime it wraps, while `Capabilities()` derives kind from the underlying runtime — inconsistent identity sources in the same file.
- **Evidence:** health.go:56 hardcoded literal vs. health.go:105-111 pass-through `caps.RuntimeKind`; `NewAdapter` (runtime.go:256-262) records nothing.
- **Architectural reason:** drift (identity authority split between call site and wrapped runtime)
- **Concrete consequence:** a second adapter built via `NewAdapter(claudeRuntime)` reports `producer_runtime_kind=opencode` in observability artifacts (agentwrap observability.go:736-737) and policy summaries (policy.go:626), poisoning diagnostics during exactly the period a new adapter is least trustworthy. Not behavior-breaking: concrete adapters stamp their own effective-config kind (opencode/health.go:315), and today only `NewOpenCode` builds production Adapters.
- **Counter-evidence searched:** verified `CheckHealth` consumers don't branch on `req.Context.RuntimeKind` behaviorally; TRD pins opencode so the literal is currently truthful.
- **Confidence:** high (code fact), low impact
- **Smallest useful action:** carry kind/name on `Adapter` set at construction (`NewOpenCode` supplies "opencode"); `Health()` uses the field.

#### CHANGE-03-F03
- **Priority:** P3
- **Claim:** Runtime construction is triplicated at the app layer in two different seam styles, none consulting `c.Runtime.Default`; the config field's only readers are validation and display.
- **Evidence:** injectable `SprintRuntimeFactory` via Dependencies (sprint_commands.go:21-25, app.go:38/119-123) vs. package var (study_commands.go:22-24) vs. direct call with no factory (health_commands.go:113) plus hardcoded `"runtime.opencode"` check IDs (health_commands.go:115,119,144). `rg 'Runtime\.Default'` shows only config.go:280/399/409, config_commands.go:47, health_commands.go:77, study_commands.go:332.
- **Architectural reason:** change-surface (drift risk among composition-root twins)
- **Concrete consequence:** the second-adapter change requires synchronized edits in three functions; missing one produces e.g. health probing opencode while sprints execute on the new runtime — reported healthy, behaving differently.
- **Counter-evidence searched:** all sites sit in `internal/app`, which owns wiring by doctrine; each already has some test seam; surface is small. This is expected edit surface — the finding is only the triplication, not the layering.
- **Confidence:** high
- **Smallest useful action:** add one dispatcher in `platform/runtime` (e.g., `New(c config.Config)` switching on `c.Runtime.Default`) and delegate all three factories to it; derive health-check IDs from the configured name.

#### CHANGE-03-F04
- **Priority:** P3
- **Claim:** Smoke protected-path write attribution scrapes JSON-serialized event payloads for tool-name substrings instead of using agentwrap's normalized `ToolObservation`, so a second runtime's tool vocabulary escapes the lexicon.
- **Evidence:** smoke_author.go:139-156 (`event.Kind != "tool"` then `strings.Contains` over marshaled payload for `"tool":"write`, `apply_patch`, `filesystem_write`, ...) vs. agentwrap events.go:55-110 (`Event.ToolObservation()` exposing normalized Name/Arguments/Status, projected by adapters per TRD §11/§12); `promoteObservablePayloadFields` (runtime.go:592-604) already normalizes nesting but not vocabulary.
- **Architectural reason:** boundary (product reimplements projection-adjacent vocabulary) / drift
- **Concrete consequence:** degraded diagnostics only — a new runtime's differently named write tools yield "changed ... without an observed OpenCode protected-path write" instead of attribution; the hard failure still fires via before/after identity comparison (smoke_author.go:93-104), and architecture.md:127-132 explicitly defines attribution as best-effort diagnosis, not the safety mechanism.
- **Counter-evidence searched:** confirmed safety does not depend on attribution; noted `ToolObservation.Name` is still raw adapter-projected vocabulary, so porting wouldn't fully solve lexicon coverage either — it would mainly give structured Arguments/path extraction.
- **Confidence:** medium
- **Smallest useful action:** match on `Event.ToolObservation()` first, keep substring scrape as fallback; fix the now-stale "OpenCode" wording in the diagnostic.

### Defended architecture / rejected hypotheses

- **"Product code parses OpenCode stdout/events directly."** Disproven. All native interaction flows through `platform/runtime`'s mapping layer; TRD §11.4 forbids otherwise; grep found no raw parsing outside the adapter package.
- **"Adding a second adapter requires touching sprint/study internals."** Largely disproven. Product modules consume `StartRun(Request)(Result)` through narrow interfaces with pervasive fake-based tests; review validation explicitly anticipates runtime differences in output retention (review_runtime_validation.go:30-33). Edit surface concentrates in `platform/runtime` (+1 file), app factories, config allowlist, scaffold templates (init.go:139,170) — good locality.
- **"`CacheDirective`/metadata transport leaks OpenCode specifics into the generic contract."** Disproven as a defect: the contract comment (runtime.go:43-45) and architecture.md:149 define metadata transport as the current honest limitation with a documented native-support evolution path; cohort-key derivation (runtime.go:505-519) is runtime-neutral.
- **"`requestVariantRuntime` should be generalized now."** Rejected — it is the necessary boundary translation TRD-style architectures want (opencode.go:65-67 documents why); generalizing before a second consumer exists would add indirection without a user.
- **"Health mislabeling breaks health checks under a second adapter."** Disproven behaviorally: concrete runtimes own effective-config identity (opencode/health.go:315); impact is observability-only (hence P3).

### Open questions

- Will the second runtime be implemented inside `agentwrap` (as a sibling of `agentwrap/opencode`) or locally in ultraplan-go against `agentwrap.Runtime`? TRD §11.1 mandates agentwrap as *the* implementation boundary but doesn't forbid a local adapter; placement changes who owns event projection and where F01/F02 fixes belong.
- Does agentwrap intend to normalize semantic tool capabilities (e.g., "mutates filesystem") beyond raw names? If yes, CHANGE-03-F04's lexicon problem dissolves at the SDK layer and the product-side action shrinks further.
