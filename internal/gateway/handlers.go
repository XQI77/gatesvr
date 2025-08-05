// Package gateway 提供消息处理功能
package gateway

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"google.golang.org/protobuf/proto"

	"gatesvr/internal/session"
	pb "gatesvr/proto"
)

// handleHeartbeat 处理心跳消息
func (s *Server) handleHeartbeat(sess *session.Session, req *pb.ClientRequest) bool {
	startTime := time.Now()
	defer func() {
		s.performanceTracker.RecordTotalLatency(time.Since(startTime))
	}()

	// 心跳消息不验证序列号（序列号应为0）
	if req.SeqId != 0 {
		log.Printf("心跳消息序列号应为0 - 会话: %s, 实际序列号: %d", sess.ID, req.SeqId)
		return false
	}

	// 解析心跳请求 - 记录解析时延
	parseStart := time.Now()
	heartbeatReq := &pb.HeartbeatRequest{}
	if err := proto.Unmarshal(req.Payload, heartbeatReq); err != nil {
		log.Printf("解析心跳请求失败: %v", err)
		return false
	}
	s.performanceTracker.RecordParseLatency(time.Since(parseStart))

	// 发送心跳响应（使用有序发送器）
	if err := s.orderedSender.SendHeartbeatResponse(sess, req.MsgId, heartbeatReq.ClientTimestamp); err != nil {
		log.Printf("发送心跳响应失败: %v", err)
		return false
	}

	log.Printf("处理心跳请求 - 客户端: %s, 会话: %s", sess.ClientID, sess.ID)
	return true
}

// handleBusinessRequest 处理业务请求
func (s *Server) handleBusinessRequest(ctx context.Context, sess *session.Session, req *pb.ClientRequest) bool {
	startTime := time.Now()
	log.Printf("处理业务请求 - 动作: %s", sess.ID)
	defer func() {
		s.metrics.ObserveRequestDuration("business", time.Since(startTime))
		s.performanceTracker.RecordTotalLatency(time.Since(startTime))
	}()

	// 验证业务消息序列号（业务消息必须有序列号） - 记录序列号验证时延
	seqValidateStart := time.Now()
	if !s.sessionManager.ValidateClientSequence(sess.ID, req.SeqId) {
		expectedSeq := s.sessionManager.GetExpectedClientSequence(sess.ID)
		log.Printf("业务消息序列号验证失败 - 会话: %s, 实际序列号: %d, 期待序列号: %d",
			sess.ID, req.SeqId, expectedSeq)
		s.sendErrorResponse(sess, req.MsgId, 400, "无效的序列号",
			fmt.Sprintf("序列号必须递增且连续，期待: %d, 实际: %d", expectedSeq, req.SeqId))
		return false
	}
	seqValidateLatency := time.Since(seqValidateStart)

	// 解析业务请求 - 记录解析时延
	parseStart := time.Now()
	businessReq := &pb.BusinessRequest{}
	if err := proto.Unmarshal(req.Payload, businessReq); err != nil {
		log.Printf("解析业务请求失败: %v", err)
		s.sendErrorResponse(sess, req.MsgId, 400, "解析请求失败", err.Error())
		return false
	}
	parseLatency := time.Since(parseStart)
	s.performanceTracker.RecordParseLatency(parseLatency)

	// 检查是否为登录请求 - 记录状态验证时延
	stateValidateStart := time.Now()
	isLoginAction := s.isLoginAction(businessReq.Action)

	// 验证会话状态权限
	if isLoginAction {
		// 登录请求：必须在Inited或Normal状态
		if !sess.CanProcessLoginRequest() {
			log.Printf("会话状态不允许登录请求 - 会话: %s, 状态: %d", sess.ID, sess.State())
			s.sendErrorResponse(sess, req.MsgId, 403, "会话状态错误", "当前状态不允许登录")
			return false
		}
	} else {
		// 其他业务请求：必须在Normal状态（已登录）
		if !sess.CanProcessBusinessRequest() {
			log.Printf("会话未登录，拒绝业务请求 - 会话: %s, 动作: %s, 状态: %d", sess.ID, businessReq.Action, sess.State())
			s.sendErrorResponse(sess, req.MsgId, 401, "未登录", "请先登录")
			return false
		}
	}
	stateValidateLatency := time.Since(stateValidateStart)

	// 构造上游请求（添加客户端序列号） - 记录构造请求时延
	buildReqStart := time.Now()
	upstreamReq := &pb.UpstreamRequest{
		SessionId:   sess.ID,
		Openid:      sess.OpenID,
		Action:      businessReq.Action,
		Params:      businessReq.Params,
		Data:        businessReq.Data,
		Headers:     req.Headers,
		ClientSeqId: req.SeqId, // 传递客户端序列号
	}
	buildReqLatency := time.Since(buildReqStart)

	// 调用上游服务 - 记录上游调用时延
	upstreamStart := time.Now()
	upstreamResp, err := s.upstreamClient.ProcessRequest(ctx, upstreamReq)
	upstreamLatency := time.Since(upstreamStart)
	s.performanceTracker.RecordUpstreamLatency(upstreamLatency)

	if err != nil {
		log.Printf("调用上游服务失败: %v", err)
		s.metrics.IncError("upstream_error")
		s.sendErrorResponse(sess, req.MsgId, 500, "上游服务错误", err.Error())
		return false
	}

	// 处理登录成功的情况 - 记录登录处理时延
	var loginProcessLatency time.Duration
	if isLoginAction && upstreamResp.Code == 200 {
		loginProcessStart := time.Now()
		if err := s.handleLoginSuccess(sess, upstreamResp); err != nil {
			log.Printf("处理登录成功失败: %v", err)
			s.sendErrorResponse(sess, req.MsgId, 500, "登录处理失败", err.Error())
			return false
		}
		loginProcessLatency = time.Since(loginProcessStart)
	}

	// 发送业务响应（使用有序发送器，gatesvr统一管理序列号） - 记录发送响应时延
	sendRespStart := time.Now()
	if err := s.orderedSender.SendBusinessResponse(sess, req.MsgId, upstreamResp.Code, upstreamResp.Message, upstreamResp.Data, upstreamResp.Headers); err != nil {
		log.Printf("发送业务响应失败: %v", err)
		return false
	}
	sendRespLatency := time.Since(sendRespStart)

	// 如果需要处理绑定的notify消息，使用保序机制 - 记录notify处理时延
	processNotifyStart := time.Now()
	grid := uint32(req.SeqId)
	if err := s.processBoundNotifies(sess, grid); err != nil {
		log.Printf("处理绑定notify消息失败: %v", err)
		// 不返回错误，因为主响应已经发送成功
	}
	processNotifyLatency := time.Since(processNotifyStart)

	// 记录各阶段详细时延到性能跟踪器

	s.performanceTracker.RecordSeqValidateLatency(seqValidateLatency)
	s.performanceTracker.RecordParseLatency(parseLatency)
	s.performanceTracker.RecordStateValidateLatency(stateValidateLatency)
	s.performanceTracker.RecordBuildReqLatency(buildReqLatency)
	s.performanceTracker.RecordUpstreamLatency(upstreamLatency)
	if loginProcessLatency > 0 {
		s.performanceTracker.RecordLoginProcessLatency(loginProcessLatency)
	}
	s.performanceTracker.RecordSendRespLatency(sendRespLatency)
	s.performanceTracker.RecordProcessNotifyLatency(processNotifyLatency)

	log.Printf("处理业务请求 - 动作: %s, 会话: %s, 响应码: %d, 时延: 解析=%.2fms, 上游=%.2fms",
		businessReq.Action, sess.ID, upstreamResp.Code,
		parseLatency.Seconds()*1000,
		upstreamLatency.Seconds()*1000)
	return true
}

// isLoginAction 判断是否为登录动作
func (s *Server) isLoginAction(action string) bool {
	// 定义登录相关的动作列表
	loginActions := []string{"login", "auth", "signin", "hello"}

	for _, loginAction := range loginActions {
		if action == loginAction {
			return true
		}
	}
	return false
}

// handleLoginSuccess 处理登录成功后的会话激活
func (s *Server) handleLoginSuccess(sess *session.Session, upstreamResp *pb.UpstreamResponse) error {
	// 从响应头中获取GID和Zone信息
	var gid, zone int64
	var err error

	if gidStr, exists := upstreamResp.Headers["gid"]; exists && gidStr != "" {
		gid, err = strconv.ParseInt(gidStr, 10, 64)
		if err != nil {
			return fmt.Errorf("解析GID失败: %v", err)
		}
	}

	if zoneStr, exists := upstreamResp.Headers["zone"]; exists && zoneStr != "" {
		zone, err = strconv.ParseInt(zoneStr, 10, 64)
		if err != nil {
			return fmt.Errorf("解析Zone失败: %v", err)
		}
	}

	// 如果没有GID，使用默认值或生成一个
	if gid == 0 {
		// 这里可以根据业务逻辑生成GID，暂时使用会话ID的哈希值
		gid = int64(hash(sess.ID))%1000000 + 1000000
		log.Printf("未提供GID，使用生成的GID: %d", gid)
	}

	// 激活会话
	if err := s.sessionManager.ActivateSession(sess.ID, gid, zone); err != nil {
		return fmt.Errorf("激活会话失败: %v", err)
	}

	// 投递缓存的消息
	if err := s.sessionManager.DeliverCachedMessages(sess.ID); err != nil {
		log.Printf("投递缓存消息失败: %v", err)
		// 不返回错误，因为登录已经成功
	}

	log.Printf("登录成功，会话已激活 - 会话: %s, GID: %d, Zone: %d", sess.ID, gid, zone)
	return nil
}

// hash 简单的字符串哈希函数
func hash(s string) uint32 {
	h := uint32(0)
	for _, c := range s {
		h = h*31 + uint32(c)
	}
	return h
}

// handleAck 处理ACK消息
func (s *Server) handleAck(sess *session.Session, req *pb.ClientRequest) bool {
	startTime := time.Now()
	defer func() {
		s.performanceTracker.RecordTotalLatency(time.Since(startTime))
	}()

	// ACK消息不验证序列号（序列号应为0）
	if req.SeqId != 0 {
		log.Printf("ACK消息序列号应为0 - 会话: %s, 实际序列号: %d", sess.ID, req.SeqId)
		return false
	}

	// 解析ACK消息 - 记录解析时延
	parseStart := time.Now()
	ack := &pb.ClientAck{}
	if err := proto.Unmarshal(req.Payload, ack); err != nil {
		log.Printf("解析ACK消息失败: %v", err)
		return false
	}
	s.performanceTracker.RecordParseLatency(time.Since(parseStart))

	// 批量确认消息（清除序列号小于等于ack.AckedSeqId的所有消息）
	ackedCount := s.sessionManager.AckMessagesUpTo(sess.ID, ack.AckedSeqId)

	// 同步有序队列的序列号状态（用于重连时的序列号同步）
	s.orderedSender.ResyncSessionSequence(sess, ack.AckedSeqId)

	// 更新队列大小指标
	s.metrics.SetOutboundQueueSize(sess.ID, s.sessionManager.GetPendingCount(sess.ID))

	if ackedCount > 0 {
		log.Printf("收到ACK确认 - 序列号: %d, 会话: %s, 清除消息数: %d", ack.AckedSeqId, sess.ID, ackedCount)
		return true
	} else {
		log.Printf("ACK确认无效 - 序列号: %d, 会话: %s", ack.AckedSeqId, sess.ID)
		return false
	}
}

// sendErrorResponse 发送错误响应
func (s *Server) sendErrorResponse(sess *session.Session, msgID uint32, code int32, message, detail string) {
	if err := s.orderedSender.SendErrorResponse(sess, msgID, code, message, detail); err != nil {
		log.Printf("发送错误响应失败: %v", err)
	}
}

// handleStart 处理连接建立请求 - 异步版本
func (s *Server) handleStart(sess *session.Session, req *pb.ClientRequest) bool {
	log.Printf("异步处理start请求 - 会话: %s, 消息ID: %d", sess.ID, req.MsgId)
	startTime := time.Now()
	defer func() {
		s.performanceTracker.RecordTotalLatency(time.Since(startTime))
	}()

	// 如果异步处理器不可用，回退到同步处理
	if s.startProcessor == nil {
		log.Printf("START异步处理器未初始化，使用同步处理 - 会话: %s", sess.ID)
		return s.handleStartSync(sess, req)
	}

	// 使用异步处理器处理START消息
	result, err := s.startProcessor.ProcessStartMessage(sess, req)
	if err != nil {
		log.Printf("START消息异步处理失败: %v", err)
		s.sendErrorResponse(sess, req.MsgId, 500, "处理失败", err.Error())
		return false
	}

	// 处理结果
	if !result.Success {
		log.Printf("START消息处理失败 - 会话: %s, 错误: %v", sess.ID, result.Error)

		// 发送错误响应
		if result.Response != nil && result.Response.Error != nil {
			s.sendErrorResponse(sess, req.MsgId, result.Response.Error.ErrorCode,
				result.Response.Error.ErrorMessage, result.Response.Error.Detail)
		} else {
			s.sendErrorResponse(sess, req.MsgId, 500, "处理失败", result.Error.Error())
		}
		return false
	}

	// 成功处理，发送响应
	if err := s.orderedSender.SendStartResponse(sess, req.MsgId,
		result.Response.Success, nil,
		result.Response.HeartbeatInterval, result.Response.ConnectionId); err != nil {
		log.Printf("发送连接建立响应失败: %v", err)
		return false
	}

	log.Printf("START消息异步处理成功 - 会话: %s, 处理时间: %.2fms",
		sess.ID, result.ProcessTime.Seconds()*1000)
	return true
}

// handleStartSync 同步处理START消息（备用方案）
func (s *Server) handleStartSync(sess *session.Session, req *pb.ClientRequest) bool {
	log.Printf("同步处理start请求 - 会话: %s, 消息ID: %d", sess.ID, req.MsgId)

	// START消息不验证序列号（序列号应为0）
	if req.SeqId != 0 {
		log.Printf("START消息序列号应为0 - 会话: %s, 实际序列号: %d", sess.ID, req.SeqId)
		s.sendErrorResponse(sess, req.MsgId, 400, "无效的序列号", "START消息序列号必须为0")
		return false
	}

	// 解析连接建立请求 - 记录解析时延
	parseStart := time.Now()
	startReq := &pb.StartRequest{}
	if err := proto.Unmarshal(req.Payload, startReq); err != nil {
		log.Printf("解析连接建立请求失败: %v", err)
		s.sendErrorResponse(sess, req.MsgId, 400, "解析请求失败", err.Error())
		return false
	}
	s.performanceTracker.RecordParseLatency(time.Since(parseStart))

	// 验证必要字段
	if startReq.Openid == "" {
		log.Printf("连接建立请求缺少OpenID - 会话: %s", sess.ID)
		s.sendErrorResponse(sess, req.MsgId, 400, "缺少OpenID", "OpenID不能为空")
		return false
	}

	// 更新session的客户端信息
	sess.OpenID = startReq.Openid
	sess.ClientID = req.Openid // 从请求头中获取openid作为备份
	sess.AccessToken = startReq.AuthToken

	// 基础认证验证（可选）
	if startReq.AuthToken != "" {
		if !s.validateAuthToken(startReq.AuthToken, startReq.Openid) {
			log.Printf("认证令牌验证失败 - OpenID: %s, 会话: %s", startReq.Openid, sess.ID)
			s.sendErrorResponse(sess, req.MsgId, 401, "认证失败", "无效的认证令牌")
			return false
		}
	}

	// 将session注册到openid索引中
	if err := s.sessionManager.BindSession(sess); err != nil {
		log.Printf("绑定session到openid失败: %v", err)
		s.sendErrorResponse(sess, req.MsgId, 500, "会话绑定失败", err.Error())
		return false
	}

	// 处理重连时的序列号同步
	if startReq.LastAckedSeqId > 0 {
		// 清除已确认的消息
		ackedCount := s.sessionManager.AckMessagesUpTo(sess.ID, startReq.LastAckedSeqId)

		// 同步有序队列的序列号
		s.orderedSender.ResyncSessionSequence(sess, startReq.LastAckedSeqId)

		log.Printf("重连时清除已确认消息 - 会话: %s, 最后确认序列号: %d, 清除数量: %d",
			sess.ID, startReq.LastAckedSeqId, ackedCount)
	}

	// 发送连接建立响应（使用有序发送器）
	connectionID := sess.ID // 使用session ID作为连接ID
	if err := s.orderedSender.SendStartResponse(sess, req.MsgId, true, nil, 30, connectionID); err != nil {
		log.Printf("发送连接建立响应失败: %v", err)
		return false
	}

	log.Printf("同步处理连接建立请求成功 - OpenID: %s, 客户端: %s, 会话: %s, 状态: %d",
		startReq.Openid, sess.ClientID, sess.ID, sess.State())

	return true
}

// validateAuthToken 验证认证令牌（示例实现）
func (s *Server) validateAuthToken(authToken, openID string) bool {
	// 这里应该实现真正的令牌验证逻辑
	// 例如：验证JWT令牌、检查令牌有效期、验证签名等

	// 基础验证：令牌不能为空且长度合理
	if authToken == "" || len(authToken) < 10 {
		return false
	}

	// 简单的示例验证：检查令牌是否包含OpenID
	// 在实际项目中，这里应该调用认证服务或验证JWT
	if len(authToken) > 50 && openID != "" {
		// 简单通过，实际项目中需要真正的验证逻辑
		return true
	}

	// 对于没有认证令牌的情况，允许连接但要求后续登录
	return authToken == "" || len(authToken) >= 10
}

// handleStop 处理连接断开请求
func (s *Server) handleStop(sess *session.Session, req *pb.ClientRequest) bool {
	startTime := time.Now()
	defer func() {
		s.performanceTracker.RecordTotalLatency(time.Since(startTime))
	}()

	// 解析连接断开请求 - 记录解析时延
	parseStart := time.Now()
	stopReq := &pb.StopRequest{}
	if err := proto.Unmarshal(req.Payload, stopReq); err != nil {
		log.Printf("解析连接断开请求失败: %v", err)
		return false
	}
	s.performanceTracker.RecordParseLatency(time.Since(parseStart))

	// 根据断开原因确定处理策略
	reason := "client_requested"
	switch stopReq.Reason {
	case pb.StopRequest_USER_LOGOUT:
		reason = "user_logout"
	case pb.StopRequest_APP_CLOSE:
		reason = "app_close"
	case pb.StopRequest_SWITCH_ACCOUNT:
		reason = "switch_account"
	default:
		reason = "unknown"
	}

	log.Printf("收到连接断开请求 - 客户端: %s, 会话: %s, 原因: %s",
		sess.ClientID, sess.ID, reason)

	// 立即删除session，不延迟清理
	s.sessionManager.RemoveSessionWithDelay(sess.ID, false, reason)

	log.Printf("处理连接断开请求完成 - 会话: %s, 原因: %s", sess.ID, reason)

	// 断开连接请求处理完成后，连接将被关闭，返回false表示连接应该结束
	return false
}

// processBoundNotifies 处理绑定在response前后的notify消息
func (s *Server) processBoundNotifies(sess *session.Session, grid uint32) error {
	// 发送绑定在response之前的notify消息
	if err := s.sendBeforeNotifies(sess, grid); err != nil {
		log.Printf("发送before-notify失败: %v", err)
		return err
	}

	// 发送绑定在response之后的notify消息
	if err := s.sendAfterNotifies(sess, grid); err != nil {
		log.Printf("发送after-notify失败: %v", err)
		return err
	}

	return nil
}

// sendBeforeNotifies 发送绑定在response之前的notify消息
func (s *Server) sendBeforeNotifies(sess *session.Session, grid uint32) error {
	if sess.GetOrderingManager() == nil {
		return nil
	}

	beforeNotifies := sess.GetOrderingManager().GetAndRemoveBeforeNotifies(grid)
	if len(beforeNotifies) == 0 {
		return nil
	}

	for _, notifyItem := range beforeNotifies {
		// 使用有序发送器发送notify，gatesvr自动分配序列号
		if err := s.orderedSender.PushBusinessData(sess, notifyItem.NotifyData); err != nil {
			log.Printf("发送before-notify失败 - 会话: %s, 错误: %v", sess.ID, err)
			return err
		}
		log.Printf("发送before-notify成功 - 会话: %s, 类型: %s", sess.ID, notifyItem.MsgType)
	}

	return nil
}

// sendAfterNotifies 发送绑定在response之后的notify消息
func (s *Server) sendAfterNotifies(sess *session.Session, grid uint32) error {
	if sess.GetOrderingManager() == nil {
		return nil
	}

	afterNotifies := sess.GetOrderingManager().GetAndRemoveAfterNotifies(grid)
	if len(afterNotifies) == 0 {
		return nil
	}

	for _, notifyItem := range afterNotifies {
		// 使用有序发送器发送notify，gatesvr自动分配序列号
		if err := s.orderedSender.PushBusinessData(sess, notifyItem.NotifyData); err != nil {
			log.Printf("发送after-notify失败 - 会话: %s, 错误: %v", sess.ID, err)
			return err
		}
		log.Printf("发送after-notify成功 - 会话: %s, 类型: %s", sess.ID, notifyItem.MsgType)
	}

	return nil
}
