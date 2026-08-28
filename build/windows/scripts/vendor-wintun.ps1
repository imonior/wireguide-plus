#Requires -Version 5
[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][string]$Version,
    [Parameter(Mandatory=$true)][string]$Sha256,
    [Parameter(Mandatory=$true)][string]$Arch,
    [Parameter(Mandatory=$true)][string]$OutDir
)

$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$dllArch = if ($Arch -eq '386') { 'x86' } else { $Arch }
$url     = "https://www.wintun.net/builds/wintun-$Version.zip"
$expect  = $Sha256.ToLower()
$tmpZip  = Join-Path $env:TEMP "wintun-$Version.zip"
$tmpDir  = Join-Path $env:TEMP ("wintun-extract-" + [guid]::NewGuid())

Write-Host "Downloading $url"
Invoke-WebRequest -Uri $url -OutFile $tmpZip

# .NET APIs instead of Get-FileHash / Expand-Archive cmdlets. GitHub
# Actions Windows runners enforce an AppLocker policy that blocks
# several Microsoft.PowerShell.Utility / Microsoft.PowerShell.Archive
# cmdlets with CommandNotFoundException — Get-FileHash was the first
# casualty here. System.Security.Cryptography.SHA256 and
# System.IO.Compression.ZipFile are first-party .NET types that aren't
# subject to the same restrictions.
$stream = [System.IO.File]::OpenRead($tmpZip)
try {
    $sha = [System.Security.Cryptography.SHA256]::Create()
    $hashBytes = $sha.ComputeHash($stream)
    $actual = ([System.BitConverter]::ToString($hashBytes) -replace '-', '').ToLower()
} finally {
    $stream.Dispose()
}
if ($actual -ne $expect) {
    throw "wintun.zip SHA256 mismatch. expected=$expect actual=$actual"
}

Add-Type -AssemblyName System.IO.Compression.FileSystem
if (Test-Path $tmpDir) { Remove-Item -Recurse -Force $tmpDir }
New-Item -ItemType Directory -Force -Path $tmpDir | Out-Null
[System.IO.Compression.ZipFile]::ExtractToDirectory($tmpZip, $tmpDir)
$src = Join-Path $tmpDir "wintun\bin\$dllArch\wintun.dll"
if (-not (Test-Path $src)) {
    throw "wintun.dll not found at $src (unknown arch: $dllArch)"
}

New-Item -ItemType Directory -Force -Path $OutDir | Out-Null
Copy-Item $src (Join-Path $OutDir 'wintun.dll') -Force
Remove-Item -Recurse -Force $tmpDir
Write-Host "Bundled $dllArch wintun.dll to $OutDir"
