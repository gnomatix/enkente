<#
.SYNOPSIS
    Forward one chat message to the enkente ingestion API.

.DESCRIPTION
    Posts an Antigravity-shaped message to POST /ingest. The server assigns
    the session and message IDs and returns the stored message, which is
    printed as compact JSON unless -Quiet is given.

    Exit code 0 on success, 1 if the request fails. Failures are written to
    the error stream as a non-terminating error, so a calling script keeps
    running and can inspect $LASTEXITCODE. Requires PowerShell 7 (mise
    installs it on any platform).

.PARAMETER Message
    The message text. Required.

.PARAMETER Type
    Message type; "user" (default) or "system".

.PARAMETER User
    Sender identity. Defaults to USERNAME, then USER, then "anonymous".

.PARAMETER SessionId
    Session (namespace) to file the message under. Defaults to the
    ENKENTE_SESSION_ID environment variable; when empty the server uses
    its default session ("live").

.PARAMETER ApiUrl
    Base URL of the enkente API. Defaults to ENKENTE_API_URL or
    http://localhost:8080.

.EXAMPLE
    ./ag-forward.ps1 "Hello from PowerShell"

.EXAMPLE
    ./ag-forward.ps1 -Message "Design note" -SessionId proj-a -Type system
#>
#Requires -Version 7.0
[CmdletBinding()]
param(
    [Parameter(Mandatory, Position = 0)]
    [string]$Message,

    [ValidateNotNullOrEmpty()]
    [string]$Type = 'user',

    [string]$User = $(if ($env:USERNAME) { $env:USERNAME } elseif ($env:USER) { $env:USER } else { 'anonymous' }),

    [string]$SessionId = $env:ENKENTE_SESSION_ID,

    [string]$ApiUrl = $(if ($env:ENKENTE_API_URL) { $env:ENKENTE_API_URL } else { 'http://localhost:8080' }),

    [switch]$Quiet
)

$ErrorActionPreference = 'Stop'

$body = @{
    type      = $Type
    user      = $User
    message   = $Message
    timestamp = (Get-Date).ToUniversalTime().ToString('o')
}
if ($SessionId) {
    $body.sessionId = $SessionId
}

$uri = "$($ApiUrl.TrimEnd('/'))/ingest"

try {
    $response = Invoke-RestMethod -Method Post -Uri $uri -ContentType 'application/json; charset=utf-8' -Body ($body | ConvertTo-Json -Depth 5)
    if (-not $Quiet) {
        $response.message | ConvertTo-Json -Compress
    }
    exit 0
}
catch {
    # The server's own reason (e.g. {"error":"message is required"}) lives in
    # ErrorDetails; the exception message only carries the status line.
    $reason = $_.Exception.Message
    if ($_.ErrorDetails -and $_.ErrorDetails.Message) {
        $reason = "$reason $($_.ErrorDetails.Message)"
    }
    # -ErrorAction Continue: under $ErrorActionPreference = 'Stop' a bare
    # Write-Error would terminate the script before exit 1 runs.
    Write-Error "enkente: failed to forward message to ${uri}: $reason" -ErrorAction Continue
    exit 1
}
