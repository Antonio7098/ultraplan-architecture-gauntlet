package gauntlet

import "fmt"

var stages = []string{"scout", "generalist", "specialist", "failure", "change", "challenge", "chair", "synth", "arbiter"}

func Stages() []string { return append([]string(nil), stages...) }

type namedFocus struct{ Title, Focus string }

func BuildTasks() []Task {
	var tasks []Task
	add := func(stage, id, title, role, focus string, broad bool) {
		tasks = append(tasks, Task{ID: id, Stage: stage, Title: title, Role: role, Focus: focus, Broad: broad, Status: StatusPending})
	}

	scouts := []namedFocus{
		{"Repository cartography", "Map packages, entrypoints, major files, manifests, generated areas, hotspots, and high fan-in/fan-out units. Produce evidence, not architectural verdicts."},
		{"Dependency cartography", "Build the actual Go package/import graph. Identify cycles, dependency directions, cross-module imports, and unusual fan-in/fan-out."},
		{"Product-contract cartography", "Map PRD, TRD, architecture, project index, roadmap, and current-vs-future requirements. Classify evidence as REALITY, CURRENT-CONTRACT, FUTURE-INTENT, or HISTORY."},
		{"Governance cartography", "Map architecture contracts, review protocols, reasoning documents, tests that encode architecture, and any stated invariants or accepted debt."},
		{"State/lifecycle cartography", "Inventory durable, derived, and ephemeral state: JSON/Markdown artifacts, SQLite, locks, run state, product state, leases, attempts, events, and terminal outcomes."},
		{"Execution cartography", "Trace CLI/TUI/web through app/product/runtime/process/agentwrap boundaries, including cancellation, shutdown, restart, and observation paths."},
	}
	for i, s := range scouts {
		add("scout", fmt.Sprintf("scout-%02d", i+1), s.Title, "arch-scout", s.Focus, true)
	}

	gens := []namedFocus{
		{"Whole-system ownership review", "Blind whole-system review emphasizing concept ownership, authority, cohesion, and boundaries."},
		{"Whole-system simplicity review", "Blind whole-system review emphasizing accidental complexity, duplicated knowledge, unearned abstraction, and change cost."},
		{"Whole-system failure review", "Blind whole-system review emphasizing correctness under crash, cancellation, restart, partial progress, concurrency, and recovery."},
		{"Whole-system evolution review", "Blind whole-system review emphasizing evolution, change locality, architectural drift, agent legibility, and future extension paths."},
	}
	for i, g := range gens {
		add("generalist", fmt.Sprintf("generalist-%02d", i+1), g.Title, "arch-orchestrator", g.Focus, true)
	}

	lenses := []namedFocus{
		{"Module ownership and authority", "Who owns each important concept and rule? Find competing authorities, hidden delegation, or ambiguous sources of truth."},
		{"Go dependency graph", "Package direction, cycles, cross-module imports, fan-in/fan-out, boundary bypass, shared helpers that became domain owners."},
		{"Study lifecycle", "Study definitions, scheduling, run-loop, applicability, analysis/synthesis, state, reports, validation, and recovery ownership."},
		{"Project catalogue", "Project discovery, project-index contracts, catalogue semantics, configuration ownership, and separation from study/sprint."},
		{"Sprint state machine", "Stage ordering, validation, flow state, artifact authority, prerequisites, transitions, resume and retry semantics."},
		{"Execute-review-smoke", "Ownership and separation across execute, review, smoke, external harness evidence, verdicts, and source mutation boundaries."},
		{"Runcontrol", "Durable run acceptance, IDs, attempts, leases, heartbeats, fencing, ordered events, cancellation, reconciliation, terminal arbitration."},
		{"Productstate", "SQLite product-state store, schema ownership, physical-vs-semantic coupling, migrations, transactional semantics, and concurrency."},
		{"Operational vs product state", "Test whether operational execution facts and product workflow truth remain separate and reconcilable under every path."},
		{"App composition/use cases", "Composition root, dependency construction, shared use cases, CLI/TUI/web reuse, typed boundaries, and business logic leakage."},
		{"CLI/TUI/web parity", "Surface isolation, duplicated workflow logic, command-as-protocol leakage, presentation state, and interface-specific authority."},
		{"Web lifecycle and SSE", "HTTP/SSE transport, subscribers, buffering/backpressure, browser disconnect, worker lifetime, draining, shutdown, restart and security boundaries."},
		{"Runtime/agentwrap/OpenCode", "Generic runtime contract, adapter leakage, runtime supervision ownership, model/provider concepts, events, cancellation, result translation."},
		{"Process/smoke harness", "External process boundary, argv/env/cwd/timeouts, process groups, evidence ownership, harness selection, cancellation, and cleanup."},
		{"Filesystem/artifact persistence", "Path ownership, atomic writes, directory contracts, artifact authority, partial writes, locking, consistency, and cross-platform assumptions."},
		{"Configuration/model selection", "Config precedence, model selection, override semantics, redaction, ownership, hidden globals, and drift across surfaces."},
		{"Errors/retry/idempotency/recovery", "Error taxonomy, wrapping, retry safety, partial progress, idempotency, cancellation races, cleanup uncertainty, and actionable recovery."},
		{"Observability", "Events, logs, metrics, correlation, truthfulness, event retention, sanitization, operational diagnosis, and duplicated projections."},
		{"Concurrency/resources", "Goroutine ownership, worker pools, boundedness, race surfaces, locks, channels, backpressure, cancellation propagation, and cleanup."},
		{"Test architecture", "Test seams, fake boundaries, fault injection, import/architecture tests, brittle tests, missing invariant tests, and testability of policy."},
		{"Security/trust boundaries", "Workspace paths, source repo access, external execution, env/secrets, local HTTP, CSRF/origin/host policy, log/event sanitization."},
		{"Agent architecture", "Prompt ownership, context packs, code-context, skills, evidence injection, caching assumptions, prompt drift, and agent-facing authority."},
		{"Simplicity and earned complexity", "Deep modules vs god modules, speculative interfaces, generic engines, duplicated knowledge, pass-through abstractions, and accidental ceremony."},
		{"Evolution/change locality", "How architecture absorbs new stages, surfaces, runtimes, state versions, events, policies, review strategies, and future roadmap changes."},
	}
	for i, l := range lenses {
		for _, twin := range []string{"a", "b"} {
			add("specialist", fmt.Sprintf("specialist-%02d%s", i+1, twin), l.Title+" (independent "+twin+")", "arch-inspector", l.Focus, false)
		}
	}

	failures := []namedFocus{
		{"Crash during durable acceptance", "Trace a process crash between request acceptance, durable run creation, owner acquisition, and work start."},
		{"Owner death after work begins", "Trace owner/process loss after external work starts but before product state reaches a terminal result."},
		{"Stale fenced owner", "Trace a stale owner attempting heartbeat/event/terminal writes after a new fenced attempt owns the run."},
		{"CLI/web concurrent mutation", "Trace two surfaces attempting overlapping mutation against the same project/sprint/study scope."},
		{"Browser disconnect", "Trace SSE/browser disconnect during a long operation; determine whether work continues and how observation resumes."},
		{"Server shutdown", "Trace graceful shutdown with active workers, slow subscribers, cleanup, cancellation, persisted truth, and uncertain outcomes."},
		{"Cancellation vs completion race", "Trace cancellation racing runtime/process completion and product-stage terminalization."},
		{"Slow SSE subscriber", "Trace a slow or disconnected SSE client and prove whether it can block or corrupt product/runtime progress."},
		{"Persistence failure", "Trace SQLite/filesystem failure mid-transition: partial writes, transaction rollback, artifact mismatch, and recovery."},
		{"Migration/restart", "Trace startup with previous product/run-state formats and interrupted migration/reconciliation."},
		{"Smoke harness timeout", "Trace harness timeout/process-tree cleanup with partial evidence and review/smoke state updates."},
		{"Corrupt or stale state", "Trace malformed/stale product or run state and determine which source wins, what is quarantined, and what recovery is safe."},
	}
	for i, f := range failures {
		add("failure", fmt.Sprintf("failure-%02d", i+1), f.Title, "arch-inspector", f.Focus, false)
	}

	changes := []namedFocus{
		{"Add sprint stage", "Without implementing, trace every required edit to add a new governed sprint stage and assess unrelated change surface."},
		{"Add local interface", "Trace every required edit to add another local UI surface over existing use cases without duplicating workflow semantics."},
		{"Add runtime adapter", "Trace adding a second runtime/provider adapter while preserving generic runtime contracts and product semantics."},
		{"Add run event", "Trace adding a new durable run event end-to-end: schema, persistence, projection, replay, surfaces, retention, tests."},
		{"Change product-state schema", "Trace a schema evolution across persistence, migration, product owners, recovery, and compatibility."},
		{"Add review/smoke strategy", "Trace adding a new review or smoke strategy without creating parallel workflow authorities."},
		{"Change reasoning resolution", "Trace changing project/workspace/builtin reasoning or prompt resolution precedence and its validation/test surface."},
		{"Add execution status", "Trace adding/changing a lifecycle status across operational run control, product state, surfaces, events, docs, and tests."},
	}
	for i, c := range changes {
		add("change", fmt.Sprintf("change-%02d", i+1), c.Title, "arch-inspector", c.Focus, false)
	}

	domains := []namedFocus{
		{"Product/domain tribunal", "Challenge candidate findings concerning study, project, sprint, artifacts, prompt/context ownership, and module boundaries. Try to make weak findings disappear."},
		{"Execution tribunal", "Challenge findings concerning runcontrol, runtime, process, concurrency, ownership leases, cancellation, terminal arbitration, and failure semantics."},
		{"State tribunal", "Challenge findings concerning filesystem, SQLite/productstate, migrations, durable/derived/ephemeral truth, consistency, and recovery."},
		{"Interface tribunal", "Challenge findings concerning app composition, CLI/TUI/web, HTTP/SSE, presentation state, and cross-surface parity."},
		{"Quality tribunal", "Challenge findings concerning security, errors, observability, testing, performance, and operational claims."},
		{"Evolution tribunal", "Challenge findings concerning simplicity, abstraction, change locality, agent legibility, documentation drift, and roadmap fitness."},
	}
	for i, d := range domains {
		add("challenge", fmt.Sprintf("challenge-%02d", i+1), d.Title, "arch-challenger", d.Focus, true)
	}
	for i, d := range domains {
		add("chair", fmt.Sprintf("chair-%02d", i+1), d.Title+" synthesis", "arch-chair", d.Focus, true)
	}
	for i, focus := range []string{
		"Whole-system synthesis emphasizing correctness, failure safety, state authority, and operational integrity.",
		"Whole-system synthesis emphasizing ownership clarity, cohesion, simplicity, dependency boundaries, and architectural drift.",
		"Whole-system synthesis emphasizing evolution, change locality, agent legibility, roadmap fit, and fitness-function opportunities.",
	} {
		add("synth", fmt.Sprintf("synth-%02d", i+1), fmt.Sprintf("Independent system synthesis %d", i+1), "arch-synth", focus, true)
	}
	add("arbiter", "arbiter-01", "Chief architect arbiter", "arch-synth", "Resolve presentation, priority, and disputes across the three independent syntheses and six chairs. Do not invent findings. Produce the final architecture-review.md package.", true)
	return tasks
}
