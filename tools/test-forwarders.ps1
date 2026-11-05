<#
.SYNOPSIS
    Integration-test the PowerShell forwarders against a real enkente server.

.DESCRIPTION
    Starts `enkente serve --headless` on a free port with a throw-away
    datastore, runs tools/ag-forward.ps1 and
    vscode-extension/scripts/forward-win.ps1 as child pwsh processes, and
    checks their exit codes, printed JSON and what the server stored
    (via GET /sessions and GET /sessions/{id}/messages).

    No Pester and no network beyond localhost. The server is always stopped
    and the temporary files removed, even when a check fails. Prints one
    line per check, then "PASS n/n" (exit 0) or "FAIL k/n" with the failed
    checks listed (exit 1).

    Works on Windows and Linux/macOS under PowerShell 7.

.PARAMETER Binary
    Path to the enkente executable. Defaults to dist/enkente (dist/enkente.exe
    on Windows) under the repository root, i.e. what `make build` produces.

.PARAMETER Port
    TCP port for the test server. Defaults to a free port chosen by binding
    a TcpListener to port 0.

.PARAMETER StartupTimeoutSec
    How long to wait for GET /health to answer after starting the server.

.EXAMPLE
    make build
    pwsh -NoProfile -NonInteractive -File tools/test-forwarders.ps1

.EXAMPLE
    ./tools/test-forwarders.ps1 -Binary ./dist/enkente.exe -Port 18080
#>
#Requires -Version 7.0
[CmdletBinding()]
param(
    [string]$Binary,

    [ValidateRange(0, 65535)]
    [int]$Port = 0,

    [ValidateRange(1, 300)]
    [int]$StartupTimeoutSec = 10
)

$ErrorActionPreference = 'Stop'

# ---------------------------------------------------------------------------
# Paths and platform
# ---------------------------------------------------------------------------

$repoRoot = Split-Path -Parent $PSScriptRoot
$onWindows = [System.Runtime.InteropServices.RuntimeInformation]::IsOSPlatform(
    [System.Runtime.InteropServices.OSPlatform]::Windows)
$exeSuffix = if ($onWindows) { '.exe' } else { '' }

if (-not $Binary) {
    $Binary = Join-Path (Join-Path $repoRoot 'dist') "enkente$exeSuffix"
}
if (-not (Test-Path -LiteralPath $Binary -PathType Leaf)) {
    Write-Error "enkente binary not found at '$Binary'. Run 'make build' or pass -Binary."
    exit 1
}
$Binary = (Resolve-Path -LiteralPath $Binary).Path

$agForward = Join-Path (Join-Path $repoRoot 'tools') 'ag-forward.ps1'
$forwardWin = Join-Path (Join-Path (Join-Path $repoRoot 'vscode-extension') 'scripts') 'forward-win.ps1'
foreach ($script in @($agForward, $forwardWin)) {
    if (-not (Test-Path -LiteralPath $script -PathType Leaf)) {
        Write-Error "forwarder script not found: $script"
        exit 1
    }
}

# The same PowerShell that runs this script runs the forwarders.
$pwshExe = (Get-Process -Id $PID).Path
if (-not $pwshExe) { $pwshExe = 'pwsh' }

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

function Get-FreeTcpPort {
    $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)
    try {
        $listener.Start()
        return $listener.LocalEndpoint.Port
    }
    finally {
        $listener.Stop()
    }
}

# Quote one argument for a Start-Process argument string. Both Windows
# (CommandLineToArgvW) and .NET on Unix treat "..." as one argument and \"
# as a literal quote, so a temp path with spaces survives on either.
function Format-ProcessArg([string]$Value) {
    if ($Value -notmatch '[\s"]') { return $Value }
    return '"' + ($Value -replace '(\\*)"', '$1$1\"') + '"'
}

# Run a forwarder script as a child pwsh and return exit code, stdout, stderr.
# ProcessStartInfo.ArgumentList passes each argument verbatim (no shell
# quoting to get wrong), and the child's environment can be edited per call.
function Invoke-Forwarder {
    param(
        [Parameter(Mandatory)] [string]$Script,
        [string[]]$Arguments = @(),
        [hashtable]$Environment = @{}
    )

    $psi = [System.Diagnostics.ProcessStartInfo]::new()
    $psi.FileName = $pwshExe
    $psi.UseShellExecute = $false
    $psi.RedirectStandardOutput = $true
    $psi.RedirectStandardError = $true
    $psi.WorkingDirectory = $repoRoot
    foreach ($a in @('-NoProfile', '-NonInteractive', '-File', $Script) + $Arguments) {
        $psi.ArgumentList.Add($a)
    }

    # Never let the developer's own shell leak forwarder defaults into a test;
    # each check states exactly which variables the child sees.
    foreach ($name in @('ENKENTE_SESSION_ID', 'ENKENTE_API_URL')) {
        [void]$psi.Environment.Remove($name)
    }
    foreach ($entry in $Environment.GetEnumerator()) {
        $psi.Environment[$entry.Key] = [string]$entry.Value
    }

    $proc = [System.Diagnostics.Process]::Start($psi)
    try {
        $stdoutTask = $proc.StandardOutput.ReadToEndAsync()
        $stderrTask = $proc.StandardError.ReadToEndAsync()
        if (-not $proc.WaitForExit(60000)) {
            try { $proc.Kill($true) } catch { }
            $proc.WaitForExit()
        }
        return [pscustomobject]@{
            ExitCode = $proc.ExitCode
            Stdout   = $stdoutTask.Result.Trim()
            Stderr   = $stderrTask.Result.Trim()
        }
    }
    finally {
        $proc.Dispose()
    }
}

# GET a JSON array and emit its elements one by one. Depending on the
# PowerShell version Invoke-RestMethod returns an array either as one
# Object[] item or already enumerated; the foreach flattens both the same.
function Get-JsonItems([string]$Url) {
    $result = Invoke-RestMethod -Method Get -Uri $Url -TimeoutSec 10
    foreach ($item in $result) { $item }
}

# Parse the single-line JSON a forwarder prints on success.
function ConvertFrom-ForwarderOutput([string]$Stdout) {
    if ([string]::IsNullOrWhiteSpace($Stdout)) { return $null }
    try { return ($Stdout | ConvertFrom-Json) } catch { return $null }
}

# Return the stored messages of a session as an array (empty if 404).
function Get-SessionMessages([string]$BaseUrl, [string]$SessionId) {
    try {
        return @(Get-JsonItems "$BaseUrl/sessions/$([uri]::EscapeDataString($SessionId))/messages")
    }
    catch {
        return @()
    }
}

# ---------------------------------------------------------------------------
# Minimal check runner
# ---------------------------------------------------------------------------

$script:results = [System.Collections.Generic.List[object]]::new()

function Test-Check {
    param(
        [Parameter(Mandatory)] [string]$Name,
        [Parameter(Mandatory)] [scriptblock]$Body
    )
    $failure = $null
    try {
        & $Body
    }
    catch {
        $failure = $_.Exception.Message
    }
    if ($failure) {
        Write-Host "FAIL $Name"
        Write-Host "     $failure"
    }
    else {
        Write-Host "ok   $Name"
    }
    $script:results.Add([pscustomobject]@{ Name = $Name; Failure = $failure })
}

# Throw a readable message when a condition is false. Used inside checks.
function Assert-True([bool]$Condition, [string]$Message) {
    if (-not $Condition) { throw $Message }
}

function Format-Run($Run) {
    return "exit=$($Run.ExitCode) stdout=[$($Run.Stdout)] stderr=[$($Run.Stderr)]"
}

# ---------------------------------------------------------------------------
# Start the server
# ---------------------------------------------------------------------------

if ($Port -eq 0) { $Port = Get-FreeTcpPort }
$baseUrl = "http://localhost:$Port"

$tempDir = Join-Path ([System.IO.Path]::GetTempPath()) ("enkente-test-" + [guid]::NewGuid().ToString('N').Substring(0, 12))
New-Item -ItemType Directory -Path $tempDir -Force | Out-Null
$dbPath = Join-Path $tempDir 'enkente.db'
$serverOut = Join-Path $tempDir 'server.stdout.log'
$serverErr = Join-Path $tempDir 'server.stderr.log'

$server = $null
$exitCode = 1

try {
    $serverArgs = @('serve', '--headless', '--port', "$Port", '--db', $dbPath) |
        ForEach-Object { Format-ProcessArg $_ }
    $server = Start-Process -FilePath $Binary -ArgumentList ($serverArgs -join ' ') `
        -WorkingDirectory $tempDir `
        -RedirectStandardOutput $serverOut -RedirectStandardError $serverErr `
        -NoNewWindow -PassThru

    Write-Host "enkente: $Binary"
    Write-Host "server:  $baseUrl (pid $($server.Id), db $dbPath)"

    $deadline = [DateTime]::UtcNow.AddSeconds($StartupTimeoutSec)
    $healthy = $false
    $health = $null
    while ([DateTime]::UtcNow -lt $deadline) {
        if ($server.HasExited) { break }
        try {
            $health = Invoke-RestMethod -Method Get -Uri "$baseUrl/health" -TimeoutSec 2
            if ($health.status -eq 'ok') { $healthy = $true; break }
        }
        catch {
            Start-Sleep -Milliseconds 200
        }
    }
    if (-not $healthy) {
        $why = if ($server.HasExited) { "server exited early with code $($server.ExitCode)" } else { "GET /health did not answer within ${StartupTimeoutSec}s" }
        Write-Host "FAIL server startup: $why"
        foreach ($log in @($serverOut, $serverErr)) {
            if ((Test-Path -LiteralPath $log) -and (Get-Item -LiteralPath $log).Length -gt 0) {
                Write-Host "--- $log"
                Get-Content -LiteralPath $log | ForEach-Object { Write-Host "    $_" }
            }
        }
        Write-Host "FAIL 0/1"
        exit 1
    }
    Assert-True ($health.persistence -eq $true) "server reports persistence=false; the test needs --db"

    # -----------------------------------------------------------------------
    # Checks
    # -----------------------------------------------------------------------

    $stamp = [guid]::NewGuid().ToString('N').Substring(0, 8)

    Test-Check 'ag-forward: default call exits 0 and prints a stored message' {
        $text = "default message $stamp"
        $run = Invoke-Forwarder -Script $agForward -Arguments @('-Message', $text, '-ApiUrl', $baseUrl)
        Assert-True ($run.ExitCode -eq 0) "expected exit 0: $(Format-Run $run)"
        Assert-True ([string]::IsNullOrEmpty($run.Stderr)) "expected empty stderr: $(Format-Run $run)"
        $json = ConvertFrom-ForwarderOutput $run.Stdout
        Assert-True ($null -ne $json) "stdout is not JSON: $(Format-Run $run)"
        Assert-True ([int]$json.messageId -gt 0) "expected messageId > 0, got '$($json.messageId)'"
        Assert-True ($json.message -eq $text) "stored text '$($json.message)' != '$text'"
        Assert-True ($json.type -eq 'user') "expected default type 'user', got '$($json.type)'"
        Assert-True (-not [string]::IsNullOrEmpty($json.sessionId)) "stored message has no sessionId"
        $sessions = @(Get-JsonItems "$baseUrl/sessions")
        Assert-True (@($sessions | Where-Object sessionId -eq $json.sessionId).Count -eq 1) `
            "GET /sessions does not list session '$($json.sessionId)'"
    }

    Test-Check 'ag-forward: -SessionId proj-a -Type system lands in proj-a' {
        $text = "system note $stamp"
        $run = Invoke-Forwarder -Script $agForward -Arguments @(
            '-Message', $text, '-SessionId', 'proj-a', '-Type', 'system', '-User', 'tester', '-ApiUrl', $baseUrl)
        Assert-True ($run.ExitCode -eq 0) "expected exit 0: $(Format-Run $run)"
        $json = ConvertFrom-ForwarderOutput $run.Stdout
        Assert-True ($null -ne $json) "stdout is not JSON: $(Format-Run $run)"
        Assert-True ($json.sessionId -eq 'proj-a') "response sessionId '$($json.sessionId)' != 'proj-a'"
        $stored = Get-SessionMessages $baseUrl 'proj-a' | Where-Object message -eq $text
        Assert-True (@($stored).Count -eq 1) "GET /sessions/proj-a/messages does not hold the message"
        $stored = @($stored)[0]
        Assert-True ($stored.type -eq 'system') "stored type '$($stored.type)' != 'system'"
        Assert-True ($stored.user -eq 'tester') "stored user '$($stored.user)' != 'tester'"
        Assert-True ([int]$stored.messageId -eq [int]$json.messageId) `
            "stored messageId $($stored.messageId) != printed $($json.messageId)"
    }

    Test-Check 'ag-forward: ENKENTE_SESSION_ID environment variable is honoured' {
        $text = "env session $stamp"
        $run = Invoke-Forwarder -Script $agForward -Arguments @('-Message', $text, '-ApiUrl', $baseUrl) `
            -Environment @{ ENKENTE_SESSION_ID = 'env-sess' }
        Assert-True ($run.ExitCode -eq 0) "expected exit 0: $(Format-Run $run)"
        $json = ConvertFrom-ForwarderOutput $run.Stdout
        Assert-True ($null -ne $json) "stdout is not JSON: $(Format-Run $run)"
        Assert-True ($json.sessionId -eq 'env-sess') "response sessionId '$($json.sessionId)' != 'env-sess'"
        $stored = Get-SessionMessages $baseUrl 'env-sess' | Where-Object message -eq $text
        Assert-True (@($stored).Count -eq 1) "GET /sessions/env-sess/messages does not hold the message"
    }

    Test-Check 'ag-forward: whitespace-only -Message exits 1 with an error' {
        $run = Invoke-Forwarder -Script $agForward -Arguments @('-Message', '   ', '-ApiUrl', $baseUrl)
        Assert-True ($run.ExitCode -eq 1) "expected exit 1: $(Format-Run $run)"
        Assert-True ([string]::IsNullOrEmpty($run.Stdout)) "expected no stdout: $(Format-Run $run)"
        Assert-True ($run.Stderr -match 'failed to forward') "expected a forwarding error on stderr: $(Format-Run $run)"
    }

    Test-Check 'ag-forward: unreachable -ApiUrl exits 1 with an error' {
        $run = Invoke-Forwarder -Script $agForward -Arguments @('-Message', 'nobody home', '-ApiUrl', 'http://localhost:1')
        Assert-True ($run.ExitCode -eq 1) "expected exit 1: $(Format-Run $run)"
        Assert-True ([string]::IsNullOrEmpty($run.Stdout)) "expected no stdout: $(Format-Run $run)"
        Assert-True ($run.Stderr -match 'localhost:1/ingest') "expected the target URL on stderr: $(Format-Run $run)"
    }

    Test-Check 'forward-win: JSON payload lands in its session' {
        $text = "forward-win payload $stamp"
        $payload = @{ type = 'user'; user = 'bridge'; sessionId = 'proj-b'; message = $text } | ConvertTo-Json -Compress
        $run = Invoke-Forwarder -Script $forwardWin -Arguments @('-Payload', $payload, '-ApiUrl', $baseUrl)
        Assert-True ($run.ExitCode -eq 0) "expected exit 0: $(Format-Run $run)"
        Assert-True ([string]::IsNullOrEmpty($run.Stderr)) "expected empty stderr: $(Format-Run $run)"
        $json = ConvertFrom-ForwarderOutput $run.Stdout
        Assert-True ($null -ne $json) "stdout is not JSON: $(Format-Run $run)"
        Assert-True ([int]$json.messageId -gt 0) "expected messageId > 0, got '$($json.messageId)'"
        Assert-True ($json.sessionId -eq 'proj-b') "response sessionId '$($json.sessionId)' != 'proj-b'"
        $stored = Get-SessionMessages $baseUrl 'proj-b' | Where-Object message -eq $text
        Assert-True (@($stored).Count -eq 1) "GET /sessions/proj-b/messages does not hold the message"
        Assert-True (@($stored)[0].user -eq 'bridge') "stored user '$(@($stored)[0].user)' != 'bridge'"
    }

    Test-Check 'forward-win: payload with whitespace-only message exits 1' {
        $payload = @{ message = '   ' } | ConvertTo-Json -Compress
        $run = Invoke-Forwarder -Script $forwardWin -Arguments @('-Payload', $payload, '-ApiUrl', $baseUrl)
        Assert-True ($run.ExitCode -eq 1) "expected exit 1: $(Format-Run $run)"
        Assert-True ([string]::IsNullOrEmpty($run.Stdout)) "expected no stdout: $(Format-Run $run)"
        Assert-True ($run.Stderr -match 'failed to forward') "expected a forwarding error on stderr: $(Format-Run $run)"
    }

    Test-Check 'forward-win: unreachable -ApiUrl exits 1 with an error' {
        $payload = @{ message = 'nobody home' } | ConvertTo-Json -Compress
        $run = Invoke-Forwarder -Script $forwardWin -Arguments @('-Payload', $payload, '-ApiUrl', 'http://localhost:1')
        Assert-True ($run.ExitCode -eq 1) "expected exit 1: $(Format-Run $run)"
        Assert-True ($run.Stderr -match 'localhost:1/ingest') "expected the target URL on stderr: $(Format-Run $run)"
    }

    Test-Check 'server: GET /sessions lists every session the forwarders wrote' {
        $ids = @(Get-JsonItems "$baseUrl/sessions" | ForEach-Object sessionId)
        foreach ($want in @('proj-a', 'proj-b', 'env-sess')) {
            Assert-True ($ids -contains $want) "GET /sessions is missing '$want' (has: $($ids -join ', '))"
        }
    }

    # -----------------------------------------------------------------------
    # Summary
    # -----------------------------------------------------------------------

    $failed = @($script:results | Where-Object { $_.Failure })
    $total = $script:results.Count
    if ($failed.Count -eq 0) {
        Write-Host "PASS $total/$total"
        $exitCode = 0
    }
    else {
        Write-Host "FAIL $($failed.Count)/$total"
        foreach ($f in $failed) {
            Write-Host "  - $($f.Name): $($f.Failure)"
        }
        $exitCode = 1
    }
}
finally {
    if ($server -and -not $server.HasExited) {
        Stop-Process -Id $server.Id -Force -ErrorAction SilentlyContinue
        try { [void]$server.WaitForExit(5000) } catch { }
    }
    if (Test-Path -LiteralPath $tempDir) {
        # On Windows the datastore handle can outlive the killed process for a moment.
        foreach ($attempt in 1..10) {
            try {
                Remove-Item -LiteralPath $tempDir -Recurse -Force -ErrorAction Stop
                break
            }
            catch {
                Start-Sleep -Milliseconds 200
            }
        }
    }
}

exit $exitCode
