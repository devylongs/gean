package metrics

func ObserveBlockProcessingTime(seconds float64) {
	observeNonNegative(metricBlockProcessingTime, seconds)
}
func ObservePqSigSigningTime(seconds float64) { observeNonNegative(metricPqSigSigningTime, seconds) }
func ObservePqSigVerificationTime(seconds float64) {
	observeNonNegative(metricPqSigVerificationTime, seconds)
}
func ObservePqSigAggBuildingTime(seconds float64) {
	observeNonNegative(metricPqSigAggBuildingTime, seconds)
}
func ObservePqSigAggVerificationTime(seconds float64) {
	observeNonNegative(metricPqSigAggVerificationTime, seconds)
}
func ObserveCommitteeSignaturesAggregationTime(seconds float64) {
	observeNonNegative(metricCommitteeSignaturesAggregationTime, seconds)
}
func ObserveAggregationPrepTime(seconds float64) {
	observeNonNegative(metricAggregationPrepTime, seconds)
}
func ObserveForkChoiceReorgDepth(depth float64) {
	observeNonNegative(metricForkChoiceReorgDepth, depth)
}
func ObserveTickIntervalDuration(seconds float64) {
	observeNonNegative(metricTickIntervalDuration, seconds)
}
func ObserveSTFTime(seconds float64)      { observeNonNegative(metricSTFTime, seconds) }
func ObserveSTFSlotsTime(seconds float64) { observeNonNegative(metricSTFSlotsTime, seconds) }
func ObserveSTFBlockTime(seconds float64) { observeNonNegative(metricSTFBlockTime, seconds) }
func ObserveSTFAttestationsTime(seconds float64) {
	observeNonNegative(metricSTFAttestationsTime, seconds)
}
func ObserveBlockBuildingTime(seconds float64) { observeNonNegative(metricBlockBuildingTime, seconds) }
func ObserveBlockBuildingPayloadAggregationTime(seconds float64) {
	observeNonNegative(metricBlockBuildingPayloadAggregationTime, seconds)
}
func ObserveBlockAggregatedPayloads(n int) {
	observeNonNegative(metricBlockAggregatedPayloads, countValue(n))
}
func ObserveGossipBlockSize(bytes int) { observeNonNegative(metricGossipBlockSize, countValue(bytes)) }
func ObserveGossipAttestationSize(bytes int) {
	observeNonNegative(metricGossipAttestationSize, countValue(bytes))
}
func ObserveGossipAggregationSize(bytes int) {
	observeNonNegative(metricGossipAggregationSize, countValue(bytes))
}

func ObserveAttestationValidationTime(seconds float64) {
	observeNonNegative(metricAttestationValidationTime, seconds)
}

func ObserveAttestationsProductionTime(seconds float64) {
	observeNonNegative(metricAttestationsProductionTime, seconds)
}

func ObserveAggregationWorkerTotalTime(seconds float64) {
	observeNonNegative(metricAggregationWorkerTotalTime, seconds)
}

func ObserveBlockSignatureVerificationTime(seconds float64) {
	observeNonNegative(metricBlockSignatureVerificationTime, seconds)
}

func ObserveProvingDuration(operation string, seconds float64) {
	observeNonNegative(metricProvingDuration.WithLabelValues(labelOrUnknown(operation)), seconds)
}

func ObserveProofSize(proofType string, bytes int) {
	observeNonNegative(metricProofSize.WithLabelValues(labelOrUnknown(proofType)), countValue(bytes))
}

func ObserveProofMergeComponents(n int) {
	observeNonNegative(metricProofMergeComponents, countValue(n))
}
