# Block-import performance at scale

Branch: `perf/block-import-at-scale`

Tracks the 512-validator devnet degradation: block-import time, CPU, time-to-
finality and reorg count all climb as the chain lengthens (observed past ~21k
slots); the 64-node devnet is fine only because it has more CPU headroom.

## Root cause (measured)

Block import verifies the post-state via `VerifyStateRoot`, which calls
`state.HashTreeRoot()` — a full re-merkleization of the entire state on **every**
imported block. The state's per-slot history arrays (`historical_block_hashes`,
`justified_slots`) grow one entry per slot, so that cost grows with chain length.

`internal/statetransition/state_scale_bench_test.go` isolates it (pure Go, no FFI):

| validators | slots elapsed | state hash_tree_root |
|---|---|---|
| 64  | 1,000   | 0.13 ms |
| 512 | 1,000   | 0.37 ms |
| 64  | 21,000  | 1.84 ms |
| 512 | 21,000  | 2.07 ms |
| 512 | 100,000 | 8.60 ms |

- **Slot count dominates, not validator count** (~14× from 1k→21k slots vs ~2.8×
  from 64→512 validators); cost is ~linear in slots.
- `VerifyStateRoot` ≈ pure HTR (2.09 vs 2.07 ms), 0 allocs — it's CPU-bound
  merkleization, nothing else.

This compounds: the block-builder's planner runs a trial transition per selection
round, and every imported block pays it once — so at 21k+ slots the state root
alone is milliseconds of CPU per block, squeezing the 800 ms interval budget and
pushing time-to-finality and reorgs up.

## Not fixable by pruning (spec finding)

leanSpec `process_slots` appends to `historical_block_hashes` every slot and
extends `justified_slots` per slot, retained up to `HISTORICAL_ROOTS_LIMIT`
(2^18) with **no pruning below the limit**. The history is spec-mandated state,
so gean cannot shrink it — the fix must make hashing it cheaper, not drop it.

## Fix: cached (incremental) tree hashing — follow-up PR

The history arrays are **append-only**: once written, `historical_block_hashes[i]`
never changes, and `justified_slots` only extends and flips bits near the tip.
So the SSZ merkle tree of those lists has a large stable prefix; only the path
from the newly-appended leaves to the root changes per block. A cached tree hash
(memoize merkle subtrees, recompute only dirtied paths) turns the per-block cost
from O(slots) into O(Δ) — the standard beacon-client optimization.

Deliberately **not** landed in this PR: it is consensus-critical (the cached root
must equal the spec/sszgen root bit-for-bit), and `state.HashTreeRoot` is
sszgen-generated (`*_encoding.go` is never hand-edited), so it needs a custom
caching HTR path for the growing fields plus spec-fixture parity as the gate.
That is a scoped change of its own, and this PR intentionally ships the
measurement first so the optimization can be proven against a baseline.

## Shipped in this PR

- **Benchmark** (`state_scale_bench_test.go`) — reproducible, offline, no devnet;
  the baseline for evaluating the caching work.
- **Fat-LTO release profile** (`xmss/rust/Cargo.toml`) — whole-program inlining
  across the prover/verifier, trimming the signature-verify CPU that is the
  other half of import cost. opt-level stays 3 (the z/s miscompile guard holds).

## Verified along the way

- Out-of-range justified-slot votes: gean already rejects them
  (`statetransition/votes.go` `IsSlotJustified` → `JustifiedSlotOutOfRangeError`),
  matching leanSpec `JUSTIFIED_SLOT_OUT_OF_RANGE`. No action.

## Not in scope (tracked elsewhere)

- Two-tier "Goldfish" fork choice on a 4×1000 ms slot — a leanSpec-bound consensus
  change still in draft upstream (fixtures not regenerated). Monitor leanSpec; it
  will reshape `onTick`'s interval grid and `internal/forkchoice` when it lands.
