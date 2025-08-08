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
	// 分配服务器序列号 - 记录序列号分配时延
	seqAllocStart := time.Now()
	serverSeq := sess.NewServerSeq()
	push.SeqId = serverSeq
	seqAllocLatency := time.Since(seqAllocStart)
	oms.server.performanceTracker.RecordSeqAllocLatency(seqAllocLatency)

	// 编码消息 - 记录消息编码时延
	msgEncodeStart := time.Now()
	data, err := oms.messageCodec.EncodeServerPush(push)
	if err != nil {
		log.Printf("编码消息失败: %v", err)
		oms.server.metrics.IncError("encode_error")
		return fmt.Errorf("编码消息失败: %w", err)
	}
	msgEncodeLatency := time.Since(msgEncodeStart)
	oms.server.performanceTracker.RecordMsgEncodeLatency(msgEncodeLatency)
	//oms.server.performanceTracker.RecordEncodeLatency(msgEncodeLatency) // 保持原有统计

	// 获取有序队列 - 记录获取队列时延
	queueGetStart := time.Now()
	orderedQueue := sess.GetOrderedQueue()
	if orderedQueue == nil {
		return fmt.Errorf("会话 %s 的有序队列未初始化", sess.ID)
	}
	queueGetLatency := time.Since(queueGetStart)
	oms.server.performanceTracker.RecordQueueGetLatency(queueGetLatency)

	// 设置发送回调函数（如果尚未设置） - 记录回调设置时延
	callbackSetStart := time.Now()
	if orderedQueue.GetSendCallback() == nil {
		orderedQueue.SetSendCallback(func(orderedMsg *session.OrderedMessage) error {
			return oms.sendMessageDirectly(sess, orderedMsg)
		})
		log.Printf("为会话设置发送回调函数 - 会话: %s", sess.ID)
	}
	callbackSetLatency := time.Since(callbackSetStart)
	oms.server.performanceTracker.RecordCallbackSetLatency(callbackSetLatency)

	// 将消息加入有序队列 - 记录消息入队时延
	enqueueStart := time.Now()
	if err := orderedQueue.EnqueueMessage(serverSeq, push, data); err != nil {
		log.Printf("消息加入有序队列失败: %v", err)
		return fmt.Errorf("消息加入有序队列失败: %w", err)
	}
	enqueueLatency := time.Since(enqueueStart)
	oms.server.performanceTracker.RecordEnqueueLatency(enqueueLatency)

	log.Printf("消息已加入有序队列 - 序列号: %d, 会话: %s, 类型: %d",
		serverSeq, sess.ID, push.Type)

	return nil
}

// sendMessageDirectly 直接发送消息（由有序队列调用）
func (oms *OrderedMessageSender) sendMessageDirectly(sess *session.Session, orderedMsg *session.OrderedMessage) error {
	// 记录整个直接发送过程的时延
	directSendStart := time.Now()
	defer func() {
		directSendLatency := time.Since(directSendStart)
		oms.server.performanceTracker.RecordDirectSendLatency(directSendLatency)
	}()

	if sess.IsClosed() {
		return fmt.Errorf("会话已关闭")
	}

	// 发送消息 - 记录写消息时延
	writeMessageStart := time.Now()
	if err := oms.messageCodec.WriteMessage(sess.Stream, orderedMsg.Data); err != nil {
		log.Printf("发送消息失败: %v", err)
		oms.server.metrics.IncError("send_error")
		return fmt.Errorf("发送消息失败: %w", err)
	}
	writeMessageLatency := time.Since(writeMessageStart)
	oms.server.performanceTracker.RecordWriteMessageLatency(writeMessageLatency)
	oms.server.performanceTracker.RecordSendLatency(writeMessageLatency) // 保持原有统计

	// 更新指标 - 记录指标更新时延
	metricsUpdateStart := time.Now()
	oms.server.metrics.AddThroughput("outbound", int64(len(orderedMsg.Data)))
	//oms.server.metrics.SetOutboundQueueSize(sess.ID, oms.server.sessionManager.GetPendingCount(sess.ID))
	oms.server.performanceTracker.RecordBytes(int64(len(orderedMsg.Data)))
	metricsUpdateLatency := time.Since(metricsUpdateStart)
	oms.server.performanceTracker.RecordMetricsUpdateLatency(metricsUpdateLatency)

	log.Printf("有序消息发送成功 - 序列号: %d, 会话: %s, 发送时延: %.2fms",
		orderedMsg.ServerSeq, sess.ID, writeMessageLatency.Seconds()*1000)

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
