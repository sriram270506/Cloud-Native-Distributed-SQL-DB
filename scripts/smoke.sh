#!/usr/bin/env sh
set -eu
base_url="${1:-http://localhost:8080}"
# Wait for leader election before sending writes.
tries=0
until curl -fsS "$base_url/ready" >/dev/null 2>&1; do
  tries=$((tries + 1))
  [ "$tries" -ge 120 ] && { echo "no Raft leader at $base_url after 30s" >&2; exit 1; }
  sleep 0.25
done
curl -fsS "$base_url/health"
printf '\n'
curl -fsS -X POST "$base_url/query" -H 'Content-Type: application/json' -d '{"sql":"CREATE TABLE users (id INTEGER, name TEXT)"}'
printf '\n'
curl -fsS -X POST "$base_url/query" -H 'Content-Type: application/json' -d "{\"sql\":\"INSERT INTO users (id, name) VALUES (1, 'Ada')\"}"
printf '\n'
curl -fsS -X POST "$base_url/transaction" -H 'Content-Type: application/json' -d "{\"statements\":[\"INSERT INTO users (id, name) VALUES (2, 'Grace')\",\"UPDATE users SET name = 'Ada Lovelace' WHERE id = 1\"]}"
printf '\n'
curl -fsS -X POST "$base_url/query" -H 'Content-Type: application/json' -d '{"sql":"SELECT * FROM users"}'
printf '\n'
