@echo off
cd /d "%~dp0"
docker compose up -d --wait db
if errorlevel 1 pause
