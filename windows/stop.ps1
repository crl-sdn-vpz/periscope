# Stops the Periscope sender and its ffmpeg process.
$stateDir = Join-Path $env:LOCALAPPDATA "periscope"
New-Item -ItemType Directory -Force -Path $stateDir | Out-Null
New-Item -ItemType File -Force -Path (Join-Path $stateDir "stop") | Out-Null

Get-CimInstance Win32_Process -Filter "Name = 'ffmpeg.exe'" |
    Where-Object { $_.CommandLine -like "*service_name=periscope*" } |
    ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }

$pidFile = Join-Path $stateDir "sender.pid"
if (Test-Path $pidFile) {
    $senderPid = Get-Content $pidFile
    Start-Sleep -Milliseconds 800
    Stop-Process -Id $senderPid -Force -ErrorAction SilentlyContinue
    Remove-Item $pidFile -ErrorAction SilentlyContinue
}
Write-Host "Periscope sender stopped."
