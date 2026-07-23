package metrics

func SetNodeInfo(name, version string) {
	metricNodeInfo.WithLabelValues(labelOrUnknown(name), labelOrUnknown(version)).Set(1)
}
func SetNodeStartTime(t float64)      { setNonNegative(metricNodeStartTime, t) }
func SetHeadSlot(s uint64)            { metricHeadSlot.Set(float64(s)) }
func SetCurrentSlot(s uint64)         { metricCurrentSlot.Set(float64(s)) }
func SetSafeTargetSlot(s uint64)      { metricSafeTargetSlot.Set(float64(s)) }
func SetLatestJustifiedSlot(s uint64) { metricLatestJustifiedSlot.Set(float64(s)) }
func SetLatestFinalizedSlot(s uint64) { metricLatestFinalizedSlot.Set(float64(s)) }
func SetJustifiedSlot(s uint64)       { metricJustifiedSlot.Set(float64(s)) }
func SetFinalizedSlot(s uint64)       { metricFinalizedSlot.Set(float64(s)) }
func SetValidatorsCount(n int)        { metricValidatorsCount.Set(countValue(n)) }

func SetIsAggregator(b bool) {
	metricIsAggregator.Set(boolValue(b))
}

func SetAttestationCommitteeCount(n uint64)  { metricAttestationCommitteeCount.Set(float64(n)) }
func SetGossipSignatures(n int)              { metricGossipSignatures.Set(countValue(n)) }
func SetNewAggregatedPayloads(n int)         { metricLatestNewAggregatedPayloads.Set(countValue(n)) }
func SetKnownAggregatedPayloads(n int)       { metricLatestKnownAggregatedPayloads.Set(countValue(n)) }
func SetPendingAttestationsTotal(n int)      { metricPendingAttestationsTotal.Set(countValue(n)) }
func SetAttestationCommitteeSubnet(n uint64) { metricAttestationCommitteeSubnet.Set(float64(n)) }
func SetGossipMeshPeers(n int)               { metricGossipMeshPeers.Set(countValue(n)) }
func SetProvingQueueDepth(operation string, n int) {
	metricProvingQueueDepth.WithLabelValues(labelOrUnknown(operation)).Set(countValue(n))
}

func SetConnectedPeers(client string, n int) {
	metricConnectedPeers.WithLabelValues(labelOrUnknown(client)).Set(countValue(n))
}

func SetSyncStatus(status string) {
	active := syncStatusLabel(status)
	for _, s := range syncStatusLabels {
		metricNodeSyncStatus.WithLabelValues(s).Set(boolValue(s == active))
	}
}

func SetAttestationAggregateCoverageValidators(section, subnet string, n int) {
	metricAttestationAggregateCoverageValidators.
		WithLabelValues(labelOrUnknown(section), labelOrUnknown(subnet)).Set(countValue(n))
}

func SetAttestationAggregateCoverageSubnets(section string, n int) {
	metricAttestationAggregateCoverageSubnets.WithLabelValues(labelOrUnknown(section)).Set(countValue(n))
}

func SetAttestationAggregateCoverageDiffValidators(direction string, n int) {
	metricAttestationAggregateCoverageDiffValidators.
		WithLabelValues(labelOrUnknown(direction)).Set(countValue(n))
}
