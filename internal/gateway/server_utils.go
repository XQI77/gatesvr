package gateway

import (
	"fmt"
	"log"

	"gatesvr/internal/upstream"
	pb "gatesvr/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// isRunning 检查服务器是否在运行
func (s *Server) isRunning() bool {
	s.runningMutex.RLock()
	defer s.runningMutex.RUnlock()
	return s.running
}

// generateClientIDFromAddr 从连接地址生成临时客户端ID
// 这是一个临时方案，实际项目中应该从客户端握手消息中获取
func generateClientIDFromAddr(addr string) string {
	// 简单的地址hash作为临时客户端ID
	// 实际项目中应该使用更可靠的客户端标识符
	return fmt.Sprintf("client_%x", addr)
}

// initUpstreamConnections 初始化上游服务连接
func (s *Server) initUpstreamConnections() error {
	// 如果没有配置多上游服务，尝试向后兼容模式
	if s.upstreamServices.GetServiceCount() == 0 && s.config.UpstreamAddr != "" {
		// 向后兼容：连接单一上游服务
		return s.connectSingleUpstream()
	}

	// 创建上游服务管理器
	s.upstreamManager = upstream.NewServiceManager(s.upstreamServices)

	// 预连接所有配置的上游服务
	services := s.upstreamServices.GetAllServices()
	connectedCount := 0
	
	for serviceType := range services {
		if s.upstreamServices.IsServiceAvailable(serviceType) {
			// 尝试预连接服务（不阻塞启动）
			go func(st upstream.ServiceType) {
				if _, err := s.upstreamManager.GetClient(st); err != nil {
					log.Printf("警告: 预连接上游服务失败 - %s: %v", st, err)
				} else {
					log.Printf("预连接上游服务成功 - %s", st)
				}
			}(serviceType)
			connectedCount++
		}
	}

	if connectedCount == 0 {
		return fmt.Errorf("没有可用的上游服务")
	}

	log.Printf("已初始化上游服务连接管理器 - 配置服务数: %d", connectedCount)
	return nil
}

// connectSingleUpstream 向后兼容的单一上游服务连接
func (s *Server) connectSingleUpstream() error {
	conn, err := grpc.Dial(s.config.UpstreamAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("连接上游服务失败: %w", err)
	}

	s.upstreamConn = conn
	s.upstreamClient = pb.NewUpstreamServiceClient(conn)

	log.Printf("已连接到上游服务 (兼容模式): %s", s.config.UpstreamAddr)
	return nil
}
