# Finding contract

A candidate finding is only useful when it contains:

- stable task-local ID
- P0/P1/P2/P3 priority
- precise claim
- concrete file:line or symbol evidence
- architectural reason: ownership, authority, boundary, lifecycle, drift, change surface, or failure semantics
- concrete consequence
- counter-evidence searched
- confidence
- smallest useful action

Challenge rules:

- Two independent observers with strong evidence and failed falsification => confirmed.
- One strong observer => may survive as `single-observer`.
- Weak one-observer claims => open question, not finding.
- Documented intentional rationale => defended architecture or accepted debt.
- Necessary boundary translation => not duplicated authority.
- Future-only requirement => future risk, not current defect.
- Code-quality/style-only complaint => exclude from architecture report.
- Recommendation that merely adds indirection => reject.
