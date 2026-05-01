## 1. Schema and row model

- [ ] 1.1 Add separate ClickHouse gauge and sum metadata lookup table schemas using `ReplacingMergeTree`, each with a compact 128-bit metadata key column, a deterministic replacement-rank/version field for non-identifying metadata, and separate non-identifying fields for description and schema URLs.
- [ ] 1.2 Replace denormalized gauge and sum table schemas with datapoint-focused schemas that store the 128-bit metadata reference to the corresponding metadata table plus measurement fields.
- [ ] 1.3 Introduce Go row types for gauge metadata, sum metadata, and normalized gauge and sum datapoint records.

## 2. Metadata identity and mapping

- [ ] 2.1 Implement deterministic metadata normalization and 128-bit identity generation over identifying fields only, with separate gauge and sum key generation and sum-specific semantics where applicable, stable across attribute ordering.
- [ ] 2.2 Refactor metric mapping so gauge and sum datapoints produce normalized metadata plus datapoint rows instead of fully denormalized rows.
- [ ] 2.3 Ensure identifying metadata changes produce a new identity while description and schema URL changes reuse the same identity and rely on deterministic replacement-rank semantics for eventual convergence.

## 3. Store write path

- [ ] 3.1 Extend the `MetricsStore` interface and ClickHouse store implementation to insert gauge metadata and sum metadata records.
- [ ] 3.2 Update gauge and sum insert paths to append metadata rows without synchronous uniqueness checks before sending datapoint rows with metadata references.
- [ ] 3.3 Refactor the gRPC export flow to use the normalized write path for both gauge and sum metrics and apply the deterministic replacement-rank policy for non-identifying metadata conflicts.

## 4. Validation

- [ ] 4.1 Add unit tests for metadata normalization and 128-bit identity generation, including reordered-attribute, gauge-vs-sum table separation, sum-semantic-change, unit-change, description-change, and schema-URL-change cases.
- [ ] 4.2 Update integration tests to verify both metadata tables are created, `ReplacingMergeTree` semantics, identifying metadata drift, non-identifying metadata handling, and normalized gauge and sum inserts using `FINAL` or explicit merge forcing instead of background-merge timing.
- [ ] 4.3 Update the gRPC-to-ClickHouse integration test to assert datapoint rows reference persisted metadata correctly.
