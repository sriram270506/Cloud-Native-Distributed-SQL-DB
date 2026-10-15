#!/usr/bin/env sh
set -eu
base_url="${1:-http://localhost:8080}"
curl -fsS "$base_url/health"
printf '\n'
curl -fsS -X POST "$base_url/query" -H 'Content-Type: application/json' -d '{"sql":"CREATE TABLE users (id INTEGER, name TEXT)"}'
printf '\n'
curl -fsS -X POST "$base_url/query" -H 'Content-Type: application/json' -d "{\"sql\":\"INSERT INTO users (id, name) VALUES (1, 'Ada')\"}"
printf '\n'
curl -fsS -X POST "$base_url/query" -H 'Content-Type: application/json' -d '{"sql":"SELECT * FROM users"}'
printf '\n'