package gateway

import (
	"crypto/tls"
	"fmt"
	"gatesvr/pkg/metrics"
	pb "gatesvr/proto"
	"log"
	"net"
	"net/http"

	"github.com/quic-go/quic-go"
	"google.golang.org/grpc"
)

// startQUICListener 启动QUIC监听器
func (s *Server) startQUICListener() error {
	// 加载TLS证书
	cert, err := tls.LoadX509KeyPair(s.config.TLSCertFile, s.config.TLSKeyFile)
	if err != nil {
		return fmt.Errorf("加载TLS证书失败: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"gatesvr"},
	}

	// 创建QUIC监听器
	listener, err := quic.ListenAddr(s.config.QUICAddr, tlsConfig, nil)
	if err != nil {
		return fmt.Errorf("创建QUIC监听器失败: %w", err)
	}

	s.quicListener = listener
	log.Printf("QUIC监听器已启动: %s", s.config.QUICAddr)
	return nil
}

// startHTTPServer 启动HTTP API服务器
func (s *Server) startHTTPServer() error {
	mux := http.NewServeMux()

	// 基础监控API（保留）
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/stats", s.handleStats)

	// Go pprof性能分析端点（替换复杂的性能监控）
	mux.HandleFunc("/debug/pprof/", http.DefaultServeMux.ServeHTTP) // pprof索引页
	mux.HandleFunc("/pprof/", http.DefaultServeMux.ServeHTTP)       // 简化路径

	// 可选：简化性能监控端点（如果需要基本统计）
	mux.HandleFunc("/performance", s.handleSimplePerformance)

	// 注释掉复杂的监控端点，使用pprof替代
	// mux.HandleFunc("/queue", s.handleQueueStatus)
	// mux.HandleFunc("/latency/detailed", s.handleDetailedLatency)
	// mux.HandleFunc("/latency/breakdown", s.handleLatencyBreakdown)
	// mux.HandleFunc("/latency/client", s.handleClientLatency)
	// mux.HandleFunc("/latency/business", s.handleBusinessRequestLatency)
	// mux.HandleFunc("/latency/send-ordered", s.handleSendOrderedMessageLatency)
	// mux.HandleFunc("/queue/async-stats", s.handleAsyncQueueStats)
	// mux.HandleFunc("/queue/optimization", s.handleQueueOptimizationAnalysis)
	// mux.HandleFunc("/queue/config", s.handleQueueConfig)
	// mux.HandleFunc("/start-processor", s.handleStartProcessor)
	// mux.HandleFunc("/overload-protection", s.handleOverloadProtection)

	s.httpServer = &http.Server{
		Addr:    s.config.HTTPAddr,
		Handler: mux,
	}

	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP服务器错误: %v", err)
		}
	}()

	log.Printf("HTTP服务器已启动: %s", s.config.HTTPAddr)
	return nil
}

// startMetricsServer 启动监控服务器
func (s *Server) startMetricsServer() error {
	s.metricsServer = metrics.NewMetricsServer(s.config.MetricsAddr)

	go func() {
		if err := s.metricsServer.Start(); err != nil && err != http.ErrServerClosed {
			log.Printf("监控服务器错误: %v", err)
		}
	}()

	log.Printf("监控服务器已启动: %s", s.config.MetricsAddr)
	return nil
}

// startGRPCServer 启动gRPC服务器供上游服务调用
func (s *Server) startGRPCServer() error {
	listener, err := net.Listen("tcp", s.config.GRPCAddr)
	if err != nil {
		return fmt.Errorf("gRPC监听失败: %w", err)
	}

	grpcServer := grpc.NewServer()

	// 注册GatewayService
	pb.RegisterGatewayServiceServer(grpcServer, s)

	// 启动gRPC服务器
	go func() {
		log.Printf("gRPC服务器已启动: %s", s.config.GRPCAddr)
		if err := grpcServer.Serve(listener); err != nil {
			log.Printf("gRPC服务器错误: %v", err)
		}
	}()

	// 保存引用以便停止时清理
	s.grpcServer = grpcServer

	return nil
}

// isRunning 检查服务器是否在运行
func (s *Server) isRunning() bool {
	s.runningMutex.RLock()
	defer s.runningMutex.RUnlock()
	return s.running
}

// initUpstreamConnections 初始化上游服务连接
func (s *Server) initUpstreamConnections() error {
	// 使用新的OpenID路由器，不需要预连接
	// 上游服务将通过RegisterUpstream gRPC调用主动注册

	if s.upstreamRouter == nil {
		return fmt.Errorf("上游服务路由器未初始化")
	}

	// 路由器已初始化，等待上游服务注册
	log.Printf("上游路由器已准备就绪，支持6个zone的OpenID路由")

	log.Printf("已初始化基于OpenID的上游服务路由器")
	log.Printf("等待上游服务通过gRPC RegisterUpstream调用进行注册...")
	return nil
}
