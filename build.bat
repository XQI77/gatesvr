@echo off
echo 正在构建高性能网关服务器...
echo.

echo [1/5] 构建上游服务...
go build -o bin\upstream.exe .\cmd\upstream
if errorlevel 1 goto error

echo [2/5] 构建网关服务器...
go build -o bin\gatesvr.exe .\cmd\gatesvr
if errorlevel 1 goto error

echo [3/5] 构建客户端...
go build -o bin\client.exe .\cmd\client
if errorlevel 1 goto error

echo [4/5] 构建性能监控工具...
go build -o bin\monitor.exe .\cmd\monitor
if errorlevel 1 goto error

echo [5/5] 构建证书生成工具...
go build -o bin\gencerts.exe .\cmd\gencerts
if errorlevel 1 goto error

echo.
echo ========================================
echo           构建成功完成！
echo ========================================
echo 可执行文件位置：
echo   上游服务:     bin\upstream.exe
echo   网关服务器:   bin\gatesvr.exe
echo   客户端:       bin\client.exe
echo   性能监控:     bin\monitor.exe
echo   证书生成:     bin\gencerts.exe
echo.
echo 快速启动命令：
echo   bin\upstream.exe -addr :9000
echo   bin\gatesvr.exe -quic :8443 -http :8080 -upstream localhost:9000
echo   bin\client.exe -server localhost:8443 -interactive
echo.
goto end

:error
echo.
echo ========================================
echo           构建失败！
echo ========================================
echo 请检查Go环境和源代码是否正确
pause
exit /b 1

:end
pause 