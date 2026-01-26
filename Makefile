.PHONY: build test clean run help generate lint

BIN_DIR := bin
BINARY := $(BIN_DIR)/gean

# Configuration
VALIDATORS ?= 8
VALIDATOR_INDEX ?=
LISTEN_ADDR ?= /ip4/0.0.0.0/udp/9000/quic-v1
LOG_LEVEL ?= info
GENESIS_TIME ?=

help: ## Show help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

generate: ## Generate SSZ encoding code
	go run github.com/ferranbt/fastssz/sszgen --path=./consensus --objs=Checkpoint,Config,Vote,SignedVote,BlockHeader,BlockBody,Block,SignedBlock,State

build: ## Build the gean binary
	@mkdir -p $(BIN_DIR)
	go build -o $(BINARY) ./cmd/gean

test: ## Run tests
	go test ./... -v

clean: ## Remove build artifacts
	rm -rf $(BIN_DIR)
	go clean

lint: ## Run go vet
	go vet ./...

run: build ## Run gean
ifdef VALIDATOR_INDEX
	./$(BINARY) --validators $(VALIDATORS) --validator-index $(VALIDATOR_INDEX) --listen "$(LISTEN_ADDR)" --log-level $(LOG_LEVEL)
else
	./$(BINARY) --validators $(VALIDATORS) --listen "$(LISTEN_ADDR)" --log-level $(LOG_LEVEL)
endif

run-validator: build ## Run as validator 0
	./$(BINARY) --validators $(VALIDATORS) --validator-index 0 --listen "$(LISTEN_ADDR)" --log-level debug
