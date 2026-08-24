The core of my assignment is the process execution layer. Let me read the platform process package and runcontrol process files in parallel.
Now the consumers: runcontrol's process wrapper, the smoke harness, and the runtime agent wrappers.
Now the smoke harness and agent wrappers — the primary external-process consumers.
Now the smoke authoring/protocol files, which handle harness selection and env construction.
Manifest env handling looks suspicious (`smokeEnvironment` seems to make `manifest.Environment` dead). Let me read the authoring path and service wiring before judging.
Now the runtime agent wrapper (opencode.go, agentwrap.go) — the other major external process boundary.
The `manifest.Environment` loop looks provably inert. Let me check the real harness manifest to see whether it declares env names beyond the default allowlist, plus how config extends the allowlist.
The real cataloged harness declares `"environment": ["OPENCODE_MODEL", "OPENCODE_TIMEOUT"]`, which are not in the default allowlist. Let me check history for whether manifest env was ever honored, and verify my dead-code reading with a quick test.
`OperationSmokeStart` from web/TUI builds a service without a stage runtime — that would fail authoring. Let me verify how web wires operations before concluding.
`OperationSmokeStart` is the only operation not using `sprintRuntimeService`. Checking whether web gates it to dry-run or a test covers this.
