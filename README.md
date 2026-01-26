# Gean

A Go implementation of the Lean Ethereum consensus protocol that is simple enough to last.

## Getting started

```sh
# Build
make build

# Run as validator 0 (genesis auto-generated 10s in future)
./bin/gean --validators 8 --validator-index 0

# Run with explicit genesis time
./bin/gean --genesis-time 1769271115 --validators 8 --validator-index 0
```

## Philosophy

> *"Even if a protocol is super decentralized with hundreds of thousands of nodes... if the protocol is an unwieldy mess of hundreds of thousands of lines of code, ultimately that protocol fails."* — Vitalik Buterin

Our goal is to build a consensus client that is simple and readable yet elegant and resilient; code that anyone can read, understand, and maintain for decades to come. A codebase developers actually enjoy contributing to. It's why we chose Go.

## Acknowledgements

- [leanSpec](https://github.com/leanEthereum/leanSpec) — Python reference specification
- [ethlambda](https://github.com/lambdaclass/ethlambda) — Rust implementation by LambdaClass

## Current status

Target: [leanSpec devnet 0](https://github.com/leanEthereum/leanSpec/tree/4b750f2748a3718fe3e1e9cdb3c65e3a7ddabff5)

### Implemented

- **Types** — SSZ containers via fastssz (Block, State, Vote, Checkpoint, Config)
- **Consensus** — 3SF-mini justification (2/3 supermajority), round-robin proposer
- **State transition** — slot processing, block header, attestations with vote tracking
- **Fork choice** — LMD-GHOST head selection, Store container
- **Networking** — libp2p host (QUIC), gossipsub (block and attestation topics)
- **Node** — slot ticker, block and attestation production

### Next

- [pq-devnet-1](https://github.com/leanEthereum/pm/blob/main/breakout-rooms/leanConsensus/pq-interop/pq-devnet-1.md) — PQ signatures, lean-quickstart integration

## License

MIT
