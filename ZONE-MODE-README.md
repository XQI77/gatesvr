# Zone模式上游服务配置指南

## 概述

这个更新将gatesvr从基于功能的上游服务拆分（hello、business、zone）改为基于Zone的OpenID路由系统。每个上游服务处理特定Zone范围内的所有用户请求，确保同一用户的所有操作都在同一个上游服务中处理。

## Zone分配规则

OpenID范围：10000-99999，分配到6个zone：

| Zone ID | OpenID范围 | 用户数量 | 推荐端口 |
|---------|------------|----------|----------|
| 001     | 10000-24999| 15,000   | 9001     |
| 002     | 25000-39999| 15,000   | 9002     |
| 003     | 40000-54999| 15,000   | 9003     |
| 004     | 55000-69999| 15,000   | 9004     |
| 005     | 70000-84999| 15,000   | 9005     |
| 006     | 85000-99999| 15,000   | 9006     |

## 配置变更

### Gateway配置 (test-config.yaml)

```yaml
server:
  quic_addr: ":8453"
  http_addr: ":8080"
  grpc_addr: ":8092"    # 上游服务注册到此端口
  
  # 不再需要预配置upstream_services
  # 上游服务将通过gRPC RegisterUpstream自动注册
```

### 上游服务启动

新的启动方式需要指定Zone ID和Gateway地址：

```bash
# 启动Zone 001服务
./upstream --zone=001 --addr=:9001 --gateway=localhost:8092

# 启动Zone 002服务  
./upstream --zone=002 --addr=:9002 --gateway=localhost:8092

# 启动Zone 003服务
./upstream --zone=003 --addr=:9003 --gateway=localhost:8092
```

### 环境变量支持

```bash
export UPSTREAM_ZONE=001
export UPSTREAM_ADDR=:9001
export GATEWAY_ADDR=localhost:8092
./upstream
```

## 快速测试

### 1. 启动测试环境

```bash
# Windows
start-zone-test.bat

# Linux/Mac
chmod +x start-zone-test.sh && ./start-zone-test.sh
```

### 2. 测试Zone路由

```bash
# Windows
test-zone-routing.bat

# Linux/Mac  
chmod +x test-zone-routing.sh && ./test-zone-routing.sh
```

### 3. 手动测试

```bash
# 测试不同OpenID的路由
./client -openid=15000 -action=hello -server=localhost:8453  # -> Zone 001
./client -openid=30000 -action=hello -server=localhost:8453  # -> Zone 002
./client -openid=45000 -action=hello -server=localhost:8453  # -> Zone 003
```

## 架构变化

### 旧架构 (按功能拆分)
```
Client -> Gateway -> Hello Service (登录)
                  -> Business Service (业务)  
                  -> Zone Service (区域)
```

### 新架构 (按Zone拆分)
```
Client (OpenID: 15000) -> Gateway -> Zone 001 Service (处理所有操作)
Client (OpenID: 30000) -> Gateway -> Zone 002 Service (处理所有操作)
Client (OpenID: 45000) -> Gateway -> Zone 003 Service (处理所有操作)
```

## 核心优势

1. **会话一致性**: 同一用户的所有请求都路由到同一个上游服务
2. **状态管理**: 每个Zone服务可以维护自己用户的完整状态
3. **水平扩展**: 可以根据用户分布启动不同数量的Zone服务
4. **故障隔离**: 一个Zone的故障不会影响其他Zone的用户
5. **动态注册**: 上游服务自动注册，无需预配置

## 监控和调试

### 查看注册的上游服务
```bash
curl http://localhost:8080/stats
```

### 查看路由信息
Gateway日志会显示每个请求的路由目标：
```
路由业务请求 - 动作: hello, OpenID: 15000, 目标服务: Zone 001: localhost:9001
```

### 上游服务日志
每个Zone服务会显示从Gateway接收的注册确认：
```
成功注册到Gateway - Zone: 001, Address: :9001, 响应: 注册成功
```

## 故障排除

### 常见问题

1. **上游服务注册失败**
   - 确保Gateway已启动并监听8092端口
   - 检查Zone ID格式（必须是001-006）
   - 确认网络连通性

2. **路由失败**
   - 检查OpenID是否在有效范围内（10000-99999）
   - 确认对应Zone的上游服务已启动并注册
   - 查看Gateway日志中的路由信息

3. **Zone ID验证失败**  
   - Zone ID必须是3位数字：001, 002, 003, 004, 005, 006
   - 不能使用1, 01, 7等格式

### 调试命令

```bash
# 查看上游服务帮助
./upstream --help

# 健康检查
./upstream --health-check --addr=:9001

# 查看Gateway状态
curl http://localhost:8080/health
curl http://localhost:8080/stats
```

## 迁移指南

如果从旧版本迁移：

1. **备份现有配置**
2. **更新配置文件** - 移除upstream_services配置
3. **重新启动服务** - 使用新的Zone模式启动脚本
4. **验证路由** - 使用测试脚本确认路由正确

## 参考

- `pkg/zone/zone.go` - Zone计算算法
- `internal/upstream/manager.go` - 新的OpenID路由器
- `internal/gateway/upstream_register.go` - 注册接口实现
- `test-config.yaml` - Zone模式配置示例