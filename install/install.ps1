# Installs the periscope binary from GitHub Releases plus ffmpeg, writes the config, creates the
# Start menu shortcuts and starts the sender once. It never starts by itself (no logon autostart):
# start and stop it from Start menu > Periscope. Windows can send in this version; viewing comes later.
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
$ps    = Join-Path $env:SystemRoot "System32\WindowsPowerShell\v1.0\powershell.exe"

function Write-Step([string]$msg) { Write-Host "==> " -ForegroundColor Cyan -NoNewline; Write-Host $msg }

Write-Host ""
Write-Host "   periscope installer" -ForegroundColor Cyan
Write-Host "   see another screen through your windows" -ForegroundColor DarkGray
Write-Host ""

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
# Earlier versions (v0.2 scripts, first v0.3 previews) started the sender at every logon through
# this task. The sender now runs only when asked, so the task goes away.
if (Get-ScheduledTask -TaskName $task -ErrorAction SilentlyContinue) {
    Write-Step "Removing the logon autostart (scheduled task $task)"
    Stop-ScheduledTask -TaskName $task -ErrorAction SilentlyContinue
    Unregister-ScheduledTask -TaskName $task -Confirm:$false
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
# The icon for the Start menu shortcuts (the exe itself carries none). Optional: fall back to the exe.
$ico = Join-Path $bin "periscope.ico"
try {
    Invoke-WebRequest -UseBasicParsing -Uri "$base/periscope.ico" -OutFile $ico
} catch {
    $ico = $exe
}
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

# --- 5. Start Menu shortcuts + PATH ---------------------------------------------
# The shortcuts run periscope through conhost --headless: no window, no taskbar entry, no PowerShell.
# No hotkey: common ones collide with other software (Ctrl+Alt+Shift+P opens Kaspersky's keyboard).
# The folder is recreated so shortcuts from an older install (with a hotkey) don't linger.
Write-Step "Creating Start menu shortcuts (folder Periscope)"
$menuDir = Join-Path ([Environment]::GetFolderPath("Programs")) "Periscope"
if (Test-Path $menuDir) { Remove-Item -Recurse -Force $menuDir }
New-Item -ItemType Directory -Force -Path $menuDir | Out-Null
$conhost = Join-Path $env:SystemRoot "System32\conhost.exe"
$wsh = New-Object -ComObject WScript.Shell
$shortcuts = @(
    @{ Name = "Periscope - Start or stop sender"; Target = $conhost; Args = "--headless `"$exe`" send toggle" },
    @{ Name = "Periscope - Start sender"; Target = $conhost; Args = "--headless `"$exe`" send start" },
    @{ Name = "Periscope - Stop sender"; Target = $conhost; Args = "--headless `"$exe`" send stop" },
    @{ Name = "Periscope - Status"; Target = $ps; Args = "-NoProfile -NoExit -Command `"& '$exe' send status`"" }
)
foreach ($sc in $shortcuts) {
    $lnk = $wsh.CreateShortcut((Join-Path $menuDir ($sc.Name + ".lnk")))
    $lnk.TargetPath = $sc.Target
    $lnk.Arguments = $sc.Args
    $lnk.WorkingDirectory = $bin
    $lnk.IconLocation = "$ico,0"
    $lnk.Save()
}

$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if (-not $userPath) { $userPath = "" }
if (($userPath -split ";") -notcontains $bin) {
    [Environment]::SetEnvironmentVariable("Path", (($userPath.TrimEnd(";") + ";" + $bin).TrimStart(";")), "User")
    Write-Step "Added $bin to your PATH (takes effect in new terminals)"
}

# --- 6. start once ---------------------------------------------------------------
# Same command as the "Start sender" shortcut, so the install proves the shortcut path works.
Write-Step "Starting the sender (this time only; afterwards use Start menu > Periscope)"
Start-Process -FilePath $conhost -ArgumentList "--headless `"$exe`" send start"
Start-Sleep -Seconds 5
& $exe send status
Write-Host ""
Write-Host "Done." -ForegroundColor Green -NoNewline
Write-Host " You can close this window. The sender runs hidden until you stop it, and does NOT"
Write-Host "start by itself after a restart. Start menu > Periscope: Start or stop, Start, Stop, Status."
Write-Host "Logs: $env:LOCALAPPDATA\periscope"
