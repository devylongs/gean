# Gean 

A Go implementation of the Lean Ethereum consensus protocol.

## Overview

Gean is a Lean Ethereum consensus client, implementing the next-generation Ethereum consensus layer with post-quantum security, fast finality, and enhanced decentralization.

## Getting Started

```sh
cd cmd/gean
go build
./gean
```

## Philosophy

We follow a lean development approach inspired by [ethlambda](https://github.com/lambdaclass/ethlambda):

## References

- [leanSpec](https://github.com/leanEthereum/leanSpec) - Python reference specification
- [ethlambda](https://github.com/lambdaclass/ethlambda) - Rust implementation

## Roadmap

### Milestone 1: Foundation (Types & Serialization) - COMPLETE

Core type system and SSZ serialization - the bedrock everything else builds on.

- [x] Core primitive types (Slot, Epoch, Root, ValidatorIndex)
- [x] Byte array types (Bytes4, Bytes20, Bytes32, Bytes48, Bytes52, Bytes96)
- [x] SSZ bitfields via `[]byte` with fastssz tags
- [x] SSZ serialization and merkleization
- [x] `HashTreeRoot()` implementation

### Milestone 2: Consensus Containers

Data structures for blocks, state, and attestations.

- [ ] Checkpoint, Validator containers
- [ ] Attestation, AttestationData, SignedAttestation
- [ ] BlockHeader, BlockBody, Block, SignedBlockWithAttestation
- [ ] State container with full validator registry
- [ ] Chain configuration constants

### Milestone 3: Clock & Genesis

Time management and chain initialization.

- [ ] SlotClock with 4-second slots
- [ ] Interval timing (sub-slot)
- [ ] Genesis state generation
- [ ] Genesis block creation
- [ ] Validator config loading

### Milestone 4: Storage & Fork Choice

Persistent storage and chain head selection.

- [ ] Block and state storage interface
- [ ] LevelDB/Pebble backend
- [ ] Fork choice store (LMD-GHOST)
- [ ] Justification and finalization tracking
- [ ] Latest message tracking per validator

### Milestone 5: P2P Networking

Peer-to-peer communication layer.

- [ ] libp2p host setup
- [ ] discv5 peer discovery
- [ ] Peer manager
- [ ] GossipSub for blocks and attestations
- [ ] Request-response protocols (Status, BlocksByRoot, BlocksByRange)

### Milestone 6: Synchronization

Chain sync from peers.

- [ ] Range sync (batch block download)
- [ ] Head sync (follow chain tip)
- [ ] Block cache for pending blocks
- [ ] Peer scoring for sync

### Milestone 7: State Transition

Block validation and state processing.

- [ ] Slot processing
- [ ] Block header validation
- [ ] Attestation processing
- [ ] Epoch boundary processing
- [ ] Justification and finalization updates

### Milestone 8: XMSS Signatures

Post-quantum cryptography.

- [ ] XMSS signature verification
- [ ] Signature aggregation
- [ ] Integration with block/attestation validation

### Milestone 9: Validator Client

Block production and attestation duties.

- [ ] Proposer duty calculation
- [ ] Attester duty calculation
- [ ] Block production
- [ ] Attestation creation and signing

### Milestone 10: Full Node

Complete integrated client.

- [ ] Node orchestrator wiring all services
- [ ] CLI with run/genesis/keys commands
- [ ] Prometheus metrics
- [ ] HTTP API endpoints
- [ ] Docker support

## Current Status

**Milestone 1: Foundation** - Complete

The core type system and SSZ serialization are fully implemented. Run `go test ./...` to verify.

**Next:** Milestone 2 - Consensus Containers

## License

MIT
