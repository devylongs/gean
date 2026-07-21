.PHONY: help build ffi test-ffi test test-spec test-all lint fmt sszgen clean tidy docker-build run-devnet run-setup run run-node1 run-node2

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT := $(shell git rev-parse HEAD 2>/dev/null || echo "unknown")
GIT_BRANCH := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")

TESTNET_DIR ?= testnet
NUM_VALIDATORS ?= 5
NUM_NODES ?= 3

help: ## Show help for each Makefile recipe
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

ffi: ## Build XMSS FFI glue libraries (hashsig-glue + multisig-glue)
	@cd xmss/rust && \
		if [ "$$(uname -m)" = "x86_64" ]; then \
			CARGO_ENCODED_RUSTFLAGS="-Ctarget-cpu=haswell" cargo build --profile multisig-release --locked; \
		else \
			cargo build --profile multisig-release --locked; \
		fi

build: ffi ## Build gean and keygen binaries
	@mkdir -p bin
	@go build -ldflags "-X github.com/geanlabs/gean/internal/node.gitCommit=$(GIT_COMMIT)" -o bin/gean ./cmd/gean
	@go build -o bin/keygen ./cmd/keygen

test: ## Run unit tests (excludes crypto FFI and spec tests)
	go test $(shell go list ./... | grep -v '/xmss$$' | grep -v '/spectests$$' | grep -v '/cmd/') -v -count=1

test-ffi: ffi ## Run XMSS crypto FFI tests (builds FFI first)
	go test ./xmss/ -v -count=1

test-spec: leanSpec/fixtures ## Run spec fixture tests only (fast, excludes xmss FFI)
	go test ./internal/spectests/  -count=1 -tags=spectests

test-all: leanSpec/fixtures ## Run all tests including spec fixtures and xmss FFI (slow)
	go test ./... -v -count=1 -tags=spectests

lint: ## Run linters for go & rust
	go vet ./...
	cd xmss/rust && cargo fmt --check
	cd xmss/rust && cargo clippy -- -D warnings -A clippy::missing_safety_doc

fmt: ## Format all Go code
	gofmt -w .
	cd xmss/rust && cargo fmt

sszgen: ## Regenerate SSZ encoding files from struct tags
	@rm -f internal/types/*_encoding.go
	sszgen --path internal/types --objs ChainConfig --output internal/types/config_encoding.go
	sszgen --path internal/types --objs Checkpoint --output internal/types/checkpoint_encoding.go
	sszgen --path internal/types --objs Validator --output internal/types/validator_encoding.go
	sszgen --path internal/types --objs AttestationData,Attestation,SignedAttestation,AggregatedAttestation,SignedAggregatedAttestation --exclude-objs Checkpoint --output internal/types/attestation_encoding.go
	sszgen --path internal/types --objs BlockHeader,BlockBody,Block,AggregatedSignatureProof,BlockSignatures,SignedBlock --exclude-objs Checkpoint,AttestationData,Attestation,AggregatedAttestation,AggregatedSignatureProof --output internal/types/block_encoding.go
	sszgen --path internal/types --objs State --exclude-objs ChainConfig,Checkpoint,Validator,BlockHeader --output internal/types/state_encoding.go
	sszgen --path internal/types --objs BlocksByRangeRequest --output internal/types/blocks_by_range_encoding.go

clean: ## Remove build artifacts and generated files
	rm -rf bin data
	cd xmss/rust && cargo clean

tidy: ## Tidy Go module dependencies
	go mod tidy

# --- Local testnet ---

run-setup: build ## Generate testnet config + XMSS keys (first run only, refreshes genesis time)
	@bin/keygen --validators $(NUM_VALIDATORS) --nodes $(NUM_NODES) --output $(TESTNET_DIR)

run: build ## Run node0 (aggregator) — requires make run-setup first
	@rm -rf data/node0
	@bin/gean \
		--custom-network-config-dir $(TESTNET_DIR) \
		--node-key $(TESTNET_DIR)/node0.key \
		--node-id node0 \
		--data-dir data/node0 \
		--is-aggregator \
		--gossipsub-port 9000 \
		--api-port 5052 \
		--metrics-port 8080

run-node1: build ## Run node1 on port 9001
	@rm -rf data/node1
	@bin/gean \
		--custom-network-config-dir $(TESTNET_DIR) \
		--node-key $(TESTNET_DIR)/node1.key \
		--node-id node1 \
		--data-dir data/node1 \
		--gossipsub-port 9001 \
		--api-port 5053 \
		--metrics-port 8081

run-node2: build ## Run node2 on port 9002
	@rm -rf data/node2
	@bin/gean \
		--custom-network-config-dir $(TESTNET_DIR) \
		--node-key $(TESTNET_DIR)/node2.key \
		--node-id node2 \
		--data-dir data/node2 \
		--gossipsub-port 9002 \
		--api-port 5054 \
		--metrics-port 8082

# --- leanSpec fixtures ---

LEAN_SPEC_COMMIT_HASH := 1589f87

leanSpec: ## Clone leanSpec at pinned main commit (contains devnet-4 changes)
	git clone https://github.com/leanEthereum/leanSpec.git --single-branch
	cd leanSpec && git checkout $(LEAN_SPEC_COMMIT_HASH)

leanSpec/fixtures: leanSpec ## Generate consensus test fixtures from leanSpec
	cd leanSpec && \
		KEYS_URL=$$(uv run python -c "from consensus_testing.keys import KEY_DOWNLOAD_URLS; print(KEY_DOWNLOAD_URLS['prod'])") && \
		KEYS_DIR=packages/testing/src/consensus_testing/test_keys && \
		mkdir -p $$KEYS_DIR && \
		curl -sSL "$$KEYS_URL" -o /tmp/prod_scheme.tar.gz && \
		tar -xzf /tmp/prod_scheme.tar.gz -C $$KEYS_DIR && \
		uv run fill --fork Lstar --scheme prod -o fixtures

# --- Docker ---

DOCKER_TAG ?= local

docker-build: ## Build Docker image
	docker build \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		--build-arg GIT_BRANCH=$(GIT_BRANCH) \
		-t gean:$(VERSION) \
		-t ghcr.io/geanlabs/gean:devnet4 .

# --- Multi-client devnet ---

lean-quickstart: ## Clone lean-quickstart for local devnet
	git clone https://github.com/blockblaz/lean-quickstart.git --depth 1 --single-branch

run-devnet: docker-build lean-quickstart ## Run local multi-client devnet
	@echo "Starting local devnet with gean client (\"$(DOCKER_TAG)\" tag)."
	@cd lean-quickstart \
		&& NETWORK_DIR=local-devnet ./spin-node.sh --node all --generateGenesis --metrics > ../devnet.log 2>&1
