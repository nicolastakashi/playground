## ADDED Requirements

### Requirement: Support metadata-first series discovery
The system SHALL allow normalized gauge and sum reads to discover matching metric series from the metadata lookup tables before fetching datapoints. The discovery step SHALL return the logical metadata rows and their `MetadataKey` values without requiring datapoint rows to duplicate the metadata payload inline.

#### Scenario: Query identifies matching metric series before fetching datapoints
- **WHEN** a query filters by metric, resource, scope, service, or datapoint attribute metadata and needs to know which series exist
- **THEN** the system MUST be able to return matching metadata rows together with their `MetadataKey` values for a later datapoint query

## MODIFIED Requirements

### Requirement: Keep datapoint storage optimized for time-bounded queries
The system SHALL store gauge and sum datapoints in time-partitioned tables and SHALL support fetching datapoints for a specified time range, optionally constrained by a previously selected set of `MetadataKey` values, without requiring full table scans of all datapoints.

#### Scenario: Datapoint tables remain partitioned by event date
- **WHEN** the ClickHouse schema is created for normalized datapoint storage
- **THEN** the gauge and sum datapoint tables MUST partition rows using the datapoint event time

#### Scenario: Datapoint rows do not duplicate full metadata payloads
- **WHEN** a normalized datapoint row is persisted
- **THEN** the row MUST contain the datapoint value fields and a metadata reference instead of repeating the full metadata maps and descriptive fields inline

#### Scenario: Time-bounded datapoint reads use selected metadata keys
- **WHEN** a gauge or sum query fetches datapoints for a specific time range after determining a relevant set of `MetadataKey` values
- **THEN** the query MUST apply the requested time predicate to the datapoint table and use those `MetadataKey` values to constrain the datapoint rows it reads

#### Scenario: Supported normalized query flow is documented and verifiable
- **WHEN** the repository defines the normalized query path for gauge or sum reads
- **THEN** the documented or shared query pattern MUST make the metadata-discovery step and the time-bounded datapoint retrieval step explicit so integration tests can verify the intended flow
