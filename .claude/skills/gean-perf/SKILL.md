---
name: gean-perf
description: Stress-test gean's performance and read its metrics against the slot/interval budget. Use this when the user wants to stress-test gean on Shadow (single-client, multi-client, or multi-subnet), sweep the XMSS prover-cost rates, capture metrics from a Shadow or devnet run, understand what a gean metric means or what its expected budget is, diagnose why finalization/justification is stalling or the tick is drifting, or decide which gean performance area needs a fix. Covers the leanEthereum recursive-aggregation-vs-slot-interval debate.
---

# gean performance & metrics

Stress-test gean and read its metrics against the 800ms-interval / 4s-slot budget and the XMSS
prover cost. The teaching detail lives in the **Shadow Simulation Mastery Guide** — the sibling
`../shadow` repo (Markdown chapters + a React app); this skill orchestrates it.

- **Metrics** — `../shadow/guide/11-reading-the-metrics.md` ("Metrics mastery — beginners to pro"):
  every key metric with meaning → budget → how to read it, plus the failure chain and the cost cliff.
- **Runbook + matrix** — `../shadow/guide/07`–`10` (setup → single → multi-subnet → interop) and
  `../shadow/guide/13-reference-cheatsheet.md`.
- **Runner** — the **`lean-shadow-fuzzer`** (`../lean-shadow-fuzzer`) drives gean under Shadow the
  same way it drives every client (its `config.toml` + `scripts/client-cmds/gean-cmd.sh`). gean ships
  only the rate model (`internal/shadow`) + the `--shadow-xmss-*` flags, published as
  `ghcr.io/geanlabs/gean:shadow` for the fuzzer to pull — no harness lives in the gean repo.

## When to use

- "Stress-test gean on Shadow" / "run a multi-subnet test" / "sweep the prover rates"
- "What does `lean_<metric>` mean?" / "what's the budget for it?" / "is this value healthy?"
- "Why is finalization stalling / the tick drifting under load?"
- "Which part of gean needs a performance fix?" (evidence-gated — see below)

## Workflow

1. **Frame the question against the budget.** Load the metrics chapter
   (`../shadow/guide/11-reading-the-metrics.md`). The timing model (slot 4s = 5×800ms intervals;
   aggregation session ~1600ms; proposal ~2400ms; per-attestation verify ~500ms) is where every
   budget comes from. Read a run in this order: liveness triple → tick duration → aggregation chain
   → drop/queue counters → RSS.

2. **Run the matrix.** Drive Shadow through the **`lean-shadow-fuzzer`** (Mastery Guide chapters
   `07`–`10`): describe a network in its `config.toml` (nodes, committee count, per-client images,
   prover-cost rates) and it generates genesis/topology/`shadow.yaml`, runs Shadow, and renders the
   Observatory. Shadow finds the *breaking rate* deterministically; `run-devnet` (devnet-runner skill)
   shows *absolute magnitude* + live Grafana on real proving.

3. **Read the result vs the limit.** For each metric, compare against its budget in the metrics
   chapter. The failure chain to watch: `proving_duration{aggregation}` rises →
   `aggregation_worker_total_time` crosses ~1.6s → aggregation `truncated` →
   `aggregation_dispatch_dropped_total` climbs + `proving_queue_depth{aggregation}` pins at 1 →
   `tick_interval_duration` drifts past 0.82s → `head − finalized` widens.

4. **Gate any fix on a signal.** Never propose a performance change without the metric that
   justifies it first crossing threshold in the matrix. Candidate fixes and their gating signals
   are catalogued in the metrics chapter (Part D) and in the effort's plan (Deliverable C):
   aggregation-dispatch capacity, bounded verify concurrency, PubKeyCache/lock contention,
   raw-signature priority under budget pressure (the zeam/ethlambda proposal), session-budget
   tuning, longer slot. Measure → report → then change.

## Conventions

- Spec is the source of truth: interpret load behavior (finalization, attestation source per
  leanSpec #1166, subnet routing `validator_index % committee_count`) against the pinned spec, and
  run `/spec-compliant` when a change could affect consensus.
- Harness edits are opt-in and must not affect normal builds. gean code fixes pass the
  verification gate (`make build`, relevant `go test` + `make test-spec` for spec-touching,
  `make lint`) and are committed separately, authored as the repo convention, no AI attribution.
