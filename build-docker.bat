@echo off
REM gatesvr Docker镜像构建脚本 (Windows版本)
setlocal enabledelayedexpansion

echo === gatesvr容器化镜像构建脚本 ===

REM 检查Docker是否运行
docker info >nul 2>&1
if %errorlevel% neq 0 (
    echo [ERROR] Docker未运行，请启动Docker
    pause
    exit /b 1
)

REM 检查docker-compose是否可用
docker-compose --version >nul 2>&1
if %errorlevel% neq 0 (
    echo [ERROR] docker-compose未安装，请安装docker-compose
    pause
    exit /b 1
)

REM 设置构建参数
for /f "tokens=1-4 delims=/ " %%a in ('date /t') do set BUILD_DATE=%%d-%%b-%%c
for /f "tokens=1-2 delims= " %%a in ('time /t') do set BUILD_TIME=%%a
set BUILD_DATE=%BUILD_DATE%T%BUILD_TIME%Z

REM 获取Git提交信息（如果可用）
git rev-parse --short HEAD >nul 2>&1
if %errorlevel% equ 0 (
    for /f %%i in ('git rev-parse --short HEAD') do set GIT_COMMIT=%%i
) else (
    set GIT_COMMIT=unknown
)

REM 设置默认版本
if not defined VERSION set VERSION=latest
if not defined REGISTRY set REGISTRY=

echo 构建信息:
echo   版本: %VERSION%
echo   构建时间: %BUILD_DATE%
echo   Git提交: %GIT_COMMIT%
echo   镜像仓库: %REGISTRY%

REM 导出环境变量
set VERSION=%VERSION%
set BUILD_DATE=%BUILD_DATE%
set GIT_COMMIT=%GIT_COMMIT%

REM 清理旧的构建缓存（如果设置了CLEAN_BUILD）
if "%CLEAN_BUILD%"=="true" (
    echo [INFO] 清理Docker构建缓存...
    docker system prune -f --volumes
)

REM 构建所有镜像
echo [INFO] 开始构建所有服务镜像...
docker-compose build --parallel
if %errorlevel% neq 0 (
    echo [ERROR] 镜像构建失败
    pause
    exit /b 1
)

echo [SUCCESS] 所有镜像构建完成

REM 显示构建结果
echo [INFO] 构建完成的镜像列表:
docker images | findstr /R "gatesvr hellosvr businesssvr zonesvr"

REM 标记镜像（如果指定了仓库）
if not "%REGISTRY%"=="" (
    echo [INFO] 标记镜像到仓库: %REGISTRY%
    
    for %%s in (gatesvr hellosvr businesssvr zonesvr) do (
        echo [INFO] 标记 %%s:%VERSION%
        docker tag %%s:%VERSION% %REGISTRY%/%%s:%VERSION%
        docker tag %%s:%VERSION% %REGISTRY%/%%s:latest
    )
    
    echo [SUCCESS] 镜像标记完成
)

REM 推送镜像（可选）
if "%PUSH_IMAGES%"=="true" (
    if not "%REGISTRY%"=="" (
        echo [INFO] 推送镜像到仓库...
        
        for %%s in (gatesvr hellosvr businesssvr zonesvr) do (
            echo [INFO] 推送 %REGISTRY%/%%s:%VERSION%
            docker push %REGISTRY%/%%s:%VERSION%
            docker push %REGISTRY%/%%s:latest
        )
    )
)

REM 镜像大小统计
echo [INFO] 镜像大小统计:
for %%s in (gatesvr hellosvr businesssvr zonesvr) do (
    for /f %%i in ('docker images %%s:%VERSION% --format "{{.Size}}"') do (
        echo   %%s: %%i
    )
)

echo [SUCCESS] === 构建脚本执行完成 ===
echo [INFO] 使用以下命令启动服务：
echo   docker-compose up -d
echo [INFO] 查看服务状态：
echo   docker-compose ps

pause