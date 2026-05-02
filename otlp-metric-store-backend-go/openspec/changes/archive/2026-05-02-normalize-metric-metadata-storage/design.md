## Context

The current implementation writes fully denormalized gauge and sum rows into ClickHouse. Every datapoint repeats resource attributes, scope metadata, metric metadata, and datapoint attributes inline, which increases write amplification and storage duplication for metadata that changes much less frequently than datapoints. The assignment requires a metadata lookup model and emphasizes high-throughput ingest, low-cardinality metadata that may still drift over time, and time-bounded queries that avoid full table scans.

This repo currently has a single ingest path in `metrics_service.go` that maps OTLP payloads into `GaugeRow` and `SumRow` values and batch-inserts them directly. There is no metadata identity layer, no lookup table, and no persisted indirection between a datapoint and its metadata.

## Goals / Non-Goals

**Goals:**
- Separate reusable metric metadata from high-volume gauge and sum datapoint storage.
- Reuse metadata rows when the same metric series metadata is observed repeatedly.
- Treat metadata changes as a new identity so drift over time is preserved rather than overwritten.
- Preserve efficient time-bounded reads by keeping datapoint tables partitioned by event time and ordered for time-range access.
- Minimize ingest coordination overhead so the solution remains practical for high-throughput batch writes.
- Keep the change scoped to gauge and sum, matching the current implementation surface.

**Non-Goals:**
- Adding mapper or storage support for histogram, exponential histogram, or summary datapoints.
- Designing a general query API on top of ClickHouse.
- Backfilling or migrating existing production data.
- Eliminating joins for all read paths; metadata joins are acceptable when metadata is needed.

## Decisions

### Use separate metadata lookup tables for gauge and sum

Gauge datapoints will reference a gauge metadata table, and sum datapoints will reference a sum metadata table. Each table will store the stable dimensions currently duplicated in every row: resource attributes, scope fields, `service.name`, metric name, metric unit, datapoint attributes, and separate non-identifying descriptive metadata such as `MetricDescription` and schema URLs. The sum metadata table will additionally store sum-specific identifying semantics.

Rationale:
- The duplicated fields describe the identity of a metric series more than the datapoint itself.
- This preserves the current gauge/sum separation already present in the mapper, store, and datapoint schemas.
- It avoids introducing sum-only columns or a metric-kind discriminator into gauge metadata rows.
- It keeps gauge- and sum-specific measurement fields and metadata semantics in their existing domains with a smaller implementation diff.

Alternatives considered:
- One shared metadata table for all metric kinds: rejected because gauge and sum no longer share an identical metadata shape once sum-specific semantics are included, and the shared table would add avoidable schema and write-path branching.
- Keep the current denormalized tables and add secondary lookup tables only for optional queries: rejected because it does not reduce write amplification or satisfy the assignment intent.

### Use deterministic 128-bit metadata identity generated in the application

The ingest path will compute a deterministic metadata key from a canonical representation of identifying metadata fields only. Canonicalization must sort attribute keys before encoding so logically identical metadata always produces the same key regardless of OTLP attribute ordering. Gauge metadata keys only need the gauge-identifying fields, while sum metadata keys additionally include aggregation temporality and monotonicity. Because gauge and sum use separate metadata tables, metric kind does not need to be encoded into the hash payload itself. The canonical payload will be hashed with a strong algorithm such as SHA-256 and truncated to 128 bits for storage in a compact binary key column such as `UUID` or `FixedString(16)`. In alignment with OpenTelemetry metric identity guidance, `MetricDescription` and schema URL fields will be stored in the metadata table but excluded from the identity hash, while `MetricUnit` remains identifying.

Rationale:
- Avoids a synchronous read-before-write or central ID allocator during ingest.
- Supports idempotent metadata reuse across concurrent requests.
- Naturally handles identity drift: changed identifying attributes or sum semantics yield a new key and a new metadata row, while metric kind remains isolated by table selection.
- Provides a much lower collision risk than `UInt64` while keeping datapoint rows compact for joins and storage.
- Matches OpenTelemetry community guidance by keeping description and schema metadata out of the identity boundary.

Alternatives considered:
- `UInt64` hash keys: rejected because collision risk is too high for a long-lived metadata identity scheme.
- Full 256-bit or hex-encoded hashes: rejected because they increase datapoint storage and join cost without providing material benefit for this assignment.
- Application-managed incremental integer IDs: rejected because it introduces allocation and concurrency complexity in the hot path.
- ClickHouse-generated IDs via insert/select workflow: rejected because it couples ingest correctness to additional coordination queries and is less predictable under high throughput.

### Store datapoints as lean fact tables keyed by compact binary metadata identity

Gauge and sum tables will retain only datapoint fields plus the 128-bit metadata key that points at the corresponding metadata table. Gauge rows need metadata key, timestamps, numeric value, and flags. Sum rows additionally need aggregation temporality and monotonicity.

Rationale:
- Removes repeated low-cardinality metadata from the high-volume write path.
- Keeps gauge and sum semantics separate with minimal behavioral change.
- Aligns with the assignment request that datapoints store value, timestamp, and a reference to a lookup table.

Alternatives considered:
- One unified datapoint table for multiple metric kinds: rejected because it adds schema branching and null-heavy rows without helping the current scope.

### Preserve time-based partitioning and bias ordering toward time-bounded reads

Datapoint tables will remain partitioned by `toDate(TimeUnix)`. The row ordering should include time and metadata key so the guaranteed time-range filter remains the primary way ClickHouse prunes data.

Rationale:
- The assignment guarantees time-bounded queries, so time remains the strongest pruning dimension.
- Ordering by metadata key plus time allows efficient retrieval for a known metric identity while still keeping time selective.

Alternatives considered:
- Keep the current order by service name, metric name, and attributes: rejected because those fields move to the metadata table and are no longer present in the datapoint tables.

### Use `ReplacingMergeTree` with a deterministic replacement rank

The gauge and sum metadata lookup tables will use `ReplacingMergeTree` with the deterministic 128-bit metadata key as the logical identity and a sortable replacement rank derived from a canonical serialization of the non-identifying metadata payload. Metadata insertion will remain append-oriented and will not require synchronous existence checks before datapoints are written. Duplicate metadata rows for the same identity are acceptable temporarily within either table. If those rows disagree on non-identifying metadata, the row with the highest replacement rank is the logical survivor after `FINAL` reads or background merges.

Rationale:
- Fits ClickHouse's append-oriented model.
- Keeps ingest logic simple and robust under concurrency.
- Improves ingestion throughput by avoiding strict uniqueness enforcement on the hot path.
- Allows datapoint writes to proceed once the metadata key is known, even if physical metadata deduplication has not yet completed.
- Makes the surviving descriptive metadata row predictable without requiring field-by-field read/merge/write coordination.

Alternatives considered:
- Plain `MergeTree` with manual cleanup: rejected because it pushes more deduplication burden onto query logic and operational cleanup even though the eventual-consistency goal matches `ReplacingMergeTree` well.
- Enforcing metadata existence through pre-insert lookups for each row: rejected because it adds read pressure and reduces throughput.
- Last-write-wins via ingest timestamps: rejected because it couples logical convergence to insert ordering and clock behavior rather than stable content-derived precedence.
- Longer-description plus last-seen schema merge policy: rejected because `ReplacingMergeTree` replaces whole rows and cannot perform that field-by-field merge without synchronous coordination.

### Keep datapoint ingestion independent from metadata physical deduplication

Datapoint tables will continue to use plain append-oriented `MergeTree` storage and will reference metadata by deterministic key only. The system will treat metadata-key generation as the synchronous requirement and metadata-row compaction as an eventual consistency concern.

Rationale:
- Preserves high-throughput datapoint ingestion.
- Avoids coupling datapoint acceptance to metadata-table merge timing.
- Makes logical identity correctness independent from physical row deduplication.

Alternatives considered:
- Block datapoint insertion until a unique physical metadata row is confirmed: rejected because it adds latency and coordination without improving logical identity correctness.

### Treat description and schema URLs as non-identifying metadata

`MetricDescription`, `ResourceSchemaUrl`, and `ScopeSchemaUrl` will be stored in the metadata lookup tables but excluded from the identity hash. If two observations share the same identifying metadata but differ only in these descriptive fields, they will reuse the same metadata key within the corresponding metadata table and may temporarily create multiple physical metadata rows. The replacement rank will choose one deterministic logical survivor for that identity without requiring a synchronous read/merge/write step. Preserving a full history of descriptive-field drift is out of scope for this change.

Rationale:
- OpenTelemetry explicitly treats `description` as non-identifying.
- Schema URLs describe schema provenance, not the semantic identity of a metric stream.
- Excluding these fields avoids unnecessary identity churn from documentation or schema evolution changes.
- A deterministic whole-row winner policy is compatible with append-only `ReplacingMergeTree` inserts.

Alternatives considered:
- Include description in the identity boundary: rejected because it conflicts with OpenTelemetry guidance and would create unnecessary series splits for documentation-only changes.
- Include schema URLs in the identity boundary: rejected because schema provenance changes do not redefine the metric stream itself.
- Preserve every descriptive variant as query-visible history for the same identity: rejected because it complicates the lookup model and is not required for the current assignment.

### Make deduplication verification explicit in tests

Integration tests should validate logical metadata convergence in both metadata tables using deterministic query patterns instead of waiting for background merges opportunistically. Tests may use `SELECT ... FINAL` when asserting the logical survivor for a metadata identity, and may use `OPTIMIZE TABLE ... FINAL` only in cases where the test needs to force a merge before checking physical deduplication behavior.

Rationale:
- Avoids flaky tests that depend on ClickHouse background merge timing.
- Matches the logical contract of `ReplacingMergeTree`, where duplicate physical rows can remain temporarily valid.
- Gives integration tests a precise way to verify both pre-merge validity and post-merge convergence.

Alternatives considered:
- Sleep-and-poll for background merges: rejected because it is slow and nondeterministic.

## Risks / Trade-offs

- [Metadata key collisions] -> Use canonical serialization plus a 128-bit hash key, store full metadata alongside the key in the appropriate metadata table, and test identical, changed, and pathological collision-handling cases explicitly.
- [Joins add read complexity] -> Keep the metadata tables narrow and stable, and document that queries needing descriptive fields will join on metadata key.
- [Metadata rows can be re-inserted across batches] -> Use `ReplacingMergeTree` so duplicates are semantically harmless and are eventually compacted without blocking ingestion.
- [Changing identity fields can increase metadata cardinality] -> Restrict identity to OpenTelemetry-identifying fields and keep descriptive metadata outside the hash boundary.
- [Non-identifying metadata conflicts can occur] -> Use a content-derived replacement rank over the canonical non-identifying payload so eventual merges converge predictably without synchronous coordination.
- [Schema migration breaks current tests] -> Update integration tests to assert both metadata persistence and datapoint references instead of denormalized columns on datapoint tables.
- [Deduplication tests become flaky] -> Use `FINAL` reads and explicit merge forcing only where required instead of waiting on background compaction timing.

## Migration Plan

1. Add gauge and sum metadata tables using `ReplacingMergeTree` and the new gauge/sum datapoint table definitions.
2. Introduce metadata row and datapoint row types plus deterministic identity generation over identifying fields only.
3. Update mapping and store insert paths so metadata is appended with eventual dedup semantics and datapoints reference metadata keys.
4. Rewrite integration tests to validate schema creation, metadata reuse, eventual metadata dedup semantics through `FINAL`-based assertions, metadata drift, and gRPC-to-ClickHouse end-to-end behavior.
5. Keep the scope limited to fresh deployments for this assignment; no historical data migration is required.

Rollback strategy:
- Revert to the prior denormalized schema and insert path if the normalized write path proves incorrect. Because this repo is an assignment codebase and not wired into a production deployment path, rollback is a code-level reversion rather than a live migration procedure.

## Open Questions
