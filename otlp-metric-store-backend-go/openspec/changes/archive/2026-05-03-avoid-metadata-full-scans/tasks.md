## 1. Clarify the normalized query flow

- [x] 1.1 Document the two-step normalized read flow: discover matching metadata and `MetadataKey` values first, then fetch datapoints for those keys within a time range.
- [x] 1.2 Keep the first implementation flow explicit in integration test queries and documentation instead of extracting a shared SQL helper.

## 2. Verify the supported query behavior

- [x] 2.1 Add integration coverage that queries metadata first to identify matching gauge and sum series and captures their `MetadataKey` values.
- [x] 2.2 Add integration coverage that fetches datapoints for a time range using those selected `MetadataKey` values and verifies the expected rows are returned.

## 3. Validate the scoped change

- [x] 3.1 Update `README.md` to explain the behavioral contract, distinguish metadata-discovery queries from time-bounded datapoint reads, and include one illustrative query example.
- [x] 3.2 Run `make test` and `make test-integration` to validate the updated documentation assumptions and integration coverage.
