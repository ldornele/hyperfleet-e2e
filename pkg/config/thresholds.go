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
