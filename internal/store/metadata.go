package store

import (
	"encoding/binary"
	"fmt"

	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/types"
)

func (s *ConsensusStore) Time() uint64 {
	return s.getMetadataUint64(storage.KeyTime)
}

func (s *ConsensusStore) SetTime(t uint64) {
	if err := s.PutTime(t); err != nil {
		logger.Error(logger.Store, "%v", err)
	}
}

func (s *ConsensusStore) PutTime(t uint64) error {
	return s.putMetadataUint64(storage.KeyTime, t, "set time")
}

func (s *ConsensusStore) Head() [32]byte {
	return s.getMetadataRoot(storage.KeyHead)
}

func (s *ConsensusStore) SetHead(root [32]byte) {
	if err := s.PutHead(root); err != nil {
		logger.Error(logger.Store, "%v", err)
	}
}

func (s *ConsensusStore) PutHead(root [32]byte) error {
	return s.putMetadataRoot(storage.KeyHead, root, "set head")
}

func (s *ConsensusStore) SafeTarget() [32]byte {
	return s.getMetadataRoot(storage.KeySafeTarget)
}

func (s *ConsensusStore) SetSafeTarget(root [32]byte) {
	if err := s.PutSafeTarget(root); err != nil {
		logger.Error(logger.Store, "%v", err)
	}
}

func (s *ConsensusStore) PutSafeTarget(root [32]byte) error {
	return s.putMetadataRoot(storage.KeySafeTarget, root, "set safe target")
}

func (s *ConsensusStore) LatestJustified() *types.Checkpoint {
	return s.getMetadataCheckpoint(storage.KeyLatestJustified)
}

func (s *ConsensusStore) SetLatestJustified(cp *types.Checkpoint) {
	if err := s.PutLatestJustified(cp); err != nil {
		logger.Error(logger.Store, "%v", err)
	}
}

func (s *ConsensusStore) PutLatestJustified(cp *types.Checkpoint) error {
	return s.putMetadataCheckpoint(storage.KeyLatestJustified, cp, "set latest justified")
}

func (s *ConsensusStore) LatestFinalized() *types.Checkpoint {
	return s.getMetadataCheckpoint(storage.KeyLatestFinalized)
}

func (s *ConsensusStore) SetLatestFinalized(cp *types.Checkpoint) {
	if err := s.PutLatestFinalized(cp); err != nil {
		logger.Error(logger.Store, "%v", err)
	}
}

func (s *ConsensusStore) PutLatestFinalized(cp *types.Checkpoint) error {
	return s.putMetadataCheckpoint(storage.KeyLatestFinalized, cp, "set latest finalized")
}

// DeriveFinalizedFromHead resolves the finalized checkpoint implied by the head's
// post-state, mirroring the spec's update_head: it climbs the head's chain to the
// ancestor block at the post-state finalized slot. Finalization is anchored to the
// canonical head rather than advanced per imported block, so a losing fork that
// finalizes a higher slot cannot latch finalization above the head and stall target
// advancement. Returns nil when no stored block sits exactly at that slot (e.g. a
// fresh checkpoint-sync anchor), so the caller keeps its current trusted checkpoint.
func DeriveFinalizedFromHead(s *ConsensusStore, headRoot [32]byte) *types.Checkpoint {
	if s == nil {
		return nil
	}
	headState := s.GetState(headRoot)
	if headState == nil || headState.LatestFinalized == nil {
		return nil
	}
	finalizedSlot := headState.LatestFinalized.Slot

	root := headRoot
	for {
		header := s.GetBlockHeader(root)
		if header == nil || header.Slot <= finalizedSlot {
			break
		}
		if s.GetBlockHeader(header.ParentRoot) == nil {
			break
		}
		root = header.ParentRoot
	}

	if header := s.GetBlockHeader(root); header != nil && header.Slot == finalizedSlot {
		return &types.Checkpoint{Root: root, Slot: finalizedSlot}
	}
	return nil
}

func (s *ConsensusStore) Config() *types.ChainConfig {
	rv, err := s.beginRead("read config")
	if err != nil {
		return &types.ChainConfig{}
	}
	val, err := rv.Get(storage.TableMetadata, storage.KeyConfig)
	if err != nil || val == nil {
		return &types.ChainConfig{}
	}
	cfg := &types.ChainConfig{}
	if err := cfg.UnmarshalSSZ(val); err != nil {
		return &types.ChainConfig{}
	}
	return cfg
}

func (s *ConsensusStore) SetConfig(cfg *types.ChainConfig) {
	if err := s.PutConfig(cfg); err != nil {
		logger.Error(logger.Store, "%v", err)
	}
}

func (s *ConsensusStore) PutConfig(cfg *types.ChainConfig) error {
	if cfg == nil {
		return fmt.Errorf("set config: config is nil")
	}
	data, err := cfg.MarshalSSZ()
	if err != nil {
		return fmt.Errorf("set config: marshal: %w", err)
	}
	return s.putMetadata(storage.KeyConfig, data, "set config")
}

func (s *ConsensusStore) getMetadataUint64(key []byte) uint64 {
	rv, err := s.beginRead("read metadata uint64")
	if err != nil {
		return 0
	}
	val, err := rv.Get(storage.TableMetadata, key)
	if err != nil || len(val) < 8 {
		return 0
	}
	return binary.LittleEndian.Uint64(val[:8])
}

func (s *ConsensusStore) putMetadataUint64(key []byte, val uint64, label string) error {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, val)
	return s.putMetadata(key, buf, label)
}

func (s *ConsensusStore) getMetadataRoot(key []byte) [32]byte {
	rv, err := s.beginRead("read metadata root")
	if err != nil {
		return [32]byte{}
	}
	val, err := rv.Get(storage.TableMetadata, key)
	if err != nil || len(val) < 32 {
		return [32]byte{}
	}
	var root [32]byte
	copy(root[:], val)
	return root
}

func (s *ConsensusStore) putMetadataRoot(key []byte, root [32]byte, label string) error {
	return s.putMetadata(key, root[:], label)
}

func (s *ConsensusStore) getMetadataCheckpoint(key []byte) *types.Checkpoint {
	rv, err := s.beginRead("read metadata checkpoint")
	if err != nil {
		return &types.Checkpoint{}
	}
	val, err := rv.Get(storage.TableMetadata, key)
	if err != nil || val == nil {
		return &types.Checkpoint{}
	}
	cp := &types.Checkpoint{}
	if err := cp.UnmarshalSSZ(val); err != nil {
		return &types.Checkpoint{}
	}
	return cp
}

func (s *ConsensusStore) putMetadataCheckpoint(key []byte, cp *types.Checkpoint, label string) error {
	if cp == nil {
		return fmt.Errorf("%s: checkpoint is nil", label)
	}
	data, err := cp.MarshalSSZ()
	if err != nil {
		return fmt.Errorf("%s: marshal: %w", label, err)
	}
	return s.putMetadata(key, data, label)
}

func (s *ConsensusStore) putMetadata(key, value []byte, label string) error {
	return s.putOne(storage.TableMetadata, key, value, label)
}
