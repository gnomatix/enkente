param(
    [string]$Payload
)
# Forward the JSON payload to the enkente ingestion API
$payload = $Payload
Invoke-RestMethod -Method Post -Uri http://localhost:8080/ingest -ContentType "application/json" -Body $payload -ErrorAction SilentlyContinue
