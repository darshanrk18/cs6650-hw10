#!/usr/bin/env bash
# From repo root after `terraform apply`. Uses terraform output for ECR URL.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
REGION=$(terraform -chdir=terraform output -raw aws_region)
REGISTRY=$(terraform -chdir=terraform output -raw ecr_repository_url)
echo "Logging in to $REGISTRY (region $REGION)..."
aws ecr get-login-password --region "$REGION" | docker login --username AWS --password-stdin "$REGISTRY"

docker build -f Dockerfile.leader-follower -t kv-leader-follower "$ROOT"
docker tag kv-leader-follower:latest "${REGISTRY}:leader-follower"
docker push "${REGISTRY}:leader-follower"

docker build -f Dockerfile.leaderless -t kv-leaderless "$ROOT"
docker tag kv-leaderless:latest "${REGISTRY}:leaderless"
docker push "${REGISTRY}:leaderless"
echo "Pushed :leader-follower and :leaderless"
