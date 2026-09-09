package metrics

import (
	"math"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestCountCounterWrappersIgnoreNonPositiveValues(t *testing.T) {
	before := metricValue(t, metricAttestationsBufferEvicted)

	IncAttestationsBufferEvicted(-2)
	IncAttestationsBufferEvicted(0)
	if got := metricValue(t, metricAttestationsBufferEvicted); got != before {
		t.Fatalf("counter changed after nonpositive adds: got %v want %v", got, before)
	}

	IncAttestationsBufferEvicted(3)
	if got, want := metricValue(t, metricAttestationsBufferEvicted)-before, float64(3); got != want {
		t.Fatalf("counter delta=%v, want %v", got, want)
	}
}

func TestIncAggregatorSkippedUsesBoundedReasons(t *testing.T) {
	reasons := []string{
		AggregatorSkipNotAggregator,
		AggregatorSkipMissingState,
		AggregatorSkipSpawnFailed,
		AggregatorSkipOther,
	}
	for _, reason := range reasons {
		before := metricValue(t, metricAggregatorSkipped.WithLabelValues(reason))
		IncAggregatorSkipped(reason)
		if got := metricValue(t, metricAggregatorSkipped.WithLabelValues(reason)); got != before+1 {
			t.Fatalf("aggregator skipped %s=%v, want %v", reason, got, before+1)
		}
	}

	beforeOther := metricValue(t, metricAggregatorSkipped.WithLabelValues(AggregatorSkipOther))
	IncAggregatorSkipped("unexpected")
	if got := metricValue(t, metricAggregatorSkipped.WithLabelValues(AggregatorSkipOther)); got != beforeOther+1 {
		t.Fatalf("unexpected reason did not map to other: got %v want %v", got, beforeOther+1)
	}
}

func TestGaugeWrappersClampNegativeCounts(t *testing.T) {
	SetValidatorsCount(-1)
	if got := metricValue(t, metricValidatorsCount); got != 0 {
		t.Fatalf("validators gauge=%v, want 0", got)
	}

	SetGossipMeshPeers(-5)
	if got := metricValue(t, metricGossipMeshPeers); got != 0 {
		t.Fatalf("mesh peers gauge=%v, want 0", got)
	}

	SetConnectedPeers("", -3)
	if got := metricValue(t, metricConnectedPeers.WithLabelValues(unknownLabel)); got != 0 {
		t.Fatalf("connected peers gauge=%v, want 0", got)
	}

	SetConnectedPeers("", 4)
	if got := metricValue(t, metricConnectedPeers.WithLabelValues(unknownLabel)); got != 4 {
		t.Fatalf("connected peers gauge=%v, want 4", got)
	}
}

func TestSetIsAggregatorUsesBooleanGauge(t *testing.T) {
	SetIsAggregator(true)
	if got := metricValue(t, metricIsAggregator); got != 1 {
		t.Fatalf("aggregator gauge=%v, want 1", got)
	}

	SetIsAggregator(false)
	if got := metricValue(t, metricIsAggregator); got != 0 {
		t.Fatalf("aggregator gauge=%v, want 0", got)
	}
}

func TestSetSyncStatusActivatesSingleStatus(t *testing.T) {
	SetSyncStatus("syncing")
	assertSyncStatus(t, "idle", 0)
	assertSyncStatus(t, "syncing", 1)
	assertSyncStatus(t, "synced", 0)
	assertSyncStatus(t, unknownLabel, 0)

	SetSyncStatus("unexpected")
	assertSyncStatus(t, "idle", 0)
	assertSyncStatus(t, "syncing", 0)
	assertSyncStatus(t, "synced", 0)
	assertSyncStatus(t, unknownLabel, 1)
}

func TestLabelOrUnknownNormalizesDynamicLabels(t *testing.T) {
	if got := labelOrUnknown("  lodestar  "); got != "lodestar" {
		t.Fatalf("label=%q, want trimmed label", got)
	}
	if got := labelOrUnknown(" \t\n "); got != unknownLabel {
		t.Fatalf("blank label=%q, want unknown", got)
	}

	long := strings.Repeat("x", maxLabelRunes+10)
	if got := labelOrUnknown(long); len([]rune(got)) != maxLabelRunes {
		t.Fatalf("truncated label rune length=%d, want %d", len([]rune(got)), maxLabelRunes)
	}

	unicodeLong := strings.Repeat("é", maxLabelRunes+1)
	if got := labelOrUnknown(unicodeLong); len([]rune(got)) != maxLabelRunes || !strings.HasSuffix(got, "é") {
		t.Fatalf("unicode label=%q was not truncated on rune boundary", got)
	}
}

func TestHistogramWrappersIgnoreInvalidObservations(t *testing.T) {
	before := histogramCount(t, metricBlockProcessingTime)

	ObserveBlockProcessingTime(-1)
	ObserveBlockProcessingTime(math.NaN())
	ObserveBlockProcessingTime(math.Inf(1))
	if got := histogramCount(t, metricBlockProcessingTime); got != before {
		t.Fatalf("histogram count changed after invalid observations: got %d want %d", got, before)
	}

	ObserveBlockProcessingTime(0.25)
	if got := histogramCount(t, metricBlockProcessingTime); got != before+1 {
		t.Fatalf("histogram count after valid observation=%d, want %d", got, before+1)
	}
}

func TestSetNodeStartTimeIgnoresInvalidValues(t *testing.T) {
	SetNodeStartTime(100)
	before := metricValue(t, metricNodeStartTime)

	SetNodeStartTime(-1)
	SetNodeStartTime(math.NaN())
	SetNodeStartTime(math.Inf(1))
	if got := metricValue(t, metricNodeStartTime); got != before {
		t.Fatalf("node start time changed after invalid values: got %v want %v", got, before)
	}
}

func assertSyncStatus(t *testing.T, status string, want float64) {
	t.Helper()
	if got := metricValue(t, metricNodeSyncStatus.WithLabelValues(status)); got != want {
		t.Fatalf("sync status %q=%v, want %v", status, got, want)
	}
}

func metricValue(t *testing.T, metric prometheus.Metric) float64 {
	t.Helper()

	var dtoMetric dto.Metric
	if err := metric.Write(&dtoMetric); err != nil {
		t.Fatalf("write metric: %v", err)
	}
	if gauge := dtoMetric.GetGauge(); gauge != nil {
		return gauge.GetValue()
	}
	if counter := dtoMetric.GetCounter(); counter != nil {
		return counter.GetValue()
	}
	t.Fatal("metric has no gauge or counter value")
	return 0
}

func histogramCount(t *testing.T, metric prometheus.Metric) uint64 {
	t.Helper()

	var dtoMetric dto.Metric
	if err := metric.Write(&dtoMetric); err != nil {
		t.Fatalf("write metric: %v", err)
	}
	histogram := dtoMetric.GetHistogram()
	if histogram == nil {
		t.Fatal("metric has no histogram value")
	}
	return histogram.GetSampleCount()
}
