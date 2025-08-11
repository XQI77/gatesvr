@echo off
echo ========================================
echo     启动Zone模式测试环境
echo ========================================

echo 1. 构建项目...
go build -o bin\gatesvr.exe .\cmd\gatesvr
go build -o bin\upstream.exe .\cmd\upstream

if not exist "bin\gatesvr.exe" (
    echo 错误: gatesvr构建失败
    pause
    exit /b 1
)

if not exist "bin\upstream.exe" (
    echo 错误: upstream构建失败
    pause
    exit /b 1
)

echo 2. 启动Gateway服务器...
start "Gateway" cmd /c "bin\gatesvr.exe -config test-config.yaml & pause"

echo 等待Gateway启动...
timeout /t 3 /nobreak >nul

echo 3. 启动上游服务 (Zone模式)...

echo 启动Zone 001 (OpenID 10000-24999)...
start "Upstream Zone 001" cmd /c "bin\upstream.exe --zone=001 --addr=:9001 --gateway=localhost:8092 & pause"

echo 启动Zone 002 (OpenID 25000-39999)...
start "Upstream Zone 002" cmd /c "bin\upstream.exe --zone=002 --addr=:9002 --gateway=localhost:8092 & pause"

echo 启动Zone 003 (OpenID 40000-54999)...
start "Upstream Zone 003" cmd /c "bin\upstream.exe --zone=003 --addr=:9003 --gateway=localhost:8092 & pause"

echo 等待上游服务注册...
timeout /t 2 /nobreak >nul

echo ========================================
echo   Zone模式测试环境启动完成!
echo ========================================
echo.
echo 测试说明:
echo - Gateway: http://localhost:8080 (监控API)
echo - Gateway QUIC: :8453
echo - Gateway gRPC: :8092 (供上游注册)
echo.
echo Zone分配:
echo - Zone 001: OpenID 10000-24999 (端口9001)
echo - Zone 002: OpenID 25000-39999 (端口9002)
echo - Zone 003: OpenID 40000-54999 (端口9003)
echo.
echo 测试用户:
echo - OpenID 15000 -> Zone 001
echo - OpenID 30000 -> Zone 002  
echo - OpenID 45000 -> Zone 003
echo.
echo 按任意键退出...
pause >nul