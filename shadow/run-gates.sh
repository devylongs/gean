#!/usr/bin/env bash
#
# Run the Shadow verification gates inside the container: generate a testnet,
# build the topology, run the simulation, and assert the chain made progress.
# Optionally rerun and assert determinism (identical per-slot block roots).
#
# Env:
#   NODES          node count                          (default: 3)
#   STOP_TIME      Shadow stop_time                    (default: 120s)
#   FINALIZE_MIN   minimum finalized slot to pass      (default: 1)
#   DETERMINISM    1 = run twice and compare roots     (default: 0)
#   SHADOW_AGGREGATE_MS / SHADOW_VERIFY_MS  prover cost knobs (default: 0)
set -euo pipefail

NODES="${NODES:-3}"
STOP_TIME="${STOP_TIME:-120s}"
FINALIZE_MIN="${FINALIZE_MIN:-1}"
DETERMINISM="${DETERMINISM:-0}"
GENESIS_DELAY="${GENESIS_DELAY:-30}"
WORK="${WORK:-/work}"

GEAN_BIN="$(command -v gean)"
KEYGEN_BIN="$(command -v keygen)"
GEN="/app/shadow/gen_shadow_yaml.sh"

# Block-import log line: "block slot=N block_root=0x.. ... finalized_slot=N ..".
finalized_max() { grep -rhoE 'finalized_slot=[0-9]+' "$1"/hosts 2>/dev/null | cut -d= -f2 | sort -n | tail -1; }
head_max()      { grep -rhoE 'head slot=[0-9]+' "$1"/hosts 2>/dev/null | grep -oE '[0-9]+' | sort -n | tail -1; }
# Canonical (slot, block_root) digest for determinism comparison.
root_digest()   { grep -rhoE 'block slot=[0-9]+ block_root=0x[0-9a-f]+' "$1"/hosts 2>/dev/null | sort -u | sha256sum | cut -d' ' -f1; }

run_sim() {
  local out="$1"
  rm -rf "$WORK/testnet" "$WORK/shadow"
  "$KEYGEN_BIN" --validators "$NODES" --nodes "$NODES" --output "$WORK/testnet" --genesis-delay "$GENESIS_DELAY"
  TESTNET_DIR="$WORK/testnet" OUT_DIR="$WORK/shadow" GEAN_BIN="$GEAN_BIN" STOP_TIME="$STOP_TIME" bash "$GEN"
  rm -rf "$out"
  ( cd "$WORK/shadow" && shadow --progress true --parallelism "$(nproc)" --data-directory "$out" shadow.yaml )
}

cd "$WORK"
echo "=== gate: run simulation (nodes=$NODES stop_time=$STOP_TIME) ==="
run_sim "$WORK/run1.data"

fin="$(finalized_max "$WORK/run1.data")"; fin="${fin:-0}"
head="$(head_max "$WORK/run1.data")"; head="${head:-0}"
echo "=== result: head_slot=$head finalized_slot=$fin (need finalized >= $FINALIZE_MIN) ==="

fail=0
if [ "$fin" -lt "$FINALIZE_MIN" ]; then
  echo "FAIL: finalized_slot=$fin < $FINALIZE_MIN"; fail=1
else
  echo "PASS: finalization gate"
fi

if [ "$DETERMINISM" = "1" ]; then
  echo "=== gate: determinism (second run) ==="
  d1="$(root_digest "$WORK/run1.data")"
  run_sim "$WORK/run2.data"
  d2="$(root_digest "$WORK/run2.data")"
  echo "digest1=$d1 digest2=$d2"
  if [ -n "$d1" ] && [ "$d1" = "$d2" ]; then echo "PASS: determinism gate"; else echo "FAIL: determinism gate"; fail=1; fi
fi

exit "$fail"
