# Failure-scenario catalogue

Each scenario is an end-to-end ownership trace, not a speculative brainstorming exercise.

1. Crash during durable run acceptance
2. Owner death after external work begins
3. Stale fenced owner writes after takeover
4. CLI/web concurrent mutation of the same scope
5. Browser/SSE disconnect during work
6. Graceful server shutdown with active workers
7. Cancellation racing terminal completion
8. Slow SSE subscriber / event-delivery backpressure
9. SQLite or filesystem failure mid-transition
10. Restart/migration from prior state formats
11. Smoke-harness timeout with partial evidence
12. Corrupt or stale product/run state

For each, identify who owns truth before, during, and after failure; what survives restart; whether authorities can disagree; and who resolves ambiguity.
