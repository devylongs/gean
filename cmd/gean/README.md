# Gean Main Package

Main entry point for the Gean Lean Ethereum consensus client.

## Purpose

This package contains the main executable that initializes and runs the Gean client, handling:
- Command-line argument parsing
- Configuration loading
- Service orchestration
- Graceful shutdown

## Usage

```bash
go run cmd/gean/main.go [flags]
```

## Components

- **main()**: Application entry point
- **config**: Client configuration management
- **services**: Core service initialization
