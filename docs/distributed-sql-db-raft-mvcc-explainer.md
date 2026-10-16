# Cloud-Native Distributed SQL Database: Raft, MVCC, WAL, and SQL Engine Explained

> **Scope note:** this is a conceptual viva Q&A. Where it mentions BadgerDB or an LSM engine it describes the general design space. The implemented system uses a BoltDB Raft log, a JSON-lines WAL and an in-memory MVCC store; see [ARCHITECTURE.md](../ARCHITECTURE.md).

This document explains the design implied by the project: a distributed SQL database built around Raft consensus, a replicated log, a durable embedded KV store, MVCC-based snapshot reads, and a SQL layer on top of an LSM-backed storage engine. The project appears to be an educational/implementation-oriented design rather than a finished production system, but the architecture is realistic and maps directly to the questions below.

## High-level architecture

At a high level, the system is organized as a cluster of nodes, each running the same database logic. The important split is:

- Raft layer: decides which node is leader and replicates writes in order.
- WAL layer: persists not-yet-committed and committed data durably before acknowledgment.
- Storage engine: stores state in an embedded key-value database such as BadgerDB.
- MVCC layer: keeps multiple versions of a row instead of overwriting in place.
- SQL layer: parses, plans, and executes SQL statements against MVCC keys.
- Transaction layer: ensures atomic multi-statement behavior and consistent reads.

The central idea is: the database state is a deterministic state machine replicated through Raft, while MVCC provides logical versioning for SQL semantics.

---

## RAFT CONSENSUS LAYER (Questions 1–6)

### 1) What exactly does the library handle for you, and what do YOU still have to implement? Where is the boundary?

The `hashicorp/raft` library handles the distributed consensus protocol itself:

- leader election
- term tracking
- log replication
- heartbeats
- append entries RPCs
- follower lag and catch-up
- log compaction / snapshots
- majority-based commit decisions
- timeout and retry logic for RPCs
- handling of leader changes and stale leadership

What you still implement is your application-specific state machine and storage semantics:

- the actual database state and mutation logic
- how a log entry is decoded into a SQL/MVCC operation
- the WAL and recovery path
- the on-disk storage layout
- how snapshots are materialized
- application-level transaction behavior
- how to map key-value writes to Raft commands
- client connection handling and request routing

The boundary is: Raft guarantees consensus on the order of commands, but it does not know what those commands mean. Your code must define the command format and apply it deterministically to the local state machine on every node.

In other words, Raft is the replication engine; your database is the state machine built on top of it.

### 2) Leader election step-by-step: Node-1 is leader, you kill -9 it. What RPCs fly between the remaining nodes? What is in each RequestVote message? How does the new leader get chosen?

When Node-1 dies, the remaining nodes stop receiving heartbeats. The election timeout on each follower eventually fires.

The flow is:

1. Node-2 and Node-3 notice no heartbeat from Node-1.
2. Their election timers expire independently.
3. Each becomes a candidate.
4. Candidate sends `RequestVote` RPCs to all peers.
5. Each follower compares candidate metadata and decides whether it can vote.
6. A follower grants its vote only if:
   - the candidate's term is newer than or equal to the follower's current term
   - the candidate's log is at least as up to date as the follower's log
   - the follower has not already voted in this term
7. The candidate collects a majority of votes.
8. The node with the majority becomes leader and starts sending heartbeats.

A typical `RequestVote` contains:

- candidate term
- candidate ID
- last log index
- last log term

This is enough for the follower to decide whether the candidate is eligible and whether it is newer.

The selection of the new leader is not based on random priority; it is based on who first acquires a majority of votes in a valid term. Once that majority is reached, the node becomes leader and can begin issuing AppendEntries heartbeats. If two candidates split votes, the election times out and a new round begins with a higher term.

### 3) What is the difference between the Raft log and your own WAL? Why do you need both? Can't you just use the Raft log as the WAL?

The Raft log is the replicated, consensus-ordered sequence of commands. It is the cluster's source of truth for state changes.

Your WAL is the local durability log on each node. It is used to recover a node's state after a crash before the in-memory state is replayed or before the local KV store is opened.

Why both are needed:

- The Raft log is replicated across nodes and is about consensus ordering.
- The WAL is local durability and crash recovery. It is usually written before the state machine applies the command.
- You may need a WAL even if the Raft log exists because the Raft log is not always directly persisted in the exact shape needed for local recovery or for efficient recovery.
- Raft stores log entries as protocol instructions; your WAL may store more complete physical records, such as page- or record-level writes, or it may store exact data before BadgerDB flushes.

You should not treat the Raft log as a complete WAL replacement because the Raft log is not necessarily the same as the storage engine's crash-safe unit. A node may crash between Raft commit and local application, so you need a recovery path that replays durable entries to reconstruct state.

### 4) What happens if a follower is partitioned from the leader for 30 seconds, then rejoins? How does it catch up? What if the leader has already snapshotted past the follower's last log index?

If a follower is partitioned, it continues to receive no AppendEntries. Its log falls behind. When the network heals:

1. the follower receives heartbeats from the leader again
2. the leader notices the follower's next index is behind
3. the leader sends the missing entries starting at the follower's next index
4. the follower appends them and continues

If the follower is behind by more than the leader's retained log, the leader sends a snapshot instead:

- snapshot contains the database state at some index
- follower installs the snapshot
- follower sets its last applied state to the snapshot index
- then it catches up from the remaining log entries after that index

If the leader has compacted the log past the follower's last log index, the leader cannot send old entries, so snapshot transfer is essential. This is a standard Raft log compaction mechanism.

### 5) Why does Raft require a majority (not all nodes) to commit an entry? What does this imply for fault tolerance with 3 nodes vs. 4 nodes vs. 5 nodes?

Raft requires a majority because the system is designed to continue operating when some nodes are down or partitioned. If all nodes had to agree, the system would be unavailable whenever any node failed.

A cluster with N nodes can tolerate up to floor((N-1)/2) failures while still committing.

- 3 nodes: majority is 2; tolerates 1 failure
- 4 nodes: majority is 3; tolerates 1 failure
- 5 nodes: majority is 3; tolerates 2 failures

The trade-off is availability vs. cost. More nodes increase fault tolerance but also add replication cost, network overhead, and write latency.

### 6) What is split-brain, and how do Raft's term numbers prevent it? Could split-brain still happen in your system under any circumstance?

Split-brain occurs when two different nodes believe they are leader and both accept writes. This would create divergent histories and violate consistency.

Raft prevents split-brain through terms:

- every election increments the term
- only one candidate can win a term
- followers only vote once per term
- stale leaders step down when they hear higher terms
- clients and peers reject writes from stale leaders

Could split-brain still happen in practice?

- under a network partition, two leaders could temporarily think they are the leader if the partition disconnects them badly enough and timeouts fire independently
- but Raft rules ensure only one can win a valid term and that a higher-term leader supersedes the other
- in real systems, split-brain is prevented by protocol rules, not by luck

A practical hazard is not a pure protocol split-brain but a stale leader bug or a software/operational bug that ignores term checks. The proper guardrail is strict term validation on every write path.

---

## STORAGE ENGINE + WAL (Questions 7–10)

### 7) You chose BadgerDB as the embedded KV store. Why BadgerDB over BoltDB or RocksDB? What does BadgerDB's LSM-tree give you that a B-tree doesn't?

BadgerDB is attractive for an embedded, local MVCC-style database because it is designed for efficient append-heavy writes and key-value workloads with compact storage.

Why BadgerDB over BoltDB:

- BoltDB is a B-tree-based embedded database with single-writer locking semantics and simpler design
- BoltDB is excellent for simplicity but less suited to heavy write amplification and range-heavy compaction patterns
- BadgerDB is designed for high write throughput and better background compaction

Why BadgerDB over RocksDB:

- RocksDB is also LSM-based and fast, but it is more C++/native-heavy and often more operationally complex for embedding
- BadgerDB is a Go-native embedded engine, easier to integrate with a Go project and easier to reason about in a transactional system

BadgerDB uses an LSM-tree, which means:

- writes are appended to an in-memory memtable and later flushed to immutable SSTables
- reads are served by checking the memtable and then old SSTables
- compaction merges overlapping keys and cleans stale versions

A B-tree keeps data sorted in-place, which is great for point lookups but bad under write-heavy workloads because every update may touch a page and create write amplification. LSM-trees optimize for append-heavy writes and large sequential flushes.

### 8) Explain your WAL durability guarantee precisely: When does fsync happen relative to (a) client receiving an ack, (b) Raft committing the entry, (c) BadgerDB writing the SSTable?

The normal durability ordering is:

1. client sends write to leader
2. leader appends to local raft log
3. leader writes to its WAL (or appends to durable log)
4. leader calls fsync on that WAL record
5. leader replicates to followers via Raft
6. leader waits for majority acknowledgement
7. leader marks the entry committed in Raft
8. leader sends success to client
9. the local state machine or storage engine applies the mutation
10. BadgerDB eventually writes the SSTable/flush and compacts later

The key guarantee is: a client receives success only after the write is durable and committed by Raft. In a strict implementation, the `fsync` of the WAL happens before returning an acknowledgment.

This means:

- the ack arrives after the WAL is durable but before or around the time the BadgerDB SSTable flush happens
- Raft commit is usually a step that happens after a majority confirms, and the client ack is tied to that committed state, not just local persistence
- BadgerDB flushes are not the point at which the system considers the transaction durable; the WAL and Raft logs are the durability barriers

### 9) A node crashes mid-write. On restart, what is the exact sequence of recovery steps? How do you ensure no committed entry is lost and no uncommitted entry is applied?

On restart, the local node must recover deterministically:

1. read the WAL and identify the last valid record
2. replay entries in log order
3. detect whether the WAL was torn or partially written
4. if the last record is incomplete, truncate it
5. reconstruct any in-memory raft state from persisted metadata
6. replay unapplied committed entries to the state machine
7. open BadgerDB and load the latest SSTables
8. reconcile the MVCC state against the last applied index

To ensure no committed entry is lost:

- the entry must be durably written to the local WAL before being considered committed
- a node should not discard a committed entry even if it has not yet applied it to the state machine
- on recovery, the node replays all committed entries from the durable log

To ensure no uncommitted entry is applied:

- only entries marked as committed by Raft are applied to the state machine
- if a crash happens before majority commit, the log is discarded or ignored during recovery
- the local database is rebuilt from the durable log and consistent snapshot state, never from a partially committed write

This is the classic state-machine recovery pattern: persist before apply, then recover by replaying committed entries.

### 10) Why do you need Raft snapshotting? What happens to log entries after a snapshot is taken? How do you ensure a slow follower can still catch up if it has been down for hours?

Raft log growth is unbounded unless the system periodically compacts history. Snapshotting is necessary because:

- the log would otherwise keep growing forever
- slow or offline followers would have to replay enormous amounts of history
- storage cost and recovery time become unacceptable

When a snapshot is taken:

- the leader writes the current state machine at a specific index
- the snapshot is stored with last-applied and last-index metadata
- old log entries before the snapshot index are discarded or truncated locally
- followers can later catch up using snapshot transfer if they are behind

To ensure a slow follower can still catch up:

- the leader keeps a snapshot available for the relevant index range
- the follower requests missing data if it is behind the snapshot bound
- the leader sends the entire snapshot, then any newer log entries after the snapshot index
- the follower installs the snapshot and resumes replication

This keeps the system resilient even when a node has been offline for hours.

---

## MVCC LAYER (Questions 11–14)

### 11) What exactly does snapshot isolation guarantee? What anomalies does it allow that serializable isolation does not? Give a concrete example of write skew in your system.

Snapshot isolation guarantees that each transaction sees a consistent snapshot of the database as of a single timestamp. A transaction does not see concurrent writes from other transactions that started later.

It guarantees:

- no dirty reads
- no non-repeatable reads within a single transaction snapshot
- a transaction sees a stable view across the duration of the transaction

However, snapshot isolation does not guarantee serializable execution. It allows certain anomalies, especially write skew.

Example in a database with payroll rules:

- transaction A checks that department `Engineering` has at least 2 managers and tries to delete one manager
- transaction B concurrently checks the same condition and tries to delete a different manager
- both see the same snapshot and both decide the condition is valid
- both commit, resulting in a state that violates the application invariant even though each transaction was individually valid

This is write skew: two concurrent transactions read overlapping data and update disjoint rows, creating an invariant violation that the system never detected.

### 12) A transaction starts at timestamp TS=100. Another transaction commits a write to the same key at TS=150. Your read transaction still sees the old value. Where is that old value stored, and how does the system know which version to return?

The old value is stored in the MVCC version chain for that key. Each version is tagged with a commit timestamp or version number.

Conceptually:

- key = `employee:42`
- versions = `(TS=90, value='Alice'), (TS=150, value='Bob')`

When a read transaction starts at `TS=100`, it is assigned a read timestamp and can only see versions with `commit_ts <= 100`.

The system stores versions in a sorted encoding such as:

- key + commit version + tombstone marker

When the read asks for key `employee:42` at `TS=100`, it scans the version chain and selects the newest version whose timestamp is less than or equal to 100. That is the visible version.

This is the core of MVCC: each write creates a new version, and each read picks the most recent visible version before its snapshot timestamp.

### 13) Two transactions write the same key concurrently. Both have passed Raft consensus and are committed in the log. How does your first-committer-wins policy decide which one wins? What happens to the loser?

The first-committer-wins rule is implemented by comparing commit timestamps and/or commit order:

- both writes are replicated and committed in the Raft log
- the system orders them by the commit sequence assigned at the leader
- the earliest committed version wins as the visible version for future reads
- the later write is treated as a loser even if it was applied after the fact

If two transactions conflict on the same key, the database can detect the conflict at commit time:

- transaction A writes key K and commits at `TS=200`
- transaction B writes key K and commits at `TS=220`
- at the MVCC layer, the last version is `TS=220`, but the application rule says the first committed write wins for the user-visible semantics
- transaction B is marked as aborted or rolled back and the client sees a conflict response

Alternatively, the system may accept both writes internally but only keep one as the visible version while preserving the loser as historical garbage-collectable data. The exact form depends on whether the system is using optimistic conflict detection or write-version ordering.

### 14) Why does storage grow unbounded if you keep every version forever? How does your garbage collector decide a version is safe to delete? What if a long-running read transaction holds the oldest timestamp for an hour?

If every old version is kept forever, storage grows without bound because each update creates a new value and the old value remains in the version chain.

A garbage collector removes versions once they are no longer visible to any active or future transaction. The general rule is:

- keep the newest version and any versions that are still visible to active snapshots
- delete older versions whose commit timestamp is older than the minimum active read timestamp

If a long-running read transaction holds the oldest timestamp for an hour, then versions older than that timestamp must remain until the transaction completes. Otherwise, the system would violate snapshot consistency.

This means the GC uses the minimum active snapshot timestamp as a safety fence. Versions older than that fence are safe to delete only once no active transaction can still need them. Long-running transactions effectively protect historical versions from collection.

---

## SQL ENGINE (Questions 15–17)

### 15) You use `vitess/sqlparser` for parsing. Draw the full path of a SELECT * FROM employees WHERE dept = 'Engineering' from the moment it hits your system to the moment results return.

The path is:

1. client sends SQL over the wire
2. API layer receives the query and validates connection/session info
3. `vitess/sqlparser` parses the SQL into an AST
4. the planner builds a logical plan, e.g. a scan over `employees` with a predicate `dept = 'Engineering'`
5. the optimizer rewrites or orders the plan for cost efficiency
6. the executor resolves table metadata and indexes
7. the engine builds key-range lookups against the storage layer
8. MVCC reads are issued with a snapshot timestamp
9. the storage engine scans rows matching the predicate
10. rows are reconstructed from key-value representation
11. results are serialized back to the client
12. client sees rows returned in the query result set

The parser only handles syntax; the real semantics live in planning, MVCC visibility, and storage access. The SQL engine sees the AST and transforms it into a sequence of key-value reads and row materialization steps.

### 16) Your storage is a key-value store (BadgerDB). How do you map SQL tables, rows, and columns into KV keys and values? What is the exact byte encoding of a row key?

A common mapping is:

- table metadata keyed by table ID or name
- row keys use a composite encoding
- columns and indexes are separate prefixes
- values store row payload or column cells

For example:

- table prefix: `t/<table_id>/`
- row id: `row_id` as a fixed-length integer or big-endian encoding
- actual row key: `t/<table_id>/<row_id>`

The exact row key may be encoded as:

- `table_id` as uint64 big-endian
- `row_id` as uint64 big-endian
- combined: `0x01 || uint64_be(table_id) || uint64_be(row_id)`

If you also support MVCC versioning, the row key carries a timestamp:

- `table_id || row_id || commit_ts`

An example representation is:

- prefix: `row/`
- row key: `row/<table_id>/<row_id>/<ts>`

The value stores the serialized row payload or individual column values. This layout makes both point lookups and range scans efficient because all rows for a table share the same prefix.

### 17) You scoped OUT joins, subqueries, and aggregates. If you had to implement a distributed join between employees and departments, what would be the challenge? How would data need to move between nodes?

A distributed join is hard because data is spread across nodes and different partitions may need to be combined.

Challenges:

- partition skew
- network latency
- rows from `employees` and `departments` may live on different nodes
- matching keys might not be co-located
- join order matters for performance

The join flow would be:

1. choose a join strategy (hash join, broadcast join, or merge join)
2. if one side is smaller, broadcast it to all relevant nodes
3. each node hashes the join key and matches local rows
4. rows are exchanged over the network when partitions differ
5. results are merged and returned

For a hash join, the system may:

- partition both tables by `dept_id`
- send rows with the same hash key to the same node
- join locally

This is why distributed joins are expensive: you move data, you coordinate partitions, and you may create hot spots if the join key is skewed.

---

## DISTRIBUTED TRANSACTIONS (Questions 18–20)

### 18) A client sends BEGIN; INSERT ...; UPDATE ...; COMMIT;. Which node coordinates this? How do you ensure all three statements are atomic — they all commit or all abort?

The Raft leader for the relevant partition or database shard coordinates the transaction. In a single-shard design, it is the leader of the shard containing the target key space. In a multi-shard design, a separate transaction coordinator may issue distributed commit messages.

Atomicity is achieved with a transaction log / prepare commit protocol:

1. client starts transaction
2. leader assigns a transaction ID and snapshot timestamp
3. all writes are staged in a transaction buffer
4. leader writes them to a transactional WAL or transaction log
5. on commit, the leader ensures the transaction is durable
6. it replicates the commit record via Raft
7. all changes are applied together or aborted together

In a single-node or single-shard design, the commit is a single Raft decision. In a distributed design, the coordinator uses two-phase commit or a similar protocol, with explicit prepare and commit states. The key rule is: nothing becomes visible until the transaction is committed as a unit.

### 19) The leader dies between statement 2 and statement 3 of a multi-statement transaction. The new leader is elected. What happens to that in-flight transaction? How do you prevent partial commit?

If the leader dies before the transaction commits, the transaction is effectively in-flight and should be considered uncommitted. The new leader must recover the transaction state from the wal or transaction metadata.

The prevention mechanism is:

- writes are not visible before commit
- transaction state is tracked in a durable transaction table or log
- if the cohort sees a transaction with status `prepared` but no `commit` record, it aborts it on recovery
- the new leader replays the transaction log and resolves the state machine consistently

This ensures partial commit does not leak into the visible state. A transaction is either committed as a unit or aborted as a unit; there is no mid-transaction partially visible state.

### 20) Why do you route all writes through the Raft leader? What would go wrong if a follower accepted a write directly?

Writes are routed through the Raft leader to preserve one total order of operations. If a follower accepted writes directly:

- there would be no global ordering across replicas
- the follower could apply a write that the rest of the cluster never accepted
- concurrent writes to the same key could diverge
- a leader would later have to reconcile divergent state with no authoritative linearization order
- consistency would be compromised and Raft's safety properties would be violated

The leader is the sole serializer of mutations; followers only apply writes that were already replicated in the Raft log. This is what makes state machine replication deterministic and safe.

---

## FAULT TOLERANCE & DEPLOYMENT (Questions 21–23)

### 21) You dockerize the cluster. A container is killed and restarted on a different physical host with a new IP. How does it rejoin the cluster? What information must persist on disk across restarts?

When a container restarts on a new host, it must recover its identity and state. The process is:

1. node boots with persisted configuration and cluster information
2. it reads its local metadata and identity from disk
3. it opens its local WAL and BadgerDB state
4. it reconnects to peers via the cluster membership config
5. it uses its node ID and persisted server certificate / token metadata to rejoin
6. the leader or peers re-establish communication and Raft state
7. it catches up by replaying its WAL and requesting missing entries or snapshots

Persist on disk:

- Raft metadata: current term, voted-for, log index, snapshot metadata
- local log or WAL
- BadgerDB files and SSTables
- membership configuration
- node identity / persistent network address or bootstrap configuration
- security material (certs, tokens) if required

Without persistent local state, the node would lose its history and cannot rejoin safely.

### 22) Your chaos script kills the leader mid-transaction. What is the exact sequence of events from docker kill to the cluster being fully healthy again? How long does each step take in your benchmarks?

The sequence is:

1. `docker kill` stops the leader container abruptly
2. followers stop receiving heartbeats after the election timeout
3. each follower starts an election after a randomized timeout
4. one follower wins a majority and becomes leader
5. the new leader sends AppendEntries heartbeats and re-establishes authority
6. the old leader is considered down; any in-flight transaction is either recovered or aborted
7. clients retry writes to the new leader if needed
8. the cluster becomes healthy when a leader is elected and the system is accepting writes again

The time is roughly:

- detection: election timeout, often 150ms-1s
- leader election: a few hundred milliseconds depending on network and timeouts
- log catch-up: depends on follower lag and snapshot size
- recovery to healthy: usually within 1-3 seconds in a small cluster if the node is not heavily behind

The exact numbers depend on election timeout, RPC latency, cluster size, and chunk of log replay/snapshot transfer.

### 23) You test with 3 nodes. With 3 nodes, the system tolerates 1 failure. With 5 nodes, it tolerates 2 failures. So why not always use 5 nodes? What is the trade-off?

Five nodes give more fault tolerance, but cost more:

- more network traffic for replication
- higher latency for each write because a leader waits for a larger majority
- more storage overhead for copies of the same log
- more coordination overhead during elections
- more operational complexity and replacement cost

Three-node clusters are cheaper and simpler; they can tolerate one failure with no loss, which is often a good production choice for small systems. Five-node clusters are chosen when fault tolerance or availability requirements justify the extra coordination and cost.

The general rule is: choose the smallest cluster size that meets your fault-tolerance and latency targets.

---

## BENCHMARKING & DESIGN DECISIONS (Questions 24–25)

### 24) Your throughput-vs-concurrency plot shows throughput plateauing at 8 concurrent clients. Is the bottleneck (a) the Raft commit path, (b) the BadgerDB write path, (c) your WAL fsync, or (d) gRPC serialization? How would you prove which one it is?

The plateau at 8 concurrency is usually a sign that one specific resource hit saturation. The most likely bottleneck in this design is the serialized durability path: WAL fsync and Raft commit coordination.

To prove it, you measure each stage:

- add timing around `raft.Apply` and `AppendEntries` send latency
- time BadgerDB writes and flushes separately
- measure WAL fsync latency under load
- compare with gRPC serialization time using a local in-memory benchmark that bypasses the network

If fsync dominates, throughput stops growing once the disk or storage controller is saturated. If Raft network latency dominates, replication time rises with follower count. If gRPC serialization dominates, the CPU will be highly loaded while the disk stays idle. If BadgerDB flush dominates, the storage engine will be the limiting factor.

A proper proof is to benchmark each layer separately and observe which one saturates first under the same concurrency levels.

### 25) CockroachDB uses etcd's Raft library and Pebble (RocksDB derivative). TiDB uses TiKV (Raft + RocksDB). You chose hashicorp/raft + BadgerDB. What is the academic cost of using libraries vs. implementing from scratch? What do you gain? What would you lose if you had to implement Raft yourself?

The academic cost of using a library is that you inherit a mature protocol implementation, but you may also inherit constraints and hidden complexity. You may also lose the ability to deeply customize or formally reason about every detail of the implementation.

What you gain:

- correctness from a well-tested consensus algorithm
- faster development
- less engineering risk
- easier debugging because the protocol is standardized

What you lose if you implement Raft from scratch:

- the custom implementation may be more specialized to your system
- you may gain more precise control over serialization, snapshotting, and state machine behavior
- but you also carry a much larger correctness burden

This is the standard trade-off in systems engineering: use the library to get correct consensus quickly, then focus your scarce engineering time on unique database semantics, indexing, and transactional behavior.

---

## DISTRIBUTED SYSTEMS FUNDAMENTALS (Questions 1–5)

### 1) The CAP theorem says you can only guarantee two of Consistency, Availability, Partition tolerance. Your distributed database claims to be CP (consistent and partition-tolerant). What exactly does "unavailable" mean in this context? Can a client still read data during a network partition?

In a CP system, the system prefers correctness over availability when a partition occurs. "Unavailable" means the system may refuse to process writes or reads that would violate consistency.

During a network partition:

- if a node cannot establish a majority for the relevant Raft quorum, it should stop accepting writes
- reads may also be blocked if the node cannot verify the latest committed state
- a client may be forced to wait, receive an error, or fail over to another reachable node

A CP database does not guarantee read availability during a partition; it guarantees the client will not observe a stale or divergent state. The price is reduced availability.

### 2) Why is consensus fundamentally harder in an asynchronous system than in a synchronous one? What does the FLP impossibility result actually tell us?

In a synchronous system, there are bounded delays and known timeouts. In an asynchronous system, messages may be delayed arbitrarily and nodes may fail or restart unpredictably.

Consensus is harder in asynchronous settings because there is no absolute way to tell whether a node is slow, partitioned, or dead. You cannot reliably distinguish delay from failure.

The FLP impossibility result says that in an asynchronous system with even one failure, no deterministic consensus protocol can guarantee both safety and liveness forever. In practice, real systems use timeouts and probabilistic assumptions to approximate leader election and guarantee liveness under network timing assumptions, while keeping safety strict.

### 3) What is the difference between safety and liveness in distributed systems? Give one example of each from the Raft protocol.

Safety means nothing bad ever happens: nodes never agree on conflicting committed states, no split-brain, no lost committed writes.

Example safety property in Raft: a committed entry is never removed from the log of a correct majority node, and no two leaders can commit conflicting entries for the same index in different terms.

Liveness means the system eventually makes progress: elections complete, a leader is elected, writes are accepted.

Example liveness property in Raft: if the network eventually heals and a majority can communicate, the system eventually elects a leader and replicates new entries.

### 4) A system uses synchronous replication to all nodes before acknowledging a write. It never loses data. Why is this still not a good idea for a production distributed database?

Because it sacrifices latency and availability. Every write waits for all replicas, which can be slow and can fail if a single node is slow or down.

In production, the system must tolerate some node failures without making the whole database unavailable. Waiting for all nodes means one slow node can stall the cluster. This is not acceptable for latency-sensitive systems.

### 5) What is the two generals problem? Why does TCP solve it for point-to-point communication but not for distributed consensus?

The two generals problem illustrates that two parties cannot perfectly coordinate via unreliable communication: no finite message exchange guarantees that they both agree on a plan.

TCP solves the point-to-point reliability problem well because it provides ordered, reliable connectivity between exactly two endpoints. However, consensus is not just reliable messaging; it requires agreement among multiple parties in the presence of failures and uncertain delays.

TCP does not solve distributed consensus because a consensus algorithm must still handle crashes, re-elections, and asynchrony among multiple nodes, not just packet loss between two endpoints.

---

## CONSENSUS ALGORITHMS (Questions 6–10)

### 6) Paxos and Raft both solve the same problem. Why did Raft become more popular in industry despite Paxos being published earlier? What did Raft sacrifice for simplicity?

Raft became popular because it is easier to explain and implement correctly. It organizes the protocol around a clear leader-follower model, term numbers, and log replication. This is easier for engineers to reason about than Paxos's lower-level quorum coordination.

Raft sacrificed some theoretical abstraction and flexibility. Paxos is more general in some ways, but Raft chooses a simpler operational model and easier implementation at the cost of some elegance and generality.

### 7) In Raft, why must a candidate's log be at least as up-to-date as a follower's log to receive its vote? What would go wrong if this rule were removed?

This rule prevents a candidate with stale history from becoming leader and overwriting a more recent but uncommitted state.

Without it, a follower could vote for a candidate whose log is behind and later that candidate could lead the cluster into an older state, causing divergence and violating the safety property.

### 8) What is log compaction, and why is it necessary in any replicated state machine? What are the trade-offs between snapshotting and log cleaning?

Log compaction is the process of removing old log entries that are no longer needed because the state machine has already incorporated them.

It is necessary because the log would otherwise grow without limit. Snapshotting keeps a compact state image plus a small log tail. Log cleaning or compaction reduces historical storage while keeping recovery possible. The trade-off is that snapshots and cleanups add complexity and can increase recovery time or require careful coordination with lagging followers.

### 9) Can Raft handle Byzantine failures? If not, what class of consensus algorithms does? Why don't production databases typically use Byzantine fault tolerance?

Raft is a crash-fault-tolerant protocol, not a Byzantine-fault-tolerant one. It assumes nodes may fail but do not intentionally lie or collude.

Byzantine fault tolerance (BFT) algorithms handle malicious or arbitrary behavior. Examples include PBFT and variants. They are used in some blockchain and highly adversarial settings, but they are more expensive, more complex, and slower than Raft. Production databases usually prefer Raft-like crash fault tolerance because the threat model is simpler and cost matters more than adversarial fault tolerance.

### 10) What happens to committed writes if a majority of nodes are permanently lost? Is the data recoverable? What does this tell us about the difference between durability and availability?

If a majority of nodes are permanently lost, the system may no longer be able to form a quorum. Committed writes are safe only so long as the cluster can still form a majority and recover. Once a majority is lost, the system cannot guarantee the ability to continue or recover safely without manual intervention or off-cluster recovery.

This is the difference between durability and availability: durable data can still exist on disk, but the system may be unavailable. Durability means the data has been safely recorded; availability means the cluster can still serve requests.

---

## TRANSACTION THEORY (Questions 11–15)

### 11) The ACID properties are Atomicity, Consistency, Isolation, Durability. Which of these is actually a property of the transaction mechanism, and which is a property of the application/database as a whole? Why does this distinction matter?

Atomicity, isolation, and durability are transaction mechanism properties. Consistency is more of a database/application invariant property.

This matters because a database can provide atomic transactions and durable writes without guaranteeing that the application-level invariants are correct. For example, a database may commit each transaction atomically but your application may incorrectly allow a negative balance if it does not check the invariant in code.

### 12) Two-phase locking (2PL) guarantees serializability. MVCC with snapshot isolation does not. Why did the database industry largely move from 2PL to MVCC despite the weaker guarantee?

The industry moved to MVCC because it provides better read scalability and lower contention. Readers can operate on consistent snapshots without blocking writers, and writes can proceed concurrently without the lock convoy behavior that tends to hurt throughput in 2PL systems.

2PL is simpler in some theoretical models but scales poorly under contention and high read concurrency. MVCC gives much better performance in modern OLTP systems with many concurrent readers and writers.

### 13) What is the phantom read anomaly? Why does snapshot isolation not prevent it? What isolation level does?

A phantom read occurs when a transaction re-executes a query and sees additional rows that were inserted or removed by concurrent transactions. For example, one transaction counts employees in Engineering and later sees a new row appear in the same query.

Snapshot isolation does not necessarily prevent phantoms because it gives a consistent snapshot, not a serializable schedule. It prevents stale reads of existing rows but does not detect conflicts across range predicates.

Serializable isolation or predicate locking is needed to prevent phantoms.

### 14) In a distributed database, a transaction touches rows on two different nodes. Node A commits successfully, but the network fails before Node B receives the commit. Is the transaction atomic? What techniques exist to solve this?

If the transaction is viewed as a single unit, then no: the system is in an atomicity failure state unless the coordinator and participants use a durable protocol. If Node A committed but Node B could not, the database is in an inconsistent state unless atomic commit procedures are used.

Techniques include:

- two-phase commit (2PC)
- saga / compensation patterns
- transactional message queues
- optimistic commit protocols with durable intent logs
- consensus-based transaction logs

A distributed transaction is atomic only if it follows a protocol that can recover from partial commit states.

### 15) What is the difference between optimistic and pessimistic concurrency control? Under what workload characteristics does each perform better?

Optimistic concurrency control assumes conflicts are rare. Transactions execute and validate at commit time. It works well when reads are common and conflicts are infrequent.

Pessimistic concurrency control assumes conflicts are likely. It locks data early to prevent others from modifying it. It works better under heavy contention or when correctness is more important than throughput.

In modern distributed OLTP, optimistic schemes such as MVCC are often better because write conflicts are rare relative to read-heavy workloads, but lock-based systems still matter for highly contended workloads.

---

## STORAGE & INDEXING (Questions 16–20)

### 16) Why do write-optimized data structures (LSM-trees) generally outperform read-optimized structures (B-trees) for write-heavy workloads? What do you pay for this advantage?

LSM-trees optimize for writes by appending to memory buffers and flushing to SSTables sequentially. B-trees update the tree in place and therefore require more random I/O and more rewriting of pages.

The cost is complexity: LSM trees require compaction, merge logic, and potentially more read-path complexity, because reads may need to check multiple layers.

### 17) What is write amplification, and why does it matter more in SSDs than in HDDs? How does an LSM-tree's compaction strategy affect write amplification?

Write amplification is the ratio of physical writes performed by the storage engine to the logical writes the application requested.

It matters more on SSDs because SSDs have finite program/erase cycles and wear, and because random writes are much more expensive relative to sequential writes. On HDDs, write amplification is less catastrophic but still hurts throughput.

LSM compaction can both reduce and increase write amplification depending on strategy. Aggressive compaction reduces read overhead but causes more rewriting. Therefore the compaction policy is a performance and storage trade-off.

### 18) A database needs to support both point lookups (exact key match) and range scans (key prefix). Can a single index structure serve both efficiently? What are the trade-offs?

Yes, a well-designed key-value or B-tree-like index can serve both point lookup and range scan efficiently, because sorted keys allow both exact match and ordered traversal. LSM-based key-value stores also handle ranges well by scanning key prefixes.

The trade-off is that range scans can become expensive if the index is optimized only for exact lookups, and point lookups can become slower if the system is arranged only for scans.

### 19) What is the difference between a physical log and a logical log in recovery? Which does ARIES use, and why?

A physical log records changed bytes or pages. A logical log records higher-level operations such as “update row 5 column x to y”.

ARIES uses a physical log with page-level recovery plus logical semantics in the transaction model. It is particularly suitable for recovery because it can restore the precise contents of modified pages with redo/undo logic while maintaining transaction semantics.

### 20) Why is fsync considered expensive? What hardware trends (battery-backed caches, NVMe, Optane) have changed the cost model of durable writes?

`fsync` is expensive because it forces data from volatile memory into stable storage, which incurs I/O latency and stalls the write path. The write is not considered durable until the drive acknowledges it.

Hardware advances such as battery-backed write caches, NVMe, and Optane reduce the latency and cost of writes, but they do not eliminate the need for durable ordering. They change the performance envelope, not the semantics.

---

## QUERY PROCESSING & OPTIMIZATION (Questions 21–25)

### 21) What is the difference between a logical query plan and a physical query plan? Why does the optimizer need both?

A logical plan describes the high-level relational operations, such as scan, filter, join, and project. A physical plan chooses how to execute those operations, such as nested loop join, hash join, or index scan.

The optimizer needs both because the same logical query can be executed in many ways, and the best execution strategy depends on indexes, data distribution, and runtime statistics.

### 22) A query planner estimates that a predicate WHERE salary > 100000 will return 10% of rows. The true selectivity is 0.1%. What are the consequences if the planner uses the wrong estimate? How does this affect join ordering?

This can cause terrible execution plans. The optimizer may choose a full scan instead of using an index, or a nested loop join instead of a hash join. Join ordering becomes skewed because the optimizer may assume one join is selective but in reality it is not.

This leads to extra I/O, poor CPU utilization, and large latency spikes. Cardinality estimation errors are among the most important reasons for poor plan quality.

### 23) What is the volcano iterator model? Why is it the dominant query execution paradigm despite not being the most efficient for analytical workloads?

The volcano iterator model uses a uniform interface of `Open`, `Next`, `Close` methods across relational operators. This makes operator composition simple and extensible.

It remains dominant because it is easy to implement, extensible, and general-purpose. Analytical databases may use vectorized or columnar execution for performance, but the volcano abstraction is still foundational in many systems.

### 24) In a distributed database, data is partitioned across nodes by hash(key). A query does WHERE key BETWEEN 100 AND 200. Why is this query expensive? What partitioning strategy would make it cheaper, and what would it cost?

A hash-partitioned system spreads keys by hash, so a range query spans many partitions. To answer the query, the system must fetch multiple partitions and merge results, which is expensive.

A range-partitioned or ordered-partitioned layout would make the query cheaper because adjacent keys reside together. The trade-off is that it may cause hot spots or uneven distribution if key distribution is skewed.

### 25) Materialized views precompute query results for faster reads. In a distributed database with continuous writes, what is the fundamental challenge in keeping a materialized view consistent with the base tables?

The challenge is maintaining asynchronous or synchronous consistency under continuous write load. Each base-table update may require an update to the materialized view, and if the view lags or is updated eventually, it may temporarily serve stale data.

This is a classic replication and consistency problem: keeping a derived view accurate without introducing major latency or conflict issues in a live distributed system.

---

## Final synthesis

This project is best understood as a layered distributed SQL database design:

- Raft provides a fault-tolerant total order of committed operations.
- The WAL provides durable local crash recovery.
- BadgerDB provides a Go-native embedded KV store with LSM optimization.
- MVCC provides versioned reads and avoids writer lock contention.
- SQL parsing and execution map relational queries onto key-value operations.
- A transaction layer makes multi-statement operations atomic and safe under leader failover.

The overall theme is: consensus gives a single source of truth, MVCC gives isolation semantics, and storage gives durability and performance. That is the core architecture behind modern distributed databases.

This design is realistic for an educational project because it shows the right separation of concerns: consensus, storage, transactions, and SQL are all independent responsibilities, each with its own failure modes and engineering constraints.
