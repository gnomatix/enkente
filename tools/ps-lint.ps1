<#
.SYNOPSIS
    Syntax-check every PowerShell script in the repository.

.DESCRIPTION
    Runs each .ps1 under tools/ and vscode-extension/scripts/ through the
    PowerShell language parser and fails (exit 1) if any has parse errors.
    Needs no network and no running server, so it is safe for CI.
#>
[CmdletBinding()]
param(
    [string[]]$Path = @('tools', 'vscode-extension/scripts')
)

$root = Split-Path -Parent $PSScriptRoot
$failed = 0
$checked = 0

foreach ($dir in $Path) {
    $full = Join-Path $root $dir
    if (-not (Test-Path $full)) { continue }
    foreach ($file in Get-ChildItem -Path $full -Filter '*.ps1' -File) {
        $tokens = $null
        $errors = $null
        [System.Management.Automation.Language.Parser]::ParseFile($file.FullName, [ref]$tokens, [ref]$errors) | Out-Null
        $checked++
        if ($errors.Count -gt 0) {
            $failed++
            Write-Host "FAIL $($file.FullName)"
            foreach ($e in $errors) {
                Write-Host ("  {0}:{1}: {2}" -f $e.Extent.StartLineNumber, $e.Extent.StartColumnNumber, $e.Message)
            }
        }
        else {
            Write-Host "ok   $($file.FullName)"
        }
    }
}

Write-Host "$checked script(s) checked, $failed with parse errors"
if ($failed -gt 0) { exit 1 }
exit 0
