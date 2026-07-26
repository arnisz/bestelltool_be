@echo off
cd /d "%~dp0"
docker compose --profile test up -d --force-recreate --wait db-test
if errorlevel 1 (pause & exit /b 1)
echo.
echo Leere Test-DB bereit auf 127.0.0.1:5433/resource_test
