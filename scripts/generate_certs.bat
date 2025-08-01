@echo off
REM TLS证书生成脚本 (Windows版本)
REM 用于生成网关服务器的自签名证书

setlocal enabledelayedexpansion

REM 证书目录
set CERT_DIR=certs
set CERT_FILE=%CERT_DIR%\server.crt
set KEY_FILE=%CERT_DIR%\server.key

REM 创建证书目录
if not exist "%CERT_DIR%" mkdir "%CERT_DIR%"

echo 正在生成TLS证书...

REM 检查OpenSSL是否可用
openssl version >nul 2>&1
if errorlevel 1 (
    echo 错误: 未找到OpenSSL命令。
    echo 请安装OpenSSL或确保它在PATH环境变量中。
    echo 可以从以下地址下载: https://slproweb.com/products/Win32OpenSSL.html
    pause
    exit /b 1
)

REM 生成私钥
openssl genpkey -algorithm RSA -pkcs8 -out "%KEY_FILE%" -pkeyopt rsa_keygen_bits:2048
if errorlevel 1 (
    echo 生成私钥失败
    pause
    exit /b 1
)

REM 生成自签名证书
openssl req -new -x509 -key "%KEY_FILE%" -out "%CERT_FILE%" -days 365 -subj "/C=CN/ST=Beijing/L=Beijing/O=GateServer/OU=IT/CN=localhost" -addext "subjectAltName=DNS:localhost,DNS:127.0.0.1,IP:127.0.0.1,IP:::1"
if errorlevel 1 (
    echo 生成证书失败
    pause
    exit /b 1
)

echo.
echo TLS证书生成完成：
echo   证书文件: %CERT_FILE%
echo   私钥文件: %KEY_FILE%
echo.
echo 证书信息：
openssl x509 -in "%CERT_FILE%" -text -noout | findstr "Subject:"
openssl x509 -in "%CERT_FILE%" -text -noout | findstr "DNS:"

echo.
echo 注意: 这是自签名证书，仅用于开发和测试。
echo 在生产环境中，请使用由受信任的证书颁发机构签发的证书。
echo.
pause 