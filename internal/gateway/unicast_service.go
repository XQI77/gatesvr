package gateway

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"gatesvr/internal/session"
	pb "gatesvr/proto"

	"google.golang.org/protobuf/proto"
)

// PushToClient 实现单播推送到指定客户端 - 支持消息保序机制
func (s *Server) PushToClient(ctx context.Context, req *pb.UnicastPushRequest) (*pb.UnicastPushResponse, error) {
	log.Printf("收到单播推送请求 - 目标类型: %s, 目标ID: %s, 消息类型: %s, 同步提示: %v",
		req.TargetType, req.TargetId, req.MsgType, req.SyncHint)

	var err error
	switch req.TargetType {
	case "session":
		err = s.handleUnicastToSessionWithOrdering(req)
	case "gid":
		if gid, parseErr := strconv.ParseInt(req.TargetId, 10, 64); parseErr == nil {
			err = s.handleUnicastToGIDWithOrdering(gid, req)
		} else {
			err = fmt.Errorf("无效的GID: %s", req.TargetId)
		}
	case "openid":
		err = s.handleUnicastToOpenIDWithOrdering(req)
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

// cacheMessageForSession 为会话缓存消息
func (s *Server) cacheMessageForSession(sessionID, msgType, title, content string, data []byte) error {
	// 构造BusinessResponse格式的消息（确保客户端能正确解析）
	businessResp := &pb.BusinessResponse{
		Code:    200,
		Message: fmt.Sprintf("[%s] %s", msgType, title),
		Data:    data,
	}

	// 序列化BusinessResponse
	payload, err := proto.Marshal(businessResp)
	if err != nil {
		return fmt.Errorf("序列化缓存BusinessResponse失败: %v", err)
	}

	// 构造ServerPush消息，但不分配序列号（缓存的消息在投递时才分配序列号）
	push := &pb.ServerPush{
		Type:    pb.PushType_PUSH_BUSINESS_DATA,
		SeqId:   0, // 缓存时不分配序列号，投递时由有序发送器分配
		Payload: payload,
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

// ======= 支持消息保序机制的新处理函数 =======

// handleUnicastToSessionWithOrdering 使用消息保序机制处理到会话的单播推送
func (s *Server) handleUnicastToSessionWithOrdering(req *pb.UnicastPushRequest) error {
	session, exists := s.sessionManager.GetSession(req.TargetId)
	if !exists {
		return fmt.Errorf("会话不存在: %s", req.TargetId)
	}

	// 检查会话状态
	if !session.IsNormal() {
		// 会话未激活，缓存消息（保持原有逻辑）
		return s.cacheMessageForSession(req.TargetId, req.MsgType, req.Title, req.Content, req.Data)
	}

	// 会话已激活，根据SyncHint处理消息
	return s.processNotifyMessage(session, req)
}

// handleUnicastToGIDWithOrdering 使用消息保序机制处理到GID的单播推送
func (s *Server) handleUnicastToGIDWithOrdering(gid int64, req *pb.UnicastPushRequest) error {
	session, exists := s.sessionManager.GetSessionByGID(gid)
	if !exists {
		// GID对应的会话不存在，可能用户未登录，缓存消息
		return s.cacheMessageForGID(gid, req.MsgType, req.Title, req.Content, req.Data)
	}

	// 检查会话状态
	if !session.IsNormal() {
		// 会话未激活，缓存消息
		return s.cacheMessageForSession(session.ID, req.MsgType, req.Title, req.Content, req.Data)
	}

	// 会话已激活，根据SyncHint处理消息
	return s.processNotifyMessage(session, req)
}

// handleUnicastToOpenIDWithOrdering 使用消息保序机制处理到OpenID的单播推送
func (s *Server) handleUnicastToOpenIDWithOrdering(req *pb.UnicastPushRequest) error {
	session, exists := s.sessionManager.GetSessionByOpenID(req.TargetId)
	if !exists {
		// OpenID对应的会话不存在，可能用户未连接，缓存消息
		return s.cacheMessageForOpenID(req.TargetId, req.MsgType, req.Title, req.Content, req.Data)
	}

	// 检查会话状态
	if !session.IsNormal() {
		// 会话未激活，缓存消息
		return s.cacheMessageForSession(session.ID, req.MsgType, req.Title, req.Content, req.Data)
	}

	// 会话已激活，根据SyncHint处理消息
	return s.processNotifyMessage(session, req)
}

// BroadcastToClients 实现广播推送到所有在线客户端
func (s *Server) BroadcastToClients(ctx context.Context, req *pb.BroadcastRequest) (*pb.BroadcastResponse, error) {
	log.Printf("收到广播推送请求 - 消息类型: %s, 标题: %s", req.MsgType, req.Title)

	// 获取所有活跃会话
	sessions := s.sessionManager.GetAllSessions()
	successCount := int32(0)
	totalCount := int32(len(sessions))

	for _, session := range sessions {
		// 只向已激活的会话广播
		if !session.IsNormal() || session.IsClosed() {
			continue
		}

		// 发送广播消息
		if err := s.sendBroadcastMessage(session, req); err != nil {
			log.Printf("广播消息发送失败 - 会话: %s, 错误: %v", session.ID, err)
			continue
		}

		successCount++
	}

	message := fmt.Sprintf("广播消息已发送到 %d/%d 个在线客户端", successCount, totalCount)
	log.Printf("广播推送完成 - %s", message)

	return &pb.BroadcastResponse{
		SentCount: successCount,
		Message:   message,
	}, nil
}

// sendBroadcastMessage 发送广播消息到指定会话
func (s *Server) sendBroadcastMessage(session *session.Session, req *pb.BroadcastRequest) error {
	if session.IsClosed() {
		return fmt.Errorf("会话已关闭: %s", session.ID)
	}

	// 构造BusinessResponse格式的广播消息（确保客户端能正确解析）
	businessResp := &pb.BusinessResponse{
		Code:    200,
		Message: fmt.Sprintf("[%s] %s", req.MsgType, req.Title),
		Data:    req.Data,
	}

	// 序列化BusinessResponse
	payload, err := proto.Marshal(businessResp)
	if err != nil {
		return fmt.Errorf("序列化广播BusinessResponse失败: %v", err)
	}

	// 直接使用有序发送器发送广播消息，让gatesvr统一管理序列号
	if err := s.orderedSender.PushBusinessData(session, payload); err != nil {
		return fmt.Errorf("发送广播消息失败: %w", err)
	}

	return nil
}

// processNotifyMessage 在gateway层处理notify消息，统一管理序列号
func (s *Server) processNotifyMessage(session *session.Session, req *pb.UnicastPushRequest) error {
	// 根据SyncHint决定处理方式
	switch req.SyncHint {
	case pb.NotifySyncHint_NSH_BEFORE_RESPONSE:
		// 绑定到指定response之前发送
		grid := uint32(req.BindClientSeqId)
		if grid == 0 {
			return fmt.Errorf("bind_client_seq_id is required for NSH_BEFORE_RESPONSE")
		}

		// 创建NotifyBindMsgItem
		notifyItem := s.createNotifyBindMsgItem(req)
		if !session.AddNotifyBindBeforeRsp(grid, notifyItem) {
			return fmt.Errorf("failed to bind notify before response for grid: %d", grid)
		}

		log.Printf("Notify消息已绑定到response之前 - grid: %d, 会话: %s", grid, session.ID)
		return nil

	case pb.NotifySyncHint_NSH_AFTER_RESPONSE:
		// 绑定到指定response之后发送
		grid := uint32(req.BindClientSeqId)
		if grid == 0 {
			return fmt.Errorf("bind_client_seq_id is required for NSH_AFTER_RESPONSE")
		}

		// 创建NotifyBindMsgItem
		notifyItem := s.createNotifyBindMsgItem(req)
		if !session.AddNotifyBindAfterRsp(grid, notifyItem) {
			return fmt.Errorf("failed to bind notify after response for grid: %d", grid)
		}

		log.Printf("Notify消息已绑定到response之后 - grid: %d, 会话: %s", grid, session.ID)
		return nil

	case pb.NotifySyncHint_NSH_IMMEDIATELY:
		fallthrough
	default:
		// 立即发送 - 构造BusinessResponse格式并使用有序发送器统一管理序列号
		businessResp := &pb.BusinessResponse{
			Code:    200,
			Message: fmt.Sprintf("[%s] %s", req.MsgType, req.Title),
			Data:    req.Data,
		}

		// 序列化BusinessResponse
		payload, err := proto.Marshal(businessResp)
		if err != nil {
			return fmt.Errorf("序列化立即notify BusinessResponse失败: %w", err)
		}

		if err := s.orderedSender.PushBusinessData(session, payload); err != nil {
			return fmt.Errorf("发送立即notify失败: %w", err)
		}

		log.Printf("立即notify消息发送成功 - 会话: %s, 类型: %s, 标题: %s", session.ID, req.MsgType, req.Title)
		return nil
	}
}

// createNotifyBindMsgItem 创建NotifyBindMsgItem
func (s *Server) createNotifyBindMsgItem(req *pb.UnicastPushRequest) *session.NotifyBindMsgItem {
	// 序列化notify消息数据
	notifyData, err := proto.Marshal(req)
	if err != nil {
		log.Printf("序列化notify消息失败: %v", err)
		// 使用原始数据作为备选
		notifyData = req.Data
	}

	return &session.NotifyBindMsgItem{
		NotifyData: notifyData,
		MsgType:    req.MsgType,
		Title:      req.Title,
		Content:    req.Content,
		Metadata:   req.Metadata,
		SyncHint:   req.SyncHint,
		BindGrid:   uint32(req.BindClientSeqId),
		CreateTime: time.Now().Unix(),
	}
}

// 确保Server实现了GatewayService接口
var _ pb.GatewayServiceServer = (*Server)(nil)
