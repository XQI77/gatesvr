// Package gateway 提供网关服务器核心功能
package gateway

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
	_ "net/http/pprof" // 导入pprof HTTP端点

	"github.com/quic-go/quic-go"
	"google.golang.org/grpc"

	"gatesvr/internal/backup"
	"gatesvr/internal/message"
	"gatesvr/internal/session"
	"gatesvr/internal/upstream"
	"gatesvr/pkg/metrics"
	pb "gatesvr/proto"
)

// Server 网关服务器
type Server struct {
	config *Config

	// 嵌入gRPC服务接口
	pb.UnimplementedGatewayServiceServer

	// 核心组件
	sessionManager     *session.Manager           // 会话管理器
	messageCodec       *message.MessageCodec      // 消息编解码器
	metrics            *metrics.GateServerMetrics // 监控指标
	performanceTracker *SimpleTracker             // 简化的性能追踪器（替代复杂的PerformanceTracker）
	orderedSender      *OrderedMessageSender      // 有序消息发送器
	startProcessor     *StartMessageProcessor     // START消息异步处理器（todo不需要这个）

	// 上游服务管理
	upstreamServices *upstream.UpstreamServices // 多上游服务管理器
	upstreamManager  *upstream.ServiceManager   // 上游服务连接管理器
	upstreamClient   pb.UpstreamServiceClient   // 保留向后兼容
	upstreamConn     *grpc.ClientConn           // 保留向后兼容

	// 服务器实例
	quicListener  *quic.Listener
	httpServer    *http.Server
	metricsServer *metrics.MetricsServer
	grpcServer    *grpc.Server

	// 备份管理器
	backupManager backup.BackupManager // 备份管理器

	// 过载保护器
	overloadProtector *OverloadProtector // 过载保护器

	// 状态管理
	running      bool
	runningMutex sync.RWMutex

	// 停止信号
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewServer 创建新的网关服务器
func NewServer(config *Config) *Server {
	server := &Server{
		config:             config,
		sessionManager:     session.NewManager(config.SessionTimeout, config.AckTimeout, config.MaxRetries),
		messageCodec:       message.NewMessageCodec(),
		metrics:            metrics.NewGateServerMetrics(),
		performanceTracker: NewSimpleTracker(), // 使用简化的性能追踪器
		upstreamServices:   upstream.NewUpstreamServices(),
		stopCh:             make(chan struct{}),
	}

	// 初始化多上游服务配置
	server.initUpstreamServices()

	// 初始化有序消息发送器
	server.orderedSender = NewOrderedMessageSender(server)

	// 初始化START消息异步处理器
	if config.StartProcessorConfig != nil && config.StartProcessorConfig.Enabled {
		server.startProcessor = NewStartMessageProcessor(
			server,
			config.StartProcessorConfig.MaxWorkers,
			config.StartProcessorConfig.QueueSize,
			config.StartProcessorConfig.Timeout,
		)
		log.Printf("START消息异步处理器已配置 - Workers: %d, 队列: %d, 超时: %v",
			config.StartProcessorConfig.MaxWorkers,
			config.StartProcessorConfig.QueueSize,
			config.StartProcessorConfig.Timeout)
	} else {
		// 使用默认配置：100个worker，1000个任务队列，30秒超时
		server.startProcessor = NewStartMessageProcessor(server, 100, 1000, 30*time.Second)
		log.Printf("START消息异步处理器使用默认配置 - Workers: 100, 队列: 1000, 超时: 30s")
	}

	// 设置读取时延回调函数
	server.messageCodec.SetReadLatencyCallback(func(latency time.Duration) {
		server.performanceTracker.RecordReadLatency(latency)
	})

	// 初始化备份管理器
	if config.BackupConfig != nil && config.BackupConfig.Sync.Enabled {
		server.backupManager = backup.NewBackupManager(config.BackupConfig, config.ServerID)
		server.backupManager.RegisterSessionManager(server.sessionManager)

		// 设置会话管理器的同步回调
		server.sessionManager.EnableSync(server.onSessionSync)

		log.Printf("备份管理器已初始化 - 服务器ID: %s, 模式: %d", config.ServerID, config.BackupConfig.Sync.Mode)
	}

	// 初始化过载保护器
	server.overloadProtector = NewOverloadProtector(config.OverloadProtectionConfig, server.metrics)

	return server
}

// initUpstreamServices 初始化多上游服务配置
func (s *Server) initUpstreamServices() {
	// 如果有新的多上游配置，使用它
	if s.config.UpstreamServices != nil && len(s.config.UpstreamServices) > 0 {
		for serviceTypeStr, addresses := range s.config.UpstreamServices {
			serviceType := upstream.ServiceType(serviceTypeStr)
			s.upstreamServices.AddService(serviceType, addresses)
			log.Printf("添加上游服务 - 类型: %s, 地址: %v", serviceType, addresses)
		}
	} else if s.config.UpstreamAddr != "" {
		// 向后兼容：如果只有单一上游地址，将其用作business服务
		s.upstreamServices.AddService(upstream.ServiceTypeBusiness, []string{s.config.UpstreamAddr})
		log.Printf("向后兼容模式 - 使用单一上游地址作为business服务: %s", s.config.UpstreamAddr)
	} else {
		// 使用默认配置
		s.upstreamServices.AddService(upstream.ServiceTypeHello, []string{"localhost:8081"})
		s.upstreamServices.AddService(upstream.ServiceTypeBusiness, []string{"localhost:8082"})
		s.upstreamServices.AddService(upstream.ServiceTypeZone, []string{"localhost:8083"})
		log.Printf("使用默认上游服务配置")
	}

	// 输出配置摘要
	stats := s.upstreamServices.GetStats()
	log.Printf("上游服务初始化完成 - 总服务数: %d, 已启用: %d",
		stats["total_services"], stats["enabled_services"])
}

// Start 启动网关服务器
func (s *Server) Start(ctx context.Context) error {
	log.Printf("启动网关服务器...")

	s.runningMutex.Lock()
	s.running = true
	s.runningMutex.Unlock()

	// 初始化上游服务连接
	if err := s.initUpstreamConnections(); err != nil {
		return fmt.Errorf("初始化上游服务连接失败: %w", err)
	}

	// 启动各种服务器
	if err := s.startQUICListener(); err != nil {
		return fmt.Errorf("启动QUIC监听器失败: %w", err)
	}

	if err := s.startHTTPServer(); err != nil {
		return fmt.Errorf("启动HTTP服务器失败: %w", err)
	}

	if err := s.startMetricsServer(); err != nil {
		return fmt.Errorf("启动监控服务器失败: %w", err)
	}

	if err := s.startGRPCServer(); err != nil {
		return fmt.Errorf("启动gRPC服务器失败: %w", err)
	}

	// 启动会话管理器
	s.sessionManager.Start(ctx)

	// 启动START消息异步处理器
	if s.startProcessor != nil {
		s.startProcessor.Start()
		log.Printf("START消息异步处理器已启动")
	}

	// 启动备份管理器
	if s.backupManager != nil {
		if err := s.backupManager.Start(ctx); err != nil {
			log.Printf("启动备份管理器失败: %v", err)
		} else {
			log.Printf("备份管理器已启动")
		}
	}

	// 启动连接处理器
	s.wg.Add(1)
	go s.acceptConnections(ctx)

	log.Printf("网关服务器启动完成")
	log.Printf("QUIC监听地址: %s", s.config.QUICAddr)
	log.Printf("HTTP监听地址: %s", s.config.HTTPAddr)
	log.Printf("gRPC监听地址: %s", s.config.GRPCAddr)
	log.Printf("监控地址: %s", s.config.MetricsAddr)
	log.Printf("上游服务地址: %s", s.config.UpstreamAddr)

	return nil
}

// Stop 停止网关服务器
func (s *Server) Stop() {
	log.Printf("正在停止网关服务器...")

	s.runningMutex.Lock()
	s.running = false
	s.runningMutex.Unlock()

	// 发送停止信号
	close(s.stopCh)

	// 停止QUIC监听器
	if s.quicListener != nil {
		s.quicListener.Close()
	}

	// 停止HTTP服务器
	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		s.httpServer.Shutdown(ctx)
		cancel()
	}

	// 停止监控服务器
	if s.metricsServer != nil {
		s.metricsServer.Stop()
	}

	// 停止gRPC服务器
	if s.grpcServer != nil {
		s.grpcServer.GracefulStop()
	}

	// 停止START消息异步处理器
	if s.startProcessor != nil {
		s.startProcessor.Stop()
		log.Printf("START消息异步处理器已停止")
	}

	// 停止备份管理器
	if s.backupManager != nil {
		if err := s.backupManager.Stop(); err != nil {
			log.Printf("停止备份管理器失败: %v", err)
		} else {
			log.Printf("备份管理器已停止")
		}
	}

	// 停止会话管理器
	s.sessionManager.Stop()

	// 关闭上游服务连接
	if s.upstreamManager != nil {
		if err := s.upstreamManager.Close(); err != nil {
			log.Printf("关闭上游服务管理器失败: %v", err)
		}
	}

	// 关闭向后兼容的单一上游连接
	if s.upstreamConn != nil {
		s.upstreamConn.Close()
	}

	// 等待所有goroutine结束
	s.wg.Wait()

	log.Printf("网关服务器已停止")
}

// onSessionSync 会话同步回调函数
func (s *Server) onSessionSync(sessionID string, session *session.Session, event string) {
	if s.backupManager == nil {
		return
	}

	// 根据事件类型触发相应的同步
	switch event {
	case "session_created", "session_reconnected", "session_activated":
		// 同步会话数据
		log.Printf("同步会话事件: %s - 会话: %s", event, sessionID)
		// 这里可以调用备份管理器的同步方法
		// 由于BackupManager接口没有直接的同步方法，这里暂时记录日志

	case "session_deleted":
		// 同步会话删除
		log.Printf("同步会话删除: %s", sessionID)

	default:
		log.Printf("未知会话同步事件: %s - 会话: %s", event, sessionID)
	}
}

// GetBackupStats 获取备份统计信息
func (s *Server) GetBackupStats() map[string]interface{} {
	if s.backupManager == nil {
		return map[string]interface{}{
			"backup_enabled": false,
		}
	}

	stats := s.backupManager.GetStats()
	stats["backup_enabled"] = true
	return stats
}

// SwitchBackupMode 切换备份模式
func (s *Server) SwitchBackupMode(mode backup.ServerMode) error {
	if s.backupManager == nil {
		return fmt.Errorf("备份功能未启用")
	}

	return s.backupManager.SwitchMode(mode)
}

// TriggerBackupSync 手动触发备份同步
func (s *Server) TriggerBackupSync() error {
	if s.backupManager == nil {
		return fmt.Errorf("备份功能未启用")
	}

	return s.backupManager.TriggerSync()
}

// TriggerFailover 手动触发故障切换
func (s *Server) TriggerFailover() error {
	if s.backupManager == nil {
		return fmt.Errorf("备份功能未启用")
	}

	return s.backupManager.TriggerFailover()
}
