package blockprocessor

import (
	"sync"
	"testing"

	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
	"github.com/geanlabs/gean/xmss"
)

var (
	benchFixtureOnce sync.Once
	benchFixtureErr  error
	benchKeyPair     *xmss.ValidatorKeyPair
	benchState       *types.State
	benchStore       *store.ConsensusStore
	benchTargetRoot  [32]byte
	benchPubkey      xmss.CPubKey
)

func loadBenchFixture(tb testing.TB) {
	tb.Helper()
	benchFixtureOnce.Do(buildBenchFixture)
	if benchFixtureErr != nil {
		tb.Fatalf("benchmark fixture: %v", benchFixtureErr)
	}
}

func buildBenchFixture() {
	xmss.EnsureProverReady()
	xmss.EnsureVerifierReady()

	kp, err := xmss.GenerateKeyPair("bench-validator", 0, 1<<18)
	if err != nil {
		benchFixtureErr = err
		return
	}
	benchKeyPair = kp

	pkBytes, err := kp.PublicKeyBytes()
	if err != nil {
		benchFixtureErr = err
		return
	}

	benchState = &types.State{
		Config:            &types.ChainConfig{GenesisTime: 1000},
		Slot:              0,
		LatestBlockHeader: &types.BlockHeader{},
		LatestJustified:   &types.Checkpoint{Slot: 0},
		LatestFinalized:   &types.Checkpoint{Slot: 0},
		Validators: []*types.Validator{{
			AttestationPubkey: pkBytes,
			ProposalPubkey:    pkBytes,
			Index:             0,
		}},
		JustifiedSlots:           types.NewBitlistSSZ(0),
		JustificationsValidators: types.NewBitlistSSZ(0),
	}

	stateRoot, err := benchState.HashTreeRoot()
	if err != nil {
		benchFixtureErr = err
		return
	}
	benchTargetRoot = stateRoot

	benchPubkey, err = xmss.ParsePublicKey(pkBytes)
	if err != nil {
		benchFixtureErr = err
		return
	}

	benchStore = store.NewConsensusStore(storage.NewInMemoryBackend())
	benchStore.InsertState(benchTargetRoot, benchState)
}

func buildBenchSignedBlock(n int) (*types.SignedBlock, error) {
	atts := make([]*types.AggregatedAttestation, n)
	inputs := make([]xmss.Type1Input, 0, n+1)
	for i := range n {
		data := &types.AttestationData{
			Slot:   uint64(i + 1),
			Head:   &types.Checkpoint{Root: benchTargetRoot, Slot: uint64(i + 1)},
			Target: &types.Checkpoint{Root: benchTargetRoot, Slot: uint64(i + 1)},
			Source: &types.Checkpoint{Root: benchTargetRoot},
		}
		root, err := data.HashTreeRoot()
		if err != nil {
			return nil, err
		}
		raw, err := benchKeyPair.Sign(uint32(data.Slot), root)
		if err != nil {
			return nil, err
		}
		signature, err := xmss.ParseSignature(raw[:])
		if err != nil {
			return nil, err
		}
		proof, err := xmss.AggregateSignatures(
			[]xmss.CPubKey{benchPubkey},
			[]xmss.CSig{signature},
			root,
			uint32(data.Slot),
		)
		xmss.FreeSignature(signature)
		if err != nil {
			return nil, err
		}
		bits := types.BitlistFromIndices([]uint64{0})
		atts[i] = &types.AggregatedAttestation{AggregationBits: bits, Data: data}
		inputs = append(inputs, xmss.Type1Input{Pubkeys: []xmss.CPubKey{benchPubkey}, Proof: proof})
	}

	block := &types.Block{
		Slot:          uint64(2 + n),
		ProposerIndex: 0,
		Body:          &types.BlockBody{Attestations: atts},
	}
	blockRoot, err := block.HashTreeRoot()
	if err != nil {
		return nil, err
	}

	blockSlot, err := slot32(block.Slot)
	if err != nil {
		return nil, err
	}
	proposerSig, err := benchKeyPair.Sign(blockSlot, blockRoot)
	if err != nil {
		return nil, err
	}
	signature, err := xmss.ParseSignature(proposerSig[:])
	if err != nil {
		return nil, err
	}
	proposerProof, err := xmss.AggregateSignatures(
		[]xmss.CPubKey{benchPubkey},
		[]xmss.CSig{signature},
		blockRoot,
		blockSlot,
	)
	xmss.FreeSignature(signature)
	if err != nil {
		return nil, err
	}
	inputs = append(inputs, xmss.Type1Input{Pubkeys: []xmss.CPubKey{benchPubkey}, Proof: proposerProof})
	proof, err := xmss.MergeType1Proofs(inputs)
	if err != nil {
		return nil, err
	}

	return &types.SignedBlock{
		Block: block,
		Proof: &types.MultiMessageAggregate{Proof: proof},
	}, nil
}

func benchN(b *testing.B, n int) {
	if testing.Short() {
		b.Skip("skipping XMSS benchmark in -short mode")
	}
	loadBenchFixture(b)

	signedBlock, err := buildBenchSignedBlock(n)
	if err != nil {
		b.Fatalf("build signed block: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := verifyBlockSignatures(benchStore, signedBlock, benchState); err != nil {
			b.Fatalf("verify failed: %v", err)
		}
	}
}

func BenchmarkVerifyBlockSignatures_1(b *testing.B) { benchN(b, 1) }
func BenchmarkVerifyBlockSignatures_8(b *testing.B) { benchN(b, 8) }
