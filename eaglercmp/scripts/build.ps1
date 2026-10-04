# Build the EaglerCMP daemon on Windows and optionally initialise the runtime.
#   .\scripts\build.ps1                         # build .\bin\eaglercmp.exe
#   .\scripts\build.ps1 -Target windows/arm64   # cross-compile one target into .\bin
#   .\scripts\build.ps1 -Version v0.2.0         # stamp the launcher version
#   .\scripts\build.ps1 -Init [-Source x.html]  # build, then import the Eaglercraft client (or run init)
param(
    [string]$Target = "",
    [string]$Version = "",
    [switch]$SkipChecks,
    [switch]$Init,
    [string]$Source = ""
)
$ErrorActionPreference = "Stop"
Set-Location (Join-Path $PSScriptRoot "..")

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw "Go 1.22+ is required: https://go.dev/dl/"
}

function Invoke-Go {
    & go @args
    if ($LASTEXITCODE -ne 0) { throw "go $($args -join ' ') failed with exit code $LASTEXITCODE" }
}

if (-not $SkipChecks) {
    Invoke-Go vet ./...
    Invoke-Go test ./...
}

$ldflags = "-s -w"
if ($Version) {
    $ldflags += " -X github.com/OnyxNeol/eaglercmp/config.LauncherVersion=$($Version.TrimStart('v'))"
}

$goos = (& go env GOOS); $goarch = (& go env GOARCH)
if ($Target) { $goos, $goarch = $Target.Split("/") }
$ext = if ($goos -eq "windows") { ".exe" } else { "" }
# Windows: GUI subsystem, so double-clicking opens only the game window.
if ($goos -eq "windows") { $ldflags += " -H windowsgui" }
$out = Join-Path "bin" "eaglercmp$ext"

$env:CGO_ENABLED = "0"
$env:GOOS = $goos
$env:GOARCH = $goarch
New-Item -ItemType Directory -Force -Path bin | Out-Null
Write-Host "building $out ($goos/$goarch)"
Invoke-Go build -trimpath -ldflags $ldflags -o $out .
Remove-Item Env:GOOS, Env:GOARCH

if ($Init) {
    if ($Source) { & (Resolve-Path $out) import $Source } else { & (Resolve-Path $out) init }
    exit $LASTEXITCODE
}
