# Aggregator stability at scale — engineering plan

Tracks issue #398. Goal: make gean sustain the aggregator seat indefinitely at
scale, degrade gracefully if it ever falls behind, and recover on its own —
matching then bettering ethlambda's stability.

Evidence: a ~31h run (gean aggregator + ethlambda + lantern) had gean's head
freeze at slot 14,349. Aggregation proof time grew with state (first budget hit
slot 10,101; 1.6–3.0s per session by 14K; 461 truncations), a single proof
exceeded the whole 1600ms budget, block import starved on the shared prover, and
gean fell off the chain irrecoverably — stalling network finality at 14,311.
ethlambda stayed at ~30–70ms import with state transition 62µs→2.2ms across 14K
slots (cached hashing), and never bore the every-slot aggregator load.

## Root cause (code-level) — corrected after log re-analysis

The dominant driver is a **finality-lag / aggregation-backlog positive feedback
loop**, not state-HTR growth. Measured on the stalled run:

- per-proof XMSS cost grew only ~2x over the run (~160ms → ~250–340ms).
- what blows the 1600ms budget is **group count per session** — the number of
  distinct attestation data roots the session must prove — which rose from 1–2
  to 6+ (session totals 350ms → 2055ms). It is noisy: some sessions still fit.
- finality lag (head − finalized) held ~10 until ~slot 11–12K, then jumped to
  ~98, and the pending-signature backlog grew in step (2–3 → 19+ roots).

The loop: a session first exceeds budget (modest per-proof creep × a few groups)
→ `aggregateFromSnapshot` truncates → incomplete vote coverage → finality lags →
`AttestationSignatureMap` (pruned only below finalized) accumulates roots across
the now-wider unfinalized window → next session has more groups → exceeds budget
harder → runaway. Eventually the session runs long enough (2–3s) to starve block
import on the shared prover, and gean falls off the chain. Stale-view
attestations from a lagging peer (lantern, behind from ~9,447) plausibly inflate
the distinct-root count and feed the loop.

State-HTR growth (`State.HashTreeRoot`, fastssz, re-merkleizes the append-only
`HistoricalBlockHashes` / bitlists every call) is real but **secondary here** —
it only reduces headroom (the ~2x per-proof creep, block import 34→56ms). It
becomes dominant only at much larger scale. So caching helps but does **not**
break the loop.

## Workstreams (corrected priority)

### WS-2 — Break the feedback loop: hard session bound + finality-first ordering (PRIMARY)
File: `internal/aggregation/aggregate.go`, `worker.go`, `orderedGroups`.

Two parts:
1. **Hard-bound the session** so it can never run long enough to starve import:
   `maxUnitsWithin` floors at 2 and models only per-unit cost, so the worker
   always starts even when it cannot finish. Skip a group (mark truncated,
   release the prover) when the estimated session-so-far + next proof would
   exceed the deadline, instead of overrunning. Converts the cliff into graceful
   degradation — gean stays synced.
2. **Order groups finality-first.** Verified against leanSpec eca701e
   (process_attestations): a source finalizes only when the checkpoint
   immediately after it is justified — no justifiable slot between source and
   target — so finalization advances one checkpoint at a time from the frontier.
   Newest-first spends the budget on the highest target (advancing head /
   justification) and leaves the frontier-adjacent target unaggregated, so
   finalization stalls while the head moves — the observed stall shape. Ordering
   is a local heuristic, not spec-mandated, so reordering is spec-safe.

Status: both parts implemented (hard bound + frontier-first ordering = ascending
target slot). Group order is the only change; every aggregate produced is
spec-valid. The head-vs-finality tension and the sparse justifiable-slot
structure make the exact benefit structure-dependent, so a devnet run (truncating
aggregator finalizes under frontier-first, stalls under newest-first) is the
remaining validation before merge.

No consensus-root change; aggregates produced remain spec-valid.

### WS-5 — Bound the backlog independent of finality (PRIMARY)
File: `internal/store/gossip.go` (`AttestationSignatureMap`), pruning.

Signatures are pruned only below finalized, so a stalled finality lets the
backlog grow unbounded — the fuel for the loop. Cap the pending window (e.g.
drop/ignore roots older than a bounded lookback from head, or above a size cap),
so group count per session is bounded regardless of finality state.

### WS-4 — Guardrails & early warning (safe; pair with the above)
File: `internal/metrics/`, aggregation worker, `internal/node/tick.go` status.

Emit/observe: session duration vs budget, group count per session, budget-hit
rate, finality lag (head − finalized), backlog size, behind-slots, prover
wait/unavailable rate. Warn when finality lag or session duration trends up —
this run gave ~3K slots (11K→14K) of warning that nothing surfaced.

### WS-1 — Cached / incremental state HTR — DROPPED (measured, not the bottleneck)
Benchmarked `State.HashTreeRoot` (state_scale_bench_test.go): ~2ms at 21K slots
(v64 and v512), ~8.5ms at 100K — on par with ethlambda's measured state
transition (62µs→2.2ms). It is <1% of the XMSS cost (aggregation proof 150–340ms,
import verify ~30–50ms). Caching would save ~2ms for a consensus-critical,
chain-split-risk change. Not worth it; the throughput bottleneck is XMSS, not
Merkle hashing. Revisit only past ~100K slots.

### WS-7 — Aggregation proving throughput (the real throughput lever)
The finality lag came from XMSS proving: group count × per-proof (~200ms)
exceeding the session budget. Levers, in risk order:

1. Skip already-justified targets (DONE). process_attestations ignores a vote
   once its target is justified, so proving it wastes budget. Filtered in
   `orderedGroups` via `IsSlotJustified`. Safe, frees budget for open targets.
2. Parallel group proving — MEASURED, DROPPED. The FFI concurrency stress test
   (xmss/concurrency_stress_test.go) proved it reentrant with independent handles
   (4 concurrent proofs, all valid, no crash) but showed **no throughput gain**:
   speedup 1.09x (serial 216ms/proof vs parallel 198ms/proof), because the STARK
   prover already saturates all cores per proof. It also added ~95MB RSS per
   concurrent proof on a ~1.3GB baseline. So concurrency only oversubscribes cores
   and multiplies memory — pure downside. The per-proof time is core-optimal
   already; throughput improves only by needing fewer proofs (skip-justified,
   frontier-first) or a faster prover upstream, not by parallelizing.

### WS-3 — Aggregator recovery / catch-up (safety net)
Files: `internal/syncer/`, `internal/node/`.

Ensure a lagging aggregator can rejoin: batch/parallel XMSS verification during
backfill so catch-up rate > 1 block / 4s; checkpoint resync when lag exceeds a
threshold; confirm the duty gate stands the node down while lagging (correct) but
that recovery is actually reachable (the gap this run exposed).

## Acceptance criteria

- Multi-machine long run, gean as aggregator with real attestation load, sustains
  past ~30K slots: zero budget-hit truncations, aggregation proof < 1s.
- State HTR / block-build cost flat (not slot-dependent) on the scale benchmark.
- An aggregator forced N slots behind recovers to head on its own.
- If the ceiling is ever hit, finality lags but the node stays synced (graceful),
  never the cliff.

## Notes

- Early aggregation (#396/#397) delays but does not remove this ceiling, and may
  worsen the failure mode (front-loads prover contention vs import). Settle with a
  controlled A/B (early-agg ON vs OFF, identical hardware + client mix + validators).
- lantern was a straggler this run (fell behind ~9,447); out of scope here.
