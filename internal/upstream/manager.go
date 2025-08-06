// Package upstream 提供上游服务管理和调用接口
package upstream

import (
	"context"
	"fmt"
	"log"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	pb "gatesvr/proto"
)

// ServiceManager 上游服务管理器
type ServiceManager struct {
	services    *UpstreamServices
	connections map[string]*grpc.ClientConn // endpoint -> connection
	clients     map[string]pb.UpstreamServiceClient // endpoint -> client
	mu          sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewServiceManager 创建服务管理器
func NewServiceManager(services *UpstreamServices) *ServiceManager {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &ServiceManager{
		services:    services,
		connections: make(map[string]*grpc.ClientConn),
		clients:     make(map[string]pb.UpstreamServiceClient),
		ctx:         ctx,
		cancel:      cancel,
	}
}

// GetClient 获取指定服务类型的客户端
func (sm *ServiceManager) GetClient(serviceType ServiceType) (pb.UpstreamServiceClient, error) {
	service, err := sm.services.GetService(serviceType)
	if err != nil {
		return nil, err
	}

	if len(service.Addresses) == 0 {
		return nil, fmt.Errorf("服务 %s 没有可用地址", serviceType)
	}

	// 简单选择第一个地址（实际应用可以实现负载均衡）
	endpoint := service.Addresses[0]

	sm.mu.RLock()
	client, exists := sm.clients[endpoint]
	sm.mu.RUnlock()

	if exists {
		return client, nil
	}

	// 创建新连接和客户端
	return sm.createClient(endpoint)
}

// createClient 创建新的客户端连接
func (sm *ServiceManager) createClient(endpoint string) (pb.UpstreamServiceClient, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// 双重检查，避免重复创建
	if client, exists := sm.clients[endpoint]; exists {
		return client, nil
	}

	// 创建gRPC连接
	conn, err := grpc.Dial(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("连接上游服务失败 %s: %w", endpoint, err)
	}

	// 创建客户端
	client := pb.NewUpstreamServiceClient(conn)

	// 保存连接和客户端
	sm.connections[endpoint] = conn
	sm.clients[endpoint] = client

	log.Printf("已连接到上游服务: %s", endpoint)
	return client, nil
}

// CallService 调用指定服务
func (sm *ServiceManager) CallService(ctx context.Context, serviceType ServiceType, req *pb.UpstreamRequest) (*pb.UpstreamResponse, error) {
	client, err := sm.GetClient(serviceType)
	if err != nil {
		return nil, err
	}

	return client.ProcessRequest(ctx, req)
}

// GetAllClients 获取所有可用客户端
func (sm *ServiceManager) GetAllClients() map[ServiceType]pb.UpstreamServiceClient {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	result := make(map[ServiceType]pb.UpstreamServiceClient)
	
	for serviceTypeStr := range sm.services.GetAllServices() {
		if client, err := sm.GetClient(serviceTypeStr); err == nil {
			result[serviceTypeStr] = client
		}
	}

	return result
}

// IsServiceConnected 检查服务是否已连接
func (sm *ServiceManager) IsServiceConnected(serviceType ServiceType) bool {
	service, err := sm.services.GetService(serviceType)
	if err != nil {
		return false
	}

	if len(service.Addresses) == 0 {
		return false
	}

	endpoint := service.Addresses[0]
	
	sm.mu.RLock()
	_, exists := sm.clients[endpoint]
	sm.mu.RUnlock()

	return exists
}

// Close 关闭所有连接
func (sm *ServiceManager) Close() error {
	sm.cancel()

	sm.mu.Lock()
	defer sm.mu.Unlock()

	for endpoint, conn := range sm.connections {
		if err := conn.Close(); err != nil {
			log.Printf("关闭上游服务连接失败 %s: %v", endpoint, err)
		}
	}

	sm.connections = make(map[string]*grpc.ClientConn)
	sm.clients = make(map[string]pb.UpstreamServiceClient)

	log.Printf("上游服务管理器已关闭")
	return nil
}

// GetStats 获取统计信息
func (sm *ServiceManager) GetStats() map[string]interface{} {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	connectionStats := make(map[string]string)
	for endpoint := range sm.connections {
		connectionStats[endpoint] = "connected"
	}

	stats := sm.services.GetStats()
	stats["connections"] = connectionStats
	stats["connection_count"] = len(sm.connections)

	return stats
}