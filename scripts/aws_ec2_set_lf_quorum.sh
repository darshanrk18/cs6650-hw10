#!/usr/bin/env bash
# Set QUORUM_PROFILE on every lf* service via /opt/kv/docker-compose.lf.yml on the EC2 host (SSM).
# Usage: ./scripts/aws_ec2_set_lf_quorum.sh w5r1|w1r5|r3w3
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PROFILE="${1:?usage: $0 w5r1|w1r5|r3w3}"
case "$PROFILE" in w5r1|w1r5|r3w3) ;; *)
  echo "Invalid profile: $PROFILE" >&2
  exit 1
  ;;
esac

REGION=$(terraform -chdir="$ROOT/terraform" output -raw aws_region)
INSTANCE=$(terraform -chdir="$ROOT/terraform" output -raw instance_id)

JSON=$(PROFILE="$PROFILE" INSTANCE="$INSTANCE" python3 -c '
import json, os
p = os.environ["PROFILE"]
i = os.environ["INSTANCE"]
cmd = "sed -i \047s|QUORUM_PROFILE:.*|QUORUM_PROFILE: " + p + "|\047 docker-compose.lf.yml"
print(json.dumps({
  "InstanceIds": [i],
  "DocumentName": "AWS-RunShellScript",
  "Comment": "kv lf quorum " + p,
  "Parameters": {"commands": [
    "set -euo pipefail",
    "cd /opt/kv",
    cmd,
    "docker-compose -f docker-compose.lf.yml -p kvlf up -d",
  ]},
}))
')

echo "SSM: instance $INSTANCE → QUORUM_PROFILE=$PROFILE"
CMD_ID=$(aws ssm send-command \
  --region "$REGION" \
  --cli-input-json "$JSON" \
  --query 'Command.CommandId' \
  --output text)

for _ in $(seq 1 45); do
  STATUS=$(aws ssm get-command-invocation \
    --region "$REGION" \
    --command-id "$CMD_ID" \
    --instance-id "$INSTANCE" \
    --query 'Status' \
    --output text 2>/dev/null || echo Pending)
  case "$STATUS" in
    Success)
      echo "SSM Success"
      aws ssm get-command-invocation \
        --region "$REGION" \
        --command-id "$CMD_ID" \
        --instance-id "$INSTANCE" \
        --query 'StandardOutputContent' \
        --output text 2>/dev/null | tail -n 8 || true
      exit 0
      ;;
    Failed|Cancelled|TimedOut|Undeliverable|Terminated)
      echo "SSM failed: $STATUS" >&2
      aws ssm get-command-invocation \
        --region "$REGION" \
        --command-id "$CMD_ID" \
        --instance-id "$INSTANCE" \
        --query '[StandardErrorContent,StandardOutputContent]' \
        --output text >&2 || true
      exit 1
      ;;
  esac
  sleep 2
done
echo "SSM: timed out waiting for command" >&2
exit 1
