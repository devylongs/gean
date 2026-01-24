package p2p

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/devylongs/gean/consensus"
)

// BlockHandler processes incoming blocks from gossipsub.
type BlockHandler func(ctx context.Context, block *consensus.SignedBlock) error

// AttestationHandler processes incoming attestations from gossipsub.
type AttestationHandler func(ctx context.Context, vote *consensus.SignedVote) error

// MessageHandlers holds handlers for different message types.
type MessageHandlers struct {
	OnBlock       BlockHandler
	OnAttestation AttestationHandler
	Logger        *slog.Logger
}

// HandleBlockMessage decodes and processes an incoming block message.
func (h *MessageHandlers) HandleBlockMessage(ctx context.Context, data []byte) error {
	// Decompress
	decoded, err := DecompressMessage(data)
	if err != nil {
		return fmt.Errorf("decompress block: %w", err)
	}

	// Decode SSZ
	var block consensus.SignedBlock
	if err := block.UnmarshalSSZ(decoded); err != nil {
		return fmt.Errorf("unmarshal block: %w", err)
	}

	if h.Logger != nil {
		h.Logger.Info("received block",
			"slot", block.Message.Slot,
			"proposer", block.Message.ProposerIndex,
		)
	}

	if h.OnBlock != nil {
		return h.OnBlock(ctx, &block)
	}

	return nil
}

// HandleAttestationMessage decodes and processes an incoming attestation.
func (h *MessageHandlers) HandleAttestationMessage(ctx context.Context, data []byte) error {
	// Decompress
	decoded, err := DecompressMessage(data)
	if err != nil {
		return fmt.Errorf("decompress attestation: %w", err)
	}

	// Decode SSZ
	var vote consensus.SignedVote
	if err := vote.UnmarshalSSZ(decoded); err != nil {
		return fmt.Errorf("unmarshal attestation: %w", err)
	}

	if h.Logger != nil {
		h.Logger.Info("received attestation",
			"slot", vote.Data.Slot,
			"validator", vote.Data.ValidatorID,
		)
	}

	if h.OnAttestation != nil {
		return h.OnAttestation(ctx, &vote)
	}

	return nil
}
