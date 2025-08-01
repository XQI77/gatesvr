package gateway

import (
	"fmt"
	"log"
	"time"

	"gatesvr/internal/message"
	"gatesvr/internal/session"
	pb "gatesvr/proto"
)

// OrderedMessageSender 有序消息发送器
type OrderedMessageSender struct {
	server       *Server
	messageCodec *message.MessageCodec
}

// NewOrderedMessageSender 创建有序消息发送器
func NewOrderedMessageSender(server *Server) *OrderedMessageSender {
	return &OrderedMessageSender{
		server:       server,
		messageCodec: server.messageCodec,
	}
}

// SendOrderedMessage 发送有序消息（使用serverSeq保证顺序）
func (oms *OrderedMessageSender) SendOrderedMessage(sess *session.Session, push *pb.ServerPush) error {
	// 分配服务器序列号
	serverSeq := sess.NewServerSeq()
	push.SeqId = serverSeq

	// 编码消息
	encodeStart := time.Now()
	data, err := oms.messageCodec.EncodeServerPush(push)
	if err != nil {
		log.Printf("编码消息失败: %v", err)
		oms.server.metrics.IncError("encode_error")
		return fmt.Errorf("编码消息失败: %w", err)
	}
	encodeLatency := time.Since(encodeStart)
	oms.server.performanceTracker.RecordEncodeLatency(encodeLatency)

	// 获取有序队列
	orderedQueue := sess.GetOrderedQueue()
	if orderedQueue == nil {
		return fmt.Errorf("会话 %s 的有序队列未初始化", sess.ID)
	}

	// 设置发送回调函数（如果尚未设置）
	if orderedQueue.GetLastSentSeq() == 0 && orderedQueue.GetNextExpectedSeq() == 1 {
		orderedQueue.SetSendCallback(func(orderedMsg *session.OrderedMessage) error {
			return oms.sendMessageDirectly(sess, orderedMsg)
		})
	}

	// 将消息加入有序队列
	if err := orderedQueue.EnqueueMessage(serverSeq, push, data); err != nil {
		log.Printf("消息加入有序队列失败: %v", err)
		return fmt.Errorf("消息加入有序队列失败: %w", err)
	}

	log.Printf("消息已加入有序队列 - 序列号: %d, 会话: %s, 类型: %d",
		serverSeq, sess.ID, push.Type)

	return nil
}

// sendMessageDirectly 直接发送消息（由有序队列调用）
func (oms *OrderedMessageSender) sendMessageDirectly(sess *session.Session, orderedMsg *session.OrderedMessage) error {
	if sess.IsClosed() {
		return fmt.Errorf("会话已关闭")
	}

	// 发送消息
	sendStart := time.Now()
	if err := oms.messageCodec.WriteMessage(sess.Stream, orderedMsg.Data); err != nil {
		log.Printf("发送消息失败: %v", err)
		oms.server.metrics.IncError("send_error")
		return fmt.Errorf("发送消息失败: %w", err)
	}
	sendLatency := time.Since(sendStart)
	oms.server.performanceTracker.RecordSendLatency(sendLatency)

	// 更新指标
	oms.server.metrics.AddThroughput("outbound", int64(len(orderedMsg.Data)))
	oms.server.metrics.SetOutboundQueueSize(sess.ID, oms.server.sessionManager.GetPendingCount(sess.ID))
	oms.server.performanceTracker.RecordBytes(int64(len(orderedMsg.Data)))

	log.Printf("有序消息发送成功 - 序列号: %d, 会话: %s, 发送时延: %.2fms",
		orderedMsg.ServerSeq, sess.ID, sendLatency.Seconds()*1000)

	return nil
}

// SendHeartbeatResponse 发送心跳响应（有序）
func (oms *OrderedMessageSender) SendHeartbeatResponse(sess *session.Session, msgID uint32, clientTimestamp int64) error {
	response := oms.messageCodec.CreateHeartbeatResponse(
		msgID,
		0, // 序列号将由SendOrderedMessage分配
		clientTimestamp,
	)

	return oms.SendOrderedMessage(sess, response)
}

// SendBusinessResponse 发送业务响应（有序）
func (oms *OrderedMessageSender) SendBusinessResponse(sess *session.Session, msgID uint32, code int32, message string, data []byte, headers map[string]string) error {
	response := oms.messageCodec.CreateBusinessResponse(
		msgID,
		0, // 序列号将由SendOrderedMessage分配
		code,
		message,
		data,
	)

	// 添加响应头
	response.Headers = headers

	return oms.SendOrderedMessage(sess, response)
}

// SendErrorResponse 发送错误响应（有序）
func (oms *OrderedMessageSender) SendErrorResponse(sess *session.Session, msgID uint32, code int32, message, detail string) error {
	errorMsg := oms.messageCodec.CreateErrorMessage(
		msgID,
		0, // 序列号将由SendOrderedMessage分配
		code,
		message,
		detail,
	)

	return oms.SendOrderedMessage(sess, errorMsg)
}

// SendStartResponse 发送连接建立响应（有序）
func (oms *OrderedMessageSender) SendStartResponse(sess *session.Session, msgID uint32, success bool, err error, heartbeatInterval int32, connectionID string) error {
	response := oms.messageCodec.CreateStartResponse(
		msgID,
		0, // 序列号将由SendOrderedMessage分配
		success,
		nil,
		heartbeatInterval,
		connectionID,
	)

	return oms.SendOrderedMessage(sess, response)
}

// PushBusinessData 推送业务数据（有序）
func (oms *OrderedMessageSender) PushBusinessData(sess *session.Session, data []byte) error {
	push := &pb.ServerPush{
		Type:    pb.PushType_PUSH_BUSINESS_DATA,
		SeqId:   0, // 序列号将由SendOrderedMessage分配
		Payload: data,
	}

	return oms.SendOrderedMessage(sess, push)
}

// GetQueueStats 获取队列统计信息
func (oms *OrderedMessageSender) GetQueueStats(sess *session.Session) map[string]interface{} {
	orderedQueue := sess.GetOrderedQueue()
	if orderedQueue == nil {
		return map[string]interface{}{
			"error": "队列未初始化",
		}
	}

	return orderedQueue.GetQueueStats()
}

// ResyncSessionSequence 重新同步会话序列号（用于重连）
func (oms *OrderedMessageSender) ResyncSessionSequence(sess *session.Session, clientAckSeq uint64) {
	orderedQueue := sess.GetOrderedQueue()
	if orderedQueue != nil {
		orderedQueue.ResyncSequence(clientAckSeq)
	}

	log.Printf("会话序列号重同步完成 - 会话: %s, 客户端ACK: %d", sess.ID, clientAckSeq)
}

// ForceFlushQueue 强制清空队列（紧急情况使用）
func (oms *OrderedMessageSender) ForceFlushQueue(sess *session.Session) int {
	orderedQueue := sess.GetOrderedQueue()
	if orderedQueue == nil {
		return 0
	}

	return orderedQueue.ForceFlushQueue()
}
