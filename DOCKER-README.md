# gatesvr Docker容器化部署指南

## 概述

本指南介绍如何使用Docker容器化部署gatesvr网关服务和相关上游服务。支持在Windows环境构建，Linux环境运行。

## 服务架构

```
┌─────────────────────────────────────────────────┐
│                Docker环境                        │
├─────────────────────────────────────────────────┤
│  ┌─────────────┐                                │
│  │   gatesvr   │ :8080(QUIC) :8090(HTTP)       │
│  │             │ :8092(gRPC) :9090(Metrics)    │
│  └─────┬───────┘                                │
│        │                                        │
│  ┌─────▼───┐  ┌─────────────┐  ┌──────────────┐│
│  │hellosvr │  │businesssvr  │  │   zonesvr    ││
│  │ :8081   │  │   :8082     │  │    :8083     ││
│  └─────────┘  └─────────────┘  └──────────────┘│
└─────────────────────────────────────────────────┘
```

## 快速开始

### 前置条件

- Docker >= 20.10
- docker-compose >= 1.29
- Windows/Linux 操作系统
- Git (用于获取构建信息)

### 1. 构建镜像

**Windows环境：**
```batch
# 构建所有服务镜像
.\build-docker.bat

# 或者使用环境变量自定义构建
set VERSION=v1.0.0
set CLEAN_BUILD=true
.\build-docker.bat
```

**Linux/macOS环境：**
```bash
# 构建所有服务镜像
chmod +x build-docker.sh
./build-docker.sh

# 或者使用环境变量自定义构建
VERSION=v1.0.0 CLEAN_BUILD=true ./build-docker.sh
```

### 2. 部署服务

**Windows环境：**
```batch
# 部署所有服务
.\deploy-docker.bat deploy

# 停止服务
.\deploy-docker.bat stop

# 查看服务状态
.\deploy-docker.bat status

# 查看日志
.\deploy-docker.bat logs [service]
```

**Linux/macOS环境：**
```bash
# 部署所有服务
chmod +x deploy-docker.sh
./deploy-docker.sh deploy

# 停止服务
./deploy-docker.sh stop

# 查看服务状态
./deploy-docker.sh status

# 查看日志
./deploy-docker.sh logs [service]
```

### 3. 验证部署

访问以下地址验证服务运行状态：

- **网关API**: http://localhost:8090/api/status
- **Hello服务**: http://localhost:8081/health
- **Business服务**: http://localhost:8082/health  
- **Zone服务**: http://localhost:8083/health

## 详细配置

### 环境变量

可通过环境变量自定义构建和部署：

```bash
# 构建相关
VERSION=v1.0.0              # 镜像版本标签
CLEAN_BUILD=true            # 是否清理构建缓存
REGISTRY=my-registry.com    # 镜像仓库地址
PUSH_IMAGES=true           # 是否推送到仓库

# 部署相关  
ENVIRONMENT=prod           # 部署环境
CLEAN_DEPLOY=true         # 是否清理部署环境
REMOVE_VOLUMES=true       # 停止时是否删除数据卷
```

### 服务端口映射

| 服务 | 容器端口 | 主机端口 | 说明 |
|------|----------|----------|------|
| gatesvr | 8080 | 8080 | QUIC网关服务 |
| gatesvr | 8090 | 8090 | HTTP API接口 |
| gatesvr | 8082 | 8092 | gRPC服务接口 |
| gatesvr | 9090 | 9090 | Prometheus指标 |
| hellosvr | 8081 | 8081 | Hello上游服务 |
| businesssvr | 8082 | 8082 | Business上游服务 |
| zonesvr | 8083 | 8083 | Zone上游服务 |

### 资源限制

每个服务都配置了资源限制：

- **gatesvr**: CPU 2核, 内存 1GB
- **上游服务**: CPU 0.5-1核, 内存 256-512MB

### 健康检查

所有服务都配置了健康检查：
- 检查间隔: 30秒
- 超时时间: 10秒
- 重试次数: 3次
- 启动等待: 40-60秒

## 日志管理

### 查看日志

```bash
# 查看所有服务日志
docker-compose logs -f

# 查看特定服务日志
docker-compose logs -f gatesvr
docker-compose logs -f hellosvr

# 查看最近的日志
docker-compose logs --tail=100 gatesvr
```

### 日志配置

日志文件自动滚动：
- 最大文件大小: 10-50MB
- 保留文件数: 3-5个
- 驱动类型: json-file

## 监控和诊断

### 服务状态检查

```bash
# 检查容器状态
docker-compose ps

# 检查容器健康状态
docker inspect gatesvr | grep -A 10 Health

# 检查资源使用
docker stats gatesvr hellosvr businesssvr zonesvr
```

### 网络诊断

```bash
# 进入容器调试
docker exec -it gatesvr sh

# 检查网络连通性
docker exec gatesvr ping hellosvr
docker exec gatesvr nslookup businesssvr

# 检查端口监听
docker exec gatesvr netstat -tlnp
```

## 故障排查

### 常见问题

1. **容器启动失败**
   ```bash
   # 查看容器启动日志
   docker-compose logs [service]
   
   # 检查镜像是否存在
   docker images | grep gatesvr
   ```

2. **服务无法访问**
   ```bash
   # 检查端口映射
   docker port gatesvr
   
   # 检查防火墙设置
   curl -v http://localhost:8090/api/status
   ```

3. **健康检查失败**
   ```bash
   # 手动执行健康检查
   docker exec gatesvr wget --spider http://localhost:8090/api/status
   
   # 检查服务内部状态
   docker exec gatesvr ./gatesvr -health-check
   ```

4. **内存不足**
   ```bash
   # 增加资源限制
   # 编辑docker-compose.yml中的resources配置
   
   # 清理无用资源
   docker system prune -f
   ```

### 重置环境

```bash
# 完全清理环境
docker-compose down -v --remove-orphans
docker system prune -f -a
docker volume prune -f

# 重新构建和部署
./build-docker.sh
./deploy-docker.sh deploy
```

## 生产环境部署

### 安全配置

1. **TLS证书**: 替换自签名证书为正式证书
2. **网络隔离**: 使用专用网络，限制外部访问
3. **用户权限**: 使用非root用户运行容器
4. **密钥管理**: 使用Docker secrets管理敏感信息

### 高可用配置

1. **多实例部署**:
   ```bash
   # 扩展上游服务实例
   docker-compose up -d --scale hellosvr=2 --scale businesssvr=3
   ```

2. **负载均衡**: 在多个gatesvr实例前添加负载均衡器

3. **数据持久化**: 配置适当的数据卷和备份策略

### 监控集成

集成Prometheus + Grafana监控：

```yaml
# monitoring/docker-compose.monitoring.yml
services:
  prometheus:
    image: prom/prometheus:latest
    ports: ["9091:9090"]
    
  grafana:
    image: grafana/grafana:latest  
    ports: ["3000:3000"]
```

## 开发和调试

### 本地开发模式

```bash
# 挂载源代码进行开发
docker-compose -f docker-compose.dev.yml up -d

# 热重载配置
# 将源码目录挂载到容器中
```

### 调试容器

```bash
# 进入容器shell
docker exec -it gatesvr sh

# 调试模式启动
docker run -it --rm gatesvr:latest sh

# 查看容器文件系统
docker exec gatesvr ls -la /app
```

## 性能优化

### 构建优化

- 使用多阶段构建减小镜像大小
- 利用构建缓存加速构建过程
- 使用Alpine Linux作为基础镜像

### 运行时优化

- 合理设置资源限制和预留
- 配置适当的健康检查间隔
- 使用专用网络减少延迟

## 支持和反馈

如遇到问题，请：

1. 查看本文档的故障排查部分
2. 检查容器日志: `docker-compose logs -f`
3. 提交问题时附上环境信息和错误日志

---

*最后更新: 2024-12-19*