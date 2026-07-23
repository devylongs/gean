package store

import (
	"encoding/binary"
	"fmt"

	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/types"
)

func (s *ConsensusStore) GetBlockHeader(root [32]byte) *types.BlockHeader {
	rv, err := s.beginRead("get block header")
	if err != nil {
		return nil
	}
	val, err := rv.Get(storage.TableBlockHeaders, root[:])
	if err != nil || val == nil {
		return nil
	}
	h := &types.BlockHeader{}
	if err := h.UnmarshalSSZ(val); err != nil {
		return nil
	}
	return h
}

func (s *ConsensusStore) InsertBlockHeader(root [32]byte, header *types.BlockHeader) {
	if err := s.PutBlockHeader(root, header); err != nil {
		logger.Error(logger.Store, "%v", err)
	}
}

func (s *ConsensusStore) PutBlockHeader(root [32]byte, header *types.BlockHeader) error {
	if header == nil {
		return fmt.Errorf("insert block header: header is nil")
	}
	data, err := header.MarshalSSZ()
	if err != nil {
		return fmt.Errorf("insert block header: marshal: %w", err)
	}
	return s.putOne(storage.TableBlockHeaders, root[:], data, "insert block header")
}

func (s *ConsensusStore) BlockRoots() (map[[32]byte]bool, error) {
	rv, err := s.beginRead("load block roots")
	if err != nil {
		return nil, err
	}

	it, err := rv.PrefixIterator(storage.TableBlockHeaders, nil)
	if err != nil {
		return nil, fmt.Errorf("iterate block headers: %w", err)
	}
	defer it.Close()

	roots := make(map[[32]byte]bool)
	for it.Next() {
		key := it.Key()
		if len(key) < len(types.ZeroRoot) {
			continue
		}
		var root [32]byte
		copy(root[:], key)
		roots[root] = true
	}
	return roots, nil
}

func (s *ConsensusStore) GetSignedBlock(root [32]byte) *types.SignedBlock {
	rv, err := s.beginRead("get signed block")
	if err != nil {
		return nil
	}
	data, err := rv.Get(storage.TableSignedBlocks, root[:])
	if err != nil || data == nil {
		return nil
	}

	block := &types.SignedBlock{}
	if err := block.UnmarshalSSZ(data); err != nil || block.Block == nil {
		return nil
	}
	return block
}

func WriteBlockData(s *ConsensusStore, root [32]byte, signedBlock *types.SignedBlock) error {
	if s == nil || s.Backend == nil {
		return fmt.Errorf("write block data: store backend is nil")
	}
	if signedBlock == nil {
		return fmt.Errorf("write block data: signed block is nil")
	}
	if signedBlock.Block == nil {
		return fmt.Errorf("write block data: block is nil")
	}
	if signedBlock.Block.Body == nil {
		return fmt.Errorf("write block data: block body is nil")
	}

	fullData, err := signedBlock.MarshalSSZ()
	if err != nil {
		return fmt.Errorf("write block data: marshal signed block: %w", err)
	}

	var bodyData []byte
	bodyData, err = signedBlock.Block.Body.MarshalSSZ()
	if err != nil {
		return fmt.Errorf("write block data: marshal body: %w", err)
	}

	wb, err := s.beginWrite("write block data")
	if err != nil {
		return err
	}
	if len(bodyData) > 0 {
		if err := wb.PutBatch(storage.TableBlockBodies, []storage.KV{{Key: root[:], Value: bodyData}}); err != nil {
			return fmt.Errorf("write block data: put body: %w", err)
		}
	}
	if err := wb.PutBatch(storage.TableSignedBlocks, []storage.KV{{Key: root[:], Value: fullData}}); err != nil {
		return fmt.Errorf("write block data: put signed block: %w", err)
	}
	if err := wb.Commit(); err != nil {
		return fmt.Errorf("write block data: commit: %w", err)
	}

	logger.Info(logger.Store, "wrote block data: root=0x%x body_bytes=%d signed_bytes=%d",
		root, len(bodyData), len(fullData))
	return nil
}

func (s *ConsensusStore) StorePendingBlock(root [32]byte, signedBlock *types.SignedBlock) error {
	err := s.storePendingBlock(root, signedBlock)
	if err != nil {
		logger.Error(logger.Store, "%v", err)
	}
	return err
}

func (s *ConsensusStore) storePendingBlock(root [32]byte, signedBlock *types.SignedBlock) error {
	if signedBlock == nil || signedBlock.Block == nil {
		return fmt.Errorf("store pending block: signed block is nil")
	}

	block := signedBlock.Block
	header := &types.BlockHeader{
		Slot:          block.Slot,
		ProposerIndex: block.ProposerIndex,
		ParentRoot:    block.ParentRoot,
		StateRoot:     block.StateRoot,
	}
	if block.Body != nil {
		bodyRoot, err := block.Body.HashTreeRoot()
		if err != nil {
			return fmt.Errorf("store pending block: body root: %w", err)
		}
		header.BodyRoot = bodyRoot
	}
	if err := s.PutBlockHeader(root, header); err != nil {
		return fmt.Errorf("store pending block: %w", err)
	}
	if err := WriteBlockData(s, root, signedBlock); err != nil {
		return fmt.Errorf("store pending block: %w", err)
	}
	return nil
}

func (s *ConsensusStore) HeadSlot() uint64 {
	h := s.GetBlockHeader(s.Head())
	if h == nil {
		return 0
	}
	return h.Slot
}

func (s *ConsensusStore) MaxStoredBlockSlot() uint64 {
	rv, err := s.beginRead("max stored block slot")
	if err != nil {
		return 0
	}
	it, err := rv.PrefixIterator(storage.TableBlockHeaders, nil)
	if err != nil {
		return 0
	}
	defer it.Close()

	var max uint64
	for it.Next() {
		v := it.Value()
		if len(v) < 8 {
			continue
		}
		slot := binary.LittleEndian.Uint64(v[:8])
		if slot > max {
			max = slot
		}
	}
	return max
}

func (s *ConsensusStore) InsertLiveChainEntry(slot uint64, root, parentRoot [32]byte) {
	if err := s.PutLiveChainEntry(slot, root, parentRoot); err != nil {
		logger.Error(logger.Store, "%v", err)
	}
}

func (s *ConsensusStore) PutLiveChainEntry(slot uint64, root, parentRoot [32]byte) error {
	key := storage.EncodeLiveChainKey(slot, root)
	return s.putOne(storage.TableLiveChain, key, parentRoot[:], "insert live chain entry")
}

func (s *ConsensusStore) GetCanonicalBlocksInRange(startSlot, count uint64) []*types.SignedBlock {
	if count == 0 || startSlot > ^uint64(0)-count {
		return nil
	}

	endSlot := startSlot + count
	var blocks []*types.SignedBlock
	root := s.Head()
	for {
		header := s.GetBlockHeader(root)
		if header == nil || header.Slot < startSlot {
			break
		}
		if header.Slot < endSlot {
			if block := s.GetSignedBlock(root); block != nil {
				blocks = append(blocks, block)
			}
		}
		if header.Slot == 0 {
			break
		}
		root = header.ParentRoot
	}

	for i, j := 0, len(blocks)-1; i < j; i, j = i+1, j-1 {
		blocks[i], blocks[j] = blocks[j], blocks[i]
	}
	return blocks
}
