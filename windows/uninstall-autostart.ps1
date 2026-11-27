& (Join-Path (Split-Path -Parent $MyInvocation.MyCommand.Path) "stop.ps1")
Unregister-ScheduledTask -TaskName "PeriscopeSender" -Confirm:$false -ErrorAction SilentlyContinue
Write-Host "Autostart removed."
