# Go pprof 性能分析使用指南

本项目已集成Go原生pprof工具，替代了复杂的自定义性能监控系统。

## 快速开始

### 1. 启动服务器
```bash
./gatesvr
```

### 2. 访问pprof Web界面
```bash
# 浏览器访问
http://localhost:8080/debug/pprof/
```

## 主要功能

### CPU分析
```bash
# 30秒CPU采样分析
go tool pprof http://localhost:8080/debug/pprof/profile?seconds=30

# 进入交互模式后常用命令：
# top10    - 显示CPU占用最高的10个函数
# list main - 查看main函数的详细信息
# web      - 生成调用图（需要graphviz）
# quit     - 退出
```

### 内存分析
```bash
# 内存使用分析
go tool pprof http://localhost:8080/debug/pprof/heap

# 交互模式命令：
# top10         - 内存占用最高的10个函数
# list funcName - 查看具体函数的内存分配
# web          - 内存分配调用图
```

### 协程分析（解决协程过多问题）
```bash
# 协程堆栈分析
go tool pprof http://localhost:8080/debug/pprof/goroutine

# 关键命令：
# top20        - 显示创建最多协程的函数
# traces       - 显示协程创建的调用栈
# list handleConnection - 查看连接处理函数
```

### 阻塞分析
```bash
# 需要先启用阻塞分析
# 在代码中添加：runtime.SetBlockProfileRate(1)

go tool pprof http://localhost:8080/debug/pprof/block

# 找出锁竞争和阻塞点
# top10 - 阻塞时间最长的函数
```

### 锁竞争分析
```bash
# 需要先启用锁分析
# 在代码中添加：runtime.SetMutexProfileFraction(1)

go tool pprof http://localhost:8080/debug/pprof/mutex

# 找出锁竞争热点
# top10 - 锁竞争最严重的位置
```

## 性能问题排查流程

### 1. CPU占用过高
```bash
# 1. CPU分析（30秒采样）
go tool pprof http://localhost:8080/debug/pprof/profile?seconds=30

# 2. 在pprof交互模式中：
(pprof) top20
(pprof) list handleMessageEvent  # 查看消息处理函数
(pprof) list proto.Unmarshal     # 查看proto解析
(pprof) web                      # 生成调用图
```

### 2. 协程数量过多
```bash
# 1. 协程分析
go tool pprof http://localhost:8080/debug/pprof/goroutine

# 2. 查看协程创建热点
(pprof) top10
(pprof) traces                   # 显示调用栈
(pprof) list handleConnection    # 查看连接处理
(pprof) list acceptConnections   # 查看连接接收
```

### 3. 内存占用过高
```bash
# 1. 堆内存分析
go tool pprof http://localhost:8080/debug/pprof/heap

# 2. 查看内存分配热点
(pprof) top20
(pprof) list NewStageLatencyStats  # 查看统计结构分配
(pprof) list make                  # 查看切片/map分配
```

### 4. 锁竞争问题
```bash
# 1. 锁分析（需要先在代码中启用）
go tool pprof http://localhost:8080/debug/pprof/mutex

# 2. 查看竞争热点
(pprof) top10
(pprof) list sync.Mutex     # 查看互斥锁竞争
(pprof) list sync.RWMutex   # 查看读写锁竞争
```

## 持续监控

### 1. 定期收集性能数据
```bash
# 每小时收集一次CPU profile
curl -o cpu-profile-$(date +%Y%m%d-%H%M).prof \
     http://localhost:8080/debug/pprof/profile?seconds=60

# 每小时收集一次内存profile  
curl -o mem-profile-$(date +%Y%m%d-%H%M).prof \
     http://localhost:8080/debug/pprof/heap
```

### 2. 对比分析
```bash
# 对比两个时间点的CPU使用
go tool pprof -diff_base cpu-profile-old.prof cpu-profile-new.prof

# 对比内存使用变化
go tool pprof -diff_base mem-profile-old.prof mem-profile-new.prof
```

## 针对当前项目的优化建议

### 协程过多问题
```bash
# 1. 分析协程创建
go tool pprof http://localhost:8080/debug/pprof/goroutine
(pprof) top20
# 预期看到：handleConnection, 异步读取协程等

# 2. 协程池化后对比
# 实施协程池后重新分析，应该看到协程数量大幅下降
```

### Proto解析性能
```bash
# 1. CPU分析查找proto.Unmarshal热点
go tool pprof http://localhost:8080/debug/pprof/profile?seconds=30
(pprof) top20 | grep proto
(pprof) list proto.Unmarshal

# 2. 内存分析查找proto对象分配
go tool pprof http://localhost:8080/debug/pprof/heap
(pprof) list pb.ClientRequest
```

## 与原有监控的对比

| 功能 | 原有复杂监控 | Go pprof |
|------|-------------|----------|
| CPU分析 | 自定义时延统计 | 精确的CPU采样 |
| 内存分析 | 简单的内存统计 | 详细的内存分配分析 |
| 协程分析 | 无 | 完整的协程堆栈 |
| 锁竞争 | 无 | 精确的锁竞争分析 |
| 代码行数 | 500+ | 0（使用标准库） |
| 性能开销 | 高（每请求多次time.Now()） | 低（按需采样） |
| 分析深度 | 表面统计 | 代码级别分析 |

## 注意事项

1. **生产环境使用**：pprof数据收集有轻微性能开销，建议按需使用
2. **数据安全**：pprof端点包含程序内部信息，建议限制访问
3. **存储空间**：profile文件可能较大，注意清理旧文件
4. **分析工具**：建议安装graphviz以支持调用图生成

## 故障排查示例

### 场景：5000连接时CPU占用过高
```bash
# 步骤1：收集CPU数据
go tool pprof http://localhost:8080/debug/pprof/profile?seconds=60

# 步骤2：分析热点
(pprof) top20
# 预期结果：看到handleConnection, proto.Unmarshal等占用高

# 步骤3：查看具体函数
(pprof) list handleConnection
# 可以看到具体哪些代码行占用CPU最多

# 步骤4：查看调用图  
(pprof) web
# 生成可视化的调用关系图
```

通过pprof，我们可以精确定位到性能瓶颈的具体代码行，而不是靠猜测。