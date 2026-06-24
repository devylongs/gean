# Dockerized Shadow gate runner.
#
# Bases on a gean binary image (built from this branch) and adds the Shadow
# network simulator so the verification gates can run on a Linux host from macOS.
# Build via `make shadow-docker-run`, which first builds the gean image.
ARG GEAN_IMAGE=gean:shadow-base
FROM ${GEAN_IMAGE}

ENV DEBIAN_FRONTEND=noninteractive

# Shadow build + runtime dependencies (Ubuntu).
RUN apt-get update && apt-get install -y --no-install-recommends \
    git curl ca-certificates build-essential cmake pkg-config \
    libglib2.0-0 libglib2.0-dev libclang-dev libc-dbg \
    python3 python3-networkx xz-utils util-linux findutils netbase \
    && rm -rf /var/lib/apt/lists/*

# Rust toolchain is only needed to compile Shadow.
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y --default-toolchain stable
ENV PATH="/root/.cargo/bin:${PATH}"
# Fetch crates via the git CLI; more resilient than the built-in fetcher in Docker.
ENV CARGO_NET_GIT_FETCH_WITH_CLI=true CARGO_NET_RETRY=10 CARGO_HTTP_MULTIPLEXING=false CARGO_HTTP_TIMEOUT=180

ARG SHADOW_VERSION=v3.3.0
# Retry the build: cargo caches successful downloads, so flaky crates.io fetches
# converge across attempts rather than failing the whole image.
RUN git clone --depth 1 -b ${SHADOW_VERSION} https://github.com/shadow/shadow.git /opt/shadow-src \
    && cd /opt/shadow-src \
    && for i in 1 2 3 4 5; do ./setup build -j"$(nproc)" --prefix /usr/local && break || { echo "build attempt $i failed, retrying"; sleep 15; }; done \
    && test -f build/src/main/shadow \
    && ./setup install \
    && rm -rf /opt/shadow-src /root/.cargo/registry

COPY shadow/gen_shadow_yaml.sh shadow/run-gates.sh /app/shadow/
RUN chmod +x /app/shadow/*.sh

ENV WORK=/work
WORKDIR /work
ENTRYPOINT ["/app/shadow/run-gates.sh"]
