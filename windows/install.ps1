# Installs, configures and starts the Periscope sender on device 1.
#
# One command (run `d1 pair` on device n to get it with your values filled in):
#   powershell -NoProfile -ExecutionPolicy Bypass -Command "& ([scriptblock]::Create((irm https://raw.githubusercontent.com/crl-sdn-vpz/periscope/main/windows/install.ps1))) -TargetHost <ip> -Port 9000 -Passphrase '<passphrase>'"
# From a checkout:
#   powershell -ExecutionPolicy Bypass -File .\install.ps1 -TargetHost <ip> -Passphrase '<passphrase>'
#
# Re-run it any time to update the scripts. The installed config is kept; only values you pass change.
# Compatible with Windows PowerShell 5.1 and PowerShell 7.
param(
    [string]$TargetHost,
    [int]$Port = 0,
    [string]$Passphrase,
    [string]$Ref = "main"
)

$ErrorActionPreference = "Stop"
$repo = "crl-sdn-vpz/periscope"
$app  = Join-Path $env:LOCALAPPDATA "periscope\app"
$cfg  = Join-Path $app "periscope.config.ps1"
$ps   = Join-Path $env:SystemRoot "System32\WindowsPowerShell\v1.0\powershell.exe"

function Write-Step([string]$msg) { Write-Host "==> $msg" }

# Runs one of the installed scripts whatever the machine's execution policy is.
function Invoke-AppScript([string]$name) {
    & $ps -NoProfile -ExecutionPolicy Bypass -File (Join-Path $app $name)
    if ($LASTEXITCODE -ne 0) { throw "$name failed (exit $LASTEXITCODE)" }
}

# Replaces the value of a `Key = value` line in the config, keeping the line's indentation.
function Set-ConfigValue([string]$text, [string]$key, [string]$value) {
    $pattern = "(?m)^(\s*$key\s*=\s*)[^\r\n]*"
    if ($text -notmatch $pattern) { throw "periscope.config.ps1 has no '$key = ...' line" }
    return [regex]::Replace($text, $pattern, { param($m) $m.Groups[1].Value + $value })
}

# --- 1. get the scripts ------------------------------------------------------
$src = $null
if ($PSScriptRoot -and (Test-Path (Join-Path $PSScriptRoot "sender.ps1"))) {
    $src = $PSScriptRoot
} else {
    Write-Step "Downloading periscope ($Ref)"
    $tmp = Join-Path ([IO.Path]::GetTempPath()) ("periscope-" + [guid]::NewGuid())
    New-Item -ItemType Directory -Path $tmp | Out-Null
    $zip = Join-Path $tmp "periscope.zip"
    Invoke-WebRequest -UseBasicParsing -Uri "https://github.com/$repo/archive/$Ref.zip" -OutFile $zip
    Expand-Archive -Path $zip -DestinationPath $tmp
    $top = Get-ChildItem -Path $tmp -Directory | Select-Object -First 1
    $src = Join-Path $top.FullName "windows"
}

New-Item -ItemType Directory -Force -Path $app | Out-Null
if (Test-Path (Join-Path $app "sender.ps1")) {
    Write-Step "Stopping the running sender"
    Invoke-AppScript "stop.ps1"
}

Write-Step "Installing to $app"
if ((Resolve-Path $src).Path -ne (Resolve-Path $app).Path) {
    Get-ChildItem -Path $src -File | Where-Object { $_.Name -ne "periscope.config.ps1" } |
        Copy-Item -Destination $app -Force
    if (-not (Test-Path $cfg)) { Copy-Item (Join-Path $src "periscope.config.ps1") $cfg }
}
Get-ChildItem -Path $app -File | Unblock-File

# --- 2. config ---------------------------------------------------------------
$text = [IO.File]::ReadAllText($cfg)
if ($TargetHost) { $text = Set-ConfigValue $text "TargetHost" ('"' + $TargetHost + '"') }
if ($Port -gt 0) { $text = Set-ConfigValue $text "Port" "$Port" }
if ($Passphrase) { $text = Set-ConfigValue $text "Passphrase" ("'" + $Passphrase.Replace("'", "''") + "'") }
[IO.File]::WriteAllText($cfg, $text)

. $cfg
if ($Config.Passphrase -eq "change-me-to-a-long-random-string") {
    throw "No passphrase set yet. Run 'd1 pair' on device n and paste the command it prints."
}
if ($Config.Passphrase.Length -lt 10 -or $Config.Passphrase.Length -gt 79) {
    throw "Passphrase must be 10-79 characters."
}
Write-Step ("Config: target {0}:{1}, monitor {2}, encoder {3}  ({4})" -f $Config.TargetHost, $Config.Port, $Config.Monitor, $Config.Encoder, $cfg)

# --- 3. ffmpeg ---------------------------------------------------------------
$haveFfmpeg = (Get-Command $Config.FfmpegPath -ErrorAction SilentlyContinue) -or
              (Test-Path (Join-Path $env:LOCALAPPDATA "Microsoft\WinGet\Links\ffmpeg.exe"))
if (-not $haveFfmpeg) {
    if (-not (Get-Command winget -ErrorAction SilentlyContinue)) {
        throw "ffmpeg not found and winget isn't available. Install ffmpeg, then set FfmpegPath in $cfg"
    }
    Write-Step "Installing ffmpeg (winget: Gyan.FFmpeg)"
    winget install --id Gyan.FFmpeg --exact --source winget --silent --accept-package-agreements --accept-source-agreements
    if ($LASTEXITCODE -ne 0) { throw "winget install Gyan.FFmpeg failed (exit $LASTEXITCODE)" }
}

# --- 4. autostart + start ----------------------------------------------------
Write-Step "Registering autostart (scheduled task PeriscopeSender)"
Invoke-AppScript "install-autostart.ps1"
Write-Step "Starting the sender"
Start-ScheduledTask -TaskName "PeriscopeSender"
Start-Sleep -Seconds 5
Invoke-AppScript "status.ps1"
Write-Host ""
Write-Host "Done. Stop: $app\stop.ps1   Status: $app\status.ps1   Remove: $app\uninstall-autostart.ps1"
