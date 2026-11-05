<#
.SYNOPSIS
    Forward a ready-made JSON payload to the enkente ingestion API.

.DESCRIPTION
    Companion to the VS Code chat bridge: posts the given JSON body as-is to
    POST /ingest. Use tools/ag-forward.ps1 to build a payload from a plain
    message instead.

    Exit code 0 on success, 1 if the request fails; failures go to the error
    stream.

.PARAMETER Payload
    A JSON object with at least a "message" field. Optional fields:
    "type", "user", "sessionId", "timestamp" (RFC 3339).

.PARAMETER ApiUrl
    Base URL of the enkente API. Defaults to ENKENTE_API_URL or
    http://localhost:8080.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory, Position = 0)]
    [string]$Payload,

    [string]$ApiUrl = $(if ($env:ENKENTE_API_URL) { $env:ENKENTE_API_URL } else { 'http://localhost:8080' }),

    [switch]$Quiet
)

$ErrorActionPreference = 'Stop'
$uri = "$($ApiUrl.TrimEnd('/'))/ingest"

try {
    $response = Invoke-RestMethod -Method Post -Uri $uri -ContentType 'application/json' -Body $Payload
    if (-not $Quiet) {
        $response.message | ConvertTo-Json -Compress
    }
    exit 0
}
catch {
    Write-Error "enkente: failed to forward payload to ${uri}: $($_.Exception.Message)"
    exit 1
}
