## Context

The current normalized schema cleanly separates datapoints from metadata, and the datapoint tables are already partitioned and ordered for time-range pruning. The ambiguity is not the write path or schema shape, but the read flow: some queries need metadata first to identify which metric series are relevant, and only then can they fetch datapoints for those series.

This change is limited to the normalized gauge and sum storage model. It must preserve append-only metadata writes, `ReplacingMergeTree` replacement semantics, and compact metadata identity generation while making the supported read pattern explicit and testable. It should avoid speculative schema changes until there is evidence that metadata discovery itself is a bottleneck.

## Goals / Non-Goals

**Goals:**
- Clarify the supported normalized query flow for the common case where callers identify matching metric series before fetching datapoints.
- Guarantee that once the relevant `MetadataKey` values are known, datapoint retrieval remains time-bounded on the datapoint tables.
- Preserve the existing metadata identity model and `ReplacingMergeTree` deduplication behavior.
- Document the two-step behavioral contract and include one illustrative query example without requiring a single exact SQL shape.

**Non-Goals:**
- Adding a public read API to the gRPC service.
- Repartitioning metadata tables by time or storing duplicate time columns in metadata rows.
- Adding new metadata-side indexes, materialized views, or helper tables before real query workloads justify them.
- Promising that metadata discovery queries will never scan metadata tables.

## Decisions

### Decision: Support metadata-first series discovery
Normalized reads will explicitly allow a first step that queries the metadata tables to discover which series match filters such as metric name, service name, or datapoint attributes. That step returns the relevant logical metadata rows and their `MetadataKey` values, which the caller can then use to fetch datapoints.

Alternatives considered:
- Force every read to start from datapoints: rejected because it does not match the common query flow where users need to know which series exist before asking for datapoints.
- Keep the current ad hoc join examples: rejected because they demonstrate correctness of joins but do not describe the intended two-step flow.

### Decision: Keep datapoint retrieval bounded after series selection
After the relevant `MetadataKey` values have been identified, datapoint queries will use those keys together with the requested time predicate to read only the relevant partitions and rows from the datapoint tables. This keeps the existing time-range guarantee where it matters most: on the high-volume datapoint tables.

Alternatives considered:
- Add event-time columns and partitions to metadata tables: rejected because metadata rows do not have stable datapoint-time semantics and this would complicate replacement behavior.
- Add secondary indexes for every likely metadata filter field now: rejected as premature optimization without evidence of a real metadata-discovery bottleneck.

### Decision: Capture the contract in documentation and focused tests
The implementation should document the two-step query flow and add focused integration coverage that exercises it end to end. The documentation will describe the behavioral contract, include one illustrative series-discovery example, and avoid prescribing a single mandatory SQL form. The first implementation will keep the flow expressed directly in integration tests and documentation rather than extracting a shared SQL helper before a real read API exists.

Alternatives considered:
- Add a new production read layer up front: rejected because the service does not yet expose a read API and that would be unnecessary design work.
- Assert exact ClickHouse physical plans: rejected because it would optimize for implementation detail rather than the behavior we need to guarantee.

## Risks / Trade-offs

- [Metadata discovery may scan the metadata tables] -> Accept this for now because metadata volume is lower than datapoint volume, and revisit only if measured workloads show a problem.
- [Two-step reads require callers to carry `MetadataKey` values between steps] -> Document the flow clearly and add integration examples that show the expected handoff.
- [The implementation could grow into a premature read abstraction] -> Keep the change scoped to docs, tests, and only the smallest helper extraction if repetition makes it worthwhile.

## Migration Plan

1. Update the spec delta to distinguish metadata discovery from time-bounded datapoint retrieval.
2. Add or update query examples in tests or docs so the two-step flow is concrete without freezing one exact SQL shape.
3. Extend integration coverage and storage documentation to verify and explain the supported query pattern.
4. Revisit metadata-side optimizations only if real usage demonstrates a bottleneck.
