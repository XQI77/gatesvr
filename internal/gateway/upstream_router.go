// Package gateway 提供基于OpenID的上游服务路由功能
package gateway

import (
	"context"
	"fmt"
	pb "gatesvr/proto"
	"log"
)

// callUpstreamService 根据OpenID调用对应的上游服务
// 这是新的路由逻辑：不再基于action，而是基于openid进行一致性路由
func (s *Server) callUpstreamService(ctx context.Context, openID string, req *pb.UpstreamRequest) (*pb.UpstreamResponse, error) {
	// 验证OpenID
	if openID == "" {
		return nil, fmt.Errorf("openID cannot be empty for upstream routing")
	}

	// 使用新的基于OpenID的路由器
	if s.upstreamRouter == nil {
		return nil, fmt.Errorf("upstream router not initialized")
	}

	// 路由到对应的上游服务实例
	response, err := s.upstreamRouter.RouteByOpenID(ctx, openID, req)
	if err != nil {
		log.Printf("上游服务调用失败 - OpenID: %s, Action: %s, 错误: %v", openID, req.Action, err)
		return nil, fmt.Errorf("upstream service call failed: %w", err)
	}

	log.Printf("上游服务调用成功 - OpenID: %s, Action: %s, Code: %d",
		openID, req.Action, response.Code)

	return response, nil
}

// getUpstreamServiceInfo 获取上游服务信息（用于日志和监控）
func (s *Server) getUpstreamServiceInfo(openID string) string {
	if s.upstreamRouter == nil {
		return "路由器未初始化"
	}

	// 验证OpenID并获取对应的zone信息
	zoneID, err := s.upstreamRouter.ValidateOpenID(openID)
	if err != nil {
		return fmt.Sprintf("无效OpenID(%s): %v", openID, err)
	}

	// 获取服务实例信息
	instance, err := s.upstreamRouter.GetInstanceByZone(zoneID)
	if err != nil {
		return fmt.Sprintf("Zone %s: 无可用服务", zoneID)
	}

	return fmt.Sprintf("Zone %s: %s", zoneID, instance.Address)
}

// registerUpstreamService 注册上游服务实例
// 由上游服务启动时调用，向gateway注册自己的地址和zone_id
func (s *Server) registerUpstreamService(address, zoneID string) error {
	if s.upstreamRouter == nil {
		return fmt.Errorf("upstream router not initialized")
	}

	err := s.upstreamRouter.RegisterUpstream(address, zoneID)
	if err != nil {
		return fmt.Errorf("failed to register upstream service: %w", err)
	}

	log.Printf("上游服务注册成功 - Zone: %s, Address: %s", zoneID, address)
	return nil
}

// getUpstreamStats 获取上游服务统计信息
func (s *Server) getUpstreamStats() map[string]interface{} {
	if s.upstreamRouter == nil {
		return map[string]interface{}{
			"error": "upstream router not initialized",
		}
	}

	return s.upstreamRouter.GetStats()
}

// validateUpstreamRouting 验证上游路由配置
func (s *Server) validateUpstreamRouting() error {
	if s.upstreamRouter == nil {
		return fmt.Errorf("upstream router not initialized")
	}

	// 检查是否有注册的上游服务
	instances := s.upstreamRouter.GetAllInstances()
	if len(instances) == 0 {
		log.Printf("警告: 没有注册的上游服务实例")
		return nil
	}

	log.Printf("上游路由验证通过 - 已注册 %d 个服务实例:", len(instances))
	for zoneID, instance := range instances {
		log.Printf("  Zone %s: %s", zoneID, instance.Address)
	}

	return nil
}

// cleanupUpstreamServices 清理不活跃的上游服务实例
func (s *Server) cleanupUpstreamServices() {
	if s.upstreamRouter == nil {
		return
	}

	// TODO: 实现基于健康检查的服务清理逻辑
	// 这里可以添加定期检查upstream服务健康状态的逻辑
	// 移除不响应的服务实例
}
