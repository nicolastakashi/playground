## Why

The normalized storage design currently leaves the read path ambiguous: users often need to discover which metric series match a metadata filter before they can fetch datapoints, but the repository does not yet describe that flow explicitly. This change is needed to clarify the supported query pattern without overengineering the schema or adding premature metadata-side optimizations.

## What Changes

- Clarify the normalized query contract as a two-step flow: discover matching series metadata and `MetadataKey` values first, then fetch time-bounded datapoints for those keys.
- Tighten the datapoint-read requirements so time-bounded retrieval remains bounded on the datapoint tables after the relevant metadata keys have been selected.
- Add focused documentation and integration coverage for the supported query flow without changing the storage schema or introducing speculative indexes, materialized views, or a new read API.

## Capabilities

### New Capabilities

### Modified Capabilities
- `metric-metadata-lookup`: Clarify how normalized reads discover matching metadata first and then use the resulting `MetadataKey` set for time-bounded datapoint retrieval.

## Impact

- Affected code: `integration_test.go`, `README.md`, and any shared SQL helpers if the implementation chooses to add them.
- Affected systems: ClickHouse query patterns for normalized gauge and sum reads.
- API impact: no gRPC API contract changes; this clarifies the supported read flow for normalized storage.
- Operational impact: tests and docs must show how metadata discovery and datapoint retrieval work together without introducing premature storage optimizations.
