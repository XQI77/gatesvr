# 网关系统支持的操作动作

本文档详细介绍了网关客户端和上游服务器支持的所有操作动作及其使用方法。

## 目录

- [客户端支持的操作动作](#客户端支持的操作动作)
- [上游服务器支持的操作动作](#上游服务器支持的操作动作)
- [交互模式命令](#交互模式命令)
- [特殊操作](#特殊操作)
- [使用示例](#使用示例)

## 客户端支持的操作动作

客户端通过 `SendBusinessRequest` 方法向网关发送各种业务请求。

### 基础操作动作

#### 1. echo - 回显消息
**描述**: 发送消息并接收服务器回显
**参数**: 无
**数据**: 要回显的文本内容
**示例**:
```bash
./client -server localhost:8443 -test echo -data "Hello World"
```

#### 2. time - 获取服务器时间
**描述**: 获取服务器当前时间
**参数**: 
- `format`: 时间格式 (可选，默认: "2006-01-02 15:04:05")
**数据**: 无
**返回**: 服务器时间字符串
**示例**:
```bash
./client -server localhost:8443 -test time
```

#### 3. hello - 登录/问候
**描述**: 用户登录或发送问候消息
**参数**: 
- `name`: 用户名称
- `version`: 客户端版本 (可选)
- `timestamp`: 请求时间戳 (可选)
**数据**: 无
**返回**: 登录成功确认
**示例**:
```bash
./client -server localhost:8443 -test hello
```

#### 4. calculate - 数学计算
**描述**: 执行数学运算
**参数**: 
- `operation`: 运算类型 ("add", "subtract", "multiply", "divide")
- `a`: 第一个操作数
- `b`: 第二个操作数
**数据**: 无
**返回**: 计算结果
**示例**:
```bash
./client -server localhost:8443 -test calculate -data '{"operation":"add","a":"10","b":"20"}'
```

#### 5. status - 获取服务状态
**描述**: 获取服务器运行状态
**参数**: 
- `type`: 状态类型 ("system", 可选)
**数据**: 无
**返回**: 服务器状态信息
**示例**:
```bash
./client -server localhost:8443 -test status
```

#### 6. session_info - 获取会话信息
**描述**: 获取当前会话详细信息
**参数**: 
- `detail`: 是否获取详细信息 ("true"/"false")
**数据**: 无
**返回**: 会话信息
**示例**:
```bash
# 交互模式下使用
session_info
```

### 高级操作动作

#### 7. business_msg - 业务消息
**描述**: 发送业务相关消息
**参数**: 
- `type`: 消息类型
- `content`: 消息内容
- `priority`: 优先级 (1-5)
**数据**: 业务数据 (JSON格式)
**用途**: 性能测试和业务场景模拟

#### 8. heartbeat - 心跳测试
**描述**: 发送心跳消息测试连接状态
**参数**: 
- `client_time`: 客户端时间戳
- `sequence`: 序列号
**数据**: 无
**用途**: 连接状态检测

#### 9. data_sync - 数据同步
**描述**: 模拟数据同步操作
**参数**: 
- `sync_type`: 同步类型 ("full"/"incremental")
- `last_sync`: 上次同步时间戳
**数据**: 同步数据 (JSON格式)
**用途**: 数据同步场景测试

#### 10. notification - 通知消息
**描述**: 发送通知消息
**参数**: 
- `notify_type`: 通知类型
- `target`: 目标 ("all"/"specific")
- `message`: 通知内容
**数据**: 无
**用途**: 通知系统测试

### 测试专用动作

#### 11. before - 测试notify在response之前
**描述**: 测试通知消息在响应消息之前到达
**参数**: 
- `message`: 测试消息内容
**数据**: 无
**用途**: 消息顺序测试

#### 12. after - 测试notify在response之后  
**描述**: 测试通知消息在响应消息之后到达
**参数**: 
- `message`: 测试消息内容
**数据**: 无
**用途**: 消息顺序测试

## 上游服务器支持的操作动作

上游服务器通过 gRPC `ProcessRequest` 接口处理来自网关的请求。

### 认证相关

#### login/auth/signin/hello - 用户登录
**描述**: 处理用户登录认证
**支持别名**: "login", "auth", "signin", "hello"
**逻辑**: 
- 验证OpenID
- 创建用户会话
- 返回登录成功确认
**返回**: 登录状态和用户信息

#### logout - 用户登出
**描述**: 处理用户登出
**逻辑**: 
- 清除用户会话
- 更新登录状态
**返回**: 登出确认

### 基础功能

#### echo - 回显处理
**描述**: 回显客户端发送的消息
**特性**: 
- 支持单播推送演示 (通过 `demo_unicast_gid` 参数)
- 异步推送消息到指定客户端
**返回**: 回显的消息内容

#### time - 时间服务
**描述**: 返回服务器当前时间
**返回**: 
- 格式化时间字符串
- 时区信息
- Unix时间戳

#### calculate - 计算服务
**描述**: 执行数学运算
**支持运算**: 
- `add`: 加法
- `subtract`: 减法  
- `multiply`: 乘法
- `divide`: 除法 (包含除零检查)
**返回**: 计算结果

#### status - 状态查询
**描述**: 返回服务器运行状态
**返回信息**: 
- 服务健康状态
- 运行时间
- 活跃连接数
- 版本信息

### 管理功能

#### user_list - 用户列表
**描述**: 获取当前在线用户列表
**返回**: 
- 在线用户数量
- 用户详细信息 (OpenID, SessionID, 登录时间等)

#### broadcast - 广播命令
**描述**: 向所有在线用户广播消息
**参数**: 
- `message`: 广播内容
- `type`: 广播类型 (可选)
**功能**: 通过单播服务推送到所有在线用户

### 测试功能

#### before - 先发送notify测试
**描述**: 先发送notify消息，再返回response
**用途**: 测试消息到达顺序
**流程**: 
1. 立即发送notify消息
2. 延迟500ms后返回response

#### after - 后发送notify测试
**描述**: 先返回response，再发送notify消息
**用途**: 测试消息到达顺序
**流程**: 
1. 立即返回response
2. 异步延迟500ms后发送notify消息

## 交互模式命令

客户端交互模式下支持以下命令:

### 基础命令
- `echo <message>` - 发送回显消息
- `time` - 获取服务器时间
- `hello [name]` - 发送问候
- `calc <op> <a> <b>` - 执行计算
- `status` - 获取服务状态
- `session_info` - 获取会话信息

### 管理命令
- `stats` - 显示客户端统计信息
- `disconnect` - 强制断开连接(保持重连状态)
- `reconnect` - 重新连接(保持序列号连续性)
- `test_reconnect` - 测试短线重连功能

### 测试命令
- `before [message]` - 测试notify在response之前
- `after [message]` - 测试notify在response之后
- `quit/exit` - 退出程序

## 特殊操作

### 连接管理
- **自动重连**: 客户端支持自动重连，保持序列号连续性
- **心跳机制**: 定期发送心跳保持连接活跃
- **序列号管理**: 确保消息顺序和去重

### 性能测试
- **并发客户端**: 支持多客户端并发测试
- **随机动作**: 随机选择动作进行压力测试
- **重连测试**: 可选的重连测试功能

### 单播推送
- **gRPC推送**: 上游服务可通过gRPC向特定客户端推送消息
- **目标定位**: 支持按 session、gid、openid 定位客户端
- **异步推送**: 推送操作异步执行，不阻塞主请求

## 使用示例

### 基础使用
```bash
# 单次测试
./client -server localhost:8443 -openid 10001 -test echo -data "Hello"

# 交互模式  
./client -server localhost:8443 -openid 10005 -interactive

# 性能测试模式
./client -server localhost:8443 -performance -clients 5 -request-interval 500ms
```

### 计算操作
```bash
# 加法运算
./client -server localhost:8443 -test calculate -data '{"operation":"add","a":"10","b":"20"}'

# 交互模式计算
calc add 15 25
```

### 连接测试
```bash
# 启用重连测试的性能模式
./client -server localhost:8443 -performance -clients 3 -enable-reconnect

# 交互模式测试重连
test_reconnect
```

### 消息顺序测试
```bash
# 交互模式下测试消息顺序
before 测试消息1
after 测试消息2
```

## 扩展性

系统设计具有良好的扩展性:

1. **新增客户端动作**: 在 `sendRandomRequest` 函数中添加新的 case
2. **新增服务端处理**: 在上游服务器的 `ProcessRequest` 方法中添加新的处理逻辑
3. **参数扩展**: 通过 `params` 和 `data` 字段传递复杂参数
4. **返回数据扩展**: 通过 `headers` 字段返回额外的元数据

所有操作都遵循统一的请求-响应模式，确保系统的一致性和可维护性。