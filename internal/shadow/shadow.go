// Package shadow models XMSS prover cost as artificial virtual-time sleeps for
// the Shadow network simulator, which does not charge CPU time — without them
// the prover would run "free" in virtual time, making multi-client sims
// unrepresentative.
//
// Each rate is expressed in signature-units per second: an n-unit operation
// sleeps n/rate seconds, mirroring the convention the other lean clients expose
// so a single sweep config feeds every client the same sig/s rates. A
// non-positive rate disables that delay, so real deployments (rates unset) are
// unaffected.
//
// Every Sleep* method must run off the Engine tick loop (aggregation worker,
// per-attestation verify goroutines) so the node clock stays accurate.
package shadow

import "time"

// Rates holds the per-operation prover rates. The zero value is fully disabled.
type Rates struct {
	AggregateSignatures        float64
	VerifySignature            float64
	VerifyAggregatedSignatures float64
}

// SleepAggregate charges virtual time for aggregating units input signatures.
func (r Rates) SleepAggregate(units int) {
	sleep(r.AggregateSignatures, units)
}

// SleepVerify charges virtual time for verifying a single signature.
func (r Rates) SleepVerify() {
	sleep(r.VerifySignature, 1)
}

// SleepVerifyAggregated charges virtual time for verifying an aggregated
// signature over units participants.
func (r Rates) SleepVerifyAggregated(units int) {
	sleep(r.VerifyAggregatedSignatures, units)
}

func sleep(rate float64, units int) {
	if rate <= 0 || units <= 0 {
		return
	}
	// rate is signature-units per second, so an n-unit op costs n/rate seconds.
	time.Sleep(time.Duration(float64(units) / rate * float64(time.Second)))
}
