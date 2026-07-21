package checkpoint

import (
	"fmt"
	"time"

	"github.com/geanlabs/gean/internal/types"
)

func FetchCheckpointAnchor(
	stateURL string,
	expectedGenesisTime uint64,
	expectedValidators []*types.Validator,
) (*types.State, *types.SignedBlock, error) {
	blockURL, err := deriveBlockURL(stateURL)
	if err != nil {
		return nil, nil, err
	}

	// State and block form one anchor pair; share a single fetch budget so a source
	// that is briefly unready at boot is retried as a whole rather than each request
	// racing its own deadline.
	deadline := time.Now().Add(checkpointFetchBudget)

	state, err := fetchState(stateURL, deadline)
	if err != nil {
		return nil, nil, fmt.Errorf("fetch state: %w", err)
	}

	signedBlock, err := fetchBlock(blockURL, deadline)
	if err != nil {
		return nil, nil, fmt.Errorf("fetch block: %w", err)
	}

	if err := verifyAnchorPair(state, signedBlock); err != nil {
		return nil, nil, err
	}
	if err := VerifyCheckpointState(state, expectedGenesisTime, expectedValidators); err != nil {
		return nil, nil, fmt.Errorf("verify state: %w", err)
	}

	return state, signedBlock, nil
}

func fetchState(stateURL string, deadline time.Time) (*types.State, error) {
	body, err := fetchSSZ(stateURL, deadline)
	if err != nil {
		return nil, err
	}

	state := &types.State{}
	if err := state.UnmarshalSSZ(body); err != nil {
		return nil, fmt.Errorf("state ssz decode: %w", err)
	}
	return state, nil
}

func fetchBlock(blockURL string, deadline time.Time) (*types.SignedBlock, error) {
	body, err := fetchSSZ(blockURL, deadline)
	if err != nil {
		return nil, err
	}

	signedBlock := &types.SignedBlock{}
	if err := signedBlock.UnmarshalSSZ(body); err != nil {
		return nil, fmt.Errorf("block ssz decode: %w", err)
	}
	if signedBlock.Block == nil {
		return nil, fmt.Errorf("block ssz decode: nil block")
	}
	return signedBlock, nil
}

func verifyAnchorPair(state *types.State, signedBlock *types.SignedBlock) error {
	if state == nil {
		return fmt.Errorf("checkpoint state is nil")
	}
	if state.LatestBlockHeader == nil {
		return fmt.Errorf("checkpoint state latest block header is nil")
	}
	if signedBlock == nil || signedBlock.Block == nil {
		return fmt.Errorf("checkpoint block is nil")
	}
	if signedBlock.Block.Body == nil {
		return fmt.Errorf("checkpoint block body is nil")
	}

	stateRoot, err := state.HashTreeRoot()
	if err != nil {
		return fmt.Errorf("state hash_tree_root: %w", err)
	}
	if signedBlock.Block.StateRoot != stateRoot {
		return fmt.Errorf(
			"checkpoint state/block pair mismatch: block.state_root=0x%x, hash_tree_root(state)=0x%x",
			signedBlock.Block.StateRoot, stateRoot,
		)
	}

	blockHeader, err := headerFromBlock(signedBlock.Block)
	if err != nil {
		return err
	}
	expectedHeader := *state.LatestBlockHeader
	if expectedHeader.StateRoot == types.ZeroRoot {
		expectedHeader.StateRoot = stateRoot
	}
	if *blockHeader != expectedHeader {
		return fmt.Errorf("checkpoint state/block pair mismatch: block header does not match state latest block header")
	}
	return nil
}

func headerFromBlock(block *types.Block) (*types.BlockHeader, error) {
	if block == nil {
		return nil, fmt.Errorf("checkpoint block is nil")
	}
	if block.Body == nil {
		return nil, fmt.Errorf("checkpoint block body is nil")
	}

	bodyRoot, err := block.Body.HashTreeRoot()
	if err != nil {
		return nil, fmt.Errorf("block body hash_tree_root: %w", err)
	}
	return &types.BlockHeader{
		Slot:          block.Slot,
		ProposerIndex: block.ProposerIndex,
		ParentRoot:    block.ParentRoot,
		StateRoot:     block.StateRoot,
		BodyRoot:      bodyRoot,
	}, nil
}
