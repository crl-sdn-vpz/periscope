@echo off
rem Starts the Periscope sender fully hidden (no console window, no taskbar entry).
rem Requires Windows 10 21H2+ or Windows 11 (conhost --headless).
start "" conhost.exe --headless "%SystemRoot%\System32\WindowsPowerShell\v1.0\powershell.exe" -NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File "%~dp0sender.ps1"
