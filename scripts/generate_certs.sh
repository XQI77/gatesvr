#!/bin/bash

# TLS证书生成脚本
# 用于生成网关服务器的自签名证书

set -e

# 证书目录
CERT_DIR="certs"
CERT_FILE="$CERT_DIR/server.crt"
KEY_FILE="$CERT_DIR/server.key"

# 创建证书目录
mkdir -p "$CERT_DIR"

echo "正在生成TLS证书..."

# 生成私钥
openssl genpkey -algorithm RSA -pkcs8 -out "$KEY_FILE" -pkeyopt rsa_keygen_bits:2048

# 生成自签名证书
openssl req -new -x509 -key "$KEY_FILE" -out "$CERT_FILE" -days 365 -subj "/C=CN/ST=Beijing/L=Beijing/O=GateServer/OU=IT/CN=localhost" -addext "subjectAltName=DNS:localhost,DNS:127.0.0.1,IP:127.0.0.1,IP:::1"

echo "TLS证书生成完成："
echo "  证书文件: $CERT_FILE"
echo "  私钥文件: $KEY_FILE"
echo ""
echo "证书信息："
openssl x509 -in "$CERT_FILE" -text -noout | grep -A 3 "Subject:"
openssl x509 -in "$CERT_FILE" -text -noout | grep -A 3 "DNS:"

echo ""
echo "注意: 这是自签名证书，仅用于开发和测试。"
echo "在生产环境中，请使用由受信任的证书颁发机构签发的证书。" 