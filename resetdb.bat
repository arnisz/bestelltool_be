@echo off
cd /d "%~dp0"
echo Loescht die persistenten Entwicklungsdaten (Volume resource_pgdata).
choice /m "Fortfahren"
if errorlevel 2 exit /b 0
docker compose rm -sfv db
docker volume rm resource_pgdata 2>nul
docker compose up -d --wait db
if errorlevel 1 pause
