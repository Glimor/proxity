@echo off
rem Proxity release build - thin wrapper around build.ps1.
rem Usage: build.bat [-Version 0.1.0] [-NoInstaller]
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0build.ps1" %*
