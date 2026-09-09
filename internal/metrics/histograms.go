package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	metricBlockProcessingTime = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "lean_fork_choice_block_processing_time_seconds",
		Help:    "Time to process a block",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 1, 1.25, 1.5, 2, 4},
	})
	metricAttestationValidationTime = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "lean_attestation_validation_time_seconds",
		Help:    "Time to validate attestation data",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 1},
	})
	metricPqSigSigningTime = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "lean_pq_sig_attestation_signing_time_seconds",
		Help:    "Time to sign an attestation",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 1},
	})
	metricAttestationsProductionTime = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "lean_attestations_production_time_seconds",
		Help:    "Time taken to produce attestation",
		Buckets: []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 0.75, 1},
	})
	metricPqSigVerificationTime = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "lean_pq_sig_attestation_verification_time_seconds",
		Help:    "Time to verify an individual attestation signature",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 1},
	})
	metricPqSigAggBuildingTime = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "lean_pq_sig_aggregated_signatures_building_time_seconds",
		Help:    "Time to build an aggregated signature proof",
		Buckets: []float64{0.1, 0.25, 0.5, 0.75, 1, 1.25, 1.5, 2, 4},
	})
	metricPqSigAggVerificationTime = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "lean_pq_sig_aggregated_signatures_verification_time_seconds",
		Help:    "Time to verify an aggregated attestation signature",
		Buckets: []float64{0.1, 0.25, 0.5, 0.75, 1, 1.25, 1.5, 2, 4},
	})
	metricCommitteeSignaturesAggregationTime = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "lean_committee_signatures_aggregation_time_seconds",
		Help:    "Time taken to aggregate committee signatures",
		Buckets: []float64{0.05, 0.1, 0.25, 0.5, 0.75, 1, 2, 3, 4},
	})
	metricAggregationPrepTime = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "lean_aggregation_prep_time_seconds",
		Help:    "Per-aggregate prep time",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 1},
	})
	metricAggregationWorkerTotalTime = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "lean_aggregation_worker_total_time_seconds",
		Help:    "End-to-end aggregation worker pass",
		Buckets: []float64{0.25, 0.5, 0.75, 1, 1.25, 1.5, 2, 2.5, 3, 4},
	})
	metricBlockSignatureVerificationTime = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "lean_block_signature_verification_time_seconds",
		Help:    "Time to verify all signatures for an incoming block",
		Buckets: []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2},
	})
	metricForkChoiceReorgDepth = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "lean_fork_choice_reorg_depth",
		Help:    "Depth of fork choice reorgs",
		Buckets: []float64{1, 2, 3, 5, 7, 10, 20, 30, 50, 100},
	})
	metricSTFTime = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "lean_state_transition_time_seconds",
		Help:    "Time to process full state transition",
		Buckets: []float64{0.25, 0.5, 0.75, 1, 1.25, 1.5, 2, 2.5, 3, 4},
	})
	metricSTFSlotsTime = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "lean_state_transition_slots_processing_time_seconds",
		Help:    "Time taken to process slots",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 1},
	})
	metricSTFBlockTime = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "lean_state_transition_block_processing_time_seconds",
		Help:    "Time taken to process block",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 1},
	})
	metricSTFAttestationsTime = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "lean_state_transition_attestations_processing_time_seconds",
		Help:    "Time taken to process attestations",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 1},
	})
	metricBlockBuildingTime = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "lean_block_building_time_seconds",
		Help:    "Time to build a block",
		Buckets: []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 0.75, 1},
	})
	metricBlockBuildingPayloadAggregationTime = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "lean_block_building_payload_aggregation_time_seconds",
		Help:    "Time taken to build aggregated_payloads during block building",
		Buckets: []float64{0.1, 0.25, 0.5, 0.75, 1, 2, 3, 4},
	})
	metricBlockAggregatedPayloads = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "lean_block_aggregated_payloads",
		Help:    "Number of aggregated_payloads in a block",
		Buckets: []float64{1, 2, 4, 8, 16, 32, 64, 128},
	})
	metricGossipBlockSize = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "lean_gossip_block_size_bytes",
		Help:    "Bytes size of a gossip block message",
		Buckets: []float64{10000, 50000, 100000, 250000, 500000, 1000000, 2000000, 5000000},
	})
	metricGossipAttestationSize = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "lean_gossip_attestation_size_bytes",
		Help:    "Bytes size of a gossip attestation message",
		Buckets: []float64{512, 1024, 2048, 4096, 8192, 16384},
	})
	metricGossipAggregationSize = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "lean_gossip_aggregation_size_bytes",
		Help:    "Bytes size of a gossip aggregated attestation message",
		Buckets: []float64{1024, 4096, 16384, 65536, 131072, 262144, 524288, 1048576},
	})
	metricTickIntervalDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "lean_tick_interval_duration_seconds",
		Help:    "Elapsed time between clock ticks in seconds",
		Buckets: []float64{0.4, 0.6, 0.75, 0.8, 0.805, 0.81, 0.815, 0.82, 0.825, 0.85, 0.9, 1.0, 1.2, 1.6},
	})
	// metricDispatchEventDuration times each case of the dispatch select, so a
	// slow handler can be attributed rather than only observed as a late tick.
	// Buckets run well past a slot: the point is to size a stall, and the
	// tick-interval histogram's 1.6s ceiling could not.
	metricDispatchEventDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "lean_dispatch_event_duration_seconds",
		Help:    "Time the dispatch loop spent handling one event, by event kind",
		Buckets: []float64{0.001, 0.01, 0.05, 0.1, 0.25, 0.5, 0.8, 1.6, 4, 10, 30, 120, 600},
	}, []string{"event"})
	metricProvingDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "lean_proving_duration_seconds",
		Help:    "Recursive proof operation duration",
		Buckets: []float64{0.1, 0.25, 0.5, 1, 2, 4, 8},
	}, []string{"operation"})
	metricProofSize = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "lean_proof_size_bytes",
		Help:    "Serialized aggregate proof size",
		Buckets: []float64{1024, 4096, 16384, 65536, 131072, 262144, 524288},
	}, []string{"type"})
	metricProofMergeComponents = promauto.NewHistogram(prometheus.HistogramOpts{
		Name: "lean_proof_merge_components", Help: "Type-1 components merged into a Type-2 proof",
		Buckets: []float64{1, 2, 3, 4, 5, 6, 7, 8, 9},
	})
	metricReqRespRequestSize = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "lean_reqresp_request_size_bytes",
		Help:    "On-wire bytes of a req/resp request frame",
		Buckets: []float64{64, 128, 256, 512, 1024, 4096, 16384, 65536},
	}, []string{"protocol"})
	metricReqRespResponseChunkSize = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "lean_reqresp_response_chunk_size_bytes",
		Help:    "On-wire bytes of a single req/resp response frame",
		Buckets: []float64{128, 1024, 10000, 100000, 500000, 1000000, 5000000, 10000000},
	}, []string{"protocol"})
	metricBlockProposalAttestationDataSelected = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "lean_block_proposal_attestation_data_selected",
		Help:    "Distinct AttestationData entries placed in the proposal block body",
		Buckets: []float64{0, 1, 2, 4, 8, 16, 32},
	})
	metricBlockProposalAggregatesSelected = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "lean_block_proposal_aggregates_selected",
		Help:    "Aggregated signature proofs selected for the proposal",
		Buckets: []float64{0, 1, 2, 4, 8, 16, 32, 64, 128},
	})
)
