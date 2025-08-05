package gateway

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"

	"gatesvr/pkg/metrics"
	pb "gatesvr/proto"

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
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/stats", s.handleStats)
	mux.HandleFunc("/performance", s.handlePerformance)
	mux.HandleFunc("/queue", s.handleQueueStatus) // 消息队列状态查询

	// 新增的时延监控API端点
	mux.HandleFunc("/latency/detailed", s.handleDetailedLatency)
	mux.HandleFunc("/latency/breakdown", s.handleLatencyBreakdown)
	mux.HandleFunc("/latency/client", s.handleClientLatency)
	
	// START消息处理器监控API
	mux.HandleFunc("/start-processor", s.handleStartProcessor)

	// 注意：单播推送现在通过gRPC接口提供，HTTP API已移除

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
