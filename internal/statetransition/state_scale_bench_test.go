package statetransition

import (
	"encoding/binary"
	"testing"

	"github.com/geanlabs/gean/internal/types"
)

// syntheticState builds a state with numValidators validators and its per-slot
// history arrays (historical_block_hashes, justified_slots) sized to historyLen,
// standing in for a chain that has advanced historyLen slots. This isolates the
// state hash-tree-root cost — the dominant, slot-growing term in block import
// (VerifyStateRoot re-merkleizes the whole state per block) — without needing a
// live devnet or any crypto/FFI.
func syntheticState(numValidators, historyLen int) *types.State {
	validators := make([]*types.Validator, numValidators)
	for i := range validators {
		var pk [types.PubkeySize]byte
		binary.LittleEndian.PutUint64(pk[:], uint64(i+1))
		validators[i] = &types.Validator{AttestationPubkey: pk, ProposalPubkey: pk, Index: uint64(i)}
	}

	hbh := make([][]byte, historyLen)
	for i := range hbh {
		entry := make([]byte, types.RootSize)
		binary.LittleEndian.PutUint64(entry, uint64(i+1))
		hbh[i] = entry
	}

	return &types.State{
		Config:                   &types.ChainConfig{GenesisTime: 1000},
		Slot:                     uint64(historyLen),
		LatestBlockHeader:        &types.BlockHeader{Slot: uint64(historyLen)},
		LatestJustified:          &types.Checkpoint{},
		LatestFinalized:          &types.Checkpoint{},
		HistoricalBlockHashes:    hbh,
		JustifiedSlots:           types.NewBitlistSSZ(uint64(historyLen)),
		Validators:               validators,
		JustificationsRoots:      [][]byte{},
		JustificationsValidators: types.NewBitlistSSZ(0),
	}
}

// benchStateHashTreeRoot measures the per-block state-root cost the import path
// pays in VerifyStateRoot. Grid the two growth axes — validator count and slot
// history — to see which dominates at scale.
func benchStateHashTreeRoot(b *testing.B, numValidators, historyLen int) {
	state := syntheticState(numValidators, historyLen)
	if _, err := state.HashTreeRoot(); err != nil {
		b.Fatalf("initial hash_tree_root: %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := state.HashTreeRoot(); err != nil {
			b.Fatalf("hash_tree_root: %v", err)
		}
	}
}

func BenchmarkStateHashTreeRoot_v64_s1000(b *testing.B)    { benchStateHashTreeRoot(b, 64, 1000) }
func BenchmarkStateHashTreeRoot_v512_s1000(b *testing.B)   { benchStateHashTreeRoot(b, 512, 1000) }
func BenchmarkStateHashTreeRoot_v64_s21000(b *testing.B)   { benchStateHashTreeRoot(b, 64, 21000) }
func BenchmarkStateHashTreeRoot_v512_s21000(b *testing.B)  { benchStateHashTreeRoot(b, 512, 21000) }
func BenchmarkStateHashTreeRoot_v512_s100000(b *testing.B) { benchStateHashTreeRoot(b, 512, 100000) }

// benchVerifyStateRoot exercises the exact call block import makes, so the
// benchmark tracks the real hot path rather than hash_tree_root in isolation.
func benchVerifyStateRoot(b *testing.B, numValidators, historyLen int) {
	state := syntheticState(numValidators, historyLen)
	root, err := state.HashTreeRoot()
	if err != nil {
		b.Fatalf("hash_tree_root: %v", err)
	}
	block := &types.Block{Slot: state.Slot + 1, StateRoot: root}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := VerifyStateRoot(state, block); err != nil {
			b.Fatalf("verify state root: %v", err)
		}
	}
}

func BenchmarkVerifyStateRoot_v512_s21000(b *testing.B) { benchVerifyStateRoot(b, 512, 21000) }
