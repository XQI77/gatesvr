# 快速开始指南

## 🎉 项目构建完成

您的高性能网关服务器项目已经成功构建！以下是所有生成的文件：

### 📁 项目结构

```
gatesvr/
├── bin/                    # 二进制文件
│   ├── gatesvr.exe        # 网关服务器
│   ├── upstream.exe       # 上游服务
│   ├── client.exe         # 客户端
│   ├── monitor.exe        # 性能监控工具 (新增)
│   └── gencerts.exe       # 证书生成工具 (新增)
├── certs/                 # TLS证书
│   ├── server.crt         # 服务器证书
│   └── server.key         # 服务器私钥
├── cmd/                   # 源代码
│   ├── gatesvr/          # 网关服务器
│   ├── upstream/         # 上游服务  
│   ├── client/           # 客户端
│   ├── monitor/          # 性能监控工具
│   └── gencerts/         # 证书生成工具
└── scripts/              # 工具脚本
    ├── test_performance.sh # 性能测试脚本 (新增)
    ├── demo.sh            # 演示脚本
    ├── generate_certs.sh  # Shell证书生成脚本
    └── generate_certs.bat # Batch证书生成脚本
```

## 🚀 快速启动（推荐）

### 方法一：使用Makefile（推荐）

```powershell
# 如果支持make命令
make run-upstream    # 启动上游服务
make run-gatesvr     # 启动网关服务器（新终端）
make run-client      # 启动客户端（新终端）
```

### 方法二：手动启动

1. **启动上游服务**

   ```powershell
   # 终端1：启动上游服务
   .\bin\upstream.exe -addr :9000
   ```
2. **启动网关服务器**

   ```powershell
   # 终端2：启动网关服务器（等待上游服务启动）
   .\bin\gatesvr.exe -quic :8443 -http :8080 -upstream localhost:9000
   ```
3. **客户端测试**

   ```powershell
   # 终端3：基础功能测试
   .\bin\client.exe -server localhost:8443 -test echo -data "Hello World!"

   # 交互模式
   .\bin\client.exe -server localhost:8443 -interactive

   # 持续测试模式（长连接，每秒发送请求）
   .\bin\client.exe -server localhost:8443
   ```

### 方法三：一键启动脚本

```powershell
# 自动启动所有服务并运行测试
Start-Process powershell -ArgumentList "-NoExit", "-Command", "cd '$(Get-Location)'; .\bin\upstream.exe -addr :9000"
Start-Sleep 3
Start-Process powershell -ArgumentList "-NoExit", "-Command", "cd '$(Get-Location)'; .\bin\gatesvr.exe -quic :8443 -http :8080 -upstream localhost:9000"
Start-Sleep 3
.\bin\client.exe -server localhost:8443 -test echo -data "快速启动测试"
```

## 🎯 客户端功能测试

### 1. 基础功能测试

```powershell

# 回显测试
.\bin\client.exe -server localhost:8443 -test echo -data "测试消息"

# 时间查询
.\bin\client.exe -server localhost:8443 -test time

# 问候测试  
.\bin\client.exe -server localhost:8443 -test hello

# 数学计算
.\bin\client.exe -server localhost:8443 -test calculate -data '{"operation":"add","a":"10","b":"20"}'

# 服务状态
.\bin\client.exe -server localhost:8443 -test status

  性能+重连混合测试：
  # 如果需要测试重连功能的性能影响
  ./bin/client.exe -server localhost:8443
  -performance -clients 5 -enable-reconnect
```

### 2. 长连接和持续测试（新功能）

```powershell
# 默认模式：长连接持续测试（每秒发送随机请求）
.\bin\client.exe -server localhost:8443

# 交互模式
.\bin\client.exe -server localhost:8443 -interactive

# 指定测试次数
.\bin\client.exe -server localhost:8443 -test echo -count 10 -interval 500ms
```

### 3. 性能测试（新功能）

```powershell
# 单客户端性能测试
.\bin\client.exe -server localhost:8443 -performance -max-requests 100

# 多客户端并发测试（10个客户端，每200ms发送一次请求）
.\bin\client.exe -server localhost:8443 -performance -clients 100 -request-interval 200ms

# 使用Makefile快捷命令
make load-test        # 5个并发客户端负载测试
make continuous-test  # 单客户端持续测试
```

## 📊 性能监控（新功能）

### 1. 使用监控工具

```powershell
# 单次查询性能数据
.\bin\monitor.exe -server localhost:8080

# 持续监控模式（每5秒更新）
.\bin\monitor.exe -server localhost:8080 -continuous -interval 5s

# JSON格式输出
.\bin\monitor.exe -server localhost:8080 -format json

# 使用Makefile快捷命令
make monitor
```

### 2. HTTP API监控

```powershell
# 详细性能指标（新增）
Invoke-RestMethod http://localhost:8080/performance

# 会话统计
Invoke-RestMethod http://localhost:8080/stats

# 健康检查
Invoke-RestMethod http://localhost:8080/health

# Prometheus指标
Invoke-RestMethod http://localhost:9090/metrics
```

### 3. 自动化性能测试

```bash
# 如果有bash环境
chmod +x scripts/test_performance.sh
./scripts/test_performance.sh --auto      # 自动化测试
./scripts/test_performance.sh --monitor  # 监控模式
./scripts/test_performance.sh            # 交互式菜单

# 使用Makefile
make perf-test
```

## 🔄 上游广播功能测试

### 基础上游广播测试

```powershell
# 启动多个客户端（保持连接）
# 在不同终端中运行：
.\bin\client.exe -server localhost:8443
.\bin\client.exe -server localhost:8443
.\bin\client.exe -server localhost:8443

# 上游服务会自动发送定时广播消息
# 观察客户端接收到的广播消息
# 广播消息格式示例: "收到广播消息: 定时广播消息 - 当前时间: 2024-01-01 12:00:00"
```

### 上游广播监控

```powershell
# 监控上游广播消息的接收情况
# 启动客户端后，上游服务会定时发送广播消息
.\bin\client.exe -server localhost:8443

# 可以观察客户端收到的广播消息日志
```

## 📈 性能指标说明（新功能）

### 关键指标

监控工具会显示以下性能指标：

- **QPS**: 每秒查询数 (Queries Per Second)
- **活跃连接数**: 当前连接的客户端数量
- **成功率**: 请求成功的百分比
- **平均延迟**: 请求平均响应时间
- **P95/P99延迟**: 延迟的95%和99%百分位数
- **吞吐量**: 网络数据传输速率 (MB/s)
- **总字节数**: 累计传输的数据量

### 实时监控示例

```
[性能统计] QPS: 45.2, 成功率: 100.0%, 平均延迟: 2.15ms, P95: 3.8ms
```

## 🔧 故障排除

### 常见问题

1. **端口占用错误**

   ```powershell
   # 检查端口占用
   netstat -ano | findstr :8443    # QUIC端口
   netstat -ano | findstr :9000    # 上游服务端口
   netstat -ano | findstr :8080    # HTTP API端口
   netstat -ano | findstr :9090    # 监控端口

   # 如果端口被占用，可以更改端口
   .\bin\gatesvr.exe -quic :18443 -http :18080 -upstream localhost:9000
   ```
2. **证书问题**

   ```powershell
   # 使用新的证书生成工具
   .\bin\gencerts.exe

   # 或使用Makefile
   make certs
   ```
3. **连接超时或失败**

   - 确保按顺序启动：上游服务 → 网关服务器 → 客户端
   - 等待每个服务完全启动（约3-5秒）
   - 检查防火墙设置
4. **性能测试工具无法运行**

   ```powershell
   # 确保工具已构建
   make tools

   # 手动构建
   go build -o bin/monitor.exe ./cmd/monitor
   go build -o bin/gencerts.exe ./cmd/gencerts
   ```

### 启动顺序很重要！

1. ✅ **上游服务** (端口 9000) - 提供gRPC业务逻辑
2. ✅ **网关服务器** (端口 8443, 8080, 9090) - 等待2-3秒
3. ✅ **客户端测试** - 等待2-3秒，确保网关完全启动

## 🎮 演示场景

### 场景1：基础功能验证

```powershell
# 启动所有服务后运行
.\bin\client.exe -server localhost:8443 -test echo -data "Hello GateSvr!"
.\bin\client.exe -server localhost:8443 -test time
.\bin\client.exe -server localhost:8443 -test calculate -data '{"operation":"multiply","a":"6","b":"7"}'
```

### 场景2：长连接持续测试（新功能）

```powershell
# 启动持续测试客户端（会一直运行）
.\bin\client.exe -server localhost:8443

# 在另一个终端启动监控工具
.\bin\monitor.exe -server localhost:8080 -continuous
```

### 场景3：多客户端性能测试（新功能）

```powershell
# 启动5个并发客户端进行性能测试
.\bin\client.exe -server localhost:8443 -performance -clients 100 -request-interval 200ms

# 同时在另一个终端监控性能
.\bin\monitor.exe -server localhost:8080 -continuous -interval 2s
```

### 场景4：上游广播功能演示

```powershell
# 在3个不同终端启动客户端
.\bin\client.exe -server localhost:8443    # 终端1
.\bin\client.exe -server localhost:8443    # 终端2  
.\bin\client.exe -server localhost:8443    # 终端3

# 上游服务会自动向所有连接的客户端发送定时广播消息
# 观察各个客户端终端中的广播消息接收日志
```

## ✨ 项目特性验证

✅ **QUIC通信**: 客户端通过QUIC协议连接网关
✅ **gRPC转发**: 网关将请求转发给上游gRPC服务
✅ **会话管理**: 每个客户端获得唯一会话ID
✅ **长连接支持**: 客户端保持持久连接，持续通信 (新增)
✅ **ACK机制**: 实现消息可靠投递确认
✅ **上游广播功能**: 上游服务向所有客户端广播消息
✅ **性能监控**: 实时QPS、延迟、吞吐量监控 (新增)
✅ **多客户端测试**: 支持并发客户端性能测试 (新增)
✅ **监控工具**: 专用性能监控工具 (新增)
✅ **TLS安全**: 自签名证书保护通信

## 🛠️ 高级用法

### 自定义配置启动

```powershell
# 自定义端口启动网关
.\bin\gatesvr.exe -quic :18443 -http :18080 -metrics :19090 -upstream localhost:9000

# 自定义客户端ID
.\bin\client.exe -server localhost:8443 -id "test-client-001"

# 心跳间隔设置
.\bin\client.exe -server localhost:8443 -heartbeat 10s
```

### 批量测试脚本

```powershell
# 创建批量测试脚本 test_batch.ps1
for ($i=1; $i -le 50; $i++) {
    $result = .\bin\client.exe -server localhost:8443 -test echo -data "Batch test $i"
    Write-Host "Test $i completed: $result"
}
```

---

🎊 **恭喜！您的高性能网关服务器已经构建完成并可以使用了！**

现在您拥有了一个功能完整、性能优秀的高并发网关系统，支持：

- 🚀 长连接持续通信
- 📊 实时性能监控
- 🔄 多客户端并发测试
- 📈 详细的性能指标
- 🎯 可靠的消息投递
