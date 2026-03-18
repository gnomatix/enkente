param(
    [string]$Message
)

$payload = @{
    type = "user"
    user = $env:USERNAME
    message = $Message
    timestamp = (Get-Date).ToString("o")
} | ConvertTo-Json -Depth 5

Invoke-RestMethod -Method Post -Uri http://localhost:8080/ingest -ContentType "application/json" -Body $payload -ErrorAction SilentlyContinue
