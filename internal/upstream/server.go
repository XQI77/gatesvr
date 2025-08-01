// Package upstream 提供上游gRPC服务实现
package upstream

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"

	pb "gatesvr/proto"
)

// Server 上游服务器
type Server struct {
	pb.UnimplementedUpstreamServiceServer

	// 服务器配置
	addr string

	// 单播推送客户端（新增）
	unicastClient *UnicastClient

	// 会话管理器（新增）
	sessionManager *SessionManager

	// 服务状态
	startTime time.Time

	// 连接统计
	activeConnections int32
	connMutex         sync.RWMutex

	// gRPC服务器
	grpcServer *grpc.Server
	listener   net.Listener

	// 停止信号
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewServer 创建新的上游服务器
func NewServer(addr string) *Server {
	return &Server{
		addr:           addr,
		startTime:      time.Now(),
		unicastClient:  NewUnicastClient("localhost:8082"), // 网关gRPC地址
		sessionManager: NewSessionManager(),                // 初始化会话管理器
		stopCh:         make(chan struct{}),
	}
}

// Start 启动上游服务器
func (s *Server) Start() error {
	log.Printf("正在启动上游服务器: %s", s.addr)

	// 连接到网关的单播推送服务
	if err := s.unicastClient.Connect(); err != nil {
		log.Printf("警告: 连接网关单播服务失败: %v", err)
	}

	// 监听端口
	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("监听端口失败: %w", err)
	}
	s.listener = listener

	// 创建gRPC服务器
	s.grpcServer = grpc.NewServer(
		grpc.UnaryInterceptor(s.unaryInterceptor),
		grpc.StreamInterceptor(s.streamInterceptor),
	)

	// 注册服务
	pb.RegisterUpstreamServiceServer(s.grpcServer, s)

	// 启动单播推送演示任务 (演示)
	//s.wg.Add(1)
	//go s.demoUnicastPush()

	log.Printf("上游服务器已启动: %s", s.addr)
	log.Printf("gRPC服务接口:")
	log.Printf("  ProcessRequest - 处理业务请求")
	log.Printf("  GetStatus      - 获取服务状态")

	// 启动gRPC服务
	return s.grpcServer.Serve(listener)
}

// Stop 停止服务器
func (s *Server) Stop() {
	log.Printf("正在停止上游服务器...")

	// 停止信号
	close(s.stopCh)

	// 停止会话管理器
	if s.sessionManager != nil {
		s.sessionManager.Stop()
	}

	// 关闭单播推送客户端
	if s.unicastClient != nil {
		s.unicastClient.Close()
	}

	// 停止gRPC服务器
	if s.grpcServer != nil {
		s.grpcServer.GracefulStop()
	}

	// 关闭监听器
	if s.listener != nil {
		s.listener.Close()
	}

	// 等待所有goroutine结束
	s.wg.Wait()

	log.Printf("上游服务器已停止")
}

// ProcessRequest 处理业务请求
func (s *Server) ProcessRequest(ctx context.Context, req *pb.UpstreamRequest) (*pb.UpstreamResponse, error) {
	log.Printf("收到业务请求 - 会话: %s, 动作: %s, 客户端序号: %d, OpenID: %s",
		req.SessionId, req.Action, req.ClientSeqId, req.Openid)

	// 验证OpenID并生成服务端序列号
	serverSeq := s.sessionManager.GenerateServerSeq(req.Openid)
	if serverSeq == 0 {
		// 无效的OpenID
		return &pb.UpstreamResponse{
			Code:        400,
			Message:     "无效的OpenID",
			Data:        []byte(fmt.Sprintf("OpenID %s 不存在或无效", req.Openid)),
			ClientSeqId: req.ClientSeqId,
			ServerSeqId: 0,
		}, nil
	}

	log.Printf("为 OpenID: %s 生成服务端序号: %d", req.Openid, serverSeq)

	// 根据动作类型处理不同的业务逻辑
	var response *pb.UpstreamResponse
	var err error

	switch req.Action {
	case "echo":
		response, err = s.handleEcho(ctx, req)
	case "time":
		response, err = s.handleTime(ctx, req)
	case "hello":
		response, err = s.handleHello(ctx, req)
	case "calculate":
		response, err = s.handleCalculate(ctx, req)
	case "status":
		response, err = s.handleStatus(ctx, req)
	case "session_info":
		response, err = s.handleSessionInfo(ctx, req)
	case "broadcast":
		response, err = s.handleBroadcastCommand(ctx, req)
	default:
		response, err = s.handleDefault(ctx, req)
	}

	// 设置序列号信息
	if response != nil {
		response.ClientSeqId = req.ClientSeqId
		response.ServerSeqId = serverSeq
	}

	return response, err
}

// GetStatus 获取服务状态
func (s *Server) GetStatus(ctx context.Context, req *pb.StatusRequest) (*pb.StatusResponse, error) {
	s.connMutex.RLock()
	connCount := s.activeConnections
	s.connMutex.RUnlock()

	uptime := time.Since(s.startTime).Seconds()

	metadata := map[string]string{
		"version":    "1.0.0",
		"go_version": "1.21",
		"build_time": s.startTime.Format(time.RFC3339),
	}

	response := &pb.StatusResponse{
		Status:            "healthy",
		Uptime:            int64(uptime),
		ActiveConnections: connCount,
		Metadata:          metadata,
	}

	log.Printf("返回服务状态 - 运行时间: %.0f秒, 活跃连接: %d",
		uptime, connCount)

	return response, nil
}

// handleEcho 处理echo请求，同时演示单播推送
func (s *Server) handleEcho(ctx context.Context, req *pb.UpstreamRequest) (*pb.UpstreamResponse, error) {
	message := string(req.Data)

	// 如果包含特殊参数，触发单播推送演示
	if gid := req.Params["demo_unicast_gid"]; gid != "" && s.unicastClient != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err := s.unicastClient.PushToClient(ctx, "gid", gid, "echo", "Echo推送",
				fmt.Sprintf("您的echo消息: %s", message), req.Data)
			if err != nil {
				log.Printf("Echo单播推送失败: %v", err)
			}
		}()
	}

	return &pb.UpstreamResponse{
		Code:    200,
		Message: "success",
		Data:    []byte(fmt.Sprintf("Echo: %s", message)),
	}, nil
}

// handleTime 处理时间请求
func (s *Server) handleTime(ctx context.Context, req *pb.UpstreamRequest) (*pb.UpstreamResponse, error) {
	now := time.Now()
	timeStr := now.Format("2006-01-02 15:04:05")

	return &pb.UpstreamResponse{
		Code:    200,
		Message: "当前时间",
		Data:    []byte(timeStr),
		Headers: map[string]string{
			"content-type": "text/plain",
			"timezone":     now.Location().String(),
			"unix":         fmt.Sprintf("%d", now.Unix()),
		},
	}, nil
}

// handleHello 处理问候请求
func (s *Server) handleHello(ctx context.Context, req *pb.UpstreamRequest) (*pb.UpstreamResponse, error) {
	name := req.Params["name"]
	if name == "" {
		name = "世界"
	}

	message := fmt.Sprintf("你好, %s! 欢迎使用网关服务器。", name)

	return &pb.UpstreamResponse{
		Code:    200,
		Message: "问候成功",
		Data:    []byte(message),
		Headers: map[string]string{
			"content-type": "text/plain",
			"language":     "zh-CN",
		},
	}, nil
}

// handleCalculate 处理计算请求
func (s *Server) handleCalculate(ctx context.Context, req *pb.UpstreamRequest) (*pb.UpstreamResponse, error) {
	operation := req.Params["operation"]
	aStr := req.Params["a"]
	bStr := req.Params["b"]

	if operation == "" || aStr == "" || bStr == "" {
		return &pb.UpstreamResponse{
			Code:    400,
			Message: "缺少必需参数: operation, a, b",
			Data:    []byte("错误: 参数不完整"),
		}, nil
	}

	// 简单的整数计算示例
	var a, b int
	if _, err := fmt.Sscanf(aStr, "%d", &a); err != nil {
		return &pb.UpstreamResponse{
			Code:    400,
			Message: "参数a不是有效整数",
			Data:    []byte("错误: 参数a格式错误"),
		}, nil
	}

	if _, err := fmt.Sscanf(bStr, "%d", &b); err != nil {
		return &pb.UpstreamResponse{
			Code:    400,
			Message: "参数b不是有效整数",
			Data:    []byte("错误: 参数b格式错误"),
		}, nil
	}

	var result int
	var opStr string

	switch operation {
	case "add":
		result = a + b
		opStr = "+"
	case "subtract":
		result = a - b
		opStr = "-"
	case "multiply":
		result = a * b
		opStr = "*"
	case "divide":
		if b == 0 {
			return &pb.UpstreamResponse{
				Code:    400,
				Message: "除数不能为零",
				Data:    []byte("错误: 除零错误"),
			}, nil
		}
		result = a / b
		opStr = "/"
	default:
		return &pb.UpstreamResponse{
			Code:    400,
			Message: "不支持的运算操作",
			Data:    []byte("错误: 操作类型无效"),
		}, nil
	}

	message := fmt.Sprintf("%d %s %d = %d", a, opStr, b, result)

	return &pb.UpstreamResponse{
		Code:    200,
		Message: "计算成功",
		Data:    []byte(message),
		Headers: map[string]string{
			"content-type": "text/plain",
			"operation":    operation,
			"result":       fmt.Sprintf("%d", result),
		},
	}, nil
}

// handleStatus 处理状态查询请求
func (s *Server) handleStatus(ctx context.Context, req *pb.UpstreamRequest) (*pb.UpstreamResponse, error) {
	s.connMutex.RLock()
	connCount := s.activeConnections
	s.connMutex.RUnlock()

	uptime := time.Since(s.startTime)

	status := map[string]interface{}{
		"service":            "upstream-server",
		"status":             "healthy",
		"uptime_seconds":     int64(uptime.Seconds()),
		"uptime_human":       uptime.String(),
		"active_connections": connCount,
		"start_time":         s.startTime.Format(time.RFC3339),
		"current_time":       time.Now().Format(time.RFC3339),
	}

	statusJSON := ""
	for k, v := range status {
		statusJSON += fmt.Sprintf("%s: %v\n", k, v)
	}

	return &pb.UpstreamResponse{
		Code:    200,
		Message: "状态查询成功",
		Data:    []byte(statusJSON),
		Headers: map[string]string{
			"content-type": "text/plain",
			"uptime":       uptime.String(),
		},
	}, nil
}

// handleSessionInfo 处理会话信息查询请求
func (s *Server) handleSessionInfo(ctx context.Context, req *pb.UpstreamRequest) (*pb.UpstreamResponse, error) {
	// 获取会话统计信息
	sessionStats := s.sessionManager.GetSessionStats()

	// 获取特定会话信息
	sessionInfo := s.sessionManager.GetSessionInfo(req.Openid)

	// 获取所有有效的OpenID
	validOpenIDs := s.sessionManager.GetAllValidOpenIDs()

	statusInfo := fmt.Sprintf(`会话管理器状态:
总会话数: %v
活跃会话数: %v
会话TTL: %v分钟

当前OpenID (%s) 信息:
存在: %v
服务端序号: %v
创建时间: %v
最后活动: %v
空闲时间: %vs

有效OpenID范围: %s 到 %s (共%d个)
`,
		sessionStats["total_sessions"],
		sessionStats["active_sessions"],
		sessionStats["session_ttl_minutes"],
		req.Openid,
		sessionInfo["exists"],
		sessionInfo["server_seq"],
		sessionInfo["create_time"],
		sessionInfo["last_activity"],
		sessionInfo["idle_seconds"],
		validOpenIDs[0],
		validOpenIDs[len(validOpenIDs)-1],
		len(validOpenIDs))

	return &pb.UpstreamResponse{
		Code:    200,
		Message: "会话信息查询成功",
		Data:    []byte(statusInfo),
		Headers: map[string]string{
			"content-type": "text/plain",
		},
	}, nil
}

// handleBroadcastCommand 处理广播命令 (已删除 - 不再支持广播)
func (s *Server) handleBroadcastCommand(ctx context.Context, req *pb.UpstreamRequest) (*pb.UpstreamResponse, error) {
	return &pb.UpstreamResponse{
		Code:    501,
		Message: "广播功能已停用",
		Data:    []byte("广播功能已从系统中移除"),
	}, nil
}

// handleDefault 处理默认请求
func (s *Server) handleDefault(ctx context.Context, req *pb.UpstreamRequest) (*pb.UpstreamResponse, error) {
	message := fmt.Sprintf("未知操作: %s\n可用操作: echo, time, hello, calculate, status", req.Action)

	return &pb.UpstreamResponse{
		Code:    404,
		Message: "操作不存在",
		Data:    []byte(message),
		Headers: map[string]string{
			"content-type": "text/plain",
		},
	}, nil
}

// unaryInterceptor gRPC拦截器，用于统计连接数
func (s *Server) unaryInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	// 增加活跃连接数
	s.connMutex.Lock()
	s.activeConnections++
	current := s.activeConnections
	s.connMutex.Unlock()

	log.Printf("处理gRPC请求: %s, 当前活跃连接: %d", info.FullMethod, current)

	// 处理请求
	start := time.Now()
	resp, err := handler(ctx, req)
	duration := time.Since(start)

	// 减少活跃连接数
	s.connMutex.Lock()
	s.activeConnections--
	current = s.activeConnections
	s.connMutex.Unlock()

	// 记录请求日志
	status := "成功"
	if err != nil {
		status = fmt.Sprintf("失败: %v", err)
	}

	log.Printf("gRPC请求完成: %s, 状态: %s, 耗时: %v, 剩余连接: %d",
		info.FullMethod, status, duration, current)

	return resp, err
}

// streamInterceptor gRPC流式拦截器，用于处理推送连接
func (s *Server) streamInterceptor(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	startTime := time.Now()

	// 增加活跃连接数
	s.connMutex.Lock()
	s.activeConnections++
	current := s.activeConnections
	s.connMutex.Unlock()

	log.Printf("处理gRPC流请求: %s, 当前活跃连接: %d", info.FullMethod, current)

	// 处理请求
	err := handler(srv, stream)

	// 减少活跃连接数
	s.connMutex.Lock()
	s.activeConnections--
	current = s.activeConnections
	s.connMutex.Unlock()

	// 记录请求日志
	status := "成功"
	if err != nil {
		status = fmt.Sprintf("失败: %v", err)
	}

	duration := time.Since(startTime)
	log.Printf("gRPC流请求完成: %s, 状态: %s, 耗时: %v, 剩余连接: %d",
		info.FullMethod, status, duration, current)

	return err
}

// demoUnicastPush 演示单播推送功能
func (s *Server) demoUnicastPush() {
	defer s.wg.Done()

	ticker := time.NewTicker(10 * time.Second) // 每2分钟演示一次
	defer ticker.Stop()

	// 启动时立即演示一次
	if s.unicastClient != nil {
		s.unicastClient.DemoUnicastPush()
	}

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			if s.unicastClient != nil {
				s.unicastClient.DemoUnicastPush()
			}
		}
	}
}

// SendBroadcast 对外广播接口 (已停用)
func (s *Server) SendBroadcast(message string, data []byte, headers map[string]string) error {
	return fmt.Errorf("广播功能已停用")
}

// GetBroadcastStats 获取广播统计 (已停用)
func (s *Server) GetBroadcastStats() map[string]interface{} {
	return map[string]interface{}{
		"status":  "disabled",
		"message": "broadcast functionality has been removed",
	}
}
