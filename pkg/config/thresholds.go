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
//
// The cluster update, delete and cascade-delete thresholds are raised ahead of
// a rebaseline. Since the e2e API requires cl-desire, the remote adapter needs
// extra sentinel rounds to see the applier's result: one on update, two on
// delete (ConfigMap, then Namespace, then Finalized). Specs poll every 10s, so
// readings land on 10s steps: update read 70s on kind, and every reconciled
// cluster in the 2026-10-14 tier1-nightly took 50.2s from delete to hard-delete.
// Recalibrate once Maestro is gone and the perf specs poll finer than 10s.
const (
	ThresholdClusterCreateReconciled  = 90 * time.Second // baseline ~60s
	ThresholdClusterUpdateReconciled  = 90 * time.Second // was 60s, baseline ~40s before cl-desire
	ThresholdClusterDeleted           = 75 * time.Second // was 60s, baseline ~40s before cl-desire
	ThresholdClusterCascadeDeleted    = 90 * time.Second // was 75s, baseline ~50s before cl-desire
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
// No Prow tier1-nightly baseline yet; recalibrate once 5+ clean nightly runs
// are available now that these specs run Serial.
const (
	ThresholdAPICreate = 50 * time.Millisecond
	ThresholdAPIUpdate = 50 * time.Millisecond
	ThresholdAPIDelete = 50 * time.Millisecond
)
