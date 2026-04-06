#!/usr/bin/env bash
# Run LF load tests for w5r1, w1r5, r3w3 against the cloud cluster, then leaderless once.
#
# By default (REMOTE_UPDATE=1) sets QUORUM_PROFILE on EC2 via SSM (scripts/aws_ec2_set_lf_quorum.sh).
# Set REMOTE_UPDATE=0 to skip SSM — use PROMPT_BETWEEN_PROFILES=1 and update the host yourself.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
mkdir -p results/aws

if [[ -n "${1:-}" ]]; then
  IP="$1"
else
  IP=$(terraform -chdir=terraform output -raw kv_public_ip)
fi

DUR="${DURATION:-45s}"
REMOTE_UPDATE="${REMOTE_UPDATE:-1}"
PROMPT="${PROMPT_BETWEEN_PROFILES:-0}"

for LF_PROFILE in w5r1 w1r5 r3w3; do
  echo ""
  echo "======== LF quorum profile: $LF_PROFILE ========"
  if [[ "$REMOTE_UPDATE" != "0" ]]; then
    "$ROOT/scripts/aws_ec2_set_lf_quorum.sh" "$LF_PROFILE"
    echo "Waiting for LF containers..."
    sleep 10
  elif [[ "$PROMPT" != "0" ]]; then
    echo "Set QUORUM_PROFILE=$LF_PROFILE on EC2, then press Enter..."
    read -r _
  fi
  export LF_PROFILE DURATION="$DUR"
  SKIP_LL=1 "$ROOT/scripts/aws_loadtest_cloud.sh" "$IP"
done

echo ""
echo "======== Leaderless (same for all runs) ========"
SKIP_LF=1 DURATION="$DUR" "$ROOT/scripts/aws_loadtest_cloud.sh" "$IP"
echo "All profiles done. Results in results/aws/"
