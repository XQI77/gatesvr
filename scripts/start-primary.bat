@echo off
echo Starting Primary GateServer...

cd /d "%~dp0.."

.\bin\gatesvr.exe -config config.yaml

pause