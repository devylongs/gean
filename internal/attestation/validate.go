package attestation

import (
	"fmt"
	"math"

	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func errMalformedAttestationData(field string) error {
	return fmt.Errorf("malformed attestation data: %s is nil", field)
}

func errUnknownSourceBlock(root [32]byte) error {
	return &store.StoreError{Kind: store.ErrUnknownSourceBlock, Message: fmt.Sprintf("unknown source block: %x", root[:4])}
}

func errUnknownTargetBlock(root [32]byte) error {
	return &store.StoreError{Kind: store.ErrUnknownTargetBlock, Message: fmt.Sprintf("unknown target block: %x", root[:4])}
}

func errUnknownHeadBlock(root [32]byte) error {
	return &store.StoreError{Kind: store.ErrUnknownHeadBlock, Message: fmt.Sprintf("unknown head block: %x", root[:4])}
}

func errSourceExceedsTarget() error {
	return &store.StoreError{Kind: store.ErrSourceExceedsTarget, Message: "source checkpoint slot exceeds target"}
}

func errHeadOlderThanTarget(headSlot, targetSlot uint64) error {
	return &store.StoreError{Kind: store.ErrHeadOlderThanTarget, Message: fmt.Sprintf("head slot %d older than target slot %d", headSlot, targetSlot)}
}

func errSourceSlotMismatch(cpSlot, blockSlot uint64) error {
	return &store.StoreError{Kind: store.ErrSourceSlotMismatch, Message: fmt.Sprintf("source checkpoint slot %d != block slot %d", cpSlot, blockSlot)}
}

func errTargetSlotMismatch(cpSlot, blockSlot uint64) error {
	return &store.StoreError{Kind: store.ErrTargetSlotMismatch, Message: fmt.Sprintf("target checkpoint slot %d != block slot %d", cpSlot, blockSlot)}
}

func errHeadSlotMismatch(cpSlot, blockSlot uint64) error {
	return &store.StoreError{Kind: store.ErrHeadSlotMismatch, Message: fmt.Sprintf("head checkpoint slot %d != block slot %d", cpSlot, blockSlot)}
}

func errAttestationTooFarInFuture(attSlot, storeTime uint64) error {
	return &store.StoreError{Kind: store.ErrAttestationTooFarInFuture, Message: fmt.Sprintf("attestation slot %d too far in future (store time %d intervals)", attSlot, storeTime)}
}

func errAttestationSlotBeforeHead(attSlot, headSlot uint64) error {
	return &store.StoreError{Kind: store.ErrAttestationSlotBeforeHead, Message: fmt.Sprintf("attestation slot %d precedes head slot %d", attSlot, headSlot)}
}

func errSourceNotAncestorOfTarget() error {
	return &store.StoreError{Kind: store.ErrSourceNotAncestorOfTarget, Message: "source checkpoint is not an ancestor of target"}
}

func errTargetNotAncestorOfHead() error {
	return &store.StoreError{Kind: store.ErrTargetNotAncestorOfHead, Message: "target checkpoint is not an ancestor of head"}
}

func errHeadNotDescendantOfFinalized() error {
	return &store.StoreError{Kind: store.ErrHeadNotDescendantOfFinalized, Message: "head checkpoint does not descend from the finalized block"}
}

func ValidateAttestationData(s *store.ConsensusStore, data *types.AttestationData) error {
	if err := validateDataShape(data); err != nil {
		return err
	}

	sourceHeader := s.GetBlockHeader(data.Source.Root)
	if sourceHeader == nil {
		return errUnknownSourceBlock(data.Source.Root)
	}
	targetHeader := s.GetBlockHeader(data.Target.Root)
	if targetHeader == nil {
		return errUnknownTargetBlock(data.Target.Root)
	}
	headHeader := s.GetBlockHeader(data.Head.Root)
	if headHeader == nil {
		return errUnknownHeadBlock(data.Head.Root)
	}

	if data.Source.Slot > data.Target.Slot {
		return errSourceExceedsTarget()
	}
	if data.Head.Slot < data.Target.Slot {
		return errHeadOlderThanTarget(data.Head.Slot, data.Target.Slot)
	}
	if sourceHeader.Slot != data.Source.Slot {
		return errSourceSlotMismatch(data.Source.Slot, sourceHeader.Slot)
	}
	if targetHeader.Slot != data.Target.Slot {
		return errTargetSlotMismatch(data.Target.Slot, targetHeader.Slot)
	}
	if headHeader.Slot != data.Head.Slot {
		return errHeadSlotMismatch(data.Head.Slot, headHeader.Slot)
	}
	if !checkpointIsAncestor(s, data.Source, data.Target) {
		return errSourceNotAncestorOfTarget()
	}
	if !checkpointIsAncestor(s, data.Target, data.Head) {
		return errTargetNotAncestorOfHead()
	}
	// Fork choice only ever descends from the finalized block, so an orphaned head
	// carries no weight. Rejecting it at admission keeps a re-gossiped
	// below-finalized aggregate from re-entering the pool. This is leanSpec's
	// prune_stale_attestation_data predicate (head above finalized, and
	// descended from it); the store's own PruneBelow currently keys on slot
	// alone, so the two are not yet the same test.
	if finalized := s.LatestFinalized(); finalized != nil && !checkpointIsAncestor(s, finalized, data.Head) {
		return errHeadNotDescendantOfFinalized()
	}
	// A vote cannot have observed its head before that head existed. This lower
	// bound also keeps the wire slot clear of the 2**64 interval-multiply overflow.
	if data.Slot < data.Head.Slot {
		return errAttestationSlotBeforeHead(data.Slot, data.Head.Slot)
	}
	if data.Slot > math.MaxUint64/types.IntervalsPerSlot ||
		data.Slot*types.IntervalsPerSlot > s.Time()+types.GossipDisparityIntervals {
		return errAttestationTooFarInFuture(data.Slot, s.Time())
	}

	return nil
}

// checkpointIsAncestor reports whether ancestor lies on descendant's parent
// chain. Mirrors leanSpec _checkpoint_is_ancestor: climb parent links from the
// descendant; the ancestor's slot must carry its exact root, otherwise the two
// checkpoints sit on forked branches.
func checkpointIsAncestor(s *store.ConsensusStore, ancestor, descendant *types.Checkpoint) bool {
	if ancestor.Slot > descendant.Slot {
		return false
	}
	current := descendant.Root
	for {
		header := s.GetBlockHeader(current)
		if header == nil {
			return false
		}
		if header.Slot == ancestor.Slot {
			return current == ancestor.Root
		}
		if header.Slot < ancestor.Slot {
			return false
		}
		current = header.ParentRoot
	}
}

func validateDataShape(data *types.AttestationData) error {
	if data == nil {
		return errMalformedAttestationData("data")
	}
	if data.Source == nil {
		return errMalformedAttestationData("source")
	}
	if data.Target == nil {
		return errMalformedAttestationData("target")
	}
	if data.Head == nil {
		return errMalformedAttestationData("head")
	}
	return nil
}
