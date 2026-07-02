package aggregation

import (
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
	"github.com/geanlabs/gean/xmss"
)

// selectChildProofs folds coverage-adding child proofs into the aggregation
// inputs. remaining caps how many more units this pass may take; child proofs
// are preferred over raw signatures because each carries many validators, so
// they buy the most coverage per unit of prover budget.
func selectChildProofs(
	entry *store.PayloadEntry,
	state *types.State,
	children *[]xmss.ChildProof,
	covered map[uint64]bool,
	cache *xmss.PubKeyCache,
	remaining *int,
) {
	if entry == nil || state == nil || cache == nil || len(entry.Proofs) == 0 {
		return
	}

	for _, proof := range entry.Proofs {
		if *remaining <= 0 {
			return
		}
		bitsLen := types.BitlistLen(proof.Participants)
		if countNewCoverage(proof.Participants, covered) == 0 {
			continue
		}

		pubkeys := make([]xmss.CPubKey, 0, int(bitsLen))
		participants := make([]uint64, 0, int(bitsLen))
		valid := true

		for vid := range bitsLen {
			if !types.BitlistGet(proof.Participants, vid) {
				continue
			}
			if vid >= uint64(len(state.Validators)) {
				valid = false
				break
			}
			pk, err := cache.Get(state.Validators[vid].AttestationPubkey)
			if err != nil {
				valid = false
				break
			}
			pubkeys = append(pubkeys, pk)
			participants = append(participants, vid)
		}
		if !valid {
			continue
		}

		for _, vid := range participants {
			covered[vid] = true
		}
		*children = append(*children, xmss.ChildProof{
			Pubkeys: pubkeys,
			Proof:   proof.Proof,
		})
		*remaining--
	}
}

func countNewCoverage(bits []byte, covered map[uint64]bool) int {
	count := 0
	for vid := range types.BitlistLen(bits) {
		if types.BitlistGet(bits, vid) && !covered[vid] {
			count++
		}
	}
	return count
}
