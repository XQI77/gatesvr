package gateway

import (
	"context"
	"fmt"
	"log"
	"strconv"

	"gatesvr/internal/session"
	pb "gatesvr/proto"
)

// PushToClient 实现单播推送到指定客户端
func (s *Server) PushToClient(ctx context.Context, req *pb.UnicastPushRequest) (*pb.UnicastPushResponse, error) {
	log.Printf("收到单播推送请求 - 目标类型: %s, 目标ID: %s, 消息类型: %s",
		req.TargetType, req.TargetId, req.MsgType)

	var err error
	switch req.TargetType {
	case "session":
		err = s.handleUnicastToSession(req.TargetId, req.MsgType, req.Title, req.Content, req.Data)
	case "gid":
		if gid, parseErr := strconv.ParseInt(req.TargetId, 10, 64); parseErr == nil {
			err = s.handleUnicastToGID(gid, req.MsgType, req.Title, req.Content, req.Data)
		} else {
			err = fmt.Errorf("无效的GID: %s", req.TargetId)
		}
	case "openid":
		err = s.handleUnicastToOpenID(req.TargetId, req.MsgType, req.Title, req.Content, req.Data)
	default:
		err = fmt.Errorf("不支持的推送目标类型: %s", req.TargetType)
	}

	success := err == nil
	message := "推送成功"
	errorCode := ""

	if err != nil {
		message = err.Error()
		errorCode = "PUSH_FAILED"
	}

	return &pb.UnicastPushResponse{
		Success:   success,
		Message:   message,
		ErrorCode: errorCode,
	}, nil
}

// handleUnicastToSession 处理到会话的单播推送
func (s *Server) handleUnicastToSession(sessionID, msgType, title, content string, data []byte) error {
	session, exists := s.sessionManager.GetSession(sessionID)
	if !exists {
		return fmt.Errorf("会话不存在: %s", sessionID)
	}

	// 检查会话状态
	if !session.IsNormal() {
		// 会话未激活，缓存消息
		return s.cacheMessageForSession(sessionID, msgType, title, content, data)
	}

	// 会话已激活，直接发送
	return s.sendUnicastMessage(session, msgType, title, content, data)
}

// handleUnicastToGID 处理到GID的单播推送
func (s *Server) handleUnicastToGID(gid int64, msgType, title, content string, data []byte) error {
	session, exists := s.sessionManager.GetSessionByGID(gid)
	if !exists {
		// GID对应的会话不存在，可能用户未登录，缓存消息
		return s.cacheMessageForGID(gid, msgType, title, content, data)
	}

	// 检查会话状态
	if !session.IsNormal() {
		// 会话未激活，缓存消息
		return s.cacheMessageForSession(session.ID, msgType, title, content, data)
	}

	// 会话已激活，直接发送
	return s.sendUnicastMessage(session, msgType, title, content, data)
}

// handleUnicastToOpenID 处理到OpenID的单播推送
func (s *Server) handleUnicastToOpenID(openID, msgType, title, content string, data []byte) error {
	session, exists := s.sessionManager.GetSessionByOpenID(openID)
	if !exists {
		// OpenID对应的会话不存在，可能用户未连接，缓存消息
		return s.cacheMessageForOpenID(openID, msgType, title, content, data)
	}

	// 检查会话状态
	if !session.IsNormal() {
		// 会话未激活，缓存消息
		return s.cacheMessageForSession(session.ID, msgType, title, content, data)
	}

	// 会话已激活，直接发送
	return s.sendUnicastMessage(session, msgType, title, content, data)
}

// sendUnicastMessage 发送单播消息到已激活的会话
func (s *Server) sendUnicastMessage(session *session.Session, msgType, title, content string, data []byte) error {
	if session.IsClosed() {
		return fmt.Errorf("会话已关闭: %s", session.ID)
	}

	// 可以根据msgType设置不同的推送类型
	if title != "" || content != "" {
		// 这里可以构造通知载荷，暂时直接使用原始数据
		log.Printf("推送通知消息 - 标题: %s, 内容: %s", title, content)
	}

	// 发送推送消息（使用有序发送器）
	if err := s.orderedSender.PushBusinessData(session, data); err != nil {
		log.Printf("发送推送消息失败: %v", err)
		return fmt.Errorf("发送推送消息失败: %w", err)
	}

	log.Printf("单播推送成功 - 会话: %s, 类型: %s", session.ID, msgType)
	return nil
}

// cacheMessageForSession 为会话缓存消息
func (s *Server) cacheMessageForSession(sessionID, msgType, title, content string, data []byte) error {
	// 构造完整的推送消息数据
	session, exists := s.sessionManager.GetSession(sessionID)
	if !exists {
		return fmt.Errorf("会话不存在: %s", sessionID)
	}

	// 创建推送消息（使用serverSeq保证顺序）
	push := &pb.ServerPush{
		Type:    pb.PushType_PUSH_BUSINESS_DATA,
		SeqId:   session.NewServerSeq(), // 使用serverSeq而不是nextSeqID
		Payload: data,
	}

	// 编码消息
	encodedData, err := s.messageCodec.EncodeServerPush(push)
	if err != nil {
		return fmt.Errorf("编码推送消息失败: %v", err)
	}

	// 缓存消息
	return s.sessionManager.CacheMessageForInactiveSession(sessionID, encodedData)
}

// cacheMessageForGID 为GID缓存消息（GID对应的会话不存在时）
func (s *Server) cacheMessageForGID(gid int64, msgType, title, content string, data []byte) error {
	// 这里可以实现基于GID的消息缓存
	// 暂时记录日志，实际项目中可能需要独立的消息队列服务
	log.Printf("为离线GID缓存消息 - GID: %d, 类型: %s, 标题: %s", gid, msgType, title)

	// 可以实现持久化缓存，比如存储到数据库或消息队列
	// 当用户登录时再从持久化存储中获取消息

	return nil // 暂时返回成功
}

// cacheMessageForOpenID 为OpenID缓存消息（OpenID对应的会话不存在时）
func (s *Server) cacheMessageForOpenID(openID, msgType, title, content string, data []byte) error {
	// 这里可以实现基于OpenID的消息缓存
	// 暂时记录日志，实际项目中可能需要独立的消息队列服务
	log.Printf("为离线OpenID缓存消息 - OpenID: %s, 类型: %s, 标题: %s", openID, msgType, title)

	// 可以实现持久化缓存，比如存储到数据库或消息队列
	// 当用户连接时再从持久化存储中获取消息

	return nil // 暂时返回成功
}

// BatchPushToClients 实现批量单播推送
func (s *Server) BatchPushToClients(ctx context.Context, req *pb.BatchUnicastPushRequest) (*pb.BatchUnicastPushResponse, error) {
	results := make([]*pb.UnicastResult, 0, len(req.Targets))
	successCount := int32(0)
	totalCount := int32(len(req.Targets))

	for _, target := range req.Targets {
		result := &pb.UnicastResult{
			TargetType: target.TargetType,
			TargetId:   target.TargetId,
		}

		// 构造单播请求
		unicastReq := &pb.UnicastPushRequest{
			TargetType: target.TargetType,
			TargetId:   target.TargetId,
			MsgType:    req.MsgType,
			Title:      req.Title,
			Content:    req.Content,
			Data:       req.Data,
			Metadata:   req.Metadata,
		}

		// 执行单播推送
		resp, err := s.PushToClient(ctx, unicastReq)
		if err == nil && resp.Success {
			result.Success = true
			successCount++
		} else {
			result.Success = false
			if err != nil {
				result.ErrorMessage = err.Error()
			} else {
				result.ErrorMessage = resp.Message
			}
		}

		results = append(results, result)
	}

	log.Printf("批量单播推送完成 - 总数: %d, 成功: %d", totalCount, successCount)

	return &pb.BatchUnicastPushResponse{
		SuccessCount: successCount,
		TotalCount:   totalCount,
		Results:      results,
	}, nil
}

// 确保Server实现了GatewayService接口
var _ pb.GatewayServiceServer = (*Server)(nil)
