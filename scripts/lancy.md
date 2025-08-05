
  已完成的工作

1. 扩展了性能跟踪基础设施

- 在performance.go中添加了6个新的时延跟踪字段：
  - seqValidateLatency - 序列号验证时延
  - stateValidateLatency - 状态验证时延
  - buildReqLatency - 构造请求时延
  - loginProcessLatency - 登录处理时延
  - sendRespLatency - 发送响应时延
  - processNotifyLatency - 处理notify时延

2. 在handleBusinessRequest中添加了精细的时延记录点

- 序列号验证阶段 - 记录ValidateClientSequence时延
- 消息解析阶段 - 记录protobuf解析时延
- 状态验证阶段 - 记录会话状态检查时延
- 请求构造阶段 - 记录构建UpstreamRequest时延
- 上游调用阶段 - 记录gRPC调用时延
- 登录处理阶段 - 记录handleLoginSuccess时延(仅登录请求)
- 响应发送阶段 - 记录SendBusinessResponse时延
- Notify处理阶段 - 记录processBoundNotifies时延

3. 添加了专门的HTTP API接口

- /latency/business - 提供业务请求详细时延分析
- 按阶段分组展示时延统计（验证、处理、上游、响应）
- 包含优化建议和性能阈值说明

4. 保持了向后兼容性

- 原有的性能统计保持不变
- 新的详细时延记录不会影响现有的监控功能
- 所有时延记录都是轻量级操作，对性能影响极小

  使用方法

1. 启动服务后访问详细时延分析：
   curl http://localhost:8080/latency/business
2. 响应包含按阶段分组的详细时延统计：
   - validation_phase: 序列号和状态验证
   - processing_phase: 解析和构造请求
   - upstream_phase: 上游服务调用
   - response_phase: 登录处理、响应发送、notify处理
3. 每个阶段都包含平均值、最小值、最大值、P95、P99统计

  现在你可以在1000并发场景下精确定位handleBusinessRequest函数中哪
  个阶段导致了200ms的时延，并进行针对性优化。



1. 扩展了性能跟踪基础设施

  在performance.go中添加了8个新的SendOrderedMessage详细时延跟踪字段：

- seqAllocLatency - 序列号分配时延
- msgEncodeLatency - 消息编码时延（更详细的版本）
- queueGetLatency - 获取队列时延
- callbackSetLatency - 设置回调函数时延
- enqueueLatency - 消息入队时延
- directSendLatency - 直接发送时延（整体）
- writeMessageLatency - 写消息时延（网络IO）
- metricsUpdateLatency - 指标更新时延

2. 在SendOrderedMessage中添加了精细的时延记录点

  SendOrderedMessage方法中的时延记录点：

- 序列号分配阶段 - sess.NewServerSeq()
- 消息编码阶段 - EncodeServerPush()
- 队列获取阶段 - sess.GetOrderedQueue()
- 回调设置阶段 - SetSendCallback()（首次设置）
- 消息入队阶段 - EnqueueMessage()

  sendMessageDirectly方法中的时延记录点：

- 直接发送整体时延 - 整个方法执行时间
- 网络写入时延 - WriteMessage()（最可能的200ms瓶颈）
- 指标更新时延 - 指标收集系统更新

3. 添加了专门的HTTP API接口

  新增 /latency/send-ordered 接口，提供：

  按阶段分组的详细分析：

- preparation_phase: 序列号分配、消息编码、队列获取、回调设置
- queuing_phase: 消息入队操作
- sending_phase: 直接发送、网络写入、指标更新

  瓶颈分析和优化建议：

- 针对每个阶段的瓶颈检测和分析
- 具体的优化建议和性能阈值
- 特别标注了write_message_high是最可能的200ms时延根源

  使用方法

1. 访问SendOrderedMessage详细时延分析：
   curl http://localhost:8080/latency/send-ordered
2. 重点关注的指标：
   - write_message_latency - 最可能的200ms瓶颈来源
   - enqueue_latency - 队列操作是否存在阻塞
   - msg_encode_latency - 消息编码是否过慢
3. 性能阈值参考：
   - 正常总发送时延: < 10ms
   - 警告级别: 10-50ms
   - 关键问题: > 50ms
   - 网络写入应占总时延的60-80%

  现在你可以精确定位SendOrderedMessage方法中的200ms时延瓶颈，很可能会发现
  问题出现在writeMessageLatency（网络IO）或enqueueLatency（队列阻塞）上。
