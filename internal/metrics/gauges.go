package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	metricHeadSlot = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "lean_head_slot", Help: "Latest head slot",
	})
	metricCurrentSlot = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "lean_current_slot", Help: "Current slot from wall clock",
	})
	metricSafeTargetSlot = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "lean_safe_target_slot", Help: "Safe target slot for attestation",
	})
	metricLatestJustifiedSlot = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "lean_latest_justified_slot", Help: "Latest justified checkpoint slot",
	})
	metricLatestFinalizedSlot = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "lean_latest_finalized_slot", Help: "Latest finalized checkpoint slot",
	})
	metricJustifiedSlot = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "lean_justified_slot", Help: "Current justified checkpoint slot",
	})
	metricFinalizedSlot = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "lean_finalized_slot", Help: "Current finalized checkpoint slot",
	})
	metricValidatorsCount = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "lean_validators_count", Help: "Number of validators managed by this node",
	})
	metricIsAggregator = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "lean_is_aggregator", Help: "Whether this node is an aggregator (0 or 1)",
	})
	metricAttestationCommitteeCount = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "lean_attestation_committee_count", Help: "Number of attestation committees/subnets",
	})
	metricGossipSignatures = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "lean_gossip_signatures", Help: "Number of gossip signatures in fork-choice store",
	})
	metricLatestNewAggregatedPayloads = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "lean_latest_new_aggregated_payloads", Help: "Number of new (pending) aggregated payloads",
	})
	metricLatestKnownAggregatedPayloads = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "lean_latest_known_aggregated_payloads", Help: "Number of known (active) aggregated payloads",
	})
	metricPendingAttestationsTotal = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "lean_pending_attestations_total", Help: "Gossip attestations buffered awaiting an unknown head block",
	})
	metricNodeInfo = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "lean_node_info", Help: "Node information",
	}, []string{"name", "version"})
	metricNodeStartTime = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "lean_node_start_time_seconds", Help: "Node start time as Unix timestamp",
	})
	metricConnectedPeers = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "lean_connected_peers", Help: "Number of connected peers",
	}, []string{"client"})
	metricAttestationCommitteeSubnet = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "lean_attestation_committee_subnet", Help: "Node's attestation committee subnet",
	})
	metricGossipMeshPeers = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "lean_gossip_mesh_peers", Help: "Number of peers in the gossipsub mesh",
	})
	metricNodeSyncStatus = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "lean_node_sync_status", Help: "Node sync status",
	}, []string{"status"})
	metricAttestationAggregateCoverageValidators = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "lean_attestation_aggregate_coverage_validators",
		Help: "Validator coverage in attestation aggregate reports, by section and subnet (subnet=combined is the section total)",
	}, []string{"section", "subnet"})
	metricAttestationAggregateCoverageSubnets = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "lean_attestation_aggregate_coverage_subnets",
		Help: "Number of covered subnets in attestation aggregate reports, by section",
	}, []string{"section"})
	metricAttestationAggregateCoverageDiffValidators = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "lean_attestation_aggregate_coverage_diff_validators",
		Help: "Validator coverage delta between block payloads and timely pre-merge payloads, by direction",
	}, []string{"direction"})
	metricProvingQueueDepth = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "lean_proving_queue_depth", Help: "Queued recursive proof work",
	}, []string{"operation"})
	metricProcessRSSBytes = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "lean_node_rss_bytes", Help: "Process resident set size in bytes (includes prover memory outside the Go heap)",
	})
)
