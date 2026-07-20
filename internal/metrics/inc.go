package metrics

func IncAttestationsValid(n uint64)          { addUint(metricAttestationsValid, n) }
func IncAttestationsInvalid()                { metricAttestationsInvalid.Inc() }
func IncAttestationsBufferEvicted(n int)     { addCount(metricAttestationsBufferEvicted, n) }
func IncForkChoiceReorgs()                   { metricForkChoiceReorgs.Inc() }
func IncBlocksSkippedLag()                   { metricBlocksSkippedLag.Inc() }
func IncAttestationsSkippedLag()             { metricAttestationsSkippedLag.Inc() }
func IncPqSigAggregatedTotal()               { metricPqSigAggregatedSignaturesTotal.Inc() }
func IncPqSigAggregatedValid()               { metricPqSigAggregatedValid.Inc() }
func IncPqSigAggregatedInvalid()             { metricPqSigAggregatedInvalid.Inc() }
func IncPqSigAttestationsInAggregated(n int) { addCount(metricPqSigAttestationsInAggregated, n) }
func IncSTFSlotsProcessed(n uint64)          { addUint(metricSTFSlotsProcessed, n) }
func IncSTFAttestationsProcessed(n int)      { addCount(metricSTFAttestationsProcessed, n) }
func IncPqSigAttestationSigsTotal()          { metricPqSigAttestationSigsTotal.Inc() }
func IncPqSigAttestationSigsValid()          { metricPqSigAttestationSigsValid.Inc() }
func IncPqSigAttestationSigsInvalid()        { metricPqSigAttestationSigsInvalid.Inc() }
func IncAggregationDispatchDropped()         { metricAggregationDispatchDropped.Inc() }
func IncAggregatorSkipped(reason string) {
	metricAggregatorSkipped.WithLabelValues(aggregatorSkipReason(reason)).Inc()
}
func IncFinalization(result string) {
	metricFinalizationsTotal.WithLabelValues(labelOrUnknown(result)).Inc()
}
func IncBlockBuildingSuccess()  { metricBlockBuildingSuccess.Inc() }
func IncBlockBuildingFailures() { metricBlockBuildingFailures.Inc() }
func IncProofOperation(operation, result string) {
	metricProofOperations.WithLabelValues(labelOrUnknown(operation), labelOrUnknown(result)).Inc()
}

func IncPeerConnection(direction, result string) {
	metricPeerConnectionEvents.WithLabelValues(labelOrUnknown(direction), labelOrUnknown(result)).Inc()
}

func IncPeerDisconnection(direction, reason string) {
	metricPeerDisconnectionEvents.WithLabelValues(labelOrUnknown(direction), labelOrUnknown(reason)).Inc()
}

func IncReqRespTimeout(protocol, direction string) {
	metricReqRespTimeout.WithLabelValues(labelOrUnknown(protocol), labelOrUnknown(direction)).Inc()
}
