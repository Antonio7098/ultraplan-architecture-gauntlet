Prior-stage evidence gathered. The upstream findings in my domain come from scout-05, specialist-07a/b (runcontrol), specialist-08a/b (productstate), specialist-17b (recovery), failure-03/07/10/12. Note: specialist-15a/b (filesystem/artifact persistence) and 09a/b (operational vs product state) produced **empty outputs** — those lenses are uncovered, so I'll do independent discovery there too.

Now orienting directly in the target repo before dispatching subagents:
The sanitize-drift chain is verified end-to-end from primary sources, including the pinned agentwrap module (OpenCode `text` events carry content under `text`, which the allowlist drops). Now verifying the shared-file/restore/migration cluster myself:
