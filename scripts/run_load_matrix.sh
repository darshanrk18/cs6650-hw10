#!/usr/bin/env bash
# Run load tests for all quorum profiles and write ratios against a running cluster.
# Leader-follower: set ENDPOINTS and LEADER (defaults below for docker compose).
# Usage:
#   ./scripts/run_load_matrix.sh leader-follower http://localhost:8080
#   ./scripts/run_load_matrix.sh leaderless http://localhost:18080

set -euo pipefail

MODE=${1:-leader-follower}
LEADER=${2:-http://localhost:8080}
OUT_DIR=${OUT_DIR:-./results}
DURATION=${DURATION:-45s}

mkdir -p "$OUT_DIR"

if [[ "$MODE" == "leader-follower" ]]; then
  EPS="http://localhost:8080,http://localhost:8081,http://localhost:8082,http://localhost:8083,http://localhost:8084"
  for PROF in w5r1 w1r5 r3w3; do
    echo "Set QUORUM_PROFILE=$PROF on all nodes, then restart cluster before this profile's runs." >&2
    for WR in 0.01 0.1 0.5 0.9; do
      f="$OUT_DIR/lf-${PROF}-w${WR}.json"
      echo "=== $PROF write_ratio=$WR -> $f"
      go run ./cmd/loadtest \
        -mode=leader-follower \
        -leader="$LEADER" \
        -endpoints="$EPS" \
        -write-ratio="$WR" \
        -duration="$DURATION" \
        -profile="$PROF" \
        -out="$f" || true
    done
  done
else
  EPS="http://localhost:18080,http://localhost:18081,http://localhost:18082,http://localhost:18083,http://localhost:18084"
  for WR in 0.01 0.1 0.5 0.9; do
    f="$OUT_DIR/leaderless-w${WR}.json"
    echo "=== leaderless write_ratio=$WR -> $f"
    go run ./cmd/loadtest \
      -mode=leaderless \
      -endpoints="$EPS" \
      -write-ratio="$WR" \
      -duration="$DURATION" \
      -out="$f" || true
  done
fi
