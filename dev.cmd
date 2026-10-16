@echo off
REM Wrapper so the dev script can be launched from cmd.exe or by double-click,
REM regardless of the PowerShell execution policy.
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0dev.ps1" %*
