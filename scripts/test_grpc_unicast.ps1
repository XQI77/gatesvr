# PowerShell脚本测试gRPC单播推送功能

Write-Host "=== 网关gRPC单播推送功能测试 ===" -ForegroundColor Green

# 检查网关是否启动
Write-Host "`n1. 检查网关服务状态..." -ForegroundColor Yellow
try {
    $response = Invoke-RestMethod -Uri "http://localhost:8081/health" -Method Get -TimeoutSec 5
    Write-Host "网关HTTP服务正常运行" -ForegroundColor Green
    $response | ConvertTo-Json
} catch {
    Write-Host "警告: 网关HTTP服务不可用: $($_.Exception.Message)" -ForegroundColor Yellow
    Write-Host "请确保网关服务已启动" -ForegroundColor Yellow
}

# 检查gRPC端口是否监听
Write-Host "`n2. 检查gRPC服务端口..." -ForegroundColor Yellow
try {
    $tcpConnection = Test-NetConnection -ComputerName localhost -Port 8082 -InformationLevel Quiet
    if ($tcpConnection) {
        Write-Host "gRPC服务端口8082正在监听" -ForegroundColor Green
    } else {
        Write-Host "警告: gRPC服务端口8082未监听" -ForegroundColor Red
    }
} catch {
    Write-Host "无法检查gRPC端口状态: $($_.Exception.Message)" -ForegroundColor Yellow
}

# 运行gRPC测试程序
Write-Host "`n3. 运行gRPC单播推送测试..." -ForegroundColor Yellow

# 编译测试程序
Write-Host "编译测试程序..." -ForegroundColor Cyan
try {
    Set-Location -Path "scripts"
    go build -o test_grpc_unicast.exe test_grpc_unicast.go
    Write-Host "测试程序编译成功" -ForegroundColor Green
} catch {
    Write-Host "编译失败: $($_.Exception.Message)" -ForegroundColor Red
    exit 1
} finally {
    Set-Location -Path ".."
}

# 运行测试程序
Write-Host "`n运行测试程序..." -ForegroundColor Cyan
try {
    & "scripts\test_grpc_unicast.exe"
    Write-Host "`n测试程序执行完成" -ForegroundColor Green
} catch {
    Write-Host "测试程序执行失败: $($_.Exception.Message)" -ForegroundColor Red
}

# 清理
Write-Host "`n4. 清理临时文件..." -ForegroundColor Yellow
if (Test-Path "scripts\test_grpc_unicast.exe") {
    Remove-Item "scripts\test_grpc_unicast.exe" -Force
    Write-Host "已清理临时文件" -ForegroundColor Green
}

Write-Host "`n=== 测试完成 ===" -ForegroundColor Green
Write-Host "`n说明:" -ForegroundColor Cyan
Write-Host "- 如果目标客户端不存在，推送将失败（这是正常行为）" -ForegroundColor Gray
Write-Host "- 要测试成功的推送，需要有真实的客户端连接到网关" -ForegroundColor Gray
Write-Host "- gRPC接口地址: localhost:8082" -ForegroundColor Gray 