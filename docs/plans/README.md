# Release Plans

Archival per-release implementation plans referenced from the
top-level `docs/roadmap.md`. Each plan is the document the implementer
read before opening the first PR against a release block. These files
are kept for historical context; current planning lives in
`docs/roadmap.md` and in the issue tracker.

## Shipped

- [Phase 4b — Services Split + Frontend Refactor](./phase-4b-refactor-block.md)
  (M7, M8, M2, M6) — landed across v0.2.x → v0.3.x
- [Phase 4c — Auth Strategics](./phase-4c-auth-strategics.md)
  (T1 TOTP, T3 PATs) — landed in v0.3.0
- [v0.3.0 Release Cut](./v0.3.0-release-cut.md)
  (breaking: S6 encryption-key hard-fail, ADR-0015 runtime_locks)
- v0.2.10 Phase 1 (Q1–Q21) — supply-chain hardening + login defense
- v0.2.11 Phase 2 (S1–S21) — server sessions, audit hash-chain,
  cosign signing, encryption-key deprecation warning
- v0.2.12 Phase 3 (M1, M3, M4, M5, M9, M10, M11, M12, M13) —
  handler.go split, schema drift check, /metrics, OpenAPI route
  coverage
- v0.2.13 Phase 4a (T5, T6, T7, T10, T11, T12, S6-docs) —
  ADRs 0013/0014/0015, argon2 m=96 MiB, audit webhook sink

The originating consolidated-review plan that introduced the
Q\*/S\*/M\*/T\* identifier scheme used in commit messages and this
folder's filenames lives in the maintainer's private notes and is not
checked in.
