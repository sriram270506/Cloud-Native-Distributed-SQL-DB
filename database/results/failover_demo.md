# Failover demo

Recorded on a local three-process cluster (`scripts/cluster.ps1`) after `database/seed_cluster.py`.

## 1. Healthy cluster

| node | state | leader | term | commit index | applied index |
|---|---|---|---|---|---|
| node-1 | Follower | node-2 | 2 | 8 | 8 |
| node-2 | Leader | node-2 | 2 | 8 | 8 |
| node-3 | Follower | node-2 | 2 | 8 | 8 |

## 2. Leader process node-2 killed (no graceful shutdown)

Write sent to node-3 immediately after the kill: `{"message":"committed"}` (returned after 1747 ms, including the new election).

| node | state | leader | term | commit index | applied index |
|---|---|---|---|---|---|
| node-1 | Follower | node-3 | 3 | 10 | 10 |
| port 8081 | down | | | | |
| node-3 | Leader | node-3 | 3 | 10 | 10 |

## 3. node-2 restarted and caught up from its Raft log

| node | state | leader | term | commit index | applied index |
|---|---|---|---|---|---|
| node-1 | Follower | node-3 | 3 | 10 | 10 |
| node-2 | Follower | node-3 | 3 | 10 | 10 |
| node-3 | Leader | node-3 | 3 | 10 | 10 |

Read through the restarted node:

```json
{"columns":["customer_id","first_name","last_name"],"rows":[{"customer_id":"11","first_name":"Ada","last_name":"Lovelace"}]}
```
