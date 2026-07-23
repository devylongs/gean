package node

import (
	"fmt"
	"time"

	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/metrics"
	"github.com/geanlabs/gean/internal/syncer"
	"github.com/geanlabs/gean/internal/types"
)

const SyncLagSlots = 2

func (e *Engine) updateSyncStatus(currentSlot uint64) {
	status := e.computeSyncStatus(currentSlot)
	metrics.SetSyncStatus(status.String())
}

func (e *Engine) computeSyncStatus(currentSlot uint64) syncer.SyncStatus {
	if e.P2P != nil && e.P2P.ConnectedPeers() == 0 {
		return syncer.SyncIdle
	}
	headSlot := e.Store.HeadSlot()
	if currentSlot <= headSlot || currentSlot-headSlot <= SyncLagSlots {
		return syncer.SyncSynced
	}
	return syncer.SyncSyncing
}

func (e *Engine) GetSyncStatus() syncer.SyncStatus {
	return e.computeSyncStatus(e.currentSlot(uint64(time.Now().UnixMilli())))
}

func (e *Engine) logChainStatus(currentSlot uint64) {
	metrics.SampleProcessRSS()
	headRoot := e.Store.Head()
	headHeader := e.Store.GetBlockHeader(headRoot)
	justified := e.Store.LatestJustified()
	finalized := e.Store.LatestFinalized()

	headSlot := uint64(0)
	parentRoot := types.ZeroRoot
	stateRoot := types.ZeroRoot
	if headHeader != nil {
		headSlot = headHeader.Slot
		parentRoot = headHeader.ParentRoot
		stateRoot = headHeader.StateRoot
	}

	behind := uint64(0)
	if currentSlot > headSlot {
		behind = currentSlot - headSlot
	}

	peerCount := 0
	if e.P2P != nil {
		peerCount = e.P2P.ConnectedPeers()
	}

	gossipSigs := e.Store.AttestationSignatures.Len()
	knownPayloads := e.Store.KnownPayloads.Len()
	statesCount := e.Store.StatesCount()
	fcNodesCount := 0
	if e.FC != nil {
		fcNodesCount = e.FC.Len()
	}

	meshInfo := ""
	if e.P2P != nil {
		meshSizes := e.P2P.TopicMeshSizes()
		for topic, size := range meshSizes {
			meshInfo += fmt.Sprintf("\n  %-60s mesh_peers=%d", topic, size)
		}
	}

	logger.Info(logger.Chain, "\n\n+===============================================================+\n  CHAIN STATUS: Current Slot: %d | Head Slot: %d | Behind: %d\n+---------------------------------------------------------------+\n  Connected Peers:    %d\n+---------------------------------------------------------------+\n  Head Block Root:    0x%x\n  Parent Block Root:  0x%x\n  State Root:         0x%x\n+---------------------------------------------------------------+\n  Latest Justified:   Slot %6d | Root: 0x%x\n  Latest Finalized:   Slot %6d | Root: 0x%x\n+---------------------------------------------------------------+\n  Gossip Sigs: %d | Known Payloads: %d | States: %d | FC Nodes: %d\n+---------------------------------------------------------------+\n  Topics:%s\n+===============================================================+\n",
		currentSlot, headSlot, behind,
		peerCount,
		headRoot, parentRoot, stateRoot,
		justified.Slot, justified.Root,
		finalized.Slot, finalized.Root,
		gossipSigs, knownPayloads, statesCount, fcNodesCount,
		meshInfo)
}

func (e *Engine) refreshGossipMeshPeers() {
	if e.P2P == nil {
		return
	}
	metrics.SetGossipMeshPeers(e.P2P.MeshPeerCount())
}
