# Gean

A Go implementation of the Lean Ethereum consensus protocol that is simple enough to last.

## Getting started

```sh
make run
```

## Why Gean?

> *"Even if a protocol is super decentralized with hundreds of thousands of nodes... if the protocol is an unwieldy mess of hundreds of thousands of lines of code, ultimately that protocol fails."* — Vitalik Buterin

## The Goal
Our goal is to build a consensus client that is simple and readable yet elegant and resilient; code that anyone can read, understand, and maintain for decades to come. A codebase developers actually enjoy contributing to. It's why we chose Go.                           


## Acknowledgements

- [leanSpec](https://github.com/leanEthereum/leanSpec) — Python reference specification
- [ethlambda](https://github.com/lambdaclass/ethlambda) — Rust implementation by LambdaClass

## Progress

**Done:**
- Core types (Slot, Epoch, Root, ValidatorIndex)
- SSZ serialization via fastssz
- HashTreeRoot implementation
- Consensus containers (Checkpoint, Validator, Attestation, Block, State)

**Next:**
- Clock & genesis
- State transition
- Fork choice (LMD-GHOST)
- Networking (libp2p, GossipSub)
- Validator client

## License

MIT
