t# Gean

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

## Devnet 0 Roadmap

Target: [leanSpec devnet 0](https://github.com/leanEthereum/leanSpec/tree/4b750f2748a3718fe3e1e9cdb3c65e3a7ddabff5)

### Milestone 1: Type Alignment ✅

Align SSZ containers with leanSpec devnet 0 specification.

- [x] Replace Attestation/AggregatedAttestation with Vote/SignedVote
- [x] Add `NumValidators` to Config
- [x] Remove Validator struct and State.Validators field
- [x] Rename HistoricalRoots → HistoricalBlockHashes
- [x] Add SignedBlock container
- [x] Regenerate SSZ encoding code
- [x] Update tests

### Milestone 2: Consensus Helpers ✅

Implement core consensus helper functions.

- [x] `Slot.IsJustifiableAfter()` - 3SF-mini justification rules
- [x] `IsProposer()` - Round-robin proposer selection
- [x] Genesis state generation aligned with spec

### Milestone 3: State Transition ✅

Implement the state transition function.

- [x] `ProcessSlot()` - Cache state root in header
- [x] `ProcessSlots()` - Advance state through empty slots
- [x] `ProcessBlockHeader()` - Validate and apply block header
- [x] `ProcessAttestations()` - Apply votes and update justification
- [x] `ProcessBlock()` - Full block processing
- [x] `StateTransition()` - Complete state transition with validation
- [x] Bitlist helper functions for SSZ bitlists

### Milestone 4: Fork Choice ⬜

Implement LMD-GHOST fork choice.

- [ ] `Store` container - Track blocks, states, votes
- [ ] `GetForkChoiceHead()` - LMD-GHOST head selection
- [ ] `GetLatestJustified()` - Find highest justified checkpoint
- [ ] `Store.ProcessBlock()` - Add blocks to store
- [ ] `Store.ProcessAttestation()` - Track validator votes
- [ ] `Store.AdvanceTime()` - Tick-based time advancement

### Milestone 5: Networking ⬜

Implement P2P networking layer.

- [ ] libp2p integration
- [ ] Status protocol (handshake)
- [ ] BlocksByRoot protocol (block recovery)
- [ ] GossipSub for blocks and attestations

### Milestone 6: Validator ⬜

Implement validator duties.

- [ ] Block production
- [ ] Attestation production
- [ ] Signature integration (placeholder for now)

### Milestone 7: Devnet Integration ⬜

Integration with lean-quickstart.

- [ ] Parse validator-config.yaml
- [ ] Parse genesis config.yaml
- [ ] ENR generation and discovery
- [ ] Docker support

## License

MIT
