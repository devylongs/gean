package node

import "github.com/geanlabs/gean/internal/types"

func (e *Engine) currentSlot(timestampMs uint64) uint64 {
	if e == nil || e.Store == nil {
		return 0
	}
	return types.CurrentSlot(e.Store.Config().GenesisTime, timestampMs)
}

func (e *Engine) currentInterval(timestampMs uint64) uint64 {
	if e == nil || e.Store == nil {
		return 0
	}
	return types.CurrentInterval(e.Store.Config().GenesisTime, timestampMs)
}

func (e *Engine) millisIntoSlot(timestampMs uint64) uint64 {
	if e == nil || e.Store == nil {
		return 0
	}
	return types.MillisIntoSlot(e.Store.Config().GenesisTime, timestampMs)
}
