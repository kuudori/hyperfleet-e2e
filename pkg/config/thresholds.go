package config

import "time"

// Performance thresholds for API read operations.
// All API reads share the same threshold since payload size
// does not meaningfully impact latency at current scales.
const (
	ThresholdAPIRead = 50 * time.Millisecond
	ThresholdAPIList = 50 * time.Millisecond
)

// Performance thresholds for reconciliation operations.
// Calibrated from Prow tier1-nightly baselines (hyperfleet-dev-prow).
// Cluster thresholds use a ~1.5x margin over baseline to absorb CI
// run-to-run variance. NodePool thresholds use a ~2.25x margin because
// their lower baselines make the same absolute jitter a higher percentage.
const (
	ThresholdClusterCreateReconciled  = 90 * time.Second // baseline ~60s
	ThresholdClusterUpdateReconciled  = 60 * time.Second // baseline ~40s
	ThresholdClusterDeleted           = 60 * time.Second // baseline ~40s
	ThresholdClusterCascadeDeleted    = 75 * time.Second // baseline ~50s
	ThresholdNodePoolCreateReconciled = 45 * time.Second // baseline ~20s
	ThresholdNodePoolDeleted          = 45 * time.Second // baseline ~20s
)

// Performance thresholds for generic (non-reconciling) resource write operations.
// Covers Channel, Version, and WifConfig create/update/delete. None of these kinds
// carry RequiredAdapters, so writes complete synchronously with no reconciliation
// to await — unlike Cluster/NodePool, there is no create/update-to-reconciled variant.
// Calibrated from a GKE-dev baseline run (~1k seeded rows per kind, 2026-07-27);
// observed latencies were 3-10ms across all three kinds, well under this shared
// threshold — see hyperfleet/docs/performance-baselines.md in the architecture repo.
// No Prow tier1-nightly baseline exists yet for these specific operations (the specs
// are new); revisit this value once the first post-merge nightly run lands.
const (
	ThresholdAPICreate = 50 * time.Millisecond
	ThresholdAPIUpdate = 50 * time.Millisecond
	ThresholdAPIDelete = 50 * time.Millisecond
)
