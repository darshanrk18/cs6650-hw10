#!/usr/bin/env python3
"""Generate latency and inter-read-write interval plots from loadtest_results.json.

Requires: pip install matplotlib
Usage:
  python3 scripts/plot_loadtest.py results/w5r1-50.json -o report/figs/
"""
from __future__ import annotations

import argparse
import json
import os
import sys

try:
    import matplotlib.pyplot as plt
except ImportError:
    print("Install matplotlib: pip install matplotlib", file=sys.stderr)
    sys.exit(1)


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("json_path")
    ap.add_argument("-o", "--out-dir", default=".", help="directory for PNG files")
    args = ap.parse_args()

    with open(args.json_path, encoding="utf-8") as f:
        doc = json.load(f)

    samples = doc.get("samples", [])
    reads = [s["latency_ms"] for s in samples if s.get("kind") == "read"]
    writes = [s["latency_ms"] for s in samples if s.get("kind") == "write"]
    intervals = [
        s["since_write_same_key_ms"]
        for s in samples
        if s.get("kind") == "read" and s.get("since_write_same_key_ms", 0) > 0
    ]

    os.makedirs(args.out_dir, exist_ok=True)
    base = os.path.splitext(os.path.basename(args.json_path))[0]

    def hist(data: list[float], title: str, xlab: str, fname: str) -> None:
        if not data:
            return
        plt.figure(figsize=(9, 5))
        plt.hist(data, bins=60, color="#2c5282", edgecolor="white", alpha=0.85)
        plt.title(title)
        plt.xlabel(xlab)
        plt.ylabel("count")
        plt.tight_layout()
        path = os.path.join(args.out_dir, f"{base}-{fname}.png")
        plt.savefig(path, dpi=150)
        plt.close()
        print(path)

    summ = doc.get("summary", {})
    tag = summ.get("quorum_profile") or summ.get("mode", "run")

    hist(reads, f"Read latency (ms) — {tag}", "latency (ms)", "read-latency")
    hist(writes, f"Write latency (ms) — {tag}", "latency (ms)", "write-latency")
    hist(
        intervals,
        f"Time since last write to same key (ms) — {tag}",
        "interval (ms)",
        "rw-interval",
    )


if __name__ == "__main__":
    main()
