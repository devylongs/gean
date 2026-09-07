package aggregation

import (
	"bytes"

	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
	"github.com/geanlabs/gean/xmss"
)

// maxChildProofsPerGroup bounds how many child proofs one group may fold in.
// A child is a recursive input: measured on a 16-core host, a group carrying one
// costs 1.68-2.96s against 0.31-0.91s for raw signatures alone, with no overlap
// between the ranges. The cap bounds the worst case a single group can spend
// once raw-first selection has already removed most recursion. ethlambda and
// lantern both settled on the same value.
const maxChildProofsPerGroup = 2

// selectChildProofs folds coverage-adding child proofs into the aggregation
// inputs. covered may be seeded with raw signers. The returned participant IDs
// identify raw inputs that must be removed to avoid raw/child double inclusion.
//
// The cap counts children already in the slice, so it holds across the separate
// new-payload and known-payload passes that share one group's inputs.
//
// childCost prices one child in raw-signature units, and rawCount is how many
// raw signatures the group already holds. Two children are exempt from that
// price, for two different reasons:
//
//   - Until rawCount+children reaches two the group is not yet spec-viable, so
//     charging for those children could leave it unable to produce anything.
//   - The first child is admitted regardless. Validators reachable only through
//     a child proof have no raw signature to fall back on, and pricing that
//     first child out under a tight budget defers exactly the votes finality is
//     waiting on, every session, for as long as the pressure lasts.
//
// Every child after that must fit the remaining budget.
//
// Selection is greedy on coverage: each round takes the proof adding the most
// still-uncovered validators, matching leanSpec's select_proofs_for_coverage.
// Walking the pool in stored order instead can take several low-coverage proofs
// where one wide proof would do, and every extra child is a recursive input the
// prover pays for.
func selectChildProofs(
	entry *store.PayloadEntry,
	state *types.State,
	children *[]xmss.ChildProof,
	covered map[uint64]bool,
	cache *xmss.PubKeyCache,
	remaining *int,
	childCost int,
	rawCount int,
) (selectedIDs []uint64) {
	if entry == nil || state == nil || cache == nil || len(entry.Proofs) == 0 {
		return
	}

	if childCost < 1 {
		childCost = 1
	}

	used := make([]bool, len(entry.Proofs))
	for {
		if len(*children) >= maxChildProofsPerGroup {
			return
		}
		exempt := len(*children) == 0 || rawCount+len(*children) < 2
		if !exempt && *remaining < childCost {
			return
		}

		best := -1
		bestNew := 0
		for i, candidate := range entry.Proofs {
			if used[i] {
				continue
			}
			n := countNewCoverage(candidate.Participants, covered)
			if n == 0 {
				continue
			}
			if best == -1 || n > bestNew {
				best, bestNew = i, n
				continue
			}
			// Ties resolve on the participant bitfield so the choice is stable
			// run to run. Without it the winner would depend on pool order for
			// proofs that cover equally much.
			if n == bestNew && bytes.Compare(candidate.Participants, entry.Proofs[best].Participants) > 0 {
				best = i
			}
		}
		// Nothing left adds coverage: at full coverage, or every remaining proof
		// is a subset of what is already included.
		if best == -1 {
			return
		}
		used[best] = true
		proof := entry.Proofs[best]

		bitsLen := types.BitlistLen(proof.Participants)
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
			selectedIDs = append(selectedIDs, vid)
		}
		*children = append(*children, xmss.ChildProof{
			Pubkeys: pubkeys,
			Proof:   proof.Proof,
		})
		*remaining -= childCost
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
