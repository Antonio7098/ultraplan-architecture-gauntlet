Non-test import graph extracted. No obvious cycle yet. Let me check test-file imports too, plus per-file sizes to find hotspots.
The first extraction was mangled by newlines in the join. Let me re-extract properly.
Full graph captured. Now reading the key packages to understand ownership. Starting with `project`, `workspace`, `platform/runtime`, and the composition root.
There's an `import_boundary_test.go` in runcontrol — the repo may already enforce import rules. Reading the package docs and boundary tests first.
Docs state explicit ownership doctrine. Now examining `platform/runtime` — a "platform"-named package consumed by both domain modules (sprint, study) and app.
`platform/runtime` is an anti-corruption layer over the `agentwrap` SDK — mapping foreign types to owned types. Now checking its siblings (`events.go`, `policy.go`, `opencode.go`) and then how sprint/study consume it.
web→app-only is an *enforced* boundary. Checking whether tui/app have similar boundary tests, and whether `platform/filesystem` is dead code (no importer found in the graph).
`platform/filesystem` is a documented future-boundary stub (doc.go only). Now checking `codeextract` consumers — study's doc says code-reference extraction is out of scope there, yet sprint has `code_context.go`. Checking for parallel implementations.
`codeextract` (study-side citation resolution) vs `sprint/code_context.go` (sprint artifact validation) are distinct concepts. Now examining the two SQLite owners — `runcontrol` and `productstate` — for state-ownership coherence.
Notable: `productstate` writes to `.ultraplan/run-control.db`. Checking what file `runcontrol` itself uses — possible shared-file collision between the two SQLite owners.
Both packages open the **same** `.ultraplan/run-control.db` file with independent pools and schemas. Comparing DSNs/PRAGMAs and checking whether runcontrol's retention/migration knows about productstate's tables.
Both DSNs confirmed against the same file, with drift already present (`_defensive=1` only in runcontrol). Now checking: who uses `productstate`, whether the duplicated path constant is test-enforced, and productstate's lifecycle (Close?).
`productstate` has no `Close`, no pragma verification, no file-mode enforcement, no migration lock — unlike its co-tenant. Checking how app wires the two stores and what sprint/study store in productstate.
The DB mirrors file-based state. Now checking runcontrol's retention scope (does it touch only its own tables?) and what the authoritative workspace docs say about DB ownership.
Authority handoff (file↔DB) is explicit and gated — good. Now checking who else calls `productstate.Ensure` (file creation paths bypassing runcontrol's mode enforcement) and study's equivalent wiring.
