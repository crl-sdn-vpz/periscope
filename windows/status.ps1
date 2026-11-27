# Shows whether the sender is running and the last log lines.
$stateDir = Join-Path $env:LOCALAPPDATA "periscope"
$ff = Get-CimInstance Win32_Process -Filter "Name = 'ffmpeg.exe'" |
      Where-Object { $_.CommandLine -like "*service_name=periscope*" }
$pidFile = Join-Path $stateDir "sender.pid"
$senderUp = (Test-Path $pidFile) -and (Get-Process -Id (Get-Content $pidFile) -ErrorAction SilentlyContinue)

Write-Host ("Sender loop : {0}" -f ($(if ($senderUp) { "running" } else { "stopped" })))
Write-Host ("Streaming   : {0}" -f ($(if ($ff) { "yes (ffmpeg pid $($ff.ProcessId))" } else { "no (waiting for viewer or stopped)" })))
Write-Host ""
Write-Host "--- sender.log ---"
Get-Content (Join-Path $stateDir "sender.log") -Tail 8 -ErrorAction SilentlyContinue
Write-Host "--- ffmpeg.log ---"
Get-Content (Join-Path $stateDir "ffmpeg.log") -Tail 8 -ErrorAction SilentlyContinue
