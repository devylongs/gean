#!/usr/bin/env bash
#
# Generate a Shadow (https://github.com/shadow/shadow) network description from a
# gean testnet produced by `bin/keygen` (e.g. `make run-setup`).
#
# Shadow gives every host its own IP, so the loopback addresses keygen writes into
# nodes.yaml are rewritten to per-host simulated IPs. A staging config dir is built
# (config.yaml + annotated_validators.yaml copied, hash-sig-keys symlinked, nodes.yaml
# rewritten) so the source testnet keeps working for the loopback `make run` flow.
#
# Output: <OUT_DIR>/net/ (per-host config dir) and <OUT_DIR>/shadow.yaml.
#
# Env overrides:
#   TESTNET_DIR        gean testnet dir                       (default: testnet)
#   OUT_DIR            output dir for net/ + shadow.yaml      (default: shadow)
#   STOP_TIME          Shadow stop_time                       (default: 90s)
#   IP_PREFIX          host IP prefix                         (default: 10.0.0.)
#   IP_OFFSET          first host's last octet                (default: 10)
#   SHADOW_AGGREGATE_MS  simulated aggregation delay (ms)     (default: 0)
#   SHADOW_VERIFY_MS     simulated verify delay (ms)          (default: 0)
set -euo pipefail

TESTNET_DIR="${TESTNET_DIR:-testnet}"
OUT_DIR="${OUT_DIR:-shadow}"
STOP_TIME="${STOP_TIME:-90s}"
IP_PREFIX="${IP_PREFIX:-10.0.0.}"
IP_OFFSET="${IP_OFFSET:-10}"
SHADOW_AGGREGATE_MS="${SHADOW_AGGREGATE_MS:-0}"
SHADOW_VERIFY_MS="${SHADOW_VERIFY_MS:-0}"

abspath() { (cd "$1" >/dev/null 2>&1 && pwd) || { echo "missing dir: $1" >&2; exit 1; }; }

ROOT="$(abspath "$(dirname "$0")/..")"
TESTNET_ABS="$(abspath "$TESTNET_DIR")"
NODES_YAML="$TESTNET_ABS/nodes.yaml"
GEAN_BIN="$ROOT/bin/gean"

[[ -f "$NODES_YAML" ]] || { echo "Error: $NODES_YAML not found — run 'make run-setup' first" >&2; exit 1; }
[[ -x "$GEAN_BIN" ]] || { echo "Error: $GEAN_BIN not built — run 'make build' first" >&2; exit 1; }

# Parse each bootnode multiaddr: /ip4/<ip>/udp/<port>/quic-v1/p2p/<peerid>
ports=(); peerids=()
while IFS= read -r line; do
  [[ "$line" =~ /udp/([0-9]+)/quic-v1/p2p/([A-Za-z0-9]+) ]] || continue
  ports+=("${BASH_REMATCH[1]}")
  peerids+=("${BASH_REMATCH[2]}")
done < "$NODES_YAML"

n="${#peerids[@]}"
[[ "$n" -ge 1 ]] || { echo "Error: no bootnodes parsed from $NODES_YAML" >&2; exit 1; }
echo "nodes: $n, stop_time: $STOP_TIME, aggregate_ms: $SHADOW_AGGREGATE_MS, verify_ms: $SHADOW_VERIFY_MS"

host_ip() { echo "${IP_PREFIX}$((IP_OFFSET + $1))"; }

# Build staging config dir with per-host IPs in nodes.yaml.
NET_DIR="$OUT_DIR/net"
rm -rf "$NET_DIR"
mkdir -p "$NET_DIR"
cp "$TESTNET_ABS/config.yaml" "$TESTNET_ABS/annotated_validators.yaml" "$NET_DIR/"
ln -s "$TESTNET_ABS/hash-sig-keys" "$NET_DIR/hash-sig-keys"
: > "$NET_DIR/nodes.yaml"
for i in $(seq 0 $((n - 1))); do
  printf -- '- "/ip4/%s/udp/%s/quic-v1/p2p/%s"\n' "$(host_ip "$i")" "${ports[$i]}" "${peerids[$i]}" >> "$NET_DIR/nodes.yaml"
done
NET_ABS="$(abspath "$NET_DIR")"

# Optional simulated prover-cost args (Shadow does not charge CPU to virtual time).
cost_args=""
[[ "$SHADOW_AGGREGATE_MS" -gt 0 ]] && cost_args+=" --shadow-aggregate-cost-ms $SHADOW_AGGREGATE_MS"
[[ "$SHADOW_VERIFY_MS" -gt 0 ]] && cost_args+=" --shadow-verify-cost-ms $SHADOW_VERIFY_MS"

YAML="$OUT_DIR/shadow.yaml"
{
  echo "general:"
  echo "  stop_time: $STOP_TIME"
  echo "  model_unblocked_syscall_latency: true"
  echo "experimental:"
  echo "  native_preemption_enabled: true"
  echo "network:"
  echo "  graph:"
  echo "    type: 1_gbit_switch"
  echo "hosts:"
  for i in $(seq 0 $((n - 1))); do
    agg=""
    [[ "$i" -eq 0 ]] && agg=" --is-aggregator"
    echo "  node$i:"
    echo "    network_node_id: 0"
    echo "    ip_addr: $(host_ip "$i")"
    echo "    processes:"
    echo "      - path: $GEAN_BIN"
    echo "        args: --custom-network-config-dir $NET_ABS --node-key $TESTNET_ABS/node$i.key --node-id node$i --data-dir $NET_ABS/data/node$i --gossipsub-port ${ports[$i]} --api-port 5052 --metrics-port 8080$agg$cost_args"
    echo "        environment: { QUIC_GO_DISABLE_GSO: \"true\" }"
    echo "        expected_final_state: running"
    echo ""
  done
} > "$YAML"

echo "wrote $YAML and $NET_DIR/nodes.yaml"
