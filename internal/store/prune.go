package store

import (
	"github.com/geanlabs/gean/internal/forkchoice"
	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/storage"
)

const (
	PruningIntervalSlots = 7200
)

func PruneOnFinalization(s *ConsensusStore, fc *forkchoice.ForkChoice, oldFinalizedSlot, newFinalizedSlot uint64, newFinalizedRoot [32]byte) {
	if s == nil || s.Backend == nil || fc == nil {
		return
	}
	if newFinalizedSlot <= oldFinalizedSlot {
		return
	}

	canonical, nonCanonical := fc.GetCanonicalAnalysis(newFinalizedRoot)

	prunedStates := pruneStatesByRoots(s, nonCanonical)
	prunedBlocks := pruneBlocksByRoots(s, nonCanonical)

	if len(canonical) > 1 {
		prunedStates += pruneStatesByRoots(s, canonical[1:])
	}

	prunedChain := pruneLiveChain(s, newFinalizedSlot)

	prunedSigs := s.AttestationSignatures.PruneBelow(newFinalizedSlot)
	prunedKnown := s.KnownPayloads.PruneBelow(newFinalizedSlot)
	prunedNew := s.NewPayloads.PruneBelow(newFinalizedSlot)

	logger.Info(logger.Store, "pruning: finalized_slot=%d states=%d blocks=%d live_chain=%d gossip_sigs=%d payloads=%d non_canonical=%d",
		newFinalizedSlot, prunedStates, prunedBlocks, prunedChain, prunedSigs,
		prunedKnown+prunedNew, len(nonCanonical))
}

func PeriodicPrune(s *ConsensusStore, fc *forkchoice.ForkChoice, currentSlot, finalizedSlot uint64) {
	if s == nil || s.Backend == nil || fc == nil {
		return
	}
	if currentSlot == 0 || currentSlot%PruningIntervalSlots != 0 {
		return
	}

	if finalizedSlot+2*PruningIntervalSlots >= currentSlot {
		return
	}

	logger.Warn(logger.Store, "finalization stalled: finalized_slot=%d current_slot=%d, running periodic pruning", finalizedSlot, currentSlot)

	ancestorRoot, ancestorSlot, ok := fc.AncestorAtDepth(s.Head(), PruningIntervalSlots)
	if !ok || ancestorSlot <= finalizedSlot {
		return
	}

	_, nonCanonical := fc.GetCanonicalAnalysis(ancestorRoot)
	prunedStates := pruneStatesByRoots(s, nonCanonical)
	prunedBlocks := pruneBlocksByRoots(s, nonCanonical)

	if prunedStates > 0 || prunedBlocks > 0 {
		logger.Info(logger.Store, "periodic pruning: ancestor_slot=%d states=%d blocks=%d non_canonical=%d",
			ancestorSlot, prunedStates, prunedBlocks, len(nonCanonical))
	}
}

func pruneStatesByRoots(s *ConsensusStore, roots [][32]byte) int {
	if len(roots) == 0 {
		return 0
	}

	keys := make([][]byte, len(roots))
	for i, root := range roots {
		k := make([]byte, 32)
		copy(k, root[:])
		keys[i] = k
	}

	wb, err := s.beginWrite("prune states")
	if err != nil {
		logger.Error(logger.Store, "%v", err)
		return 0
	}
	if err := wb.DeleteBatch(storage.TableStates, keys); err != nil {
		logger.Error(logger.Store, "prune states: delete failed: %v", err)
		return 0
	}
	if !commitDeletes(wb, "prune states") {
		return 0
	}
	return len(roots)
}

func pruneBlocksByRoots(s *ConsensusStore, roots [][32]byte) int {
	if len(roots) == 0 {
		return 0
	}

	keys := make([][]byte, len(roots))
	for i, root := range roots {
		k := make([]byte, 32)
		copy(k, root[:])
		keys[i] = k
	}

	wb, err := s.beginWrite("prune blocks")
	if err != nil {
		logger.Error(logger.Store, "%v", err)
		return 0
	}
	if err := wb.DeleteBatch(storage.TableBlockHeaders, keys); err != nil {
		logger.Error(logger.Store, "prune blocks: delete headers failed: %v", err)
		return 0
	}
	if err := wb.DeleteBatch(storage.TableBlockBodies, keys); err != nil {
		logger.Error(logger.Store, "prune blocks: delete bodies failed: %v", err)
		return 0
	}
	if err := wb.DeleteBatch(storage.TableSignedBlocks, keys); err != nil {
		logger.Error(logger.Store, "prune blocks: delete signatures failed: %v", err)
		return 0
	}
	if !commitDeletes(wb, "prune blocks") {
		return 0
	}
	return len(roots)
}

func pruneLiveChain(s *ConsensusStore, finalizedSlot uint64) int {
	rv, err := s.beginRead("prune live chain")
	if err != nil {
		return 0
	}

	iter, err := rv.PrefixIterator(storage.TableLiveChain, nil)
	if err != nil {
		return 0
	}
	defer iter.Close()

	var keysToDelete [][]byte
	for iter.Next() {
		key := iter.Key()
		if len(key) < storage.LiveChainKeySize {
			continue
		}
		slot, _ := storage.DecodeLiveChainKey(key)
		if slot < finalizedSlot {
			k := make([]byte, len(key))
			copy(k, key)
			keysToDelete = append(keysToDelete, k)
		}
	}

	if len(keysToDelete) == 0 {
		return 0
	}

	wb, err := s.beginWrite("prune live chain")
	if err != nil {
		logger.Error(logger.Store, "%v", err)
		return 0
	}
	if err := wb.DeleteBatch(storage.TableLiveChain, keysToDelete); err != nil {
		logger.Error(logger.Store, "prune live chain: delete failed: %v", err)
		return 0
	}
	if !commitDeletes(wb, "prune live chain") {
		return 0
	}
	return len(keysToDelete)
}
