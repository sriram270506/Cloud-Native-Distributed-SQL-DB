# Cloud-Native Distributed SQL Database

Runnable educational database MVP with Hashicorp Raft, a fsynced WAL, MVCC transactions, SQL CRUD, snapshots, and an HTTP API.

## Requirements

- Go 1.26 or newer
- PowerShell on Windows, or `curl` on Linux/macOS
- Docker Desktop is optional

## Run locally on Windows

From the repository root:

```powershell
go mod tidy
go test ./...
go run ./cmd/server -port 8080 -nodes 3 -data .\data\runtime
```

The server starts a three-node in-process Raft cluster and listens on `http://localhost:8080`.

In another PowerShell window, run `.\scripts\smoke.ps1`.

## Run with Docker

```powershell
docker compose up --build
```

The service is then available at `http://localhost:8080`. The Compose image runs the same three-node local Raft cluster with its WAL and snapshots stored in a named volume.

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

For a multi-statement transaction, POST `{"statements":["...", "..."]}` to `/transaction`. `/health` reports the elected leader and each node's Raft state.

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

Subqueries, authentication, sharding, and separate-process TCP membership are future work. The current join support is an inner equi-join between two tables, and `HAVING` supports conditions on grouped columns and aggregate aliases.

## Verification

`go test ./...` covers MVCC snapshot isolation/conflicts, SQL CRUD, filtering,
grouping, aggregates, and ordering. The PowerShell smoke script exercises
cluster health, DDL, DML, a multi-statement transaction, and a final read.

The supermarket design is documented in
`docs/relational-schema.md`. The domain ER and UML diagrams are in
`docs/supermarket_er.mmd` and `docs/supermarket_uml.mmd`.
