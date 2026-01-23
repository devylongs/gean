.PHONY: build test clean run help generate

BIN_DIR := bin
BINARY := $(BIN_DIR)/gean

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

run: build ## Build and run gean
	./$(BINARY)

lint: ## Run go vet
	go vet ./...
