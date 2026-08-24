Probe #7 is mine: "Change reasoning/prompt override resolution". Let me find the resolution code.
Two distinct resolution chains already visible. Let me map all override-resolution sites and the shared builtin mechanism.
TRD:1111 states the lookup contract. Let me read the architecture doc section and trace how `sprintPromptTemplate` failures reach runtime.
One last sweep for bypass sites and shared helpers before writing up.
### Scope inspected

- `internal/project/reasoning_defaults.go` (+ `reasoning_defaults_test.go`, `service.go:25-36`, `validation.go:80-93`)
- `internal/sprint/prompts.go` (`sprintPromptTemplate`, `projectReasoningPromptTemplate`, `renderPromptFromDefault`, `appendInjectedWorkspaceFile`) + `reasoning_test.go`, `service.go` prompt/flow call sites
- `internal/study/prompts.go` (`readWorkspaceFile`) + `prompts_test.go`
- `internal/sprint/review.go` (`loadReviewAsset`:1739, manifest build :192-199); `internal/sprint/smoke_author.go:213`
- `internal/workspace/init.go` (embedded registry, `DefaultOverrideFile`:315), `defaults.go`, `validation.go`; `workspace_test.go`
- App-level CLI tests pinning source labels (`app/sprint_commands_test.go`, `app/project_commands_test.go`, `app/study_prompt_commands_test.go`)
- Authoritative planning workspace: `docs/ARCHITECTURE.md` (workspace ownership section), `docs/TRD.md` §10.2 (:1111) and §18.7, `docs/PRD.md` :233/:984
- Git history probes (`git log -S sprintPromptTemplate|ResolveReasoningDefault|readWorkspaceFile`)
- Verified via grep: these four are the complete set of default-resolution sites; no bypassing readers; web package has no label coupling.

### Architecture assessment

The **reasoning** path is the model citizen: one authoritative 3-tier resolver (`project.ResolveReasoningDefault`, project > workspace > builtin) with an explicit allowlist, strict content validation, status surfacing of `Source` per tier, and a two-layer test surface — unit precedence (`TestResolveReasoningDefaultPrecedence`, including cross-project non-inheritance and invalid-override rejection) plus integration assertions through rendered prompts (`TestReasoningPromptsUseProjectThenWorkspaceThenBuiltinDefaults`, including the shadowing assertion at reasoning_test.go:98). Changing precedence for reasoning defaults is genuinely local.

The **mechanical** two-step rule that ARCHITECTURE.md assigns to `workspace` (“Embedded prompt/template default registry and override resolution … readable workspace file → intentional local override, otherwise embedded binary default”) is not owned by `workspace`. The package only owns the embedded map. Every consumer re-implements the lookup, each with different failure semantics — including three different behaviors *inside the sprint module* depending on whether the path is in the reasoning allowlist. For probe #7 (change override resolution), locality is good for reasoning paths but poor for any change that touches the shared mechanical rule.

### Candidate findings

#### CHANGE-07-F01

- **Priority:** P1
- **Claim:** The documented workspace-owned mechanical override lookup (workspace file → else builtin) is duplicated across four sites in three packages with divergent error contracts; a change to resolution policy requires synchronized edits in all of them.
- **Evidence:** Four independent implementations of the identical mechanical rule:
  - `internal/project/reasoning_defaults.go:52-86` (3-tier, fail-closed, strict validation)
  - `internal/study/prompts.go:175-191` `readWorkspaceFile` (2-tier, propagates errors)
  - `internal/sprint/prompts.go:241-257` `sprintPromptTemplate` (2-tier, converts read errors to inline `"# Prompt Load Error"` text at :250 and missing-default to `"# Missing Prompt Default"` at :256; silently falls through to builtin on `ResolveInside` error at :244)
  - `internal/sprint/review.go:1739-1765` `loadReviewAsset` (2-tier + placeholder/required-substring validation)
  - Source-label construction `"builtin:"+rel` duplicated at reasoning_defaults.go:82-83, study/prompts.go:188, sprint/prompts.go:254. Contract assigning this mechanism to workspace: planning-workspace `docs/ARCHITECTURE.md` “workspace owns … Embedded prompt/template default registry and override resolution” with the exact two-step policy spelled out.
- **Architectural reason:** drift / change-surface / ownership
- **Concrete consequence:** Probe #7 measured: reordering tiers, adding a tier (e.g., user-global), or renaming source labels touches ≥4 code files in 3 packages plus ~5 test files (`project/reasoning_defaults_test.go`, `sprint/reasoning_test.go`, `study/prompts_test.go`, `app/*_test.go` label assertions). Nothing forces coherence: skipping one site compiles and passes its siblings' tests, yielding per-stage inconsistent precedence (e.g., plan honors project overrides while smoke does not). History shows this already happened: `sprintPromptTemplate`/`readWorkspaceFile` date from 50a065e/d1e4ab4/a5dcb78; the strict resolver arrived later (0e7294e) and older sites were never migrated.
- **Counter-evidence searched:** Each site's extra behavior is partly justified boundary translation — review assets add product-owned required-content validation (consistent with ARCHITECTURE.md "product modules own semantics"); study legitimately lacks a project tier (studies are workspace-scoped; PRD/TRD define only workspace overrides); project's allowlist check in `projectReasoningPromptTemplate` (sprint/prompts.go:260) gives good extension locality for new reasoning paths. None of this explains four copies of the fallback step itself.
- **Confidence:** high
- **Smallest useful action:** Add one helper in `internal/workspace` (e.g., `ResolveOverrideFile(root, rel) (content, source string, err error)`) implementing the documented rule once; have the four sites delegate and keep their own semantic validation layers on top. No new layer invented — it just moves the mechanism into its documented owner.

#### CHANGE-07-F02

- **Priority:** P2
- **Claim:** On the non-reasoning sprint planning paths, an unreadable existing override (or a default missing from the embedded registry) neither fails nor prefers a readable source — it embeds error text into the runtime prompt and proceeds.
- **Evidence:** `sprint/prompts.go:249-256`: non-NotExist read error → returns `"# Prompt Load Error\n\nCould not read …"` as template body; unknown rel → `"# Missing Prompt Default"`. These bodies flow unvalidated into `RenderPlanPrompt`/`RenderSprintIndexPrompt`/etc. and are used at execution time (`service.go:568,653,1126,1263` feed actual run flows, not only previews). Contrast within the same module: reasoning renderers fail closed first (`prompts.go:123-126,148-153`). Review assets fail via validation findings (`review.go:192-199`). Study propagates errors (`study/prompts.go:184-186`). Contract: TRD §10.2 :1111 — lookup must prefer “an intentional **readable** workspace override and otherwise use the embedded default.”
- **Architectural reason:** failure-semantics
- **Concrete consequence:** A chmod mishap or a directory placed at `templates/sprint-plan.md` makes the plan stage execute against instructions reading "# Prompt Load Error…" instead of the template; tokens are spent, the agent produces off-contract artifacts, and failure surfaces much later in stage validation with misleading attribution. Same for a newly added prompt path forgotten in the `defaultOverrideFiles` registry — every affected stage silently sends the missing-default page, whereas the strict resolver catches exactly this class at reasoning_defaults.go:78 with a hard error.
- **Counter-evidence searched:** Searched for downstream guards: no validator inspects rendered prompt text (`session_state_test.go:58` explicitly rejects an exact-match gate); nothing consumes the literal error strings. Checked whether inline rendering is an intentional dry-run affordance — no doc, comment, or test pins it; the same function serves preview and runtime. Edge-triggered (requires unreadable override or registry omission), hence P2 not P1.
- **Confidence:** medium (code facts verified high; impact conditional on edge trigger)
- **Smallest useful action:** In `sprintPromptTemplate`, treat non-NotExist read failures as errors returned from the Render* functions (which already return errors on reasoning paths) or fall back to the builtin; add one test covering the unreadable-override branch — currently no test anywhere references the `"# Prompt Load Error"`/`"invalid:"` branches.

### Defended architecture / rejected hypotheses

- **“Study prompts need a project tier”** — rejected: studies are workspace-scoped; PRD:233/TRD define workspace-level overrides only; the project tier was introduced specifically for project-bound reasoning defaults (commit 0e7294e). Two-tier study lookup matches scope.
- **“`appendInjectedReasoningTemplate` lacking builtin fallback is drift”** — rejected: it reads catalog-selected user-authored templates (`system/reasoning/*.md`), not shipped defaults; the registry cannot own them, and its failure-to-finding rendering is deliberate manifest reporting.
- **“Review asset substring validation is duplicated policy”** — rejected: it is product-semantic validation layered above the mechanical lookup, exactly the split ARCHITECTURE.md prescribes; only the lookup beneath it is duplicated (F01).
- **“Silent builtin fallback when `ResolveInside` errors is wrong”** — rejected as written: rel values are hardcoded literals, so that branch is effectively unreachable defensive code.
- **“Reasoning double-resolution (pre-check then re-resolve inside `renderPromptFromDefault`) is a defect”** — downgraded to noise: redundant but consistent today; consolidating per F01 removes it as a side effect.

### Open questions

- Was inline-error-text rendering chosen so dry-run previews always render deterministically (TRD “prompt builders must be deterministic”)? If intentional, it is undocumented and untested; the answer could downgrade F02 to a documentation gap.
- Do any external consumers (scripts/previews snapshots, web JSON passthrough) depend on exact source-label strings beyond the app tests? This would widen F01's edit surface if labels change.
