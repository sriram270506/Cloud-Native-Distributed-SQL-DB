param([string]$BaseUrl = "http://localhost:8080")
$headers = @{ "Content-Type" = "application/json" }
# Wait for leader election before sending writes.
$deadline = (Get-Date).AddSeconds(30)
while ($true) {
  try { Invoke-RestMethod "$BaseUrl/ready" | Out-Null; break } catch {
    if ((Get-Date) -gt $deadline) { throw "no Raft leader at $BaseUrl after 30s" }
    Start-Sleep -Milliseconds 250
  }
}
Invoke-RestMethod "$BaseUrl/health" | ConvertTo-Json -Depth 4 | Out-Host
function Invoke-Sql([string]$Sql) {
  Invoke-RestMethod -Uri "$BaseUrl/query" -Method Post -Headers $headers -Body (@{ sql = $Sql } | ConvertTo-Json)
}
Invoke-Sql "CREATE TABLE users (id INTEGER, name TEXT)" | Out-Host
Invoke-Sql "INSERT INTO users (id, name) VALUES (1, 'Ada')" | Out-Host
$transaction = @{ statements = @(
  "INSERT INTO users (id, name) VALUES (2, 'Grace')",
  "UPDATE users SET name = 'Ada Lovelace' WHERE id = 1"
) } | ConvertTo-Json
Invoke-RestMethod -Uri "$BaseUrl/transaction" -Method Post -Headers $headers -Body $transaction | Out-Host
Invoke-Sql "SELECT * FROM users" | ConvertTo-Json -Depth 8
