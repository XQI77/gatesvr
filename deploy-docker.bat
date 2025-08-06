@echo off
REM gatesvr Docker服务部署脚本 (Windows版本)
setlocal enabledelayedexpansion

echo === gatesvr服务集群部署脚本 ===

REM 解析命令行参数
set ACTION=%1
set ENVIRONMENT=%2

if "%ACTION%"=="" set ACTION=deploy
if "%ENVIRONMENT%"=="" set ENVIRONMENT=local

REM 检查操作类型
if "%ACTION%"=="deploy" goto :check_docker
if "%ACTION%"=="start" goto :check_docker
if "%ACTION%"=="stop" goto :stop_services
if "%ACTION%"=="restart" goto :restart_services
if "%ACTION%"=="logs" goto :show_logs
if "%ACTION%"=="status" goto :show_status

echo [ERROR] 未知的操作: %ACTION%
echo 使用方法: %0 [deploy^|start^|stop^|restart^|logs^|status] [local^|dev^|prod]
pause
exit /b 1

:check_docker
REM 检查Docker和docker-compose
docker info >nul 2>&1
if %errorlevel% neq 0 (
    echo [ERROR] Docker未运行，请启动Docker
    pause
    exit /b 1
)

docker-compose --version >nul 2>&1
if %errorlevel% neq 0 (
    echo [ERROR] docker-compose未安装
    pause
    exit /b 1
)

REM 检查必要的文件
echo [INFO] 检查必要文件...
if not exist "docker-compose.yml" (
    echo [ERROR] 缺少文件: docker-compose.yml
    pause
    exit /b 1
)

if not exist "docker\config\docker-config.yaml" (
    echo [ERROR] 缺少文件: docker\config\docker-config.yaml
    pause
    exit /b 1
)

if not exist "certs\server.crt" (
    echo [WARNING] 缺少证书文件，正在生成自签名证书...
    if not exist "certs" mkdir certs
    openssl req -x509 -newkey rsa:4096 -keyout certs\server.key -out certs\server.crt -days 365 -nodes -subj "/CN=localhost" >nul 2>&1
    if %errorlevel% equ 0 (
        echo [SUCCESS] 自签名证书生成完成
    ) else (
        echo [ERROR] 证书生成失败，请手动创建证书文件
        pause
        exit /b 1
    )
)

REM 设置环境变量
if not defined VERSION set VERSION=latest
for /f "tokens=1-4 delims=/ " %%a in ('date /t') do set BUILD_DATE=%%d-%%b-%%cT
for /f "tokens=1-2 delims= " %%a in ('time /t') do set BUILD_DATE=!BUILD_DATE!%%aZ

git rev-parse --short HEAD >nul 2>&1
if %errorlevel% equ 0 (
    for /f %%i in ('git rev-parse --short HEAD') do set GIT_COMMIT=%%i
) else (
    set GIT_COMMIT=unknown
)

echo [INFO] 部署环境: %ENVIRONMENT%
echo [INFO] 镜像版本: %VERSION%

goto :deploy_services

:deploy_services
echo [INFO] 启动gatesvr服务集群...

REM 停止现有服务
echo [INFO] 停止现有服务...
docker-compose down --remove-orphans >nul 2>&1

REM 清理环境（如果设置了CLEAN_DEPLOY）
if "%CLEAN_DEPLOY%"=="true" (
    echo [INFO] 清理环境...
    docker system prune -f >nul 2>&1
)

REM 启动服务
echo [INFO] 启动服务...
docker-compose up -d
if %errorlevel% neq 0 (
    echo [ERROR] 服务集群启动失败
    pause
    exit /b 1
)

echo [SUCCESS] 服务集群启动成功

REM 等待服务启动
echo [INFO] 等待服务启动...
timeout /t 15 /nobreak >nul

REM 检查服务状态
echo [INFO] 检查服务状态...
docker-compose ps

REM 健康检查
call :check_services_health

REM 显示访问信息
call :show_access_info

goto :end

:stop_services
echo [INFO] 停止gatesvr服务集群...
docker-compose down --remove-orphans
if "%REMOVE_VOLUMES%"=="true" (
    docker-compose down -v
    echo [INFO] 已删除数据卷
)
echo [SUCCESS] 服务集群已停止
goto :end

:restart_services
echo [INFO] 重启gatesvr服务集群...
docker-compose restart
timeout /t 10 /nobreak >nul
docker-compose ps
call :check_services_health
echo [SUCCESS] 服务集群重启完成
goto :end

:show_logs
set SERVICE=%3
if "%SERVICE%"=="" (
    echo [INFO] 显示所有服务的日志...
    docker-compose logs -f
) else (
    echo [INFO] 显示服务 %SERVICE% 的日志...
    docker-compose logs -f %SERVICE%
)
goto :end

:show_status
echo [INFO] 服务状态:
docker-compose ps
echo.
echo [INFO] 网络信息:
docker network ls | findstr gatesvr
echo.
echo [INFO] 数据卷信息:
docker volume ls | findstr gatesvr
goto :end

:check_services_health
echo [INFO] 执行健康检查...

REM 检查hellosvr
curl -f -s "http://localhost:8081/health" >nul 2>&1
if %errorlevel% equ 0 (
    echo [SUCCESS] ✓ hellosvr 运行正常
) else (
    echo [ERROR] ✗ hellosvr 启动异常
)

REM 检查businesssvr
curl -f -s "http://localhost:8082/health" >nul 2>&1
if %errorlevel% equ 0 (
    echo [SUCCESS] ✓ businesssvr 运行正常
) else (
    echo [ERROR] ✗ businesssvr 启动异常
)

REM 检查zonesvr
curl -f -s "http://localhost:8083/health" >nul 2>&1
if %errorlevel% equ 0 (
    echo [SUCCESS] ✓ zonesvr 运行正常
) else (
    echo [ERROR] ✗ zonesvr 启动异常
)

REM 检查gatesvr
curl -f -s "http://localhost:8090/api/status" >nul 2>&1
if %errorlevel% equ 0 (
    echo [SUCCESS] ✓ gatesvr 运行正常
) else (
    echo [ERROR] ✗ gatesvr 启动异常
)

goto :eof

:show_access_info
echo.
echo [SUCCESS] === 服务访问信息 ===
echo 网关服务 (QUIC):    localhost:8080
echo 网关API:            http://localhost:8090
echo 网关gRPC:           localhost:8092
echo 监控指标:           http://localhost:9090
echo.
echo 上游服务:
echo   Hello服务:        http://localhost:8081
echo   Business服务:     http://localhost:8082
echo   Zone服务:         http://localhost:8083
echo.
echo 常用命令:
echo   查看状态:         docker-compose ps
echo   查看日志:         docker-compose logs -f [service]
echo   停止服务:         docker-compose down
echo   重启服务:         docker-compose restart [service]
goto :eof

:end
pause