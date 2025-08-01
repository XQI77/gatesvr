package session

import (
	"fmt"
	"gatesvr/proto"
	"time"

	protobuf "google.golang.org/protobuf/proto"
)

// processNotify 处理notify消息的入口函数
// 根据SyncHint判断消息的下发时机
func (s *Session) ProcessNotify(notifyReq *proto.UnicastPushRequest) error {
	if s == nil || notifyReq == nil {
		return fmt.Errorf("invalid session or notify request")
	}

	// 序列化notify消息数据以便后续发送
	notifyData, err := protobuf.Marshal(notifyReq)
	if err != nil {
		return fmt.Errorf("failed to marshal notify request: %w", err)
	}

	// 创建NotifyBindMsgItem
	notifyItem := &NotifyBindMsgItem{
		NotifyData: notifyData,
		MsgType:    notifyReq.MsgType,
		Title:      notifyReq.Title,
		Content:    notifyReq.Content,
		Metadata:   notifyReq.Metadata,
		SyncHint:   notifyReq.SyncHint,
		BindGrid:   uint32(notifyReq.BindClientSeqId),
		CreateTime: time.Now().Unix(),
	}

	// 根据SyncHint决定处理方式
	switch notifyReq.SyncHint {
	case proto.NotifySyncHint_NSH_BEFORE_RESPONSE:
		// 绑定到指定response之前发送
		grid := uint32(notifyReq.BindClientSeqId)
		if grid == 0 {
			return fmt.Errorf("bind_client_seq_id is required for NSH_BEFORE_RESPONSE")
		}
		success := s.AddNotifyBindBeforeRsp(grid, notifyItem)
		if !success {
			return fmt.Errorf("failed to bind notify before response for grid: %d", grid)
		}
		return nil

	case proto.NotifySyncHint_NSH_AFTER_RESPONSE:
		// 绑定到指定response之后发送
		grid := uint32(notifyReq.BindClientSeqId)
		if grid == 0 {
			return fmt.Errorf("bind_client_seq_id is required for NSH_AFTER_RESPONSE")
		}
		success := s.AddNotifyBindAfterRsp(grid, notifyItem)
		if !success {
			return fmt.Errorf("failed to bind notify after response for grid: %d", grid)
		}
		return nil

	case proto.NotifySyncHint_NSH_IMMEDIATELY:
		fallthrough
	default:
		// 立即下发 - 但需要通过gateway的有序发送器处理
		// 这里先暂存到orderingManager中，等待外部调用处理
		// 或者直接通过有序队列发送
		return s.sendNotifyImmediately(notifyItem)
	}
}

// procWithNotifyBinds 统一处理绑定消息并进行下发的核心函数
// 这是保证消息顺序的关键函数，执行顺序：Before-Notifies -> Response -> After-Notifies
func (s *Session) ProcWithNotifyBinds(responseData []byte, grid uint32) error {
	if s == nil {
		return fmt.Errorf("invalid session")
	}

	// 1. 下发 Before-Notifies
	if err := s.sendBeforeNotifies(grid); err != nil {
		return fmt.Errorf("failed to send before notifies for grid %d: %w", grid, err)
	}

	// 2. 下发 Response
	if err := s.sendResponse(responseData); err != nil {
		return fmt.Errorf("failed to send response for grid %d: %w", grid, err)
	}

	// 3. 下发 After-Notifies
	if err := s.sendAfterNotifies(grid); err != nil {
		return fmt.Errorf("failed to send after notifies for grid %d: %w", grid, err)
	}

	return nil
}

// sendBeforeNotifies 发送绑定在response之前的notify消息
func (s *Session) sendBeforeNotifies(grid uint32) error {
	if s.orderingManager == nil {
		return nil // 如果没有排序管理器，直接返回
	}

	beforeNotifies := s.orderingManager.GetAndRemoveBeforeNotifies(grid)
	if len(beforeNotifies) == 0 {
		return nil // 没有需要发送的before notify
	}

	for _, notifyItem := range beforeNotifies {
		// 为每个notify分配新的ServerSeq
		serverSeq := s.IncrementAndGetSeq()

		// 发送notify消息
		if err := s.sendNotifyWithSeq(notifyItem, serverSeq); err != nil {
			return fmt.Errorf("failed to send before notify (seq=%d): %w", serverSeq, err)
		}
	}

	return nil
}

// sendAfterNotifies 发送绑定在response之后的notify消息
func (s *Session) sendAfterNotifies(grid uint32) error {
	if s.orderingManager == nil {
		return nil // 如果没有排序管理器，直接返回
	}

	afterNotifies := s.orderingManager.GetAndRemoveAfterNotifies(grid)
	if len(afterNotifies) == 0 {
		return nil // 没有需要发送的after notify
	}

	for _, notifyItem := range afterNotifies {
		// 为每个notify分配新的ServerSeq
		serverSeq := s.IncrementAndGetSeq()

		// 发送notify消息
		if err := s.sendNotifyWithSeq(notifyItem, serverSeq); err != nil {
			return fmt.Errorf("failed to send after notify (seq=%d): %w", serverSeq, err)
		}
	}

	return nil
}

// sendResponse 发送response消息
func (s *Session) sendResponse(responseData []byte) error {
	if len(responseData) == 0 {
		return nil
	}

	// 为response分配新的ServerSeq
	serverSeq := s.IncrementAndGetSeq()

	// 构造ServerPush消息（业务响应类型）
	serverPush := &proto.ServerPush{
		MsgId:   0,
		SeqId:   serverSeq,
		Type:    proto.PushType_PUSH_BUSINESS_DATA,
		Payload: responseData,
	}

	// 通过有序队列发送消息
	if s.orderedQueue != nil {
		// 序列化消息
		pushData, err := protobuf.Marshal(serverPush)
		if err != nil {
			return fmt.Errorf("failed to marshal response: %w", err)
		}

		return s.orderedQueue.EnqueueMessage(serverSeq, serverPush, pushData)
	}

	// 如果没有有序队列，回退到直接发送
	return s.sendMessageDirectly(serverPush)
}

// sendNotifyImmediately 立即发送notify消息
// 注意：这个方法现在通过有序队列发送，实际的序列号由OrderedMessageSender统一分配
func (s *Session) sendNotifyImmediately(notifyItem *NotifyBindMsgItem) error {
	// 构造ServerPush消息
	serverPush := &proto.ServerPush{
		Type:    proto.PushType_PUSH_BUSINESS_DATA,
		SeqId:   s.IncrementAndGetSeq(), // 临时分配序列号，有序发送器会覆盖
		Payload: notifyItem.NotifyData,
		Headers: notifyItem.Metadata,
	}

	// 通过有序队列发送消息，让OrderedMessageSender统一管理序列号
	if s.orderedQueue != nil {
		// 序列化消息
		pushData, err := protobuf.Marshal(serverPush)
		if err != nil {
			return fmt.Errorf("failed to marshal notify push: %w", err)
		}

		return s.orderedQueue.EnqueueMessage(serverPush.SeqId, serverPush, pushData)
	}

	// 如果没有有序队列，直接发送（这种情况下需要保留序列号管理）
	return s.sendMessageDirectly(serverPush)
}

// sendNotifyWithSeq 使用指定序列号发送notify消息
func (s *Session) sendNotifyWithSeq(notifyItem *NotifyBindMsgItem, serverSeq uint64) error {
	// 构造ServerPush消息结构
	serverPush := &proto.ServerPush{
		MsgId:   0, // 可以设置为0或根据需要生成唯一ID
		SeqId:   serverSeq,
		Type:    proto.PushType_PUSH_BUSINESS_DATA, // notify消息类型
		Payload: notifyItem.NotifyData,
		Headers: notifyItem.Metadata,
	}

	// 通过有序队列发送消息以保证可靠性和顺序
	if s.orderedQueue != nil {
		// 序列化消息
		pushData, err := protobuf.Marshal(serverPush)
		if err != nil {
			return fmt.Errorf("failed to marshal server push: %w", err)
		}

		// 使用有序队列发送，确保消息的可靠传输和ACK机制
		return s.orderedQueue.EnqueueMessage(serverSeq, serverPush, pushData)
	}

	// 如果没有有序队列，回退到直接发送
	return s.sendMessageDirectly(serverPush)
}

// sendMessageDirectly 直接发送ServerPush消息（不经过有序队列）
func (s *Session) sendMessageDirectly(serverPush *proto.ServerPush) error {
	if s.Stream == nil {
		return fmt.Errorf("session stream is nil")
	}

	// 序列化消息
	pushData, err := protobuf.Marshal(serverPush)
	if err != nil {
		return fmt.Errorf("failed to marshal server push: %w", err)
	}

	// 记录最后活动时间
	s.UpdateActivity()

	// 发送数据到QUIC流
	_, err = s.Stream.Write(pushData)
	if err != nil {
		return fmt.Errorf("failed to write to stream: %w", err)
	}

	return nil
}

// GetPendingNotifyStats 获取待发送notify的统计信息
func (s *Session) GetPendingNotifyStats() (beforeCount, afterCount int) {
	if s.orderingManager == nil {
		return 0, 0
	}

	return s.orderingManager.GetPendingBeforeCount(), s.orderingManager.GetPendingAfterCount()
}
