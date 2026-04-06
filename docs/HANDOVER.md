# Handover: how this replication homework works

Use this if you are taking over the codebase. Read it alongside `internal/kvsrv/lf.go`, `internal/kvsrv/leaderless.go`, and `internal/kvsrv/peer_client.go`.

## Big picture

- **Five replicas**; peers are listed in `PEERS` in the **same order on every process**. `NODE_INDEX` picks which URL is “this” node.
- **Leader–follower:** only **index 0** accepts client writes (`403` on others). Reads can hit any node; followers forward to the leader’s **cluster-read** so the leader can apply the configured **R** rule.
- **Leaderless:** any node accepts writes; the receiver is the **coordinator** for that write and runs **W=N** (sequential replication to every other peer with simulated delays). Reads are **local only (R=1)** — no quorum read — so **stale reads are expected** under concurrency.

## Simulated delays (`internal/kvsrv/delays.go`)

Replication is slow on purpose: sleeps model network / disk so load tests show quorum and tail latency effects. **Changing these constants changes all graphs.**

## Leader–follower: write path (`LeaderFollowerServer.handleSet` in `lf.go`)

1. **Follower:** immediate `403`.
2. **Leader:** decode JSON; empty key → `400`.
3. **`WriteLocal`** in `store.Store` bumps a per-key monotonic **version** and stores value.
4. Depending on **`QUORUM_PROFILE`:**
   - **`w5r1` (W=5):** Synchronously `replicateToAllFollowers` to every follower; any failure → `500`. Client only sees success after all replicas ack (modulo HTTP errors).
   - **`w1r5` (W=1):** Return **`201` to client after leader commit.** Replication to followers runs in a **background goroutine** (`asyncWg`); failures are **logged**, not surfaced to the original client — classic async replication / inconsistency window.
   - **`r3w3` (W=3):** `replicateToSubset` blocks until **two followers** (plus leader = 3 copies) ack; remaining followers get **async** best-effort replication (same log-and-continue pattern on failure).
5. **`Close()`** on the server waits for async goroutines so tests can shut down cleanly.

**Tricky bit:** For `w1r5`, a successful write response does **not** mean followers are up to date — only that the leader committed. Staleness on `/kv/local_read` on followers is real; **`/kv/get` still goes through cluster-read** so you usually see strong read behavior when R=5 (after replication has propagated).

## Leader–follower: read path (`handleGet` / `runClusterRead`)

- Non-leader: **HTTP forward** to leader `GET /internal/cluster-read`.
- Leader **`runClusterRead`:**
  - **`w5r1` (R=1):** local `Get` only.
  - **`w1r5` (R=5):** parallel `peer-read` to the first five peers (leader uses local read). Each follower adds delay in `handlePeerRead`. Response = entry with **max version** across successful results.
  - **`r3w3` (R=3):** same pattern, first **three** peers only.

**Tricky bit:** If a parallel read fails (network), that peer is skipped; merge uses whatever returned. Production code would retry or treat partial quorum as failure — here it’s simplified.

## Leaderless: write (`handleSet` in `leaderless.go`)

1. Local `WriteLocal`.
2. **Sequential** `Replicate` to each other peer; any error → **`500` to client** (write is not “complete” on all nodes). Between RPCs, leader-like pauses apply.

**Tricky bit:** Coordinator returns failure if **any** replica fails — stricter than “eventual retry” systems. Successful return implies all peers ack’d in this model.

## Leaderless: read

- **`/kv/get`:** local store only — fast, can be **stale** vs keys recently written via another coordinator until replication finishes.

## Internal HTTP

- **`POST /internal/replicate`:** apply a versioned write on a replica (`store.ReplicateApply` semantics — must match version ordering rules in `store`).
- **`GET /internal/peer-read`:** read helper for LF quorum reads.
- **`GET /internal/cluster-read`:** leader-only; LF read quorum logic.

## Errors (what to tell ops)

| Situation | Typical behavior |
|-----------|------------------|
| Empty key on set | `400` |
| Write to follower (LF) | `403` |
| Sync replication failure (W=5 or blocked part of W=3) | `500` on write |
| Async replication failure (W=1 tail, W=3 tail) | logged; client already got `201` |
| Bad JSON | `400` |
| Leaderless replicate to peer fails | `500` |

## Tests (`internal/kvsrv/cluster_test.go`)

- **`TestLeaderFollower_W5R1_ConsistentAfterWrite`** — after leader set, `/kv/get` on leader and follower sees value (strong visibility).
- **`TestLeaderFollower_LocalReadDuringReplication` (W1R5)** — may observe follower lag via `local_read` (timing-dependent).
- **`TestLeaderless_EventualConsistencyWindow`** — reads during replication may miss key on non-coordinators; after completion, peers should converge.
- **`TestFollowerWriteRejected`**, **`TestEmptyKeyRejected`**
- **`TestR3W3_WriteAndRead`** — write + read through follower succeeds after sync quorum + routing.

Run: `go test ./... -race -timeout 120s`

## Load test (`cmd/loadtest`)

- Writes always go to **leader URL** in LF mode; reads spread across endpoints.
- Tracks **stale** when read `version` < client’s last known version for that key (leaderless shows this more often).

If you change quorum rules, **update tests and README** before trusting graphs.
