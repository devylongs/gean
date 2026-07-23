# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`gean` is a Go implementation of an Ethereum **lean consensus** client (module `github.com/geanlabs/gean`). It targets the **devnet-5** milestone and tracks the `leanEthereum/leanSpec` Python spec. Consensus uses **XMSS post-quantum hash-based signatures**, implemented in Rust and reached over CGo FFI.

## Build & test

The build has a hard ordering: **Rust FFI must be built before any Go code that touches `xmss`**. `make build` does this for you (`ffi` is a prerequisite). The toolchain is pinned — Rust 1.92.0, Go 1.25.

```
make build        # builds Rust FFI glue, then bin/gean and bin/keygen
make ffi          # build XMSS Rust glue libs only (output: xmss/rust/target/multisig-release/)
make test         # unit tests — EXCLUDES xmss (FFI), spectests, and cmd/
make test-ffi     # xmss FFI tests (runs `make ffi` first)
make test-spec    # spec fixture tests only — needs leanSpec checkout, uses -tags=spectests
make test-all     # everything incl. fixtures + FFI (slow)
make lint         # go vet + cargo fmt --check + cargo clippy -D warnings
make fmt          # gofmt + cargo fmt
make sszgen       # regenerate internal/types/*_encoding.go from struct tags (after changing SSZ types)
```

Run a single Go test: `go test ./internal/node -run TestName -v -count=1`. For spec tests you must pass the build tag: `go test ./internal/spectests/ -run TestName -tags=spectests -count=1`.

Hive test-driver routes are excluded from normal binaries. Build with `go build -tags hive_testdriver ./cmd/gean` and set `HIVE_LEAN_TEST_DRIVER=1` to expose them.

`make test` deliberately omits `xmss`, `spectests`, and `cmd/` — running a plain `go test ./...` will try to link the FFI and fail unless `make ffi` has run. When in doubt, use the make targets.

### Spec fixtures
Spec tests depend on a separately-cloned, **pinned** spec repo (not vendored):
```
make test-spec           # pins leanSpec, downloads prod keys, generates fixtures, and runs them
```
When catching up to spec changes, the pinned `LEAN_SPEC_COMMIT_HASH` in the Makefile is the source of truth for "what spec version are we on."

## Running a node

A node needs a config dir + XMSS keys. `make run-setup` generates a local 5-validator / 3-node testnet via `keygen`; `make run`, `make run-node1`, `make run-node2` start nodes on ports 9000/9001/9002. For multi-client Docker devnets use `make run-devnet` (builds the image and drives `lean-quickstart`). The `devnet-runner` and `devnet-log-review` skills automate this.

Key CLI flags (`cmd/gean/main.go`): `--custom-network-config-dir`, `--node-key`, `--node-id` (all required), `--is-aggregator`, `--checkpoint-sync-url`, `--data-dir`, `--gossipsub-port` (QUIC/UDP), `--api-port`, `--metrics-port`.

## Architecture

### Slot/interval timing model
A slot is **4s**, divided into **5 intervals of 800ms** (`internal/types/constants.go`). The whole node is driven by an 800ms ticker through `Engine.onTick`, which derives `currentSlot`/`currentInterval` from wall-clock time. Interval responsibilities:
- **Interval 0** — propose a block (if we're the proposer) and update head
- **Interval 2** — build aggregated attestations (aggregators only)
- **Interval 0 / 4** — recompute fork-choice head after promoting attestations

Behavior is matched against the spec's `get_proposal_head` / `accept_new_attestations` ordering — preserve the order of head-update vs. proposal in `onTick`.

### Engine (`internal/node/`) — the coordination core
`Engine` (`internal/node/engine.go`) owns runtime wiring and is single-threaded over a `select` loop in `Run`: it multiplexes the ticker, `BlockCh`, `AggregationCh`, and `FailedRootCh`. P2P runs on its own goroutine and feeds these channels. Expensive proof work runs **off** the tick loop:
- **Aggregation worker** (`aggregation.RunWorker`, `internal/aggregation/worker.go`) — XMSS aggregation is slow FFI work, dispatched via a capacity-1 channel; backlog is dropped on purpose (best-effort per slot).
- **Proposal worker** — builds the Type-2 block proof without blocking ticks.
- **Recovery worker** — splits block Type-2 proofs back into pending Type-1 proofs.
- **Attestation verification** — each gossip attestation gets its own goroutine (~500ms XMSS verify each); `AttestationSignatureMap` is mutex-protected.

`Engine` holds sibling components — `store.ConsensusStore`, `ForkChoice`, `p2p.Host`, `xmss.KeyManager`, `role.Controller`, `dutygate.Gate`, and the `pending.BlockBuffer`/`pending.AttestationBuffer` out-of-order buffers — and wires metrics/p2p hooks at `Run` startup. **`ForkChoice` does NOT live inside `ConsensusStore`** — the Engine calls fork choice with store data as parameters.

Pending-block / pending-attestation buffers handle out-of-order gossip: blocks whose parents are unknown are tracked by `pending.BlockBuffer` and fetched via `runFetchBatcher` (coalesces fetch roots into BlocksByRange requests); attestations referencing unknown head roots are buffered in `pending.AttestationBuffer`.

### State transition (`internal/statetransition/`)
Pure spec logic, no I/O. `StateTransition` = `ProcessSlots` → `ProcessBlock` → verify `state_root`. This is the package that must mirror `leanSpec` most closely; spec-compliance work concentrates here and in `internal/types/`.

### Fork choice (`internal/forkchoice/`)
LMD-GHOST over a `ProtoArray` + `VoteStore`. `UpdateHead` uses all known votes; `UpdateSafeTarget` uses only freshly-received votes (`LatestNew`) with a ceil(2n/3) supermajority threshold. `Prune` removes finalized-below nodes **and remaps vote indices** — skipping the remap corrupts weights, so keep them together.

### Types & SSZ (`internal/types/`)
All consensus types plus SSZ serialization. The `*_encoding.go` files are **generated** by `sszgen` — never hand-edit them; change the struct + tags and run `make sszgen`. The exact object→file mapping is in the `sszgen` Makefile target.

### XMSS crypto (`xmss/` + `xmss/rust/`)
Post-quantum signatures via CGo. The Rust workspace has three crates (`hashsig-glue`, `multisig-glue`, `cgo-glue`); `cgo-glue` bundles them into one staticlib so std/alloc symbols dedup at link. `ffi.go` holds the cgo bindings (build tags/LDFLAGS point at `rust/target/multisig-release`). On amd64 the FFI is built with `-Ctarget-cpu=haswell` for AVX2 (~6× prover speedup). `KeyManager`, `PubKeyCache`, and `proof_pool` sit on top of the raw FFI.

### Storage (`internal/storage/` + `internal/store/`)
Pluggable `Backend` interface (`BeginRead`/`BeginWrite` with table-scoped batches). Two impls: `pebble.go` (production, on-disk) and `memory.go` (tests). Tables and key layout are in `tables.go`/`keys.go`.

`internal/store/` is the consensus store layered on top of raw storage: block/state access, metadata, payload buffers, attestation signatures, pruning, and tick-time promotion.

### Networking (`internal/p2p/`)
libp2p over QUIC. Gossipsub topics (`topics.go`, `gossip.go`), req/resp protocols incl. BlocksByRange (`reqresp.go`), ENR/discovery (`enr.go`, `bootnode.go`). Wire encoding in `encoding.go`.

### Other packages
`internal/api/` — HTTP API + admin handlers; `internal/blockprocessor/` — received block import; `internal/blockbuilder/` — proposal block building; `internal/attestation/` — attestation production/validation; `internal/aggregation/` — aggregated attestation building; `internal/role/` — local role state; `internal/dutygate/` — stale-view duty gating; `internal/metrics/` — Prometheus metrics; `internal/syncer/` — backfill/status polling; `internal/genesis/` — parses `config.yaml`; `internal/checkpoint/` — checkpoint sync; `internal/logger/` — structured logging; `internal/specfixtures/` — hive/spec-test fixture shapes; `cmd/keygen/` — testnet config + XMSS key generation.

## Conventions

- **Spec is the source of truth.** Consensus behavior must track `leanSpec` (pinned at `LEAN_SPEC_COMMIT_HASH` in the Makefile). When changing consensus logic, consult the spec rather than inferring intent from existing code. The `spec-compliant` skill checks gean against spec changes since a devnet version.
- `gitCommit` is injected at link time via `-ldflags` and surfaced through the `lean_node_info` Prometheus gauge — don't hardcode it.
- Buffer caps and consensus limits are package-level constants aligned with the pinned spec.
