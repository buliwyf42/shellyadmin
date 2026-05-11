# Release Plans

Per-release implementation plans referenced from the top-level
`docs/roadmap.md`. Each plan is the document an implementer reads
before opening their first PR against a release block.

## Active

- [Phase 4b — Services Split + Frontend Refactor](./phase-4b-refactor-block.md)
  (M7, M8, M2, M6 — XL block, 5–10 sessions)
- [Phase 4c — Auth Strategics](./phase-4c-auth-strategics.md)
  (T1 TOTP, T3 PATs — 4–6 sessions; parallel to 4b)
- [v0.3.0 Release Cut](./v0.3.0-release-cut.md)
  (breaking: S6 encryption-key hard-fail, ADR-0015 runtime_locks)

## Done

- v0.2.10 Phase 1 (Q1–Q21) — supply-chain hardening + login defense
- v0.2.11 Phase 2 (S1–S21) — server sessions, audit hash-chain,
  cosign signing, encryption-key deprecation warning
- v0.2.12 Phase 3 (M1, M3, M4, M5, M9, M10, M11, M12, M13) —
  handler.go split, schema drift check, /metrics, OpenAPI route
  coverage
- v0.2.13 Phase 4a (T5, T6, T7, T10, T11, T12, S6-docs) —
  ADRs 0013/0014/0015, argon2 m=96 MiB, audit webhook sink

The originating consolidated-review plan lives outside this repo
(in `~/.claude/plans/du-agierst-als-erfahrener-indexed-seahorse.md`)
and stays the canonical reference for the IDs (Q*, S*, M*, T*) used
in commit messages and this folder's filenames.
