# Gean RPC Package

JSON-RPC API layer for the Gean Lean Ethereum client.

## Purpose

This package provides the JSON-RPC interface for external communication, including:
- Standard Ethereum JSON-RPC methods
- Lean Consensus-specific endpoints
- WebSocket and HTTP transport
- API authentication and rate limiting
- Request validation and response formatting

## Key Components

- **server**: HTTP and WebSocket server
- **handlers**: RPC method implementations
- **auth**: Authentication and authorization
- **middleware**: Request/response middleware
- **types**: RPC data structures and validation

## Supported Methods

- Standard Ethereum methods (eth_, net_, web3_)
- Lean Consensus methods (lean_, consensus_)
- Debug and development endpoints
- WebSocket subscriptions
- Batch request support
