I have completed my investigation. Here is my specialist report.

---

# SPECIALIST-10A — App composition / use cases (independent a)

## Scope inspected

- **Composition root & dispatch:** `cmd/ultraplan/main.go`; `internal/app/app.go` (`Run`, `dependencies`, global flags, exit classes), `surfaces.go`
- **Shared use cases:** `internal/app/usecases.go`, `operations.go`, `web_usecases.go`, `project_usecases.go`, `sprint_usecases.go`, `study_usecases.go`, `run_usecases.go`
- **Durable/run-control composition:** `internal/app/durable_operations.go`, `operation_runner.go`, `run_control.go`, `run_commands.go`
- **Surface commands:** `serve_commands.go`, `tui_commands.go`, `sprint_commands.go`, `study_commands.go`, `health_commands.go`, `storage_commands.go`, `code_commands.go`, `defaults/workspace_commands.go`
- **Consumers:** `internal/tui` (app.go, model.go), `internal/web` (server.go, operations.go, handlers, import_boundary_test.go)
- **Product seams:** `internal/sprint/service.go`, `index.go`, `cleanup_uncertain.go`, `review.go`; `internal/study` (service construction surface); `internal/productstate`
- **Authoritative docs:** planning-workspace `projects/ultraplan-go/docs/ARCHITECTURE.md` (app/tui/web ownership sections, durable-execution-control boundary, Phase 4–6 intent), `project-index.md`
- **Verification:** `go build ./...`, `go test ./internal/app ./internal/web ./internal/tui` (all pass)

## Architecture assessment

**Sound.** The composition topology matches the authoritative contract closely:

- Dependency direction is clean and *enforced*: `tui` and `web` depend only on `app` types; `web/import_boundary_test.go` rejects any non-stdlib import besides `internal/app`; `app` never imports surfaces — runners are inverted function types constructed in `cmd/ultraplan/main.go:27-41` and injected via `app.Config` (`app.go:36-37`), exactly as ARCHITECTURE.md:308 requires ("constructed explicitly in cmd/ultraplan … Do not add package-global mutable runner registration").
- Typed boundaries are real, not decorative: `WebOperations`, `RunUseCases`, `DurableOperationManager`, `OperationCleanupRecorder`, `OperationReconciler` carry no transport or product types; the web request mapper (`web/operation_handlers.go:589-600`) structurally cannot smuggle `ExpectedFingerprint` ("server-issued authority", operations.go:120-123), and `RunOperation` re-verifies it (operations.go:283-291).
- Lifecycle ownership is deliberate: `runControlState` caches one SQLite handle per workspace behind a pointer because `dependencies` is copied (`run_control.go:18-21`), guards retention-policy flips (line 45-47), and is closed via deferred `Close()` before `os.Exit`. Durable acceptance for CLI entries is guarded by an inventory test (`run_control_inventory_test.go`) that fails if any runtime-backed entry bypasses `beginDurableCLICommand`.
- Surface reuse is genuine where it matters: `sharedOperationRunner` is the single runtime-backed implementation for TUI and web (`operation_runner.go:15-17`); the read-only web posture (`WithoutStatusWrites`, sprint/service.go:67-71) is asserted by tests (`web_usecases_test.go:37`, `:132`).
- Apparent "leakage" candidates mostly dissolve under inspection: `dashboardUseCases.RunOperation`'s big dispatch is sanctioned workflow semantics ("workflow semantics remain here", operation_runner.go:16-17); `supportedPreviewPath` is a single display allowlist shared by both surfaces; `RecordCleanupUncertain` deliberately writes a sidecar marker without the mutation lease (sprint/cleanup_uncertain.go:14-17).

The stresses below are consistency and change-locality risks, none of which is a current behavioral defect.

## Candidate findings

---

### SPECIALIST-10A-F01
- **Priority:** P3
- **Claim:** Runtime-service dependency construction is duplicated four times instead of composed once, and each copy self-serves workspace discovery + config, so one user action re-loads configuration up to three times.
- **Evidence:** `sprint_commands.go:477-499` (`sprintRuntimeService`), `study_commands.go:307-341` (`runLoopService`), `study_commands.go:648-676` (`runAllService`), `study_commands.go:824-852` (`executionService`). All four repeat the identical pipeline `loadEffectiveConfig → runtimepkg.RequestFromConfig → factory(cfg) → controlledRuntimeFor(deps, root, cfg, rt) → NewService(...)`. The three study variants differ only in error-prefix strings and parallelism handling. A non-dry `sprint flow` performs workspace discovery ×2 and `loadEffectiveConfig` ×3 (`sprint_commands.go:73-80` → `run_commands.go:42-58` via `beginDurableCLICommand` → `sprint_commands.go:478`).
- **Architectural reason:** change-surface (composition not owned at one point; ARCHITECTURE.md assigns "shared dependency construction" to app).
- **Concrete consequence:** Adding one construction concern (a new `With*` option, credential refresh, config-source pinning) requires editing four sites; missing one yields silently divergent surfaces (e.g., progress observation wired on flow but not run-all). Repeated config reads also create a consistency window where preflight validation and the service that executes use different effective configs.
- **Counter-evidence searched:** Checked whether differences are semantic (they are not: options lists currently agree across sites, verified against `runSprint:81`); checked tests for intentional per-site variation (none found); compiler catches missing interface methods, so drift today is latent.
- **Confidence:** high (facts), medium (impact)
- **Smallest useful action:** Extract one `studyRuntimeService(deps, root)` used by the three study sites (and optionally pass preflight `config.Effective` through `dependencies` instead of reloading).

---

### SPECIALIST-10A-F02
- **Priority:** P3
- **Claim:** Capability wiring for runs/durable operations exists twice in parallel (`dashboardUseCases.runs/durable` and `webUseCases.runs/durable`) with 16 near-identical forwarding methods; the two surfaces populate different halves, leaving the web-embedded dashboard permanently capability-nil.
- **Evidence:** `usecases.go:72-119` — eight nil-guarded forwarders on `dashboardUseCases`; `web_usecases.go:263-317` — eight structurally identical forwarders on `webUseCases`; `NewWebUseCases` (`web_usecases.go:242-261`) sets `runner` on the embedded dashboard but leaves its `runs`/`durable` nil while populating the outer pair (`serve_commands.go:59-66`); conversely `runTUI` populates only the dashboard pair (`tui_commands.go:46-48`).
- **Architectural reason:** lifecycle / boundary (single capability should have single source of truth; type-assertion probes in `tui/app.go:109,235,287,362` and `web/server.go:76` always succeed regardless of wiring).
- **Concrete consequence:** Any future dashboard-internal code path that calls `u.runs`/`u.durable` works on TUI but silently returns `ErrWebUnavailable` on the web surface (or vice versa), and new methods on `RunUseCases`/`DurableOperationManager` must be added to both forwarder sets. The compile-time interface check limits but does not eliminate drift (per-method bodies can diverge independently).
- **Counter-evidence searched:** Verified no current call path reaches the nil half on either surface; verified Go field-composition (rather than type embedding) was forced by differing `Dashboard` result types, but populating `u.dashboard.runs = opts.RunControl` would still eliminate the outer duplicates; checked tests — none assert the dual-field design.
- **Confidence:** high (facts), medium (impact)
- **Smallest useful action:** In `NewWebUseCases`, wire `opts.RunControl`/`opts.DurableOperations` into the embedded dashboard and delegate the outer forwarders to it (or delete them via embedding).

---

### SPECIALIST-10A-F03
- **Priority:** P3
- **Claim:** The sprint stage→method dispatch table exists five times within `app`, and two of the five exactly duplicate in-package helpers that already exist.
- **Evidence:** `validateSprintStage` helper (`sprint_usecases.go:254-278`) vs. the identical inline validate switch in `sprint_commands.go:138-161`; `promptSprintStage` helper (`operations.go:676-699`) vs. the identical inline prompt switch in `sprint_commands.go:177-198` vs. `PromptBundle`'s variant (`web_usecases.go:511-539`); plus `planningStageRuntime` map (`sprint_commands.go:606-654`).
- **Architectural reason:** change-surface (ARCHITECTURE.md:23 explicitly makes "add a stage surfaced uniformly through all interfaces" the boundary proof — the cross-surface repetition is sanctioned cost, but exact same-package duplicates are not).
- **Concrete consequence:** The next stage addition (the pattern Phase 5 established) touches five switches; the two CLI switches can be replaced by existing helpers with zero behavior change, cutting the surface to three genuinely distinct mappings (CLI reuse, use-case layer, web scope annotations).
- **Counter-evidence searched:** Confirmed the web switch carries extra per-stage presentation (`Scope`, `UnavailableReason`) and the validate/prompt helper signatures match the CLI call sites exactly (`service.ValidateRequirements(args[0], args[1])` ≡ `validateSprintStage(service, args[0], args[1], stage)`), so no translation is lost by reuse.
- **Confidence:** high
- **Smallest useful action:** Replace `sprint_commands.go:138-161` and `177-198` with calls to the existing helpers.

---

### SPECIALIST-10A-F04
- **Priority:** P3
- **Claim:** Runtime-factory injection is inconsistent at the composition root: the sprint factory flows through `app.Config.SprintRuntimeFactory` with a default fallback (`app.go:38,122-124`), while the study factory, health-check hook, and clock are package-global mutable vars in production code.
- **Evidence:** `study_commands.go:22` (`var studyRuntimeFactory = ...`), `study_commands.go:1089` (`var timeNow`), `health_commands.go:28` (`var runtimeHealthChecks`); tests mutate them globally with save/restore (`study_run_commands_test.go:195-198`, `study_status_commands_test.go:90-94`), whereas the sprint seam is passed structurally (`app_test.go:285`).
- **Architectural reason:** boundary / lifecycle (dependency visibility; process-global mutable state).
- **Concrete consequence:** The composition root does not reveal that study runtime construction is swappable; any second concurrent `Run` consumer (test parallelism, future embedding) races on the globals, and the asymmetry invites the next contributor to add another global seam. Currently safe only because no relevant test uses `t.Parallel()`.
- **Counter-evidence searched:** Searched docs/comments for a rationale distinguishing the two mechanisms (none found; ARCHITECTURE.md's "package-global mutable runner registration" prohibition strictly targets surface runners, so this is convention-adjacent rather than contract violation); confirmed `dependencies` can carry the factory with no cycle.
- **Confidence:** high (facts), medium (consequence)
- **Smallest useful action:** Add `StudyRuntimeFactory` to `app.Config`/`dependencies` mirroring the sprint field; default to the current implementation when nil.

---

### SPECIALIST-10A-F05
- **Priority:** P3
- **Claim:** `webUseCases.sprintOverview` re-implements parsing of governed `requirements.md` structure ("## Sprint Goal" heading extraction) in the app layer, duplicating vocabulary owned by the sprint module.
- **Evidence:** `web_usecases.go:559-581` (hand-rolled heading scanner, backtick/asterisk trimming); the sprint module is the owner of requirements.md structure — `ValidateRequirementsContent` enforces the exact heading set including "Sprint Goal" (`sprint/index.go:43`) and no exported accessor for the goal text exists.
- **Architectural reason:** drift (artifact-format knowledge leaking out of the owning product module).
- **Concrete consequence:** If requirements.md conventions evolve (heading renamed, goal rendered as list/bold), sprint validation and prompts update in one place while the web overview silently degrades to empty — no test failure points at the coupling.
- **Counter-evidence searched:** Confirmed the function is display-only with graceful "" fallback (bounded harm); confirmed no existing sprint export could have been reused (so the fix is additive, not a refactor); considered whether this counts as necessary boundary translation — it does not, since it interprets governed artifact semantics, not transport concerns.
- **Confidence:** high (facts), medium (impact)
- **Smallest useful action:** Expose `sprint.SprintGoal(content string) string` next to `ValidateRequirementsContent` and call it from `sprintOverview`.

---

### SPECIALIST-10A-F06
- **Priority:** P3
- **Claim:** Parallelism authority for the same logical study operation diverges by surface: web `PrepareOperation` enforces 1–64; the CLI enforces only ≥1.
- **Evidence:** `operations.go:227-229` (`req.Parallelism < 1 || req.Parallelism > 64`) vs. `study_commands.go:316-318` and `parsePositiveIntFlag` (`study_commands.go:600`) with no upper bound.
- **Architectural reason:** authority (request-validation rule split across two owners instead of living with the operation).
- **Concrete consequence:** An operator value the web refuses (say 200) is durably accepted via CLI; behavior differs only because of entry point, and the bound cannot be changed without knowing both sites exist. Mitigated by the memory-pressure throttle (`study/run_loop.go:323`).
- **Counter-evidence searched:** Checked whether the study module itself bounds parallelism (it throttles adaptively but sets no hard max); considered operator-trust rationale for CLI — plausible, but nothing documents it, and the durable acceptance record treats both identically.
- **Confidence:** medium
- **Smallest useful action:** Either share one bound constant between `PrepareOperation` and the CLI parsers or document the intentional divergence at both sites.

## Defended architecture / rejected hypotheses

1. **"readOnly flag is unenforced convention; web cleanup/reconcile writes bypass it."** Investigated whether `RecordCleanupUncertain`/`ReconcileOperations` constructing bare `sprint.NewService(u.root)` (`web_usecases.go:336,344,359`) violates the read-only posture. Rejected: those are sanctioned durability-repair paths (ARCHITECTURE.md durable-boundary contract: "reconcile stale, interrupted, and cleanup-uncertain work conservatively"); the marker write deliberately avoids the mutation lease (`cleanup_uncertain.go:14-17`); `WithoutStatusWrites` is narrowly scoped to flow-state refresh and the query-path read-only posture is test-enforced (`web_usecases_test.go:37,132`).
2. **"webUseCases.refs map leaks memory."** Rejected: HMAC refs are deterministic per (kind, values), so polling re-hits the same keys; growth is bounded by distinct workspace targets, verified `issue()`/`resolve()` at `web_usecases.go:870-889`.
3. **"Two progress-coalescing implementations (durable_operations.go:133-147 vs run_control.go:175-187) are drift-prone duplication."** Downgraded after tracing inputs: one coalesces `OperationEvent` tuples at the operation boundary, the other hashes `runtimepkg.Event` payloads at the runtime boundary with different equivalence requirements (content deltas must not coalesce). Unifying them would couple two event vocabularies for modest gain.
4. **"Payload key promotion in run_control.go couples durable storage to the web timeline JS."** Real but defended: the comment admits the design center (`run_control.go:408-410`), yet promotion happens at the single choke point through which both CLI and web read events, keys are generic observability fields consumed defensively (`web/run_handlers.go:288-296`), and readers use fallback chains. Documenting the key set would be nice-to-have, not a defect.
5. **"CLI bypasses the use-case layer by building services directly."** Rejected as violation: ARCHITECTURE.md:298 sanctions "shared app use cases **or product services**"; direct service use from CLI handlers is the intended shape, with the dashboardUseCases layer existing for TUI/web sharing.
6. **"Capability type-assertions that always succeed make probing meaningless."** Partially rejected: probes (`tui/app.go:235`, `web/server.go:76`) do always succeed today, but nil-guarded forwarders convert absent wiring into graceful `ErrWebUnavailable` handling (`tui/app.go:290-293`), which appears intentional; residual risk captured in F02 rather than as an independent flaw.

## Open questions

- Is the ≤64 web parallelism cap (F06) a deliberate operator-trust distinction or an incidental bound from the web-hardening sprint? A roadmap/TRD statement either way would resolve it.
- Was the `webUseCases` dual-capability-field shape chosen to keep the embedded dashboard provably query-only (defense against future internal misuse), or is it incidental? If deliberate, F02's smallest action should be replaced by a comment declaring the invariant.
- Should `defaults install` honoring `--workspace` as its default path (`defaults_commands.go:17-19`) while `init-workspace` ignores it be read as intentional flag scoping? Cosmetic today; noted only because both are composition-root commands.
