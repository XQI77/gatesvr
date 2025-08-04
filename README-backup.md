# 热备份功能使用说明

## 功能概述

热备份功能为gatesvr提供了高可用性支持，通过主备服务器架构实现故障自动切换和数据同步。

## 核心特性

- **实时数据同步**：会话状态和消息队列状态实时同步
- **自动故障检测**：基于心跳检测的故障发现机制
- **快速故障切换**：10秒内完成主备切换
- **会话保持**：95%以上的会话可无缝切换
- **性能影响最小**：主服务器性能损失<5%

## 部署架构

```
客户端群 → 负载均衡器 → 主服务器(primary) ←→ 备份服务器(backup)
                    ↓                    ↓
                上游服务              上游服务(standby)
```

## 启动参数

### 主服务器启动
```bash
./gatesvr.exe \
  -backup-enable \
  -backup-mode primary \
  -peer-addr backup-server:8444 \
  -server-id primary-1 \
  -heartbeat-interval 2s \
  -sync-batch-size 50 \
  -sync-timeout 200ms
```

### 备份服务器启动
```bash
./gatesvr.exe \
  -backup-enable \
  -backup-mode backup \
  -peer-addr primary-server:8443 \
  -server-id backup-1 \
  -heartbeat-interval 2s \
  -readonly
```

## 配置参数说明

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-backup-enable` | false | 启用热备份功能 |
| `-backup-mode` | primary | 服务器模式：primary/backup |
| `-peer-addr` | - | 对端服务器地址（必须） |
| `-server-id` | hostname | 服务器唯一标识 |
| `-heartbeat-interval` | 2s | 心跳检测间隔 |
| `-sync-batch-size` | 50 | 同步批次大小 |
| `-sync-timeout` | 200ms | 同步超时时间 |
| `-readonly` | false | 备份服务器只读模式 |

## 运行模式

### 1. 主服务器模式 (Primary)
- 接受客户端连接
- 处理业务请求
- 向备份服务器同步数据
- 发送心跳检测

### 2. 备份服务器模式 (Backup)
- 只读模式，不接受客户端连接
- 接收主服务器的数据同步
- 监控主服务器心跳
- 故障时自动切换为主模式

## 故障切换流程

1. **故障检测**：连续3次心跳失败（6秒）
2. **切换决策**：备份服务器检测到故障
3. **模式切换**：备份服务器切换为主模式
4. **会话恢复**：基于同步数据恢复会话状态
5. **服务重启**：开始接受客户端连接

## 数据同步策略

### 会话数据同步
- **触发时机**：会话创建、激活、删除
- **同步内容**：会话基本信息、状态、序列号
- **同步方式**：实时同步 + 定期校验

### 消息队列同步
- **触发时机**：消息发送、确认状态变更
- **同步内容**：队列元数据、待确认消息
- **同步方式**：批量同步（50条/200ms）

### 数据校验
- **校验和验证**：CRC32校验确保数据完整性
- **序列号校验**：确保消息顺序一致性
- **定期全量校验**：每5分钟进行一次数据校验

## 性能指标

### 可用性指标
- **故障恢复时间**：< 10秒
- **数据丢失率**：< 1%
- **会话保持率**：> 95%

### 性能影响
- **主服务器CPU影响**：< 5%
- **网络带宽占用**：< 10%
- **内存增加**：< 20%

## 监控指标

### HTTP API端点
```bash
# 健康检查
curl http://localhost:8080/health

# 备份统计
curl http://localhost:8080/backup/stats

# 手动切换（管理接口）
curl -X POST http://localhost:8080/backup/switch?mode=primary
```

### 关键监控指标
- 心跳状态和延迟
- 同步成功率和错误计数
- 故障切换次数和时长
- 会话同步延迟

## 故障排除

### 常见问题

1. **连接失败**
   - 检查网络连通性
   - 验证对端地址配置
   - 确认防火墙设置

2. **同步延迟过高**
   - 调整sync-timeout参数
   - 检查网络带宽
   - 优化sync-batch-size

3. **频繁切换**
   - 增加heartbeat-interval
   - 检查服务器负载
   - 验证网络稳定性

### 日志分析
```bash
# 查看备份相关日志
grep "备份\|同步\|心跳\|故障切换" gatesvr.log

# 监控同步统计
grep "同步统计\|心跳统计" gatesvr.log
```

## 最佳实践

1. **网络配置**
   - 使用专用网络进行数据同步
   - 配置合适的带宽和延迟
   - 启用网络冗余

2. **参数调优**
   - 根据业务量调整sync-batch-size
   - 根据网络条件调整超时参数
   - 定期监控性能指标

3. **运维管理**
   - 定期演练故障切换
   - 监控同步状态和延迟
   - 及时处理告警信息

## 使用示例

### 1. 本地测试环境
```bash
# 启动主服务器
./scripts/start-primary.bat

# 启动备份服务器
./scripts/start-backup.bat
```

### 2. 生产环境部署
```bash
# 服务器A（主）
./gatesvr -backup-enable -backup-mode primary -peer-addr 192.168.1.102:8444 -server-id prod-primary

# 服务器B（备）
./gatesvr -backup-enable -backup-mode backup -peer-addr 192.168.1.101:8443 -server-id prod-backup -readonly
```

### 3. 手动切换测试
```bash
# 切换备份服务器为主模式
curl -X POST http://backup-server:8090/backup/switch?mode=primary

# 查看切换状态
curl http://backup-server:8090/backup/stats
```