#!/usr/bin/env bash
# Plot every *.json loadtest file in a directory (e.g. results/aws → results/aws/figs).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
IN_DIR="${1:?usage: $0 <json-dir> [out-dir]}"
OUT_DIR="${2:-$IN_DIR/figs}"
mkdir -p "$OUT_DIR"
shopt -s nullglob
for f in "$IN_DIR"/*.json; do
  python3 "$ROOT/scripts/plot_loadtest.py" "$f" -o "$OUT_DIR"
done
echo "Wrote PNGs under $OUT_DIR"
