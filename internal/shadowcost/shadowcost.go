// Package shadowcost models XMSS prover CPU cost as wall-clock delay.
//
// A network simulator's virtual clock advances on blocking/sleeping/IO but not
// during pure computation, so the post-quantum prover — which is the dominant CPU
// consumer — would otherwise register as free in simulated time and distort slot
// pacing. Injecting a sleep proportional to that cost makes simulated time reflect
// it. The zero value is disabled: every delay method is a no-op, so production
// builds that never set these values carry no behavior change.
//
// Only operations that run off the engine tick loop are modeled here; delaying
// work on the tick loop would desync slot derivation.
package shadowcost

import "time"

// Costs holds the simulated per-operation prover delays. A zero duration leaves
// the corresponding operation unmodeled.
type Costs struct {
	Aggregate time.Duration // signature aggregation (aggregation worker)
	Verify    time.Duration // single attestation signature verification
}

// Enabled reports whether any delay is configured.
func (c Costs) Enabled() bool {
	return c.Aggregate > 0 || c.Verify > 0
}

// DelayAggregate sleeps for the configured aggregation delay, if any.
func (c Costs) DelayAggregate() { sleep(c.Aggregate) }

// DelayVerify sleeps for the configured verification delay, if any.
func (c Costs) DelayVerify() { sleep(c.Verify) }

func sleep(d time.Duration) {
	if d > 0 {
		time.Sleep(d)
	}
}
