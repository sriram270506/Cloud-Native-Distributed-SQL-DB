# Cloud-Native Distributed SQL Database

A distributed SQL database built for learning. Each process is one Raft voter. Nodes replicate over TCP with Hashicorp Raft and keep a durable BoltDB Raft log. Transactions use MVCC. Any node accepts SQL over HTTP, and followers forward requests to the leader. It runs as local processes, as Docker Compose containers or as a Kubernetes StatefulSet. See [ARCHITECTURE.md](ARCHITECTURE.md).

## Requirements

- Go 1.26 or newer
- PowerShell on Windows, or `curl` on Linux/macOS
- Docker Desktop (optional)
- A Kubernetes cluster (optional)

## Run a three-node cluster locally

From the repository root:

```powershell
go test ./...
.\scripts\cluster.ps1        # Linux/macOS: ./scripts/cluster.sh
.\scripts\smoke.ps1          # Linux/macOS: ./scripts/smoke.sh
```

This starts `node-1`, `node-2` and `node-3` as separate processes. HTTP is on ports 8080 to 8082 and Raft on ports 7000 to 7002. Logs are written to `data/runtime/node-N.log`. To stop the cluster, run `Get-Process cndsdb | Stop-Process` on Windows or `kill $(cat data/runtime/*.pid)` on Linux/macOS.

To test failover, kill the leader's process. `/health` on any node shows the current leader. The other two nodes elect a new leader within a couple of seconds, and writes keep working. A restarted node catches up from its log and the new leader.

For a single-node cluster, run `go run ./cmd/server` with no flags.

### Server flags

| Flag | Meaning |
|------|---------|
| `-id` | Unique Raft server ID, for example `node-1` |
| `-port` | HTTP port |
| `-raft-addr` | Raft TCP bind address |
| `-advertise` | Raft address peers dial to reach this node. Defaults to `-raft-addr` |
| `-peers` | Every voter, this node included, as `id=raft_host:port=http_host:port`, comma-separated |
| `-data` | Data directory. Each node uses `<data>/<id>` |
| `-probe URL` | Run a health check against URL instead of starting the server (used by containers) |

## Run with Docker Compose

```powershell
docker compose up -d --build
.\scripts\smoke.ps1
```

This runs three containers, `node1` to `node3`, each with its own volume. They publish HTTP on ports 8080 to 8082. To test failover, run `docker compose stop node1`.

## Run on Kubernetes

```powershell
docker build -t cndsdb:latest .
kind load docker-image cndsdb:latest   # or push to your registry and edit the image name
kubectl apply -f deploy/k8s/cndsdb.yaml
kubectl rollout status statefulset/cndsdb
kubectl port-forward svc/cndsdb 8080:8080
```

The manifest creates:
- a StatefulSet with three pods (`cndsdb-0` to `cndsdb-2`), each with a 1Gi PersistentVolumeClaim
- a headless service that gives the pods stable DNS names for Raft
- a client service
- a PodDisruptionBudget that keeps two of the three pods running

Pods are marked Ready only once a leader is elected.

## HTTP endpoints

| Endpoint | Meaning |
|----------|---------|
| `POST /query` | Run one SQL statement, as `{"sql": "..."}` |
| `POST /transaction` | Run several statements atomically, as `{"statements": ["...", "..."]}` |
| `GET /tables` | List tables |
| `GET /health` | This node's Raft state, term, commit and applied index, the current leader and the members |
| `GET /ready` | 200 once a leader is known, 503 before that |

## API examples

```powershell
$headers = @{ "Content-Type" = "application/json" }
$body = @{ sql = "CREATE TABLE users (id INTEGER, name TEXT)" } | ConvertTo-Json
Invoke-RestMethod http://localhost:8080/query -Method Post -Headers $headers -Body $body

$body = @{ sql = "INSERT INTO users (id, name) VALUES (1, 'Ada')" } | ConvertTo-Json
Invoke-RestMethod http://localhost:8080/query -Method Post -Headers $headers -Body $body

$body = @{ sql = "SELECT * FROM users" } | ConvertTo-Json
Invoke-RestMethod http://localhost:8080/query -Method Post -Headers $headers -Body $body
```


## Supported SQL

The implemented scope is intentionally explainable and testable:

- `CREATE TABLE users (id INTEGER, name TEXT)`
- `INSERT INTO users (id, name) VALUES (1, 'Ada')`
- `SELECT * FROM users`
- `SELECT id, name FROM users WHERE id = 1 LIMIT 10`
- `SELECT category, COUNT(*) AS products FROM products GROUP BY category HAVING products > 1`
- `SELECT customers.name, orders.total FROM customers JOIN orders ON customers.id = orders.customer_id`
- `SELECT * FROM products WHERE category = 'food' OR stock < 10`
- `UPDATE users SET name = 'Grace' WHERE id = 1`
- `DELETE FROM users WHERE id = 1`
- `DROP TABLE users`

Subqueries, authentication, TLS, sharding into multiple Raft groups and runtime membership changes are future work. The current join support is an inner equi-join between two tables, and `HAVING` supports conditions on grouped columns and aggregate aliases.

## Verification

`go test ./...` covers:
- MVCC snapshot isolation and conflicts
- SQL CRUD, filtering, grouping, aggregates and ordering
- WAL durability across rollback
- a three-node Raft cluster over TCP: replication, leader failover, and a restarted node catching up from its BoltDB log

The smoke scripts wait for a leader, then run DDL, DML, a multi-statement transaction and a final read.

The supermarket design is documented in
`docs/relational-schema.md`. The domain ER and UML diagrams are in
`docs/supermarket_er.mmd` and `docs/supermarket_uml.mmd`.
