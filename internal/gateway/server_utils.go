package gateway

import (
	"fmt"
	"log"

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

// connectUpstream 连接到上游gRPC服务
func (s *Server) connectUpstream() error {
	conn, err := grpc.Dial(s.config.UpstreamAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("连接上游服务失败: %w", err)
	}

	s.upstreamConn = conn
	s.upstreamClient = pb.NewUpstreamServiceClient(conn)

	log.Printf("已连接到上游服务: %s", s.config.UpstreamAddr)
	return nil
}
