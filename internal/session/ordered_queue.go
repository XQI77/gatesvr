package session

import (
	"container/heap"
	"fmt"
	"sync"
	"time"

	pb "gatesvr/proto"
)

// OrderedMessage 有序消息结构
type OrderedMessage struct {
	ServerSeq uint64         // 服务器序列号，用于排序
	Push      *pb.ServerPush // 实际的推送消息
	Data      []byte         // 编码后的消息数据
	Timestamp time.Time      // 消息创建时间
	Retries   int            // 重试次数
	Sent      bool           // 是否已发送
}

// MessageQueue 消息优先队列（基于heap实现的最小堆）
type MessageQueue []*OrderedMessage

func (mq MessageQueue) Len() int { return len(mq) }

func (mq MessageQueue) Less(i, j int) bool {
	return mq[i].ServerSeq < mq[j].ServerSeq
}

func (mq MessageQueue) Swap(i, j int) {
	mq[i], mq[j] = mq[j], mq[i]
}

func (mq *MessageQueue) Push(x interface{}) {
	*mq = append(*mq, x.(*OrderedMessage))
}

func (mq *MessageQueue) Pop() interface{} {
	old := *mq
	n := len(old)
	item := old[n-1]
	*mq = old[0 : n-1]
	return item
}

const (
	// 消息超时时间
	MessageTimeout = 30 * time.Second
	// 最大重试次数
	MaxRetries = 3
	// 清理间隔
	CleanupInterval = 10 * time.Second
)

// OrderedMessageQueue 有序消息队列管理器
type OrderedMessageQueue struct {
	sessionID string

	// 消息队列和控制
	waitingQueue MessageQueue               // 等待发送的消息队列（有序）
	sentMessages map[uint64]*OrderedMessage // 已发送待确认的消息
	queueMux     sync.Mutex                 // 保护队列的锁

	// 序列号管理
	nextExpectedSeq uint64 // 下一个期望的序列号
	lastSentSeq     uint64 // 最后发送的序列号
	lastAckedSeq    uint64 // 最后确认的序列号

	// 流控制
	maxQueueSize int // 最大队列长度

	// 状态管理
	stopped       bool
	stopCh        chan struct{}
	cleanupTicker *time.Ticker // 清理定时器

	// 发送回调函数
	sendCallback func(*OrderedMessage) error
}

// NewOrderedMessageQueue 创建新的有序消息队列
func NewOrderedMessageQueue(sessionID string, maxQueueSize int) *OrderedMessageQueue {
	omq := &OrderedMessageQueue{
		sessionID:       sessionID,
		waitingQueue:    make(MessageQueue, 0),
		sentMessages:    make(map[uint64]*OrderedMessage),
		nextExpectedSeq: 1, // 从1开始
		lastSentSeq:     0,
		lastAckedSeq:    0,
		maxQueueSize:    maxQueueSize,
		stopCh:          make(chan struct{}),
	}

	// 初始化堆
	heap.Init(&omq.waitingQueue)

	// 启动清理协程
	omq.cleanupTicker = time.NewTicker(CleanupInterval)
	go omq.cleanupLoop()

	return omq
}

// SetSendCallback 设置消息发送回调函数
func (omq *OrderedMessageQueue) SetSendCallback(callback func(*OrderedMessage) error) {
	omq.sendCallback = callback
}

// GetSendCallback 检查是否已设置发送回调函数
func (omq *OrderedMessageQueue) GetSendCallback() func(*OrderedMessage) error {
	return omq.sendCallback
}

// EnqueueMessage 将消息加入队列
func (omq *OrderedMessageQueue) EnqueueMessage(serverSeq uint64, push *pb.ServerPush, data []byte) error {
	omq.queueMux.Lock()
	defer omq.queueMux.Unlock()

	if omq.stopped {
		return fmt.Errorf("队列已停止")
	}

	// 检查队列长度限制
	if len(omq.waitingQueue) >= omq.maxQueueSize {
		return fmt.Errorf("队列已满，最大长度: %d", omq.maxQueueSize)
	}

	// 创建有序消息
	orderedMsg := &OrderedMessage{
		ServerSeq: serverSeq,
		Push:      push,
		Data:      data,
		Timestamp: time.Now(),
		Retries:   0,
		Sent:      false,
	}

	// 检查是否可以立即发送
	if serverSeq == omq.nextExpectedSeq {
		// 可以立即发送
		if err := omq.sendMessageDirectly(orderedMsg); err != nil {
			return fmt.Errorf("发送消息失败: %w", err)
		}

		// 标记为已发送并加入待确认队列
		orderedMsg.Sent = true
		omq.sentMessages[serverSeq] = orderedMsg
		omq.nextExpectedSeq++
		omq.lastSentSeq = serverSeq

		// 尝试发送队列中的后续消息
		omq.processWaitingMessages()
	} else if serverSeq > omq.nextExpectedSeq {
		// 需要等待前序消息，加入队列
		heap.Push(&omq.waitingQueue, orderedMsg)
		fmt.Printf("消息 %d 加入等待队列，期望序列号: %d, 队列长度: %d\n",
			serverSeq, omq.nextExpectedSeq, len(omq.waitingQueue))
	} else {
		// serverSeq < nextExpectedSeq，说明是重复或过期的消息
		return fmt.Errorf("消息序列号 %d 小于期望序列号 %d，忽略", serverSeq, omq.nextExpectedSeq)
	}

	return nil
}

// processWaitingMessages 处理等待队列中的消息
func (omq *OrderedMessageQueue) processWaitingMessages() {
	for len(omq.waitingQueue) > 0 {
		// 查看队列顶部的消息
		topMsg := omq.waitingQueue[0]

		if topMsg.ServerSeq == omq.nextExpectedSeq {
			// 可以发送
			msg := heap.Pop(&omq.waitingQueue).(*OrderedMessage)

			if err := omq.sendMessageDirectly(msg); err != nil {
				fmt.Printf("发送队列消息失败: %v\n", err)
				// 发送失败，重新加入队列
				heap.Push(&omq.waitingQueue, msg)
				break
			}

			// 标记为已发送并加入待确认队列
			msg.Sent = true
			omq.sentMessages[msg.ServerSeq] = msg
			omq.nextExpectedSeq++
			omq.lastSentSeq = msg.ServerSeq
			fmt.Printf("从队列发送消息 %d，剩余队列长度: %d\n", msg.ServerSeq, len(omq.waitingQueue))
		} else {
			// 还需要等待
			break
		}
	}
}

// sendMessageDirectly 直接发送消息
func (omq *OrderedMessageQueue) sendMessageDirectly(msg *OrderedMessage) error {
	if omq.sendCallback != nil {
		return omq.sendCallback(msg)
	}
	return fmt.Errorf("未设置发送回调函数")
}

// GetQueueStats 获取队列统计信息
func (omq *OrderedMessageQueue) GetQueueStats() map[string]interface{} {
	omq.queueMux.Lock()
	defer omq.queueMux.Unlock()

	stats := map[string]interface{}{
		"session_id":        omq.sessionID,
		"queue_size":        len(omq.waitingQueue),
		"pending_count":     len(omq.sentMessages),
		"max_queue_size":    omq.maxQueueSize,
		"next_expected_seq": omq.nextExpectedSeq,
		"last_sent_seq":     omq.lastSentSeq,
		"last_acked_seq":    omq.lastAckedSeq,
		"stopped":           omq.stopped,
	}

	// 如果队列不为空，显示等待的序列号范围
	if len(omq.waitingQueue) > 0 {
		minSeq := omq.waitingQueue[0].ServerSeq
		maxSeq := minSeq
		for _, msg := range omq.waitingQueue {
			if msg.ServerSeq > maxSeq {
				maxSeq = msg.ServerSeq
			}
		}
		stats["waiting_seq_range"] = fmt.Sprintf("%d-%d", minSeq, maxSeq)
	}

	// 如果有待确认消息，显示范围
	if len(omq.sentMessages) > 0 {
		minSeq := uint64(0)
		maxSeq := uint64(0)
		first := true
		for seqID := range omq.sentMessages {
			if first {
				minSeq = seqID
				maxSeq = seqID
				first = false
			} else {
				if seqID < minSeq {
					minSeq = seqID
				}
				if seqID > maxSeq {
					maxSeq = seqID
				}
			}
		}
		stats["pending_seq_range"] = fmt.Sprintf("%d-%d", minSeq, maxSeq)
	}

	return stats
}

// ResyncSequence 重新同步序列号（用于处理客户端重连）
func (omq *OrderedMessageQueue) ResyncSequence(clientAckSeq uint64) {
	omq.queueMux.Lock()
	defer omq.queueMux.Unlock()

	// 批量确认客户端已收到的消息
	ackedCount := 0
	for seqID := range omq.sentMessages {
		if seqID <= clientAckSeq {
			delete(omq.sentMessages, seqID)
			ackedCount++
		}
	}

	// 如果客户端已确认的序列号大于我们的下一个期望序列号，需要调整
	if clientAckSeq >= omq.nextExpectedSeq {
		omq.nextExpectedSeq = clientAckSeq + 1
		omq.lastSentSeq = clientAckSeq

		// 清理队列中已过期的消息
		newQueue := make(MessageQueue, 0)
		for _, msg := range omq.waitingQueue {
			if msg.ServerSeq > clientAckSeq {
				newQueue = append(newQueue, msg)
			}
		}
		omq.waitingQueue = newQueue
		heap.Init(&omq.waitingQueue)
	}

	// 更新最后确认序列号
	if clientAckSeq > omq.lastAckedSeq {
		omq.lastAckedSeq = clientAckSeq
	}

	fmt.Printf("序列号重同步 - 会话: %s, 客户端ACK: %d, 新期望序列号: %d, 确认数量: %d, 清理后队列长度: %d\n",
		omq.sessionID, clientAckSeq, omq.nextExpectedSeq, ackedCount, len(omq.waitingQueue))
}

// ForceFlushQueue 强制清空队列并发送所有消息（忽略顺序，用于紧急情况）
func (omq *OrderedMessageQueue) ForceFlushQueue() int {
	omq.queueMux.Lock()
	defer omq.queueMux.Unlock()

	flushedCount := 0
	for len(omq.waitingQueue) > 0 {
		msg := heap.Pop(&omq.waitingQueue).(*OrderedMessage)
		if err := omq.sendMessageDirectly(msg); err != nil {
			fmt.Printf("强制发送消息 %d 失败: %v\n", msg.ServerSeq, err)
		} else {
			flushedCount++
			omq.lastSentSeq = msg.ServerSeq
		}
	}

	fmt.Printf("强制清空队列完成 - 会话: %s, 发送消息数: %d\n", omq.sessionID, flushedCount)
	return flushedCount
}

// AckMessage 确认消息（从待确认队列中移除）
func (omq *OrderedMessageQueue) AckMessage(seqID uint64) bool {
	omq.queueMux.Lock()
	defer omq.queueMux.Unlock()

	if _, exists := omq.sentMessages[seqID]; exists {
		delete(omq.sentMessages, seqID)
		// 更新最后确认序列号
		if seqID > omq.lastAckedSeq {
			omq.lastAckedSeq = seqID
		}
		fmt.Printf("消息 %d 已确认 - 会话: %s\n", seqID, omq.sessionID)
		return true
	}
	return false
}

// AckMessagesUpTo 批量确认消息（到指定序列号为止）
func (omq *OrderedMessageQueue) AckMessagesUpTo(ackSeqID uint64) int {
	omq.queueMux.Lock()
	defer omq.queueMux.Unlock()

	ackedCount := 0
	for seqID := range omq.sentMessages {
		if seqID <= ackSeqID {
			delete(omq.sentMessages, seqID)
			ackedCount++
		}
	}

	// 更新最后确认序列号
	if ackSeqID > omq.lastAckedSeq {
		omq.lastAckedSeq = ackSeqID
	}

	fmt.Printf("批量确认消息 - 会话: %s, 确认到: %d, 数量: %d\n",
		omq.sessionID, ackSeqID, ackedCount)

	return ackedCount
}

// GetPendingMessages 获取所有待确认的消息
func (omq *OrderedMessageQueue) GetPendingMessages() []*OrderedMessage {
	omq.queueMux.Lock()
	defer omq.queueMux.Unlock()

	messages := make([]*OrderedMessage, 0, len(omq.sentMessages))
	for _, msg := range omq.sentMessages {
		messages = append(messages, msg)
	}
	return messages
}

// GetPendingCount 获取待确认消息数量
func (omq *OrderedMessageQueue) GetPendingCount() int {
	return len(omq.sentMessages)
}

// cleanupLoop 清理过期消息的后台循环
func (omq *OrderedMessageQueue) cleanupLoop() {
	for {
		select {
		case <-omq.stopCh:
			return
		case <-omq.cleanupTicker.C:
			omq.cleanupExpiredMessages()
			omq.retryTimedOutMessages()
		}
	}
}

// cleanupExpiredMessages 清理过期消息
func (omq *OrderedMessageQueue) cleanupExpiredMessages() {
	omq.queueMux.Lock()
	defer omq.queueMux.Unlock()

	now := time.Now()
	expiredCount := 0

	// 清理待确认消息中的过期消息
	for seqID, msg := range omq.sentMessages {
		if now.Sub(msg.Timestamp) > MessageTimeout && msg.Retries >= MaxRetries {
			delete(omq.sentMessages, seqID)
			expiredCount++
			fmt.Printf("消息 %d 超过最大重试次数，放弃发送 - 会话: %s\n", seqID, omq.sessionID)
		}
	}

	// 清理等待队列中的过期消息
	newQueue := make(MessageQueue, 0)
	for _, msg := range omq.waitingQueue {
		if now.Sub(msg.Timestamp) <= MessageTimeout {
			newQueue = append(newQueue, msg)
		} else {
			expiredCount++
			fmt.Printf("等待消息 %d 超时，从队列中移除 - 会话: %s\n", msg.ServerSeq, omq.sessionID)
		}
	}
	omq.waitingQueue = newQueue
	heap.Init(&omq.waitingQueue)

	if expiredCount > 0 {
		fmt.Printf("清理过期消息 - 会话: %s, 数量: %d\n", omq.sessionID, expiredCount)
	}
}

// retryTimedOutMessages 重试超时消息
func (omq *OrderedMessageQueue) retryTimedOutMessages() {
	omq.queueMux.Lock()
	defer omq.queueMux.Unlock()

	now := time.Now()
	retryCount := 0

	for _, msg := range omq.sentMessages {
		// 检查是否需要重试
		if now.Sub(msg.Timestamp) > MessageTimeout/2 && msg.Retries < MaxRetries {
			// 重试发送
			if omq.sendCallback != nil {
				if err := omq.sendCallback(msg); err != nil {
					fmt.Printf("重试发送消息 %d 失败: %v\n", msg.ServerSeq, err)
				} else {
					msg.Retries++
					msg.Timestamp = now // 更新时间戳
					retryCount++
					fmt.Printf("消息 %d 第 %d 次重试发送成功 - 会话: %s\n",
						msg.ServerSeq, msg.Retries, omq.sessionID)
				}
			}
		}
	}

	if retryCount > 0 {
		fmt.Printf("重试超时消息 - 会话: %s, 数量: %d\n", omq.sessionID, retryCount)
	}
}

// Stop 停止队列
func (omq *OrderedMessageQueue) Stop() {
	omq.queueMux.Lock()
	defer omq.queueMux.Unlock()

	if !omq.stopped {
		omq.stopped = true
		close(omq.stopCh)

		// 停止清理定时器
		if omq.cleanupTicker != nil {
			omq.cleanupTicker.Stop()
		}

		// 清空队列
		omq.waitingQueue = make(MessageQueue, 0)
		heap.Init(&omq.waitingQueue)
		omq.sentMessages = make(map[uint64]*OrderedMessage)
	}
}

// IsStopped 检查队列是否已停止
func (omq *OrderedMessageQueue) IsStopped() bool {
	omq.queueMux.Lock()
	defer omq.queueMux.Unlock()
	return omq.stopped
}

// GetNextExpectedSeq 获取下一个期望的序列号
func (omq *OrderedMessageQueue) GetNextExpectedSeq() uint64 {
	omq.queueMux.Lock()
	defer omq.queueMux.Unlock()
	return omq.nextExpectedSeq
}

// GetLastSentSeq 获取最后发送的序列号
func (omq *OrderedMessageQueue) GetLastSentSeq() uint64 {
	omq.queueMux.Lock()
	defer omq.queueMux.Unlock()
	return omq.lastSentSeq
}

// GetLastAckedSeq 获取最后确认的序列号
func (omq *OrderedMessageQueue) GetLastAckedSeq() uint64 {
	omq.queueMux.Lock()
	defer omq.queueMux.Unlock()
	return omq.lastAckedSeq
}

// DebugPrintQueue 调试打印队列状态
func (omq *OrderedMessageQueue) DebugPrintQueue() {
	omq.queueMux.Lock()
	defer omq.queueMux.Unlock()

	fmt.Printf("=== 队列状态 - 会话: %s ===\n", omq.sessionID)
	fmt.Printf("期望序列号: %d, 最后发送: %d, 队列长度: %d\n",
		omq.nextExpectedSeq, omq.lastSentSeq, len(omq.waitingQueue))

	if len(omq.waitingQueue) > 0 {
		fmt.Printf("等待消息:\n")
		for i, msg := range omq.waitingQueue {
			fmt.Printf("  [%d] 序列号: %d, 时间: %s\n",
				i, msg.ServerSeq, msg.Timestamp.Format("15:04:05.000"))
		}
	}
	fmt.Printf("========================\n")
}
