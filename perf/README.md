# Performance Tests

Lightweight performance tests that measure baseline latencies for core HyperFleet operations. Tests run inside the cluster for production-representative numbers.

## Table of contents

- [Prerequisites](#prerequisites)
- [Seeding data](#seeding-data)
- [Running tests](#running-tests)
- [Parsing results](#parsing-results)

## Prerequisites
- **[Setup Guide](../docs/setup.md)** - Setup environment to run e2e tests
The full HyperFleet stack (API, Sentinel, adapters, broker) must be deployed and healthy.
- **[Verify Configuration](../docs/setup.md#configure-test-settings)** - Configure test settings
Ensure the environment variables are set correctly in `env/env.local` and they match your infra deployment settings.

Run from the repository root:
```bash
source env/env.local
```

## Seeding data

For realistic baselines, seed the database before running tests. Seeded rows add
realistic table size so query planner behavior and index performance reflect
production conditions. Clusters and NodePools share the `resources` table with
Channel, Version, and WifConfig (see [HYPERFLEET-1159](https://issues.redhat.com/browse/HYPERFLEET-1159)),
so seeding any of these kinds grows the same underlying table.

### Check api settings
```bash
curl -f -X GET ${HYPERFLEET_API_URL}/api/hyperfleet/v1/clusters/
# Expected result: HTTP 200 OK.
```

```bash
# Seed 1000 clusters
./perf/seed-clusters.sh 1000

# Check what's in the database
./perf/seed-clusters.sh status

# Clean up seeded clusters only
./perf/seed-clusters.sh cleanup

# Or delete ALL clusters (clean slate)
./perf/seed-clusters.sh reset
```

For the generic (non-reconciling) resource kinds — Channel, WifConfig, Version —
use `seed-resources.sh`, which takes the kind as its first argument. Versions are
nested under channels, so seeding versions creates (or reuses) a single dedicated
parent channel and seeds all versions under it:

```bash
# Seed 1000 channels / wifconfigs / versions
./perf/seed-resources.sh channels 1000
./perf/seed-resources.sh wifconfigs 1000
./perf/seed-resources.sh versions 1000

# Check what's in the database
./perf/seed-resources.sh channels status

# Clean up seeded resources only
./perf/seed-resources.sh channels cleanup

# Or delete ALL resources of that kind (clean slate)
./perf/seed-resources.sh channels reset
```

## Running tests

Make sure you have [seeded the database](#seeding-data) first for realistic baselines.

```bash
./perf/run-in-cluster.sh
```

This builds the image, pushes it, and runs the tests in-cluster.

## Parsing results

```bash
./perf/parse-report.sh
```

This extracts `[PERF]` and `[FAIL]` lines from the latest output file and generates a summary report.
