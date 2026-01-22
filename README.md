# Gean

A Go implementation of the Lean Ethereum consensus protocol that is simple enough to last.

## Getting started

```sh
make run
```

## Philosophy

> *"Even if a protocol is super decentralized with hundreds of thousands of nodes... if the protocol is an unwieldy mess of hundreds of thousands of lines of code, ultimately that protocol fails."* — Vitalik Buterin

Our goal is to build a consensus client that is simple and readable yet elegant and resilient; code that anyone can read, understand, and maintain for decades to come. A codebase developers actually enjoy contributing to. It's why we chose Go.

## Acknowledgements

- [leanSpec](https://github.com/leanEthereum/leanSpec) — Python reference specification
- [ethlambda](https://github.com/lambdaclass/ethlambda) — Rust implementation by LambdaClass

## Current status

Target: [leanSpec devnet 0](https://github.com/leanEthereum/leanSpec/tree/4b750f2748a3718fe3e1e9cdb3c65e3a7ddabff5)

The client implements:

- **Types** — SSZ containers aligned with leanSpec (Block, State, Vote, Checkpoint)
- **Consensus helpers** — 3SF-mini justification rules, round-robin proposer selection
- **State transition** — slot processing, block header validation, attestation handling

## Incoming features

- Fork choice (LMD-GHOST head selection, Store container)
- Networking (libp2p, gossipsub, status protocol)
- Validator duties (block and attestation production)
- Devnet integration (lean-quickstart support)

## License

MIT
