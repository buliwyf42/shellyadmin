# ADR-0004: Job Concurrency and Execution Semantics

- Status: `Accepted`
- Date: 2026-04-09

## Context

Even with single-user operation, job overlap rules must be explicit to avoid accidental conflicts.

## Decision

- `scan` is exclusive (only one scan at a time).
- Other operations can run in parallel when safe.
- Provisioning is allowed while scan is not running.
- Firmware update can run in parallel with provisioning only on disjoint device sets.
- If overlap exists between firmware and provisioning targets, overlapping devices are excluded and the rest continue.

## Consequences

- Throughput remains practical without requiring global serialization of all operations.
- Scan keeps clear ownership of network discovery workload.
- Device-target overlap checks become part of job admission control.
