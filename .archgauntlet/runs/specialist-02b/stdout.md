Single module with ~12 internal packages. Let me map the actual import graph with the Go toolchain — both non-test and test imports — plus check for cycles.
Clean DAG so far. Now test imports and cycle detection:
Zero cycles including test imports. Two packages have `import_boundary_test.go` — explicit architectural enforcement worth reading. Let me inspect those plus the doc.go contract statements:
productstate is a generic kind/scope blob store — but its DB path is `.ultraplan/run-control.db`, which smells like shared file ownership with runcontrol's own sqlite. Checking both:
Both `productstate` and `runcontrol` open the **same** `.ultraplan/run-control.db` through independent pools. Now checking whether runcontrol's migration logic operates on that whole file (which would make it own productstate's tables too):
runcontrol's migration owns `user_version`, backs up and restores the **entire** file, and treats any pre-existing non-empty schema as legacy input. Now I need the call order: who opens productstate vs runcontrol first, and where:
