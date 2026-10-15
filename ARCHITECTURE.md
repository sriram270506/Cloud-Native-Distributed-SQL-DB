# Cloud-Native Distributed SQL Database

This implementation uses Hashicorp Raft as the consensus substrate and keeps the original project work in the storage, WAL, MVCC, SQL execution, and transaction boundaries.

## Runtime flow

1. The HTTP API accepts SQL or a list of SQL statements.
2. Writes are serialized as one Raft command and submitted to the current leader.
3. Hashicorp Raft replicates the command to a majority and applies it to every FSM.
4. Each FSM executes the command through the SQL engine.
5. The SQL engine commits row mutations through MVCC and persists the resulting state through the fsynced WAL.
6. Reads use the durable state on a cluster replica.

The default server starts three Raft nodes in one process using Hashicorp's in-memory transport. This makes the cluster deterministic and easy to run on a student laptop while still exercising leader election, majority commit, FSM application, snapshots, WAL recovery, and follower replication. The transport can be replaced with TCP when separate-process deployment is required.

## Package ownership

- `raft`: cluster creation, leader lookup, command application, FSM snapshots and restore.
- `storage`: JSON-lines WAL with fsync-before-apply and a durable key-value state store.
- `mvcc`: timestamped versions, snapshot reads, first-committer-wins conflicts, rollback, and garbage collection.
- `sql`: bounded SQL surface: `CREATE TABLE`, `INSERT`, `SELECT`, `UPDATE`, and `DELETE`.
- `cmd/server`: HTTP API and transaction batch coordination.

## Durability boundary

The WAL record is written and synced before the in-memory store is changed. Raft success is returned only after the command has been committed and applied by the leader. On restart, the WAL is replayed to reconstruct the durable state. Raft snapshots are supported through the FSM interface.

## Deliberate scope

The SQL parser supports simple single-table statements and equality predicates. Joins, aggregates, subqueries, authentication, sharding, and separate-process TCP membership are future extensions rather than hidden claims of this implementation.