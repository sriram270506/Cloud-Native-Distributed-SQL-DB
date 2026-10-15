# Cloud-Native Distributed SQL Database

Each database process is one Raft voter. Processes talk Raft over TCP, keep the Raft log on disk, and replicate every write to a majority before acknowledging it. The SQL engine runs MVCC transactions on top of the replicated state machine.

## Runtime flow

1. A client sends SQL to any node over HTTP (`/query`, `/transaction`, `/tables`).
2. If the node is a follower, it forwards the request to the current leader. If no leader is known yet, it waits up to 5 seconds for an election.
3. The leader serializes a write (or a multi-statement transaction) into one Raft log entry.
4. Hashicorp Raft replicates the entry over TCP. It commits once a majority has it durably in its BoltDB log.
5. Every node applies committed entries in log order through the FSM. The FSM runs the statements in the SQL engine, and the engine commits them through MVCC.
6. If any statement in a batch fails, the FSM restores the pre-batch state on every replica. A transaction is all-or-nothing everywhere.
7. Reads go through the leader. The leader first confirms with a quorum that it is still leader (`VerifyLeader`). Then it waits for its FSM to apply everything committed. So reads are linearizable.

## Membership and failure handling

- The cluster has a static voter list (`-peers`). On first start every member bootstraps with the same list. Once Raft state exists on disk, the list is ignored.
- If the leader fails, the remaining majority elects a new leader in about 1 to 2 seconds. Followers that get a request in that window retry forwarding until the new leader is known. They retry only after a connection failure, so a write is never sent twice.
- A restarted node reloads its Raft term, vote and log from BoltDB, restores its latest snapshot, and catches up from the leader.
- On `SIGTERM` the leader hands leadership to another voter before shutting down. This keeps rolling restarts fast.
- `/ready` returns 200 only once a leader is known. Kubernetes and load balancers use it to send traffic only to nodes that can serve it.

## Durability boundary

The Raft log (`raft.db`, BoltDB) and the Raft snapshots are the source of truth. On start, a node clears its FSM state and rebuilds it from the latest snapshot plus the log entries after it. Nothing is applied twice.

The storage WAL is fsynced before each in-memory change. Rollbacks and snapshot restores rewrite it atomically through a temp file and a rename, so it always matches the in-memory state.

## Package ownership

- `raft`: TCP transport, BoltDB log and stable store, snapshots, bootstrap, leader discovery, linearizable read barrier, leadership transfer.
- `storage`: JSON-lines WAL with fsync-before-apply and atomic rewrite, plus the key-value state store.
- `mvcc`: timestamped versions, snapshot reads, first-committer-wins conflicts, rollback, garbage collection.
- `sql`: `CREATE TABLE`, `INSERT`, `SELECT` (WHERE, GROUP BY/HAVING, aggregates, inner join, ORDER BY, LIMIT), `UPDATE`, `DELETE`, `DROP TABLE`.
- `cmd/server`: HTTP API, forwarding to the leader, the transaction batch FSM, graceful shutdown, the container health probe.

## Deployment

- Local: `scripts/cluster.ps1` or `scripts/cluster.sh` start three processes on one machine.
- Docker Compose: `docker-compose.yml` runs one container per node, each with its own volume.
- Kubernetes: `deploy/k8s/cndsdb.yaml` defines a StatefulSet with three replicas and a PVC per pod. It also has a headless service for stable peer DNS, a client service and a PodDisruptionBudget that keeps a majority available.

## Deliberate scope

Not implemented: dynamic membership changes (join or remove at runtime), sharding into multiple Raft groups, authentication and TLS, and subqueries. All data lives in a single Raft group, so write throughput is bounded by one leader.
