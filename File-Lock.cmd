@echo off
setlocal
chcp 65001 >nul
if not exist "%~dp0FileLock.exe" (
    echo FileLock.exe is missing. Extract the complete release ZIP first.
    pause
    exit /b 1
)
"%~dp0FileLock.exe"
if errorlevel 1 pause
