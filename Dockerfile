# Build stage: Rust FFI + Go binary
FROM golang:1.25-bookworm AS builder

# Install Rust 1.92.0 (pinned for leansig/leanMultisig compatibility)
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y --default-toolchain 1.92.0
ENV PATH="/root/.cargo/bin:${PATH}"

# Install build dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    pkg-config \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy Rust FFI dependencies first for better caching
COPY xmss/rust/ xmss/rust/

# Build Rust FFI libraries
# On amd64: -Ctarget-cpu=haswell enables AVX2 SIMD in leanMultisig's backend
# for ~6x prover speedup. Haswell (2013+) is the portable x86 baseline.
# On arm64: build without x86 flags; native NEON is used automatically.
ARG TARGETARCH
RUN cd xmss/rust && \
    if [ "$TARGETARCH" = "amd64" ]; then \
      CARGO_ENCODED_RUSTFLAGS="-Ctarget-cpu=haswell" cargo build --profile multisig-release --locked; \
    else \
      cargo build --profile multisig-release --locked; \
    fi

# Stage leanVM Python sources at the exact checkout path the binary expects.
# The lean_compiler resolves .py files via CARGO_MANIFEST_DIR baked at compile time;
# on arm64 the pre-committed bytecode cache misses and triggers a recompile from source.
# Match the checkout by crate, not by pinned rev: cargo names the rev subdir after
# the leanVM commit, so a hardcoded short hash breaks on every dependency bump.
RUN CHECKOUT_DIR=$(ls -d /root/.cargo/git/checkouts/leanvm-*/*/crates/rec_aggregation | head -1 | sed 's|/crates/rec_aggregation||') && \
    mkdir -p /leanvm-staged && \
    echo "$CHECKOUT_DIR" > /leanvm-staged/.checkout_root && \
    cp -r "$CHECKOUT_DIR/crates/rec_aggregation" /leanvm-staged/rec_aggregation && \
    cp -r "$CHECKOUT_DIR/crates/lean_compiler" /leanvm-staged/lean_compiler

# Copy Go module files for dependency caching
COPY go.mod go.sum ./
RUN go mod download

# Copy all source code
COPY . .

# Build Go binaries
ARG GIT_COMMIT=unknown
ARG GIT_BRANCH=unknown
RUN mkdir -p bin && \
    go build -tags hive_testdriver -ldflags "-X github.com/geanlabs/gean/internal/node.gitCommit=$GIT_COMMIT" -o bin/gean ./cmd/gean && \
    go build -o bin/keygen ./cmd/keygen

# Runtime stage
FROM ubuntu:24.04 AS runtime
WORKDIR /app

LABEL org.opencontainers.image.source=https://github.com/geanlabs/gean
LABEL org.opencontainers.image.description="Go Ethereum Lean Consensus Client"
LABEL org.opencontainers.image.licenses="MIT"

ARG GIT_COMMIT=unknown
ARG GIT_BRANCH=unknown
LABEL org.opencontainers.image.revision=$GIT_COMMIT
LABEL org.opencontainers.image.ref.name=$GIT_BRANCH

# Copy binaries
COPY --from=builder /app/bin/gean /usr/local/bin/
COPY --from=builder /app/bin/keygen /usr/local/bin/

# leanVM's lean_compiler reads .py files at runtime when the embedded
# cached_bytecode.bin fingerprint doesn't match the build target (arm64 builds
# hit this because the repo's cache is x86-only). Restore the Python sources
# at the exact CARGO_MANIFEST_DIR path baked into the binary at compile time.
COPY --from=builder /leanvm-staged/ /tmp/leanvm-staged/
RUN CHECKOUT_ROOT=$(cat /tmp/leanvm-staged/.checkout_root) && \
    mkdir -p "$CHECKOUT_ROOT/crates" && \
    cp -r /tmp/leanvm-staged/rec_aggregation "$CHECKOUT_ROOT/crates/" && \
    cp -r /tmp/leanvm-staged/lean_compiler "$CHECKOUT_ROOT/crates/" && \
    rm -rf /tmp/leanvm-staged


# Keep the Go heap tight so the XMSS prover's transient multi-GB proving
# peaks (allocated by the Rust arena, invisible to the Go GC) land on free
# memory instead of an uncollected heap. Operators can override.
ENV GOMEMLIMIT=2GiB

# 9000/udp - P2P QUIC
# 5052 - API
# 5054 - Prometheus metrics
EXPOSE 9000/udp 5052 5054

ENTRYPOINT ["/usr/local/bin/gean"]
