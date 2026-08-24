# SPECIALIST REPORT — Security/trust boundaries (independent b)

### Scope inspected

**Authoritative contracts/docs:** `ultraplan-workspace/system/contracts/core/security.md`, `surfaces/api-contracts.md`; repo docs `local-web.md`, `architecture.md`, `user-guide.md` (clone flags), `planning-smoke.md`.

**Implementation (`ultraplan-go` @ eeaa034):**
- Local HTTP / origin-host-CSRF: `internal/web/security.go`, `server.go`, `server_policy.go`, `routes.go`, `handlers.go`, `operation_handlers.go`, `operations.go` (hub), `run_handlers.go`, `artifacts.go`, `import_boundary_test.go`, `security_test.go`
- Workspace/artifact containment: `internal/workspace/{discovery,paths,init,skills}.go`, `internal/app/web_usecases.go` (`resolve`/`issue`/`containedArtifactPath`/`Artifact`), `internal/app/usecases.go` (`supportedPreviewPath`, `displaySafe`)
- Source-repo access: `internal/codeextract/{resolver,service}.go`, `internal/sprint/{execute_target,direct_inputs,prompt_context}.go`
- External execution: `internal/platform/process/{process,process_unix}.go`, `internal/sprint/{smoke,smoke_protocol,smoke_author,verify}.go`, `internal/study/{init,init_clone}.go`, agentwrap v0.0.0-20260821190033 options/runtime
- Env/secrets/logs: `internal/platform/config/{config,redaction}.go`, `internal/platform/logging/logging.go`, `internal/runcontrol/{sanitize,local_log,sqlite}.go`, `internal/app/durable_operations.go`
- Tests run: `go test ./internal/web ./internal/workspace ./internal/codeextract ./internal/runcontrol` and sprint smoke tests — all pass.

### Architecture assessment

The trust-boundary architecture is unusually coherent for a local tool. Each assigned concern has one owning mechanism, and the failure direction is consistently fail-closed:

- **HTTP authority**: numeric-loopback-only listen validation (`serve_commands.go:92-112`), authority re-derived and re-validated from the bound listener (`server.go:68-75`), exact Host match, exact-Origin mutations, HMAC-signed per-process session cookie (HttpOnly, SameSite=Strict) plus an independent CSRF proof (API header at `security.go:154`; HTML `_csrf` form field in each mutation handler). Port-stripped-Origin tolerance requires `Sec-Fetch-Site: same-origin` **and** an exact Referer match (`security.go:279-297`) — defense-in-depth, not a loosening; tests cover rejection without those proofs.
- **Containment layering is real, not decorative**: artifact previews require an issued opaque ref (in-memory map) *and* `ResolveInside` *and* symlink-evaluated `Rel` containment *and* regular-file check *and* a 256 KiB+1 bounded read (`web_usecases.go:700-747, 892-910`); the web adapter's only allowed import is `internal/app` (enforced by `import_boundary_test.go`), so no arbitrary filesystem reader can be added quietly.
- **Execution**: one bounded process boundary (`platform/process`: mandatory positive timeout, stdout/stderr caps, setpgid + group TERM/KILL with grace); smoke harness executables must live inside a cataloged harness root, resolved through symlink-canonicalized containment (`smoke_protocol.go:130-176`), with env names restricted to the configured allowlist where manifest requests can only intersect it, never extend it (`smoke_protocol.go:623-642`) — fail-closed.
- **Secrets/redaction**: a single choke-point (`config.RedactValue`/`Sensitive`) backs `config show`, the structured logger, TUI display (`displaySafe`), runtime error mapping (`runtime.go:669-715`), and SSE/result projections (`safeProjectedText`). Durable events pass a final allowlist gate at the repository boundary (`sqlite.go:617` → `sanitizeEventDraft`), so nothing upstream can bypass it.
- **Trust model honesty**: `local-web.md:84-113` explicitly disclaims loopback as an authentication boundary against hostile local accounts; the session/CSRF machinery targets browser-origin threats only, which matches its implementation.

Two minor stress points found; both are P3 (below).

### Candidate findings

#### SPECIALIST-21B-F01
- **Priority:** P3
- **Claim:** Study-init `git clone` executes outside UltraPlan's bounded process boundary: no timeout, no context cancellation wiring, no output-size cap before buffering.
- **Evidence:** `internal/study/init_clone.go:44-50` — `exec.Command("git", "clone", "--depth", "1", url, dest)` + `cmd.CombinedOutput()`. Contrast `platform/process/process.go:60-152` where timeout/caps/group-kill are mandatory, and `smoke.go:75,128` which route all subprocesses through it. Caller chain confirmed unbounded: `app/study_commands.go:1390` → `study.Init` (`init.go:84`) → `runCloneActions`. Destination and URL handling are otherwise sound (`init.go:131-146,173-177` contains dest inside the study root; error output is credential-redacted and truncated at `init_clone.go:52-65`).
- **Architectural reason:** boundary / failure-semantics — two execution boundaries with different failure semantics for the same operation class (network subprocess). SEC-NET-001 requires bounded outbound calls; every other external process path in the repo satisfies this.
- **Concrete consequence:** a hung clone (unreachable remote, huge repo, tty-less credential/host-key prompt edge cases) blocks `study init` indefinitely when run from scripts/CI; `CombinedOutput` also accumulates the full output in memory before truncation. Interactive Ctrl-C mitigates but does not bound the non-interactive case.
- **Counter-evidence searched:** searched callers and app layer for a timeout wrapper (none); searched docs (`user-guide.md:112-119` documents `--dry-run`/`--no-clone` as the mitigation, no bound promised); tests use a fake runner with no timing semantics (`init_test.go:70-89`). Stdin is not connected, which prevents most interactive credential hangs — partial mitigation only.
- **Confidence:** high
- **Smallest useful action:** wrap the clone in `exec.CommandContext` with a default timeout (or reuse `platform/process.Runner`), keeping the existing redaction.

#### SPECIALIST-21B-F02
- **Priority:** P3
- **Claim:** The execute/review/smoke source-repository boundary is pinned to a developer-machine-specific absolute path hardcoded in product source, while all other workspace inputs are governed content.
- **Evidence:** `internal/sprint/execute_target.go:11` — `const ApprovedExecuteTargetPath = "/home/antonioborgerees/coding/ultraplan/ultraplan-go"`; enforced fail-closed at `execute_target.go:22-24` ("this sprint only approves %s"); asserted by `execute_target_test.go:13`.
- **Architectural reason:** change-surface / drift — a machine-specific policy constant lives in source rather than governed workspace input or build configuration; retargeting UltraPlan to any other repository (the tool's core purpose) requires a code edit and rebuild. `docs/planning-smoke.md:12,66` reads as if the target directory is a workspace-authored input.
- **Concrete consequence:** on any other machine or after relocating the repo, execute/review/smoke resolution always fails closed with "unsupported execute target"; contributors outside the author's home layout cannot use the product's central workflow without patching source. The constraint is secure (fail-closed, tested) but its ownership sits in the wrong place for a product whose other boundaries (harness root, sources, docs) are all workspace-governed.
- **Counter-evidence searched:** the finding message and test show this is deliberate current-sprint scoping (CURRENT-CONTRACT, intentional debt), not an accident; asymmetry with the catalog-driven smoke harness root is explicit. No doc declares it permanent.
- **Confidence:** high (facts) / medium (that it is unintended long-term)
- **Smallest useful action:** none required for security; if kept, add one sentence to `docs/architecture.md` stating the target is compile-time-pinned this sprint, so the planning-smoke instructions don't imply configurability.

### Defended architecture / rejected hypotheses

1. **"`webUseCases.refs` grows without bound" — rejected.** `issue()` (`web_usecases.go:870-883`) HMACs kind+values with a process-fixed secret, so ref strings are deterministic; repeated requests re-insert identical map keys. Map size is bounded by distinct workspace entities ever seen, not by request count. No eviction needed at current scope.
2. **"Session/CSRF tokens are handed out before policy rejection, leaking them cross-site" — rejected.** `Set-Cookie`/`X-CSRF-Token` are written before the rejection switch (`security.go:110-119`), but responses carry no CORS headers, the cookie is HttpOnly+SameSite=Strict, and browsers cannot read cross-origin response headers. The disclosed threat (hostile local account) can connect anyway by design (`local-web.md:109-113`), so this leaks nothing across the stated boundary.
3. **"Port-stripped Origin tolerance weakens mutation checks" — rejected.** The fallback requires browser-controlled fetch metadata (`Sec-Fetch-Site: same-origin`) that a cross-site page cannot produce, plus byte-exact Referer equality with the expected authority (`security.go:283-297`); mutations additionally need session + CSRF + single-use confirmation bound to canonical request and fingerprint (`security.go:412-436`). Tests `TestSecurityRejectsPortStripped*` cover the negative cases.
4. **"Deterministic secret fallbacks weaken HMACs" — rejected as impractical.** Both `securityMiddleware.secret` (`security.go:96-98`) and `webUseCases.secret` (`web_usecases.go:257-259`) degrade only if `crypto/rand.Read` fails, which does not occur on supported platforms; ref validity is anyway enforced by map membership, not HMAC verification.
5. **"Durable operation status/cancel by ID is a cross-session authorization gap" — rejected as documented contract.** `handleOperationStatus`/`CancelRun` fall back to durable runs visible across sessions (`operation_handlers.go:157-171,194-216`), matching `local-web.md:137-141,303-306` (workspace-visible durable state, session binding only for the ephemeral hub). Mutation routes still enforce Host/Origin/session/CSRF.
6. **"Prompt/source composition can read arbitrary paths" — rejected.** Every prompt input read funnels through `workspace.ResolveInside` (`direct_inputs.go:29-44`), codeextract resolves sources only inside workspace-contained roots with symlink evaluation (`resolver.go:49-62,113-134`), and the execute workdir must sit inside the approved absolute target (`execute_target.go:36-52`).
7. **"Smoke authoring/harness changes escape governance" — rejected.** Authoring writes are checked against the manifest path allowlist (`smoke_author.go:86`), harness executable/cwd/evidence roots are symlink-canonicalized into the harness root (`smoke_protocol.go:153-176`), and env names are validated at config load (`config.go:458-462`) with intersection-only extension at runtime.

### Open questions

1. Is bounding `study init` clones (F01) already planned under a later sprint's runtime-work contract? If yes, treat F01 as FUTURE-INTENT, not debt.
2. Is the compile-time-pinned execute target (F02) intended to become a governed workspace/build input once multi-target support lands, or is single-target the stable product shape?
