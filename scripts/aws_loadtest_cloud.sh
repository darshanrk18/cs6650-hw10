#!/usr/bin/env bash
# Run load tests against the Elastic IP from terraform (or pass IP as first arg).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
mkdir -p results/aws

if [[ -n "${1:-}" ]]; then
  IP="$1"
else
  IP=$(terraform -chdir=terraform output -raw kv_public_ip)
fi

EPS_LF="http://$IP:8080,http://$IP:8081,http://$IP:8082,http://$IP:8083,http://$IP:8084"
EPS_LL="http://$IP:18080,http://$IP:18081,http://$IP:18082,http://$IP:18083,http://$IP:18084"
DUR="${DURATION:-45s}"
# Must match QUORUM_PROFILE on the LF cluster (terraform quorum_profile or /opt/kv/docker-compose.lf.yml on EC2).
LF_PROFILE="${LF_PROFILE:-w5r1}"
# SKIP_LF=1 or SKIP_LL=1 to run only one mode (used by aws_loadtest_cloud_all_lf.sh).
SKIP_LF="${SKIP_LF:-}"
SKIP_LL="${SKIP_LL:-}"

if [[ -z "$SKIP_LF" ]]; then
  echo "Health check $IP ..."
  for p in 8080 8081 8082 8083 8084; do
    curl -sf --connect-timeout 5 "http://$IP:$p/health" >/dev/null || { echo "FAIL http://$IP:$p/health"; exit 1; }
  done
  echo "LF OK. Leader-follower matrix for profile=$LF_PROFILE (cluster must use the same QUORUM_PROFILE)."
  for WR in 0.01 0.1 0.5 0.9; do
    go run ./cmd/loadtest -mode=leader-follower -leader="http://$IP:8080" -endpoints="$EPS_LF" \
      -write-ratio="$WR" -duration="$DUR" -workers=8 -profile="$LF_PROFILE" -out="results/aws/lf-${LF_PROFILE}-w${WR}.json"
  done
fi

if [[ -z "$SKIP_LL" ]]; then
  echo "Leaderless ports ..."
  for p in 18080 18081 18082 18083 18084; do
    curl -sf --connect-timeout 5 "http://$IP:$p/health" >/dev/null || { echo "WARN: http://$IP:$p/health failed (leaderless may still be starting)"; }
  done
  for WR in 0.01 0.1 0.5 0.9; do
    go run ./cmd/loadtest -mode=leaderless -endpoints="$EPS_LL" \
      -write-ratio="$WR" -duration="$DUR" -workers=8 -out="results/aws/ll-w${WR}.json"
  done
fi
echo "Done. Results in results/aws/"
