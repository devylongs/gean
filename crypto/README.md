# Gean Crypto Package

Cryptographic primitives for the Gean Lean Ethereum client.

## Purpose

This package implements Lean Ethereum's hash-based cryptography, including:
- leanSig: Post-quantum hash-based signatures
- leanMultisig: Signature aggregation schemes
- Hash function implementations
- Key generation and management
- Cryptographic utilities

## Key Components

- **leansig**: Hash-based signature scheme implementation
- **multisig**: Signature aggregation for consensus
- **hash**: Hash function utilities and optimizations
- **keys**: Key generation, storage, and management
- **utils**: Common cryptographic utilities

## Post-Quantum Security

All cryptographic primitives are designed to be:
- Quantum-resistant
- SNARK-friendly
- Compatible with zero-knowledge proofs
- Optimized for Lean Consensus requirements
