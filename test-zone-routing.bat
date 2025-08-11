@echo off
echo ========================================
echo      Zone路由功能测试脚本
echo ========================================

echo 构建测试客户端...
go build -o bin\client.exe .\cmd\client

if not exist "bin\client.exe" (
    echo 错误: client构建失败
    pause
    exit /b 1
)

echo.
echo 开始测试不同Zone的路由...
echo.

echo 测试 Zone 001 (OpenID: 15000) - 应该路由到 :9001
echo ----------------------------------------
bin\client.exe -openid=15000 -action=hello -server=localhost:8453

echo.
echo 测试 Zone 002 (OpenID: 30000) - 应该路由到 :9002  
echo ----------------------------------------
bin\client.exe -openid=30000 -action=hello -server=localhost:8453

echo.
echo 测试 Zone 003 (OpenID: 45000) - 应该路由到 :9003
echo ----------------------------------------
bin\client.exe -openid=45000 -action=hello -server=localhost:8453

echo.
echo 测试边界值 Zone 001 (OpenID: 10000) - 应该路由到 :9001
echo ----------------------------------------
bin\client.exe -openid=10000 -action=hello -server=localhost:8453

echo.
echo 测试边界值 Zone 002 (OpenID: 25000) - 应该路由到 :9002
echo ----------------------------------------
bin\client.exe -openid=25000 -action=hello -server=localhost:8453

echo.
echo 测试边界值 Zone 003 (OpenID: 40000) - 应该路由到 :9003
echo ----------------------------------------
bin\client.exe -openid=40000 -action=hello -server=localhost:8453

echo.
echo ========================================
echo      Zone路由测试完成
echo ========================================
echo 检查上面的日志，确认每个OpenID都路由到了正确的Zone服务器
echo.
pause