## Context

The current export path maps normalized gauge rows and writes them to ClickHouse before doing the same for sum rows. Each populated kind already benefits from request-scoped batching and mapper-level metadata deduplication, but mixed requests still serialize the two kinds even though they target separate tables and do not depend on each other.

This change is intentionally narrow. The repository is not ready to take on async buffering, cross-request write coalescing, retry queues, idempotency keys, or transactional coordination across metadata and datapoint tables. The immediate goal is to improve the throughput ceiling of one export request while keeping the write model simple, synchronous, and easy to verify in unit and integration tests.

## Goals / Non-Goals

**Goals:**
- Preserve the existing per-request batch insert model for normalized gauge and sum ingestion.
- Allow gauge and sum persistence work for the same export request to proceed independently when both kinds are present.
- Keep the existing per-kind ordering guarantee that metadata is inserted before datapoints for that kind.
- Make the write-path failure semantics explicit so tests and future changes do not assume retries or atomicity that the service does not provide.

**Non-Goals:**
- Adding background workers, async buffers, or cross-request batching.
- Adding retry, replay, or idempotency mechanisms.
- Providing all-or-nothing transactional guarantees across metadata and datapoint tables or across gauge and sum kinds.
- Changing the OTLP/gRPC API surface or introducing a public read/write coordination layer.

## Decisions

### Decision: Split one export into independent per-kind persistence pipelines
The export handler will continue to derive normalized rows for gauge and sum data, but once those batches are prepared it will treat each populated kind as its own persistence pipeline. A gauge pipeline performs gauge metadata insert followed by gauge datapoint insert. A sum pipeline performs sum metadata insert followed by sum datapoint insert.

When both pipelines are present, they should run independently so one kind does not need to wait for the other kind to finish before beginning storage work. This is the smallest useful throughput improvement because the two kinds already target separate tables and share no write-time dependency.

Alternatives considered:
- Keep full serialization across kinds: rejected because it leaves throughput on the table for mixed requests without adding any safety guarantee.
- Introduce finer-grained parallelism inside one kind: rejected because metadata must remain ahead of datapoints for that kind, and more granular concurrency would add complexity faster than value.

### Decision: Keep per-kind sequencing and current request-scoped batching
Within each kind, the service will continue to write one metadata batch and one datapoint batch per request, in that order. This preserves the existing mental model and keeps the `MetadataKey` reference flow straightforward.

Alternatives considered:
- Write datapoints before metadata: rejected because it weakens the normalized storage contract and makes partial-write behavior harder to reason about.
- Split one kind into multiple smaller batches within the export handler: rejected because it does not address the current bottleneck and would complicate tests and store interactions.

### Decision: Explicitly document basic synchronous failure semantics
If any storage step fails, the export call should fail and return that error. The implementation will not add internal retries, compensation, or background recovery. Because the write path remains synchronous and non-transactional, partial persistence across tables or kinds remains possible and must be documented as an accepted trade-off for this stage of the project.

Alternatives considered:
- Add retries inside the request path: rejected because it introduces policy questions, duplicate-write risk, and longer tail latency without an idempotency story.
- Add transactional coordination across tables: rejected because ClickHouse and the current storage model do not justify that complexity for this step.

## Risks / Trade-offs

- [A failed export may still leave some rows persisted] -> Accept and document this explicitly; add tests so future contributors understand the non-atomic behavior.
- [Concurrency could make unit tests more timing-sensitive] -> Verify behavior through controlled fake stores and ordering signals rather than sleep-based assertions.
- [The change could become a stepping stone to premature buffering features] -> Keep the scope limited to in-request concurrency and call out buffering, retries, and coalescing as non-goals.

## Migration Plan

1. Add the new ingestion capability spec that defines batching, per-kind sequencing, independent kind execution, and failure semantics.
2. Update the export handler and any small supporting helpers needed to execute populated gauge and sum pipelines independently.
3. Extend unit tests with fake-store coverage for mixed requests, per-kind ordering, skipped empty kinds, and failure propagation.
4. Extend integration coverage only as needed to confirm the persisted rows remain correct under the updated execution model.

## Open Questions

- None for this scoped change. If higher sustained throughput becomes a measured problem later, that should be proposed separately with concrete requirements for buffering, retries, and idempotency.
