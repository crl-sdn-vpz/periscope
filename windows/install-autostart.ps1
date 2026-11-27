# Registers a hidden per-user scheduled task that starts the sender at logon.
# Run once:  powershell -ExecutionPolicy Bypass -File .\install-autostart.ps1
$here   = Split-Path -Parent $MyInvocation.MyCommand.Path
$script = Join-Path $here "sender.ps1"
$ps     = Join-Path $env:SystemRoot "System32\WindowsPowerShell\v1.0\powershell.exe"

$action   = New-ScheduledTaskAction -Execute "conhost.exe" `
            -Argument "--headless `"$ps`" -NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File `"$script`""
$trigger  = New-ScheduledTaskTrigger -AtLogOn -User $env:USERNAME
$settings = New-ScheduledTaskSettingsSet -Hidden -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries `
            -ExecutionTimeLimit ([TimeSpan]::Zero) -MultipleInstances IgnoreNew
$principal = New-ScheduledTaskPrincipal -UserId $env:USERNAME -LogonType Interactive -RunLevel Limited

Register-ScheduledTask -TaskName "PeriscopeSender" -Action $action -Trigger $trigger `
    -Settings $settings -Principal $principal -Force | Out-Null
Write-Host "Installed. It starts at next logon; to start now:  Start-ScheduledTask -TaskName PeriscopeSender"
