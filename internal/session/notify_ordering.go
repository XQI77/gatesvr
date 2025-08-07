package session

import (
	"gatesvr/proto"
	"sync"
)

// NotifyBindMsgItem 用于存储被绑定的notify消息及其元数据
type NotifyBindMsgItem struct {
	// notify消息的原始数据
	NotifyData []byte

	// notify消息的类型和内容信息（可选，用于调试和日志）
	MsgType string
	Title   string
	Content string

	// 元数据
	Metadata map[string]string

	// 绑定信息
	SyncHint proto.NotifySyncHint // 同步提示
	BindGrid uint32               // 绑定的Grid（client_seq_id）

	// 创建时间，用于超时清理
	CreateTime int64
}

// MessageOrderingManager 管理消息保序的核心组件
type MessageOrderingManager struct {
	// 用于存储需要在response之前下发的notify
	// key: grid (client_seq_id), value: notify列表
	beforeRspNotifies map[uint32][]*NotifyBindMsgItem

	// 用于存储需要在response之后下发的notify
	// key: grid (client_seq_id), value: notify列表
	afterRspNotifies map[uint32][]*NotifyBindMsgItem

	// 保护并发访问的互斥锁
	mu sync.RWMutex
}

// NewMessageOrderingManager 创建新的消息排序管理器
func NewMessageOrderingManager() *MessageOrderingManager {
	return &MessageOrderingManager{
		beforeRspNotifies: make(map[uint32][]*NotifyBindMsgItem),
		afterRspNotifies:  make(map[uint32][]*NotifyBindMsgItem),
	}
}

// AddNotifyBindBeforeRsp 将notify消息添加到beforeRspNotifies映射中
// 返回是否成功添加
func (m *MessageOrderingManager) AddNotifyBindBeforeRsp(grid uint32, notify *NotifyBindMsgItem) bool {
	if notify == nil {
		return false
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// 将notify添加到对应grid的列表中
	m.beforeRspNotifies[grid] = append(m.beforeRspNotifies[grid], notify)
	return true
}

// AddNotifyBindAfterRsp 将notify消息添加到afterRspNotifies映射中
// 返回是否成功添加
func (m *MessageOrderingManager) AddNotifyBindAfterRsp(grid uint32, notify *NotifyBindMsgItem) bool {
	if notify == nil {
		return false
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// 将notify添加到对应grid的列表中
	m.afterRspNotifies[grid] = append(m.afterRspNotifies[grid], notify)
	return true
}

// GetAndRemoveBeforeNotifies 获取并移除指定grid的before notifies
func (m *MessageOrderingManager) GetAndRemoveBeforeNotifies(grid uint32) []*NotifyBindMsgItem {
	m.mu.Lock()
	defer m.mu.Unlock()

	notifies := m.beforeRspNotifies[grid]
	delete(m.beforeRspNotifies, grid)
	return notifies
}

// GetAndRemoveAfterNotifies 获取并移除指定grid的after notifies
func (m *MessageOrderingManager) GetAndRemoveAfterNotifies(grid uint32) []*NotifyBindMsgItem {
	m.mu.Lock()
	defer m.mu.Unlock()

	notifies := m.afterRspNotifies[grid]
	delete(m.afterRspNotifies, grid)
	return notifies
}

// GetPendingBeforeCount 获取等待在response之前发送的notify数量
func (m *MessageOrderingManager) GetPendingBeforeCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, notifies := range m.beforeRspNotifies {
		count += len(notifies)
	}
	return count
}

// GetPendingAfterCount 获取等待在response之后发送的notify数量
func (m *MessageOrderingManager) GetPendingAfterCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, notifies := range m.afterRspNotifies {
		count += len(notifies)
	}
	return count
}

// CleanupExpiredNotifies 清理过期的notify消息
// maxAge: 最大存活时间（秒）
func (m *MessageOrderingManager) CleanupExpiredNotifies(currentTime int64, maxAge int64) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	cleanedCount := 0

	// 清理before notifies
	for grid, notifies := range m.beforeRspNotifies {
		filtered := make([]*NotifyBindMsgItem, 0, len(notifies))
		for _, notify := range notifies {
			if currentTime-notify.CreateTime <= maxAge {
				filtered = append(filtered, notify)
			} else {
				cleanedCount++
			}
		}

		if len(filtered) == 0 {
			delete(m.beforeRspNotifies, grid)
		} else {
			m.beforeRspNotifies[grid] = filtered
		}
	}

	// 清理after notifies
	for grid, notifies := range m.afterRspNotifies {
		filtered := make([]*NotifyBindMsgItem, 0, len(notifies))
		for _, notify := range notifies {
			if currentTime-notify.CreateTime <= maxAge {
				filtered = append(filtered, notify)
			} else {
				cleanedCount++
			}
		}

		if len(filtered) == 0 {
			delete(m.afterRspNotifies, grid)
		} else {
			m.afterRspNotifies[grid] = filtered
		}
	}

	return cleanedCount
}

// Clear 清空所有待发送的notify
func (m *MessageOrderingManager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.beforeRspNotifies = make(map[uint32][]*NotifyBindMsgItem)
	m.afterRspNotifies = make(map[uint32][]*NotifyBindMsgItem)
}
