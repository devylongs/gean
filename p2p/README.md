# Gean P2P Package

Peer-to-peer networking layer for the Gean Lean Ethereum client.

## Purpose

This package handles all network communication for Lean Consensus, including:
- libp2p integration and configuration
- Gossipsub v2.0 implementation
- Peer discovery and management
- Message routing and propagation
- Network security and encryption

## Key Components

- **network**: Main network manager and configuration
- **gossipsub**: Gossipsub v2.0 protocol implementation
- **discovery**: Peer discovery mechanisms
- **protocols**: Custom Lean Consensus protocols
- **security**: Network security and authentication

## Lean Consensus Networking

- Optimized for 4-second block times
- Support for increased validator count
- QUIC protocol implementation
- Rateless set reconciliation
- Grid topology support
