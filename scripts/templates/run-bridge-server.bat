@echo off
set SCRIPT_DIR=%~dp0
"%SCRIPT_DIR%bridge-server.exe" -config "%SCRIPT_DIR%config\server.json"
