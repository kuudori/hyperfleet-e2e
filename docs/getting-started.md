# Getting Started

New to HyperFleet E2E? This guide will help you run your first test in 10 minutes.

## Prerequisites

- **Go 1.25+** - Required for building the framework
- **HyperFleet deployment** - Running HyperFleet API and Maestro instance
- **10 minutes** - Time to complete this guide

> **Need to set up a HyperFleet environment first?** See the [Setup Guide](setup.md) for complete instructions using Kind (local) or GCP.

## Installation

### Clone and Build

```bash
git clone https://github.com/openshift-hyperfleet/hyperfleet-e2e.git
cd hyperfleet-e2e
make build
```

### Verify Installation

```bash
./bin/hyperfleet-e2e --help
```

You should see the command help output.

## Your First Test

**Step 1**: Set required environment variables

```bash
export HYPERFLEET_API_URL=<your-hyperfleet-api-url>
export MAESTRO_URL=<your-maestro-url>
export NAMESPACE=<your-deployment-namespace>
```

**Step 2**: Configure JWT authentication (optional)

When the API has JWT enforcement enabled (`server.jwt.enabled=true`), the E2E framework must authenticate requests. It acquires a JWT from K8s using the TokenRequest API — the test runner needs cluster credentials with `serviceaccounts/token` create permission.

```bash
export HYPERFLEET_IDENTITY_TOKENREQUEST_SERVICEACCOUNTNAME=hyperfleet-e2e-sa
export HYPERFLEET_IDENTITY_TOKENREQUEST_NAMESPACE=hyperfleet
export HYPERFLEET_IDENTITY_EXPECTEDIDENTITY=system:serviceaccount:hyperfleet:hyperfleet-e2e-sa
```

Skip this step if JWT is not enabled — the framework works without authentication.

**Step 3**: Run tier0 tests

```bash
./bin/hyperfleet-e2e test --label-filter=tier0
```

**What happens**:
1. Framework creates a new cluster via API
2. Waits for cluster to reach Reconciled state
3. Validates adapter conditions
4. Deletes cluster after test completes

## What Just Happened?

The framework:

1. **Loaded configuration** - Merged config file, environment variables, and CLI flags
2. **Executed tests** - Ran all tests matching your filter
3. **Managed resources** - Created and deleted temporary test clusters
4. **Generated results** - Displayed test outcomes

## Running Specific Tests

```bash
# Run critical tests only (parallel, 4 procs by default)
make e2e GINKGO_LABEL_FILTER=tier0

# Run important features
make e2e GINKGO_LABEL_FILTER=tier1

# Run cluster tier0 tests only
make e2e GINKGO_LABEL_FILTER=tier0 GINKGO_FOCUS="\[Suite: cluster\]"

# Run single-process (useful for debugging)
make e2e PROCS=1 GINKGO_LABEL_FILTER=tier0

# Deep debug mode (add API calls and framework internals)
./bin/hyperfleet-e2e test --log-level=debug
```

## Listing Tests Without Execution

Use `--dry-run` to discover which specs match your filters without connecting to the API or creating resources. No `--api-url` is required in dry-run mode.

```bash
# List all tier0 tests
./bin/hyperfleet-e2e test --dry-run --label-filter=tier0

# List tier1 cluster tests
./bin/hyperfleet-e2e test --dry-run --label-filter=tier1 --focus "\[Suite: cluster\]"

# List tests for each tier via Makefile
make list-tests
```

**Note**: The default output already shows detailed test execution steps. If a test fails, you can usually diagnose the issue from the logs without re-running in debug mode. Use `--log-level=debug` when you need to see API calls and framework internals. See [Debugging Guide](debugging.md) for more debugging techniques.

## Common Commands

```bash
make build       # Build binary
make test        # Run unit tests
make e2e         # Run E2E tests in parallel (PROCS=4 by default)
make e2e-ci      # Run E2E tests for CI (parallel + JUnit + flake retries)
make list-tests  # List tests by tier (dry-run, no API required)
make lint        # Run linter
make check       # Run all checks (fmt, vet, lint, test)
```

## Troubleshooting

### Common Issues

**API connection errors**:
```bash
# Verify API URLs are set
echo $HYPERFLEET_API_URL
echo $MAESTRO_URL
echo $NAMESPACE

# Test connectivity
curl -f -X GET ${HYPERFLEET_API_URL}/api/hyperfleet/v1/clusters/
```

**Test timeouts**: Increase timeouts via environment variables:
```bash
HYPERFLEET_TIMEOUTS_CLUSTER_RECONCILED=45m make e2e
```

**Namespace mismatch**: Ensure `NAMESPACE` matches your deployment namespace. Some tests deploy adapters dynamically and must target the same namespace where HyperFleet components are running.

**Configuration not taking effect**:

Priority order (highest to lowest):
1. CLI flags (`--api-url`)
2. Environment variables (`HYPERFLEET_API_URL`, `MAESTRO_URL`, `NAMESPACE`)
3. Config file (`configs/config.yaml`)
4. Built-in defaults

**Need detailed logs**:
```bash
# Default (info) shows test execution steps
./bin/hyperfleet-e2e test

# Debug mode shows API calls and framework internals
./bin/hyperfleet-e2e test --log-level=debug
```

For more troubleshooting help and environment issues, see the [Runbook](runbook.md#troubleshooting) or [Setup Guide](setup.md).

## Next Steps

- **[Runbook](runbook.md)** - Running tests and troubleshooting guide
- **[Architecture](architecture.md)** - Understand how the framework works
- **[Development](development.md)** - Write your own tests
- **CLI Reference** - Run `./bin/hyperfleet-e2e --help`
- **Configuration** - See detailed comments in `configs/config.yaml`
