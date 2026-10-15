# Starts a three-process Raft cluster on this machine: HTTP 8080-8082, Raft 7000-7002.
# Stop it with: Get-Process cndsdb | Stop-Process
param([string]$DataDir = ".\data\runtime")
$ErrorActionPreference = "Stop"
New-Item -ItemType Directory -Force bin, $DataDir | Out-Null
go build -o bin\cndsdb.exe .\cmd\server
$peers = "node-1=127.0.0.1:7000=127.0.0.1:8080,node-2=127.0.0.1:7001=127.0.0.1:8081,node-3=127.0.0.1:7002=127.0.0.1:8082"
foreach ($i in 0..2) {
  $id = "node-$($i + 1)"
  Start-Process -FilePath .\bin\cndsdb.exe -WindowStyle Hidden `
    -RedirectStandardError "$DataDir\$id.log" -RedirectStandardOutput "$DataDir\$id.out" `
    -ArgumentList "-id", $id, "-port", (8080 + $i), "-raft-addr", "127.0.0.1:$(7000 + $i)", "-peers", $peers, "-data", $DataDir
}
Write-Host "Started node-1..3. Logs: $DataDir\node-N.log"
