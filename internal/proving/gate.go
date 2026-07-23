package proving

import (
	"context"
	"sync/atomic"
)

type Gate struct {
	token           chan struct{}
	proposalPending atomic.Bool
}

func NewGate() *Gate {
	g := &Gate{token: make(chan struct{}, 1)}
	g.token <- struct{}{}
	return g
}

func (g *Gate) Acquire(ctx context.Context, proposal bool) bool {
	if proposal {
		g.proposalPending.Store(true)
	} else if g.proposalPending.Load() {
		return false
	}
	select {
	case <-ctx.Done():
		if proposal {
			g.proposalPending.Store(false)
		}
		return false
	case <-g.token:
		if !proposal && g.proposalPending.Load() {
			g.token <- struct{}{}
			return false
		}
		return true
	}
}

func (g *Gate) Release(proposal bool) {
	// Clear the priority flag before returning the token: a background waiter
	// woken by the token send re-checks the flag, and the reverse order lets it
	// observe a stale true and spuriously cancel its work.
	if proposal {
		g.proposalPending.Store(false)
	}
	g.token <- struct{}{}
}
