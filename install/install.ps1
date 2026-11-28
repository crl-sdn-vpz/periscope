# Installs the periscope binary from GitHub Releases plus ffmpeg, writes the config, registers
# autostart at logon and starts the sender. Windows can send in this version; viewing comes later.
#
# One command (`periscope pair` on the viewer prints it with your values filled in):
#   powershell -NoProfile -ExecutionPolicy Bypass -Command "& ([scriptblock]::Create((irm https://raw.githubusercontent.com/crl-sdn-vpz/periscope/main/install/install.ps1))) -Role send -TargetHost <ip> -Port 9000 -Passphrase '<passphrase>'"
#
# Re-run it any time to update. The config is kept; only values you pass change.
# $env:PERISCOPE_DOWNLOAD_BASE replaces the release URL (used by tests).
# Compatible with Windows PowerShell 5.1 and PowerShell 7.
param(
    [string]$Role = "send",
    [string]$TargetHost,
    [int]$Port = 0,
    [string]$Passphrase,
    [string]$Version = "latest"
)

$ErrorActionPreference = "Stop"
$repo  = "crl-sdn-vpz/periscope"
$asset = "periscope_windows_amd64.exe"
$bin   = Join-Path $env:LOCALAPPDATA "periscope\bin"
$exe   = Join-Path $bin "periscope.exe"
$task  = "PeriscopeSender"

function Write-Step([string]$msg) { Write-Host "==> $msg" }

if ($Role -ne "send") { throw "Only -Role send is supported on Windows for now (viewing on Windows arrives in milestone 3)." }

# --- 1. download and verify ----------------------------------------------------
$base = $env:PERISCOPE_DOWNLOAD_BASE
if (-not $base) {
    if ($Version -eq "latest") { $base = "https://github.com/$repo/releases/latest/download" }
    else { $base = "https://github.com/$repo/releases/download/$Version" }
}
$tmp = Join-Path ([IO.Path]::GetTempPath()) ("periscope-" + [guid]::NewGuid())
New-Item -ItemType Directory -Path $tmp | Out-Null
Write-Step "Downloading $asset ($Version)"
Invoke-WebRequest -UseBasicParsing -Uri "$base/$asset" -OutFile (Join-Path $tmp $asset)
Invoke-WebRequest -UseBasicParsing -Uri "$base/SHA256SUMS" -OutFile (Join-Path $tmp "SHA256SUMS")
$line = Get-Content (Join-Path $tmp "SHA256SUMS") | Where-Object { $_ -match "\s\*?$([regex]::Escape($asset))$" } | Select-Object -First 1
if (-not $line) { throw "SHA256SUMS has no entry for $asset" }
$want = ($line -split "\s+")[0].ToLower()
$got  = (Get-FileHash -Algorithm SHA256 -Path (Join-Path $tmp $asset)).Hash.ToLower()
if ($got -ne $want) { throw "Checksum mismatch for $asset (got $got, want $want)" }

# --- 2. install ----------------------------------------------------------------
if (Get-ScheduledTask -TaskName $task -ErrorAction SilentlyContinue) {
    Write-Step "Stopping the running sender"
    Stop-ScheduledTask -TaskName $task -ErrorAction SilentlyContinue
}
# The v0.2 script sender (same task name) leaves its ffmpeg running unless stop.ps1 runs.
$oldStop = Join-Path $env:LOCALAPPDATA "periscope\app\stop.ps1"
if (Test-Path $oldStop) {
    Write-Step "Stopping the v0.2 script sender"
    & (Join-Path $env:SystemRoot "System32\WindowsPowerShell\v1.0\powershell.exe") -NoProfile -ExecutionPolicy Bypass -File $oldStop | Out-Null
}
if (Test-Path $exe) {
    # Windows PowerShell 5.1 turns a native command's redirected stderr into a terminating error under
    # "Stop", and "send stop" writes to stderr when no sender runs.
    $ErrorActionPreference = "Continue"
    & $exe send stop 2>&1 | Out-Null
    $ErrorActionPreference = "Stop"
}
New-Item -ItemType Directory -Force -Path $bin | Out-Null
Copy-Item -Force (Join-Path $tmp $asset) $exe
Unblock-File -Path $exe
Remove-Item -Recurse -Force $tmp
Write-Step ("Installed {0} to {1}" -f (& $exe version), $exe)

# --- 3. ffmpeg -----------------------------------------------------------------
$haveFfmpeg = (Get-Command ffmpeg -ErrorAction SilentlyContinue) -or
              (Test-Path (Join-Path $env:LOCALAPPDATA "Microsoft\WinGet\Links\ffmpeg.exe"))
if (-not $haveFfmpeg) {
    if (-not (Get-Command winget -ErrorAction SilentlyContinue)) {
        throw "ffmpeg not found and winget isn't available. Install ffmpeg, then: periscope config set send.ffmpeg <path-to-ffmpeg.exe>"
    }
    Write-Step "Installing ffmpeg (winget: Gyan.FFmpeg)"
    winget install --id Gyan.FFmpeg --exact --source winget --silent --accept-package-agreements --accept-source-agreements
    if ($LASTEXITCODE -ne 0) { throw "winget install Gyan.FFmpeg failed (exit $LASTEXITCODE)" }
}

# --- 4. config -----------------------------------------------------------------
Write-Step "Config"
$initArgs = @("config", "init")
if ($TargetHost) { $initArgs += @("--target-host", $TargetHost) }
if ($Port -gt 0) { $initArgs += @("--port", "$Port") }
if ($Passphrase) { $initArgs += @("--passphrase", $Passphrase) }
& $exe @initArgs
if ($LASTEXITCODE -ne 0) { throw "periscope config init failed (exit $LASTEXITCODE)" }
if (-not (& $exe config get send.target_host)) {
    throw "No target host yet. Run 'periscope pair' on the viewing device and paste the command it prints."
}

# --- 5. autostart + start ------------------------------------------------------
# conhost --headless runs the console app with no window (Windows 10 21H2+ / Windows 11).
Write-Step "Registering autostart (scheduled task $task)"
$action    = New-ScheduledTaskAction -Execute "conhost.exe" -Argument "--headless `"$exe`" send"
$trigger   = New-ScheduledTaskTrigger -AtLogOn -User $env:USERNAME
$settings  = New-ScheduledTaskSettingsSet -Hidden -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries `
             -ExecutionTimeLimit ([TimeSpan]::Zero) -MultipleInstances IgnoreNew
$principal = New-ScheduledTaskPrincipal -UserId $env:USERNAME -LogonType Interactive -RunLevel Limited
Register-ScheduledTask -TaskName $task -Action $action -Trigger $trigger -Settings $settings `
    -Principal $principal -Force | Out-Null

Write-Step "Starting the sender"
Start-ScheduledTask -TaskName $task
Start-Sleep -Seconds 5
& $exe send status
Write-Host ""
Write-Host "Done. Status: periscope send status   Stop: periscope send stop   Logs: $env:LOCALAPPDATA\periscope"
Write-Host "Add $bin to your PATH to type 'periscope' anywhere. Remove autostart: Unregister-ScheduledTask -TaskName $task"
