# Running gean under Shadow

[Shadow](https://github.com/shadow/shadow) is a discrete-event network simulator that
runs the real `gean` binary while emulating time and the network. Each host gets its
own IP on a configurable graph, syscalls (clock, UDP, timers) are intercepted, and
execution is single-stepped deterministically. This gives reproducible multi-node
devnets and deterministic network-condition tests.

**Linux + x86_64 only.** Shadow does not run on macOS and does not build on arm64. Use an
x86_64 Linux host, or the Dockerized runner below (which pins `linux/amd64`).

## Dockerized gates (run from any host)

`make shadow-docker-run` builds a `linux/amd64` image containing the gean binary plus Shadow
and runs the verification gates (generate testnet → build topology → simulate → assert the
chain finalized). Tunables: `SHADOW_DOCKER_NODES`, `SHADOW_DOCKER_STOP_TIME`,
`SHADOW_DETERMINISM=1` (rerun and compare per-slot block roots). On an arm64 host the image
runs under emulation and is slow; run it in native amd64 CI for real timing.

## Quick start (native x86_64 Linux)

```bash
make shadow-setup     # build, generate testnet keys, write shadow/shadow.yaml
make shadow-run       # run the simulation (needs `shadow` on PATH)
```

Per-host stdout lands in `shadow/shadow.data/hosts/<node>/`. A run is healthy when every
host reaches `expected_final_state: running` and head/justified/finalized advance with
`finalized ≈ head − 3`.

Tunables (env or make vars): `NUM_NODES`, `SHADOW_STOP_TIME`, `SHADOW_GENESIS_DELAY`, and
the prover-cost knobs below. Example:

```bash
make shadow-setup NUM_NODES=4 SHADOW_STOP_TIME=120s
SHADOW_AGGREGATE_MS=300 SHADOW_VERIFY_MS=80 ./shadow/gen_shadow_yaml.sh   # re-emit with cost modeling
make shadow-run
```

## Why these adaptations

- **No source patches.** gean has no jemalloc (so the allocator deadlock that affects some
  clients does not apply) and is already a dynamically-linked CGo binary — Shadow requires
  dynamic linking, so keep CGo on; never build `CGO_ENABLED=0`/static.
- **GSO off.** quic-go uses UDP generic segmentation offload, which Shadow's UDP emulation
  does not support. The harness sets `QUIC_GO_DISABLE_GSO=true` in each host's environment;
  no code change needed.
- **Per-host IPs.** `keygen` writes loopback bootnode addresses; `gen_shadow_yaml.sh`
  rewrites them to per-host simulated IPs in a staging config dir (`shadow/net/`), leaving
  the source `testnet/` intact for the loopback `make run` flow.
- **Genesis window.** Genesis time is wall-clock relative (`keygen --genesis-delay`, default
  120s here) so a slow build/launch does not miss genesis. For byte-identical reruns, pin an
  absolute time with `keygen --genesis-time <unix>`.

## Prover cost modeling

Shadow advances virtual time only on blocking/sleeping/IO, not during computation, so the
post-quantum prover would register as free in simulated time. Two off-tick costs can be
modeled as virtual-time delay (0 = disabled, the default):

- `--shadow-aggregate-cost-ms` — signature aggregation (aggregation worker)
- `--shadow-verify-cost-ms` — single attestation verification

Block proposal and aggregated-attestation verification run on the engine tick loop, so they
are intentionally **not** modeled — sleeping there would desync slot derivation. Cost
modeling makes virtual-time slot pacing realistic; it does not reduce real wall-clock (the
real prover still runs, and CPU is the floor on run time — keep sims small/short).

## Runtime file dependency

The proving stack reads its leanVM Python sources from the path baked in at build time. On a
local build the source checkout already satisfies this; if you stage the binary onto another
host, make those sources available at the same path (see the repo `Dockerfile`).
