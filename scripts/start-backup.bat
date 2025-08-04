@echo off
echo Starting Backup GateServer...

cd /d "%~dp0.."

.\bin\gatesvr.exe -config config-backup.yaml

pause