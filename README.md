# CS 6650 Homework 10 — Replication (Leader-Follower & Leaderless KV)

In-memory HTTP key-value services that simulate replication latency so you can observe quorum behavior, stale reads, and tail latency.

**AI assistance:** Scaffold, replication logic, tests, Docker/Terraform, load test, and this README were developed with help from an AI coding agent; you should trace through the code and run tests before submitting.

**Course submission:** The assignment asks for a **Khoury Git** URL and a **PDF** report. See **`docs/SUBMISSION.md`** (checklist, Khoury push, PDF export) and **`docs/HANDOVER.md`** (how writes/reads/N-R-W work, errors, tricky bits — so you can explain the code yourself).

## API

| Operation | Method | Notes |
|-----------|--------|--------|
| Write | `POST /kv/set` | JSON `{"key":"...","value":"..."}`. Empty key rejected; empty value allowed. Returns `201` and `{"ok":true,"version":n}`. |
| Read | `GET /kv/get?key=...` | `200` + `{"value":"...","version":n}` or `404`. |
| Local read (testing) | `GET /kv/local_read?key=...` | Single-node view; may be stale during replication. |
| Health | `GET /health` | `200`. |

Internal: `POST /internal/replicate`, `GET /internal/peer-read`, and (leader-follower only) `GET /internal/cluster-read`.

## Configuration

All nodes need:

- `PEERS` — comma-separated base URLs, **same order on every node**. For leader-follower, **index `0` is always the leader**.
- `NODE_INDEX` — `0` … `len(PEERS)-1`.
- `PORT` — listen port (default `8080`).

Leader-follower only:

- `QUORUM_PROFILE` — `w5r1` | `w1r5` | `r3w3`.

## How N, R, W map to this code

**Leader-follower (`internal/kvsrv/lf.go`):**

- **Writes** hit only the leader (`403` on followers). The leader runs `WriteLocal`, which assigns a monotonically increasing per-key logical version, then:
  - **W=5 (`w5r1`):** Synchronously replicates to all four followers (each round-trip includes follower-side `100ms` sleep). After each follower ack, the leader sleeps `200ms` before contacting the next.
  - **W=1 (`w1r5`):** Returns after the leader commit; followers receive async replication with the same delays (so the inconsistency window is visible under load).
  - **W=3 (`r3w3`):** Waits for two followers (plus the leader) synchronously, then **best-effort async** replicates to the remaining followers so all replicas eventually match.
- **Reads:** Any node accepts `GET /kv/get`; non-leaders forward to the leader’s `cluster-read`. The leader then:
  - **R=1 (`w5r1`):** Single local read.
  - **R=5 (`w1r5`):** Reads the first five entries in `PEERS` in parallel (leader uses local `Get`; others use `peer-read`). Each follower sleeps `50ms` when serving `peer-read` to simulate fetch cost. The response is the tuple with the **maximum version**.
  - **R=3 (`r3w3`):** Same merge rule but only contacts the first three peers in `PEERS`.
- **Errors:** A failed synchronous replication surfaces as `500` on the write path; async paths log errors (production code would add retries/backoff).

**Leaderless (`internal/kvsrv/leaderless.go`):**

- **W=N:** The node that receives `POST /kv/set` is the write coordinator: it runs `WriteLocal`, then sequentially replicates to every other peer, applying the same `200ms` / `100ms` delays.
- **R=1:** `GET /kv/get` returns only the local replica (no quorum read), so readers can observe staleness until replication finishes.

## Run locally

```bash
go test ./... -race -timeout 120s
```

**Leader-follower** (change quorum via env):

```bash
QUORUM_PROFILE=w5r1 docker compose -f docker-compose.leader-follower.yml up --build
```

**Leaderless:**

```bash
docker compose -f docker-compose.leaderless.yml up --build
```

**Load test** (example; leader-follower must use leader URL for writes):

```bash
go run ./cmd/loadtest \
  -mode=leader-follower \
  -leader=http://localhost:8080 \
  -endpoints=http://localhost:8080,http://localhost:8081,http://localhost:8082,http://localhost:8083,http://localhost:8084 \
  -write-ratio=0.5 \
  -duration=60s \
  -out=results/run.json \
  -profile=w5r1
```

The client keeps a **key pool** (default 40 keys) and, on most reads, targets keys that were **recently written** so reads and writes are *local in time* — see `-key-pool` and the `hotFrac` logic in `cmd/loadtest/main.go`. It records per-operation latency, stale reads (read `version` lower than the client’s last write version for that key), and time since the last write to the same key.

Graphs (single file or whole directory):

```bash
pip install matplotlib
python3 scripts/plot_loadtest.py results/run.json -o results/figs
./scripts/plot_results_dir.sh results              # all results/*.json → results/figs/
./scripts/plot_results_dir.sh results/aws results/aws/figs   # cloud JSON → results/aws/figs/
```

## AWS (Terraform + EC2 + ECR)

Terraform creates **ECR**, a **security group** (8080–8084 and 18080–18084), an **Elastic IP**, and a **single EC2** host that runs **both** leader–follower and leaderless stacks via Docker (`user-data` in `terraform/user-data.sh.tftpl`).

### Restricted IAM (AWS Academy / Vocareum)

If `terraform apply` fails on `iam:CreateRole`, use the lab’s instance profile instead of creating IAM in Terraform:

```bash
cd terraform
cp terraform.tfvars.example terraform.tfvars   # edit existing_instance_profile if needed
terraform apply -var='manage_iam=false' -var='existing_instance_profile=LabInstanceProfile'
```

Use the **exact** profile name shown when you launch an instance manually in the console (often similar to `LabInstanceProfile`). That role must allow **ECR pull** (`ecr:GetAuthorizationToken` + `BatchGetImage`, etc.).

### Images for `linux/amd64` (required on `t3` / x86 EC2)

Build and push with an **amd64** image (Apple Silicon defaults to `arm64`, which exits immediately on EC2):

```bash
# From repo root (zsh: always use ${REGISTRY}:tag — not $REGISTRY:tag)
REGION=$(terraform -chdir=terraform output -raw aws_region)
REGISTRY=$(terraform -chdir=terraform output -raw ecr_repository_url)
aws ecr get-login-password --region "$REGION" | docker login --username AWS --password-stdin "$REGISTRY"

docker build --platform linux/amd64 -f Dockerfile.leader-follower -t kv-lf .
docker tag kv-lf:latest "${REGISTRY}:leader-follower"
docker push "${REGISTRY}:leader-follower"

docker build --platform linux/amd64 -f Dockerfile.leaderless -t kv-ll .
docker tag kv-ll:latest "${REGISTRY}:leaderless"
docker push "${REGISTRY}:leaderless"
```

Or run `./scripts/aws_push_images.sh` (uses Terraform outputs; tags use `${REGISTRY}`).

After the first push, wait for cloud-init on the instance (or re-`apply -replace=aws_instance.kv` if you pushed **after** the host booted and pulls failed). Health checks:

`http://<EIP>:8080` … `:8084` (LF), `:18080` … `:18084` (leaderless).

### Load tests against the cloud

```bash
export DURATION=60s   # optional
./scripts/aws_loadtest_cloud.sh                      # LF (LF_PROFILE=w5r1 default) + leaderless
./scripts/aws_loadtest_cloud_all_lf.sh               # w5r1 + w1r5 + r3w3 LF, then leaderless (SSM quorum switch)
```

`aws_loadtest_cloud_all_lf.sh` uses **SSM** (`scripts/aws_ec2_set_lf_quorum.sh`) to rewrite `QUORUM_PROFILE` in `/opt/kv/docker-compose.lf.yml` on the instance. If your account cannot use SSM, use `REMOTE_UPDATE=0 PROMPT_BETWEEN_PROFILES=1` and update compose on the host between profiles.

Results land in `results/aws/`. Regenerate histograms with `./scripts/plot_results_dir.sh results/aws results/aws/figs`.

### Tear down

```bash
cd terraform && terraform destroy
```

## Assignment report

Collect JSON from four write ratios × three leader-follower profiles + leaderless, generate histograms (read latency, write latency, read–write interval), and analyze **why** tail latency and staleness differ (sync vs async replication, quorum size, fan-out reads). Add Piazza reflection per course rubric.

Draft bullets tied to the **AWS** run live in `results/aws/WRITEUP_SNIPPETS.txt`; aggregate ops/stale counts in `results/aws/AWS_RUN_SUMMARY.txt`.

Word report (summary + analysis + all AWS histogram PNGs). **Submit a PDF:** open the `.docx` and use *Save As → PDF* (or your PDF printer).

```bash
pip install python-docx
python3 scripts/build_hw10_report_docx.py    # writes CS6650_HW10_Report.docx in repo root
```
