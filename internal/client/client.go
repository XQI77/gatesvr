// Package client 提供QUIC客户端实现
package client

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/quic-go/quic-go"
	"google.golang.org/protobuf/proto"

	"gatesvr/internal/message"
	pb "gatesvr/proto"
)

// Config 客户端配置
type Config struct {
	ServerAddr        string        // 服务器地址
	ClientID          string        // 客户端ID
	OpenID            string        // 客户端唯一身份标识 (5位数字)
	TLSSkipVerify     bool          // 是否跳过TLS验证
	ConnectTimeout    time.Duration // 连接超时时间
	HeartbeatInterval time.Duration // 心跳间隔
}

// Client QUIC客户端
type Client struct {
	config *Config

	// 连接和流
	connection *quic.Conn
	stream     *quic.Stream

	// 消息处理
	messageCodec    *message.MessageCodec
	nextMsgID       uint32 // 下一个消息ID（原子操作）
	nextBusinessSeq uint64 // 下一个业务序列号（原子操作）

	// 响应处理
	pendingRequests sync.Map // map[uint32]*PendingRequest

	// 状态管理
	connected    bool
	connectedMux sync.RWMutex

	// 性能监控 - 新增
	latencyTracker *ClientLatencyTracker

	// 停止控制
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// ClientLatencyTracker 客户端时延跟踪器
type ClientLatencyTracker struct {
	latencies         []time.Duration // 时延记录
	latencyMutex      sync.RWMutex
	maxLatencyEntries int
	totalRequests     int64 // 总请求数
	totalLatency      int64 // 总时延（纳秒）
	minLatency        int64 // 最小时延（纳秒）
	maxLatency        int64 // 最大时延（纳秒）
}

// NewClientLatencyTracker 创建新的客户端时延跟踪器
func NewClientLatencyTracker() *ClientLatencyTracker {
	return &ClientLatencyTracker{
		maxLatencyEntries: 1000,
		latencies:         make([]time.Duration, 0, 1000),
		minLatency:        int64(^uint64(0) >> 1), // 初始化为最大值
	}
}

// RecordLatency 记录时延
func (lt *ClientLatencyTracker) RecordLatency(latency time.Duration) {
	lt.latencyMutex.Lock()
	defer lt.latencyMutex.Unlock()

	latencyNs := latency.Nanoseconds()
	atomic.AddInt64(&lt.totalRequests, 1)
	atomic.AddInt64(&lt.totalLatency, latencyNs)

	// 更新最小最大值
	for {
		currentMin := atomic.LoadInt64(&lt.minLatency)
		if latencyNs >= currentMin || atomic.CompareAndSwapInt64(&lt.minLatency, currentMin, latencyNs) {
			break
		}
	}

	for {
		currentMax := atomic.LoadInt64(&lt.maxLatency)
		if latencyNs <= currentMax || atomic.CompareAndSwapInt64(&lt.maxLatency, currentMax, latencyNs) {
			break
		}
	}

	// 保存最近的时延记录用于百分位数计算
	if len(lt.latencies) >= lt.maxLatencyEntries {
		lt.latencies = lt.latencies[1:]
	}
	lt.latencies = append(lt.latencies, latency)
}

// GetLatencyStats 获取时延统计
func (lt *ClientLatencyTracker) GetLatencyStats() map[string]interface{} {
	lt.latencyMutex.RLock()
	defer lt.latencyMutex.RUnlock()

	totalReqs := atomic.LoadInt64(&lt.totalRequests)
	if totalReqs == 0 {
		return map[string]interface{}{
			"total_requests": 0,
			"avg_latency_ms": 0.0,
			"min_latency_ms": 0.0,
			"max_latency_ms": 0.0,
			"p95_latency_ms": 0.0,
			"p99_latency_ms": 0.0,
			"samples":        0,
		}
	}

	totalLatNs := atomic.LoadInt64(&lt.totalLatency)
	minLatNs := atomic.LoadInt64(&lt.minLatency)
	maxLatNs := atomic.LoadInt64(&lt.maxLatency)

	avgLatencyMs := float64(totalLatNs) / float64(totalReqs) / 1e6
	minLatencyMs := float64(minLatNs) / 1e6
	maxLatencyMs := float64(maxLatNs) / 1e6

	var p95LatencyMs, p99LatencyMs float64
	if len(lt.latencies) >= 20 {
		// 创建副本并排序
		latencies := make([]time.Duration, len(lt.latencies))
		copy(latencies, lt.latencies)

		// 简单冒泡排序
		for i := 0; i < len(latencies)-1; i++ {
			for j := i + 1; j < len(latencies); j++ {
				if latencies[i] > latencies[j] {
					latencies[i], latencies[j] = latencies[j], latencies[i]
				}
			}
		}

		p95Index := int(float64(len(latencies)) * 0.95)
		p99Index := int(float64(len(latencies)) * 0.99)

		if p95Index < len(latencies) {
			p95LatencyMs = float64(latencies[p95Index].Nanoseconds()) / 1e6
		}
		if p99Index < len(latencies) {
			p99LatencyMs = float64(latencies[p99Index].Nanoseconds()) / 1e6
		}
	}

	return map[string]interface{}{
		"total_requests": totalReqs,
		"avg_latency_ms": avgLatencyMs,
		"min_latency_ms": minLatencyMs,
		"max_latency_ms": maxLatencyMs,
		"p95_latency_ms": p95LatencyMs,
		"p99_latency_ms": p99LatencyMs,
		"samples":        len(lt.latencies),
	}
}

// PendingRequest 待响应的请求
type PendingRequest struct {
	MsgID     uint32
	StartTime time.Time
	Response  chan *pb.ServerPush
	Timeout   time.Duration
}

// Response 响应结构
type Response struct {
	Code    int32
	Message string
	Data    []byte
	Headers map[string]string
	Error   error
}

// NewClient 创建新的客户端
func NewClient(config *Config) *Client {
	if config.ClientID == "" {
		config.ClientID = uuid.New().String()
	}

	// 验证或生成5位数字OpenID
	if config.OpenID == "" {
		config.OpenID = generateRandomOpenID()
	} else if !isValidOpenID(config.OpenID) {
		log.Printf("警告: OpenID %s 格式无效，将使用随机生成的OpenID", config.OpenID)
		config.OpenID = generateRandomOpenID()
	}

	return &Client{
		config:         config,
		messageCodec:   message.NewMessageCodec(),
		latencyTracker: NewClientLatencyTracker(), // 新增
		stopCh:         make(chan struct{}),
	}
}

// generateRandomOpenID 生成随机的5位数字OpenID (范围: 10000-10099)
func generateRandomOpenID() string {
	// 生成10000-10099范围内的随机数
	randomNum := 10000 + (time.Now().UnixNano() % 100)
	return fmt.Sprintf("%05d", randomNum)
}

// isValidOpenID 验证OpenID是否为有效的5位数字格式
func isValidOpenID(openID string) bool {
	if len(openID) != 5 {
		return false
	}

	// 检查是否都是数字
	for _, char := range openID {
		if char < '0' || char > '9' {
			return false
		}
	}

	// 检查是否在有效范围内 (10000-10099)
	var num int
	if _, err := fmt.Sscanf(openID, "%d", &num); err != nil {
		return false
	}

	return num >= 10000 && num <= 10099
}

// Connect 连接到服务器
func (c *Client) Connect(ctx context.Context) error {
	log.Printf("正在连接到服务器: %s", c.config.ServerAddr)

	// 配置TLS
	tlsConfig := &tls.Config{
		InsecureSkipVerify: c.config.TLSSkipVerify,
		NextProtos:         []string{"gatesvr"},
	}

	// 连接超时控制
	connectCtx := ctx
	if c.config.ConnectTimeout > 0 {
		var cancel context.CancelFunc
		connectCtx, cancel = context.WithTimeout(ctx, c.config.ConnectTimeout)
		defer cancel()
	}

	// 建立QUIC连接
	conn, err := quic.DialAddr(connectCtx, c.config.ServerAddr, tlsConfig, nil)
	if err != nil {
		return fmt.Errorf("QUIC连接失败: %w", err)
	}
	c.connection = conn

	// 打开双向流
	stream, err := conn.OpenStreamSync(connectCtx)
	if err != nil {
		conn.CloseWithError(0, "open stream failed")
		return fmt.Errorf("打开流失败: %w", err)
	}
	c.stream = stream

	// 更新连接状态
	c.connectedMux.Lock()
	c.connected = true
	c.connectedMux.Unlock()

	// 启动消息处理goroutines
	c.wg.Add(2)
	go c.messageReceiver(ctx)
	go c.heartbeatSender(ctx)

	// 发送start消息建立会话
	if err := c.sendStartMessage(); err != nil {
		c.Disconnect() // 发送失败时清理连接
		return fmt.Errorf("发送start消息失败: %w", err)
	}

	log.Printf("已连接到服务器: %s, 客户端ID: %s, OpenID: %s", c.config.ServerAddr, c.config.ClientID, c.config.OpenID)
	return nil
}

// Disconnect 断开连接
func (c *Client) Disconnect() error {
	c.connectedMux.Lock()
	if !c.connected {
		c.connectedMux.Unlock()
		return nil
	}
	c.connected = false
	c.connectedMux.Unlock()

	log.Printf("正在断开连接...")

	// 发送stop消息通知服务器
	if err := c.sendStopMessage(pb.StopRequest_USER_LOGOUT); err != nil {
		log.Printf("发送stop消息失败: %v", err)
	}

	// 发送停止信号
	close(c.stopCh)

	// 关闭流和连接
	if c.stream != nil {
		c.stream.Close()
	}
	if c.connection != nil {
		c.connection.CloseWithError(0, "client disconnect")
	}

	// 等待goroutines完成
	c.wg.Wait()

	// 清理待处理的请求
	c.pendingRequests.Range(func(key, value interface{}) bool {
		if req, ok := value.(*PendingRequest); ok {
			select {
			case req.Response <- nil:
			default:
			}
		}
		c.pendingRequests.Delete(key)
		return true
	})

	log.Printf("已断开连接")
	return nil
}

// IsConnected 检查是否已连接
func (c *Client) IsConnected() bool {
	c.connectedMux.RLock()
	defer c.connectedMux.RUnlock()
	return c.connected
}

// SendHeartbeat 发送心跳
func (c *Client) SendHeartbeat() error {
	msgID := c.getNextMsgID()

	req := c.messageCodec.CreateHeartbeatRequest(msgID, 0, c.config.OpenID)

	return c.sendRequest(req)
}

// SendBusinessRequest 发送业务请求
func (c *Client) SendBusinessRequest(action string, params map[string]string, data []byte) (*Response, error) {
	return c.SendBusinessRequestWithTimeout(action, params, data, 30*time.Second)
}

// SendBusinessRequestWithTimeout 发送带超时的业务请求
func (c *Client) SendBusinessRequestWithTimeout(action string, params map[string]string, data []byte, timeout time.Duration) (*Response, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("客户端未连接")
	}

	msgID := c.getNextMsgID()
	businessSeq := c.getNextBusinessSeq() // 获取业务序列号

	// 创建业务请求（只有业务消息使用序列号）
	req := c.messageCodec.CreateBusinessRequest(msgID, businessSeq, action, params, data, c.config.OpenID)

	// 创建待响应请求
	pendingReq := &PendingRequest{
		MsgID:     msgID,
		StartTime: time.Now(), // 记录请求开始时间
		Response:  make(chan *pb.ServerPush, 1),
		Timeout:   timeout,
	}

	// 注册待响应请求
	c.pendingRequests.Store(msgID, pendingReq)
	defer c.pendingRequests.Delete(msgID)

	// 发送请求
	if err := c.sendRequest(req); err != nil {
		return nil, fmt.Errorf("发送请求失败: %w", err)
	}

	// 等待响应或超时
	select {
	case response := <-pendingReq.Response:
		if response == nil {
			return nil, fmt.Errorf("连接已关闭")
		}

		// 计算并记录时延
		latency := time.Since(pendingReq.StartTime)
		c.latencyTracker.RecordLatency(latency)

		log.Printf("消息往返时延: %.2fms, 消息ID: %d", latency.Seconds()*1000, msgID)

		return c.parseServerPush(response)
	case <-time.After(timeout):
		return nil, fmt.Errorf("请求超时")
	}
}

// sendRequest 发送请求
func (c *Client) sendRequest(req *pb.ClientRequest) error {
	if !c.IsConnected() {
		return fmt.Errorf("客户端未连接")
	}

	// 编码消息
	data, err := c.messageCodec.EncodeClientRequest(req)
	if err != nil {
		return fmt.Errorf("编码请求失败: %w", err)
	}

	// 发送消息
	if err := c.messageCodec.WriteMessage(c.stream, data); err != nil {
		return fmt.Errorf("发送消息失败: %w", err)
	}

	log.Printf("发送请求 - 类型: %d, 消息ID: %d", req.Type, req.MsgId)
	return nil
}

// messageReceiver 消息接收器
func (c *Client) messageReceiver(ctx context.Context) {
	defer c.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopCh:
			return
		default:
			if !c.IsConnected() {
				return
			}

			// 读取消息
			data, err := c.messageCodec.ReadMessage(c.stream)
			if err != nil {
				if err == io.EOF {
					log.Printf("服务器断开连接")
				} else {
					log.Printf("读取消息失败: %v", err)
				}
				return
			}

			// 解析服务器推送消息
			serverPush, err := c.messageCodec.DecodeServerPush(data)
			if err != nil {
				log.Printf("解码服务器推送失败: %v", err)
				continue
			}

			// 处理消息
			c.handleServerPush(serverPush)
		}
	}
}

// handleServerPush 处理服务器推送消息
func (c *Client) handleServerPush(push *pb.ServerPush) {
	log.Printf("收到服务器推送 - 类型: %d, 消息ID: %d, 序列号: %d",
		push.Type, push.MsgId, push.SeqId)

	// 发送ACK确认
	if push.SeqId > 0 {
		ackMsg := c.messageCodec.CreateAckMessage(push.SeqId, c.config.OpenID)
		if err := c.sendRequest(ackMsg); err != nil {
			log.Printf("发送ACK失败: %v", err)
		} else {
			log.Printf("发送ACK确认 - 序列号: %d", push.SeqId)
		}
	}

	// 如果是响应消息，查找对应的待处理请求
	if push.MsgId > 0 {
		if value, ok := c.pendingRequests.Load(push.MsgId); ok {
			if pendingReq, ok := value.(*PendingRequest); ok {
				select {
				case pendingReq.Response <- push:
				default:
					// 通道已满或已关闭
				}
				return
			}
		}
	}

	// 处理其他类型的推送消息（如广播）
	c.handleBroadcastMessage(push)
}

// handleBroadcastMessage 处理广播消息
func (c *Client) handleBroadcastMessage(push *pb.ServerPush) {
	switch push.Type {
	case pb.PushType_PUSH_BUSINESS_DATA:
		// 解析业务数据
		businessResp := &pb.BusinessResponse{}
		if err := proto.Unmarshal(push.Payload, businessResp); err != nil {
			log.Printf("解析广播业务数据失败: %v", err)
			return
		}

		log.Printf("收到广播消息: %s - %s", businessResp.Message, string(businessResp.Data))

	case pb.PushType_PUSH_ERROR:
		// 解析错误消息
		errorMsg := &pb.ErrorMessage{}
		if err := proto.Unmarshal(push.Payload, errorMsg); err != nil {
			log.Printf("解析错误消息失败: %v", err)
			return
		}

		log.Printf("收到错误消息: [%d] %s - %s",
			errorMsg.ErrorCode, errorMsg.ErrorMessage, errorMsg.Detail)

	default:
		log.Printf("收到未知类型的推送消息: %d", push.Type)
	}
}

// heartbeatSender 心跳发送器
func (c *Client) heartbeatSender(ctx context.Context) {
	defer c.wg.Done()

	if c.config.HeartbeatInterval <= 0 {
		return // 不发送心跳
	}

	ticker := time.NewTicker(c.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopCh:
			return
		case <-ticker.C:
			if !c.IsConnected() {
				return
			}

			if err := c.SendHeartbeat(); err != nil {
				log.Printf("发送心跳失败: %v", err)
			}
		}
	}
}

// parseServerPush 解析服务器推送为响应
func (c *Client) parseServerPush(push *pb.ServerPush) (*Response, error) {
	response := &Response{
		Headers: push.Headers,
	}

	switch push.Type {
	case pb.PushType_PUSH_BUSINESS_DATA:
		// 解析业务响应
		businessResp := &pb.BusinessResponse{}
		if err := proto.Unmarshal(push.Payload, businessResp); err != nil {
			return nil, fmt.Errorf("解析业务响应失败: %w", err)
		}

		response.Code = businessResp.Code
		response.Message = businessResp.Message
		response.Data = businessResp.Data

	case pb.PushType_PUSH_HEARTBEAT_RESP:
		// 解析心跳响应
		heartbeatResp := &pb.HeartbeatResponse{}
		if err := proto.Unmarshal(push.Payload, heartbeatResp); err != nil {
			return nil, fmt.Errorf("解析心跳响应失败: %w", err)
		}

		response.Code = 200
		response.Message = "心跳响应"
		response.Data = []byte(fmt.Sprintf("服务器时间: %d", heartbeatResp.ServerTimestamp))

	case pb.PushType_PUSH_ERROR:
		// 解析错误响应
		errorMsg := &pb.ErrorMessage{}
		if err := proto.Unmarshal(push.Payload, errorMsg); err != nil {
			return nil, fmt.Errorf("解析错误响应失败: %w", err)
		}

		response.Code = errorMsg.ErrorCode
		response.Message = errorMsg.ErrorMessage
		response.Data = []byte(errorMsg.Detail)
		response.Error = fmt.Errorf("[%d] %s: %s",
			errorMsg.ErrorCode, errorMsg.ErrorMessage, errorMsg.Detail)

	default:
		return nil, fmt.Errorf("未知的推送类型: %d", push.Type)
	}

	return response, nil
}

// getNextMsgID 获取下一个消息ID
func (c *Client) getNextMsgID() uint32 {
	return atomic.AddUint32(&c.nextMsgID, 1)
}

// getNextBusinessSeq 获取下一个业务序列号（仅用于业务消息）
func (c *Client) getNextBusinessSeq() uint64 {
	return atomic.AddUint64(&c.nextBusinessSeq, 1)
}

// GetStats 获取客户端统计信息
func (c *Client) GetStats() map[string]interface{} {
	pendingCount := 0
	c.pendingRequests.Range(func(key, value interface{}) bool {
		pendingCount++
		return true
	})

	// 获取时延统计
	latencyStats := c.latencyTracker.GetLatencyStats()

	return map[string]interface{}{
		"client_id":         c.config.ClientID,
		"connected":         c.IsConnected(),
		"server_addr":       c.config.ServerAddr,
		"pending_requests":  pendingCount,
		"next_msg_id":       atomic.LoadUint32(&c.nextMsgID),
		"next_business_seq": atomic.LoadUint64(&c.nextBusinessSeq), // 新增业务序列号
		"latency_stats":     latencyStats,                          // 新增时延统计
	}
}

// sendStartMessage 发送连接建立消息
func (c *Client) sendStartMessage() error {
	if c.config.OpenID == "" {
		return fmt.Errorf("OpenID不能为空")
	}

	msgID := c.getNextMsgID()

	// 创建start请求
	req := c.messageCodec.CreateStartRequest(msgID, 0, c.config.OpenID, "default-token", 0)

	// 创建待响应请求
	pendingReq := &PendingRequest{
		MsgID:     msgID,
		StartTime: time.Now(),
		Response:  make(chan *pb.ServerPush, 1),
		Timeout:   10 * time.Second, // start消息10秒超时
	}

	// 注册待响应请求
	c.pendingRequests.Store(msgID, pendingReq)
	defer c.pendingRequests.Delete(msgID)

	// 发送请求
	if err := c.sendRequest(req); err != nil {
		return fmt.Errorf("发送start请求失败: %w", err)
	}

	log.Printf("已发送start消息 - OpenID: %s, 消息ID: %d", c.config.OpenID, msgID)

	// 等待响应
	select {
	case response := <-pendingReq.Response:
		if response == nil {
			return fmt.Errorf("连接已关闭")
		}

		// 解析start响应
		if response.Type == pb.PushType_PUSH_START_RESP {
			startResp := &pb.StartResponse{}
			if err := proto.Unmarshal(response.Payload, startResp); err != nil {
				return fmt.Errorf("解析start响应失败: %w", err)
			}

			if !startResp.Success {
				errorMsg := "未知错误"
				if startResp.Error != nil {
					errorMsg = startResp.Error.ErrorMessage
				}
				return fmt.Errorf("start消息被服务器拒绝: %s", errorMsg)
			}

			log.Printf("start消息成功 - 连接ID: %s, 心跳间隔: %d秒",
				startResp.ConnectionId, startResp.HeartbeatInterval)
			return nil
		}

		return fmt.Errorf("收到非预期的响应类型: %d", response.Type)

	case <-time.After(100 * time.Second):
		return fmt.Errorf("start消息超时")
	}
}

// sendStopMessage 发送连接断开消息
func (c *Client) sendStopMessage(reason pb.StopRequest_Reason) error {
	if !c.IsConnected() {
		return nil // 如果已经断开，不需要发送stop消息
	}

	msgID := c.getNextMsgID()

	// 创建stop请求
	req := c.messageCodec.CreateStopRequest(msgID, 0, reason, c.config.OpenID)

	// 发送请求（不等待响应，因为即将断开连接）
	if err := c.sendRequest(req); err != nil {
		log.Printf("发送stop消息失败: %v", err)
		return err
	}

	log.Printf("已发送stop消息 - OpenID: %s, 原因: %s", c.config.OpenID, reason.String())

	// 给服务器一些时间处理stop消息
	time.Sleep(100 * time.Millisecond)

	return nil
}
