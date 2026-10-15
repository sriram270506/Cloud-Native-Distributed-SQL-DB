#!/usr/bin/env sh
# Starts a three-process Raft cluster on this machine: HTTP 8080-8082, Raft 7000-7002.
# Stop it with: kill $(cat data/runtime/*.pid)
set -eu
data_dir="${1:-./data/runtime}"
mkdir -p bin "$data_dir"
go build -o bin/cndsdb ./cmd/server
peers="node-1=127.0.0.1:7000=127.0.0.1:8080,node-2=127.0.0.1:7001=127.0.0.1:8081,node-3=127.0.0.1:7002=127.0.0.1:8082"
for i in 0 1 2; do
  id="node-$((i + 1))"
  ./bin/cndsdb -id "$id" -port $((8080 + i)) -raft-addr "127.0.0.1:$((7000 + i))" -peers "$peers" -data "$data_dir" \
    >"$data_dir/$id.log" 2>&1 &
  echo $! >"$data_dir/$id.pid"
done
echo "Started node-1..3. Logs: $data_dir/node-N.log"
