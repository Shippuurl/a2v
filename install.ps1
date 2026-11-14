# a2v installer for Windows
#
#   irm https://tool.kervein.club/a2v/install.ps1 | iex
#
# Options (environment variables, since piped scripts cannot take params):
#   A2V_DIR    install directory   (default: %LOCALAPPDATA%\a2v)
#   A2V_BASE   mirror root         (default: https://tool.kervein.club/a2v)
#   A2V_VERSION  pin a release     (default: whatever latest.json points at)
#
# ASCII only on purpose: this script is executed by Windows PowerShell 5.1 as
# well as pwsh 7, and both decode a text/plain download differently.
param([string]$Dir = $env:A2V_DIR)

$ErrorActionPreference = 'Stop'
try {
    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
} catch {}

$Base = if ($env:A2V_BASE) { $env:A2V_BASE } else { 'https://tool.kervein.club/a2v' }

Write-Host ""
Write-Host "  a2v  a2vec / codex launcher" -ForegroundColor Cyan
Write-Host ""

# ARM64 Windows reports AMD64 under x64 emulation; the real arch is here.
$procArch = $env:PROCESSOR_ARCHITEW6432
if (-not $procArch) { $procArch = $env:PROCESSOR_ARCHITECTURE }
switch ($procArch) {
    'AMD64' { $arch = 'amd64' }
    'ARM64' { $arch = 'arm64' }
    default { throw "unsupported architecture: $procArch (a2v ships amd64 and arm64)" }
}
Write-Host "  platform windows-$arch" -ForegroundColor Green

$name = "a2v-windows-$arch.exe"
$url  = "$Base/$name"
if ($env:A2V_VERSION) { $url = "$Base/releases/$($env:A2V_VERSION)/$name" }

Write-Host "  downloading $url" -ForegroundColor DarkGray
$tmpDir = $env:TEMP
if (-not $tmpDir) { $tmpDir = [IO.Path]::GetTempPath() }
$tmp = Join-Path $tmpDir ("a2v-" + [guid]::NewGuid().ToString('N') + ".exe")
Invoke-WebRequest -Uri $url -OutFile $tmp -UseBasicParsing

$resp = Invoke-WebRequest -Uri "$url.sha256" -UseBasicParsing
$expectText = $resp.Content
# .sha256 is served as octet-stream, which PowerShell hands back as byte[].
if ($expectText -is [byte[]]) { $expectText = [Text.Encoding]::UTF8.GetString($expectText) }
$expect = ($expectText -split '\s+')[0].Trim().ToLower()
if (-not $expect) {
    Remove-Item $tmp -Force -ErrorAction SilentlyContinue
    throw "no sha256 published for $name; refusing to install"
}
$got = (Get-FileHash -Path $tmp -Algorithm SHA256).Hash.ToLower()
if ($got -ne $expect) {
    Remove-Item $tmp -Force -ErrorAction SilentlyContinue
    throw "sha256 mismatch (got $got, expected $expect)"
}
Write-Host "  sha256 $($got.Substring(0,12))..." -ForegroundColor Green

if (-not $Dir) { $Dir = Join-Path $env:LOCALAPPDATA 'a2v' }
New-Item -ItemType Directory -Force -Path $Dir | Out-Null
$dest = Join-Path $Dir 'a2v.exe'
try {
    Move-Item -Force $tmp $dest
} catch {
    Remove-Item $tmp -Force -ErrorAction SilentlyContinue
    throw "cannot replace $dest (is a2v still running? close it and retry): $($_.Exception.Message)"
}
Write-Host "  installed $dest" -ForegroundColor Green

$onWindows = $true
if (Get-Variable IsWindows -ErrorAction SilentlyContinue) { $onWindows = $IsWindows }

if ($onWindows) {
    try {
        $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
        if (-not $userPath) { $userPath = '' }
        if (($userPath -split ';') -notcontains $Dir) {
            $trimmed = $userPath.TrimEnd(';')
            $newPath = if ($trimmed) { "$trimmed;$Dir" } else { $Dir }
            [Environment]::SetEnvironmentVariable('Path', $newPath, 'User')
            $env:Path = $env:Path + ';' + $Dir
            Write-Host "  added $Dir to the user PATH (open a new terminal to use it)" -ForegroundColor Yellow
        }
    } catch {
        Write-Host "  could not update PATH: $($_.Exception.Message)" -ForegroundColor Yellow
        Write-Host "  add this directory manually: $Dir" -ForegroundColor Yellow
    }
}

Write-Host ""
if ($onWindows) { & $dest version } else { Write-Host "  (skipping version check: not running on Windows)" }
Write-Host ""
Write-Host "  next"
Write-Host "    a2v           interactive menu"
Write-Host "    a2v doctor    check codex, relay and key"
Write-Host "    a2v --help    every command"
Write-Host ""
Write-Host "  docs  $Base/" -ForegroundColor DarkGray
Write-Host ""
