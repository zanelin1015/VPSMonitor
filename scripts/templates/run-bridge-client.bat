@echo off
set SCRIPT_DIR=%~dp0
"%SCRIPT_DIR%bridge-client.exe" -config "%SCRIPT_DIR%config\client.json"
