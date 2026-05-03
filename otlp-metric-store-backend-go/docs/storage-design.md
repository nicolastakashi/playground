# Storage Design

## Overview

Gauge and sum ingestion uses a normalized ClickHouse layout:

- `otel_metrics_gauge_metadata` and `otel_metrics_sum_metadata` store reusable series metadata.
- `otel_metrics_gauge` and `otel_metrics_sum` store datapoint values and a `MetadataKey` reference.
- Histogram, exponential histogram, and summary tables exist in schema only. This document describes the implemented gauge and sum path.

The main goal is to keep datapoint tables optimized for time-bounded reads while avoiding full metadata duplication on every row.

## Two Kinds of Keys

The schema uses two 128-bit values with different jobs:

- `MetadataKey` identifies one logical metric series.
- `ReplacementRank` orders competing versions of the same logical metadata row.

They are intentionally not interchangeable.

### `MetadataKey`

`MetadataKey` is stored as `UUID` and is derived from the identifying metadata fields for a series.

For gauge rows, that identity includes:

- resource attributes
- scope name and version
- scope attributes and dropped attribute count
- service name
- metric name
- metric unit
- datapoint attributes

For sum rows, the identity additionally includes:

- aggregation temporality
- monotonicity

The key does not include `MetricDescription`, `ResourceSchemaUrl`, or `ScopeSchemaUrl`. Those fields are preserved as descriptive metadata, but they do not split the logical series identity.

Why `UUID`:

- it matches the idea of a stable identity token
- ClickHouse has a native `UUID` type
- the code derives a deterministic 128-bit value and stores it directly as a UUID-sized identifier

### `ReplacementRank`

`ReplacementRank` is stored as `UInt128` in ClickHouse and as `[16]byte` in Go.

It is derived from the non-identifying metadata fields that may drift over time while still referring to the same logical series:

- `ResourceSchemaUrl`
- `ScopeSchemaUrl`
- `MetricDescription`

Why it is not a UUID:

- it is not an identity token
- it exists only to provide deterministic ordering for replacement
- the metadata table needs a value that `ReplacingMergeTree` can compare numerically when duplicate `MetadataKey` rows exist

Why it is `[16]byte` in Go instead of a default integer type:

- the project already derives a 128-bit hash for deterministic replacement ordering
- using 128 bits keeps the replacement space aligned with the metadata key width
- a `uint64` or signed `BIGINT` would be a smaller hash space and a different storage contract than the current design intends

The ClickHouse client converts `[16]byte` to `UInt128` at insert time.

## Why Metadata Uses `ReplacingMergeTree`

Metadata tables use:

```sql
ENGINE ReplacingMergeTree(ReplacementRank)
ORDER BY (MetadataKey)
```

This supports eventual deduplication of metadata rows without requiring a synchronous uniqueness check before inserts.

That behavior matters because the write path is request-scoped and synchronous:

- a request may include repeated datapoints for the same metadata identity
- different requests may race to insert the same metadata identity
- the service does not perform a read-before-write existence check

`ReplacingMergeTree` lets the service append metadata rows and converge later to one logical row per `MetadataKey`.

### Replacement Semantics

When multiple metadata rows share the same `MetadataKey`, ClickHouse keeps the row with the greatest `ReplacementRank` when merges or `FINAL` queries apply replacement semantics.

This gives the system a deterministic winner for non-identifying metadata drift. For example, if only `MetricDescription` changes, the logical series identity remains the same, but one metadata row will eventually win as the canonical descriptive row.

What this does guarantee:

- duplicate physical metadata rows are logically valid during ingestion
- identical identifying metadata resolves to one logical metadata identity
- replacement outcome is derived from metadata content, not insertion order

What this does not guarantee:

- immediate physical deduplication at insert time
- transactional coupling between metadata and datapoint writes
- any notion of version chronology beyond the numeric ordering of `ReplacementRank`

## Why Datapoint Tables Order By Time Then `MetadataKey`

Datapoint tables use:

```sql
PARTITION BY toDate(TimeUnix)
ORDER BY (toUnixTimestamp64Nano(TimeUnix), MetadataKey)
```

This ordering is optimized for the supported normalized read flow:

1. discover relevant `MetadataKey` values from the metadata table
2. fetch datapoints for a bounded time range using those keys

Putting event time first keeps the primary access path aligned with time-bounded reads. Adding `MetadataKey` second clusters rows with the same timestamp by series and supports the common predicate shape used by integration tests and the documented query model.

This ordering is a deliberate trade-off:

- it is strong for time-range-first reads
- it is weaker for key-only reads without a time predicate
- it assumes the repository's query contract is time-bounded datapoint access, not arbitrary point lookups by metadata alone

## Supported Query Model

The supported normalized query path is two-step:

1. query the metadata table to discover matching series and collect `MetadataKey` values
2. query the datapoint table for a time range using those keys

Example:

```sql
SELECT MetadataKey, ServiceName, MetricName
FROM otel_metrics_gauge_metadata FINAL
WHERE ServiceName = 'checkout' AND MetricName = 'http.server.duration';

SELECT TimeUnix, Value, Flags
FROM otel_metrics_gauge
WHERE MetadataKey IN (<selected keys>)
  AND TimeUnix >= toDateTime64('2026-05-03 10:00:00', 9)
  AND TimeUnix <= toDateTime64('2026-05-03 11:00:00', 9)
ORDER BY TimeUnix;
```

## Scan Guarantees And Limits

The repository should document a narrower guarantee than "never full table scans".

What is well supported today:

- datapoint tables are partitioned by event date
- datapoint reads are expected to include a bounded time predicate
- datapoint reads may also constrain by `MetadataKey`

What is only partially addressed today:

- metadata tables are ordered only by `MetadataKey`
- there is no stronger secondary ordering for time-range-only metadata access
- metadata discovery queries may still scan metadata tables depending on filter shape

So the current bounded-read guarantee applies to datapoint tables once the relevant `MetadataKey` values are known. It does not mean every metadata discovery query is scan-free.

## Write Path And Failure Semantics

The implemented write model is intentionally simple:

1. one export request is mapped into normalized gauge and sum batches
2. each populated kind writes one metadata batch and one datapoint batch
3. within a kind, metadata is written before datapoints
4. gauge and sum pipelines may run independently in the same request

Current strengths:

- per-request batch inserts
- in-request metadata deduplication by `MetadataKey` and `ReplacementRank`
- deterministic metadata identity derivation

Current non-goals and non-guarantees:

- no async buffering
- no cross-request write coalescing
- no retry or replay strategy
- no idempotency contract
- no transactional all-or-nothing guarantee across metadata and datapoint tables
- no transactional guarantee across gauge and sum pipelines

If a storage step fails, the export fails, but earlier writes from the same request may already be persisted.

## Operability

The service has lightweight ingest observability today:

- OpenTelemetry traces, metrics, and logs are initialized in-process
- store operations are instrumented
- startup logs make storage connection behavior explicit

The repository does not yet document or implement a richer storage operability model such as:

- explicit backpressure behavior
- partial failure recovery strategy
- async queue depth or buffer metrics
- retry visibility
- stronger storage health or saturation metrics

Those changes introduced the normalized metadata lookup model, deterministic metadata identity, replacement semantics for non-identifying drift, documented query flow, and the current request-scoped throughput behavior.

