package node

import (
	"github.com/geanlabs/gean/internal/dutygate"
	"github.com/geanlabs/gean/internal/logger"
)

// networkSeenSlot is the highest slot this node has evidence the network
// reached: stored blocks plus gossip it heard, admitted or not. The duty
// gate's network-stall carve-out keys off this value; feeding it stored slots
// alone lets a node whose imports are failing conclude the whole network is
// stalled and keep proposing on a stale head — observed on devnet as a node
// extending a dead fork for a day with its pending cache rejecting every
// block it heard.
func (e *Engine) networkSeenSlot() uint64 {
	stored := e.Store.MaxStoredBlockSlot()
	if seen := e.maxSeenGossipSlot.Load(); seen > stored {
		return seen
	}
	return stored
}

func logDutyGateEvent(event dutygate.Event) {
	switch event.Reason {
	case dutygate.ReasonNetworkStall:
		logger.Info(logger.Validator, "duty gate reopened: network stall detected. duty=%s slot=%d head_slot=%d lag=%d max_seen_slot=%d network_lag=%d",
			event.Duty, event.Slot, event.HeadSlot, event.Lag, event.MaxStoredSlot, event.NetworkLag)
	case dutygate.ReasonCaughtUp:
		logger.Info(logger.Validator, "duty gate reopened: local view caught up. duty=%s slot=%d head_slot=%d lag=%d",
			event.Duty, event.Slot, event.HeadSlot, event.Lag)
	case dutygate.ReasonLocalLag:
		logger.Info(logger.Validator, "duty gate closed: local view is stale. duty=%s slot=%d head_slot=%d lag=%d max_seen_slot=%d network_lag=%d",
			event.Duty, event.Slot, event.HeadSlot, event.Lag, event.MaxStoredSlot, event.NetworkLag)
	}
}
