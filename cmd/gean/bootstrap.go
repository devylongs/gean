package main

import (
	"fmt"

	"github.com/geanlabs/gean/internal/checkpoint"
	"github.com/geanlabs/gean/internal/genesis"
	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/p2p"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
	"github.com/geanlabs/gean/xmss"
	"github.com/multiformats/go-multiaddr"
)

type startupInputs struct {
	genesisConfig *genesis.GenesisConfig
	bootnodes     []multiaddr.Multiaddr
	keyManager    *xmss.KeyManager
}

func loadStartupInputs(cfg config) (*startupInputs, error) {
	paths := cfg.paths()

	genesisConfig, err := genesis.LoadGenesisConfig(paths.config)
	if err != nil {
		logger.Error(logger.Node, "load genesis config: %v", err)
		return nil, err
	}
	logger.Info(logger.Node, "genesis: time=%d validators=%d", genesisConfig.GenesisTime, len(genesisConfig.GenesisValidators))

	bootnodes, err := p2p.LoadBootnodes(paths.bootnodes)
	if err != nil {
		logger.Error(logger.Node, "load bootnodes: %v", err)
		return nil, err
	}
	logger.Info(logger.Node, "bootnodes: %d loaded", len(bootnodes))

	keyManager, err := xmss.LoadValidatorKeys(paths.validators, paths.keysDir, cfg.NodeID)
	if err != nil {
		logger.Error(logger.Node, "load validator keys: %v", err)
		return nil, err
	}
	logger.Info(logger.Node, "validators: %d keys loaded for %s", len(keyManager.ValidatorIDs()), cfg.NodeID)

	return &startupInputs{
		genesisConfig: genesisConfig,
		bootnodes:     bootnodes,
		keyManager:    keyManager,
	}, nil
}

func bootstrapStore(s *store.ConsensusStore, genesisConfig *genesis.GenesisConfig, checkpointURL string) error {
	// Surface the checkpoint-sync configuration up front: a node expected to
	// checkpoint-sync that silently starts from genesis (no url reached the
	// binary) otherwise looks identical to a normal genesis start in the logs.
	if checkpointURL != "" {
		logger.Info(logger.Node, "checkpoint sync configured: url=%s", checkpointURL)
	} else {
		logger.Info(logger.Node, "checkpoint sync not configured (no --checkpoint-sync-url)")
	}

	existingHead := s.Head()
	existingHeader := s.GetBlockHeader(existingHead)
	existingState := s.GetState(existingHead)

	if existingHeader != nil && existingState != nil && existingHeader.Slot > 0 {
		if s.GetSignedBlock(existingHead) == nil {
			return fmt.Errorf("incompatible pre-devnet-5 data directory: signed head block missing; reset the data directory")
		}
		logger.Info(logger.Node, "restoring from database: slot=%d head=%x justified=%d finalized=%d",
			existingHeader.Slot, existingHead,
			s.LatestJustified().Slot, s.LatestFinalized().Slot)
		return nil
	}

	if checkpointURL != "" {
		return bootstrapFromCheckpoint(s, genesisConfig, checkpointURL)
	}

	return bootstrapFromGenesis(s, genesisConfig)
}

func bootstrapFromCheckpoint(s *store.ConsensusStore, genesisConfig *genesis.GenesisConfig, checkpointURL string) error {
	logger.Info(logger.Sync, "checkpoint sync: %s", checkpointURL)
	validators, err := genesisConfig.Validators()
	if err != nil {
		return fmt.Errorf("genesis validators: %w", err)
	}
	state, signedBlock, err := checkpoint.FetchCheckpointAnchor(checkpointURL, genesisConfig.GenesisTime, validators)
	if err != nil {
		logger.Error(logger.Sync, "checkpoint sync failed: %v", err)
		return fmt.Errorf("checkpoint sync failed: %w", err)
	}

	canonicalRoot, err := initStoreFromState(s, state)
	if err != nil {
		return err
	}
	stateRoot := state.LatestBlockHeader.StateRoot
	logger.Info(logger.Sync, "checkpoint sync: slot=%d finalized_root=%x justified_root=%x head_root=%x parent_root=%x state_root=%x",
		state.Slot, state.LatestFinalized.Root, state.LatestJustified.Root, canonicalRoot, state.LatestBlockHeader.ParentRoot, stateRoot)
	if err := s.StorePendingBlock(canonicalRoot, signedBlock); err != nil {
		return fmt.Errorf("store checkpoint block: %w", err)
	}
	return nil
}

func bootstrapFromGenesis(s *store.ConsensusStore, genesisConfig *genesis.GenesisConfig) error {
	logger.Info(logger.Node, "initializing from genesis")
	genesisState, err := genesisConfig.GenesisState()
	if err != nil {
		return fmt.Errorf("build genesis state: %w", err)
	}
	canonicalRoot, err := initStoreFromState(s, genesisState)
	if err != nil {
		return err
	}
	genesisSignedBlock := &types.SignedBlock{
		Block: &types.Block{
			Slot:          genesisState.LatestBlockHeader.Slot,
			ProposerIndex: genesisState.LatestBlockHeader.ProposerIndex,
			ParentRoot:    genesisState.LatestBlockHeader.ParentRoot,
			StateRoot:     genesisState.LatestBlockHeader.StateRoot,
			Body:          &types.BlockBody{},
		},
		Proof: &types.MultiMessageAggregate{},
	}
	if err := s.StorePendingBlock(canonicalRoot, genesisSignedBlock); err != nil {
		return fmt.Errorf("store genesis block: %w", err)
	}
	return nil
}
