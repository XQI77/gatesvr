// Package session 提供会话管理功能
package session

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/quic-go/quic-go"
)

// Manager 会话管理器
type Manager struct {
	sessions      sync.Map // map[string]*Session - 所有活跃会话（按sessionID索引）
	sessionsByUID sync.Map // map[string]*Session - 按用户ID索引的会话（支持重连检测）

	// 超时配置
	sessionTimeout      time.Duration // 会话超时时间
	ackTimeout          time.Duration // ACK超时时间
	maxRetries          int           // 最大重试次数
	connectionDownDelay time.Duration // 连接断开延迟清理时间

	// 用于停止清理goroutine
	stopCh chan struct{}
	wg     sync.WaitGroup

	// 备份同步相关
	syncEnabled     bool                                                   // 是否启用同步
	syncCallback    func(sessionID string, session *Session, event string) // 同步回调函数
	syncCallbackMux sync.RWMutex                                           // 保护同步回调的锁
	backupMode      bool                                                   // 是否为备份模式（只读）
}

// NewManager 创建新的会话管理器
func NewManager(sessionTimeout, ackTimeout time.Duration, maxRetries int) *Manager {
	return &Manager{
		sessionTimeout:      sessionTimeout,
		ackTimeout:          ackTimeout,
		maxRetries:          maxRetries,
		connectionDownDelay: 30 * time.Second, // 默认30秒延迟清理
		stopCh:              make(chan struct{}),
	}
}

// CreateOrReconnectSession 创建或重连会话
func (m *Manager) CreateOrReconnectSession(conn *quic.Conn, stream *quic.Stream, clientID, openID, userIP string) (*Session, bool) {
	// 阶段1：无锁检查重连（超高并发性能）
	var oldSession *Session
	var isReconnect bool

	// 直接使用OpenID进行O(1)重连检查，删除O(n)遍历
	if openID != "" {
		if value, exists := m.sessionsByUID.Load(openID); exists {
			existingSession := value.(*Session)
			// 检查是否可以重连（比如时间间隔不太长）
			if time.Since(existingSession.LastActivity) < m.connectionDownDelay*2 {
				oldSession = existingSession
				isReconnect = true
			}
		}
	}

	// 阶段2：锁外创建新会话对象（耗时操作无锁并发）
	sessionID := uuid.New().String()
	newSession := &Session{
		ID:           sessionID,
		Connection:   conn,
		Stream:       stream,
		CreateTime:   time.Now(),
		LastActivity: time.Now(),

		// 客户端信息
		ClientID: clientID,
		OpenID:   openID,
		UserIP:   userIP,
		connIdx:  1, // 正常连接

		// 初始状态
		nextSeqID: 1,
		state:     int32(SessionInited),

		closeCh: make(chan struct{}),
	}

	// 初始化有序消息队列
	newSession.orderedQueue = NewOrderedMessageQueue(sessionID, 1000) // 最大队列长度1000

	// 初始化消息排序管理器 - 新增
	newSession.orderingManager = NewMessageOrderingManager()

	// 阶段3：原子操作处理重连和注册（无锁高并发）
	if isReconnect && oldSession != nil {
		// 双重检查：确保重连状态仍然有效
		if currentValue, exists := m.sessions.Load(oldSession.ID); exists {
			if currentOldSession := currentValue.(*Session); currentOldSession == oldSession {
				// 重连处理：继承旧会话数据
				newSession.InheritFrom(oldSession)
				newSession.SetSuccessor(true)

				// 原子操作处理旧会话
				m.handleOldSessionOnReconnectAtomic(oldSession, newSession)

				fmt.Printf("检测到重连 - 客户端: %s, 用户: %s, 旧会话: %s, 新会话: %s\n",
					clientID, openID, oldSession.ID, newSession.ID)
			} else {
				// 旧会话已被其他goroutine清理，取消重连状态
				isReconnect = false
			}
		} else {
			// 旧会话不存在，取消重连状态
			isReconnect = false
		}
	}

	// 原子注册新会话到所有索引
	m.sessions.Store(sessionID, newSession)
	if openID != "" {
		m.sessionsByUID.Store(openID, newSession)
	}

	return newSession, isReconnect
}

// handleOldSessionOnReconnectAtomic 处理重连时的旧会话（原子操作版本）
func (m *Manager) handleOldSessionOnReconnectAtomic(oldSession, newSession *Session) {
	// 标记旧会话为关闭但不立即删除
	oldSession.SetClosed()

	// 从主索引中原子移除旧会话
	m.sessions.Delete(oldSession.ID)

	// 延迟清理旧会话
	go func() {
		time.Sleep(5 * time.Second) // 给重传一些时间
		oldSession.Close()
	}()
}

// GetSession 获取指定会话
func (m *Manager) GetSession(sessionID string) (*Session, bool) {
	if value, ok := m.sessions.Load(sessionID); ok {
		return value.(*Session), true
	}
	return nil, false
}

// GetAllSessions 获取所有活跃会话
func (m *Manager) GetAllSessions() []*Session {
	sessions := make([]*Session, 0)
	m.sessions.Range(func(key, value interface{}) bool {
		if session := value.(*Session); session != nil {
			sessions = append(sessions, session)
		}
		return true // 继续遍历
	})
	return sessions
}

// RemoveSession 移除会话（立即删除，用于兼容原有调用）
func (m *Manager) RemoveSession(sessionID string) {
	m.RemoveSessionWithDelay(sessionID, false, "immediate removal")
}

// GetSessionCount 获取会话总数
func (m *Manager) GetSessionCount() int {
	count := 0
	m.sessions.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	return count
}

// Start 启动会话管理器
func (m *Manager) Start(ctx context.Context) {
	// 启动会话清理器
	m.wg.Add(1)
	go m.cleanupExpiredSessions(ctx)
}

// Stop 停止会话管理器
func (m *Manager) Stop() {
	close(m.stopCh)
	m.wg.Wait()

	// 关闭所有会话
	m.sessions.Range(func(key, value interface{}) bool {
		if session := value.(*Session); session != nil {
			session.Close()
		}
		return true
	})
	// 清空所有sync.Map（没有直接方法，需要逐个删除）
	m.sessions.Range(func(key, value interface{}) bool {
		m.sessions.Delete(key)
		return true
	})
	m.sessionsByUID.Range(func(key, value interface{}) bool {
		m.sessionsByUID.Delete(key)
		return true
	})
}

// cleanupExpiredSessions 清理过期会话
func (m *Manager) cleanupExpiredSessions(ctx context.Context) {
	defer m.wg.Done()

	ticker := time.NewTicker(30 * time.Second) // 每30秒清理一次
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.removeExpiredSessions()
		}
	}
}

// removeExpiredSessions 移除过期会话
func (m *Manager) removeExpiredSessions() {
	expiredSessions := make([]string, 0)

	// 遍历找出过期会话
	m.sessions.Range(func(key, value interface{}) bool {
		sessionID := key.(string)
		session := value.(*Session)
		if session.IsExpired(m.sessionTimeout) {
			expiredSessions = append(expiredSessions, sessionID)
		}
		return true
	})

	// 移除过期会话
	for _, sessionID := range expiredSessions {
		if value, exists := m.sessions.Load(sessionID); exists {
			session := value.(*Session)
			m.sessions.Delete(sessionID)
			session.Close()
			fmt.Printf("会话 %s 已过期并被移除\n", sessionID)
		}
	}
}

// AckMessagesUpTo 确认到指定序列号为止的所有消息（批量ACK）
func (m *Manager) AckMessagesUpTo(sessionID string, ackSeqID uint64) int {
	session, exists := m.GetSession(sessionID)
	if !exists {
		return 0
	}

	// 更新会话的ACK序列号
	session.UpdateAckServerSeq(ackSeqID)

	// 使用OrderedMessageQueue的批量确认功能
	orderedQueue := session.GetOrderedQueue()
	if orderedQueue == nil {
		return 0
	}

	return orderedQueue.AckMessagesUpTo(ackSeqID)
}

// ActivateSession 激活会话（处理登录成功后的会话激活）
func (m *Manager) ActivateSession(sessionID string, zone int64) error {
	value, exists := m.sessions.Load(sessionID)
	if !exists {
		return fmt.Errorf("会话不存在: %s", sessionID)
	}

	session := value.(*Session)

	// 激活会话状态
	if !session.ActivateSession(zone) {
		return fmt.Errorf("会话激活失败，当前状态不是Inited: %s", sessionID)
	}

	fmt.Printf("会话激活成功 - 会话: %s, Zone: %d\n", sessionID, zone)
	return nil
}

// DeliverCachedMessages 投递缓存的消息到已激活的会话
func (m *Manager) DeliverCachedMessages(sessionID string) error {
	session, exists := m.GetSession(sessionID)
	if !exists {
		return fmt.Errorf("会话不存在: %s", sessionID)
	}

	if !session.IsNormal() {
		return fmt.Errorf("会话未激活: %s", sessionID)
	}

	// 获取并清空缓存的消息
	cachedMessages := session.GetAndClearCachedMessages()

	if len(cachedMessages) == 0 {
		return nil // 没有缓存消息
	}

	// 发送缓存的消息
	successCount := 0
	for _, msgData := range cachedMessages {
		if session.IsClosed() {
			break // 会话已关闭，停止发送
		}

		// 直接写入流
		if err := m.writeMessageToSession(session, msgData); err != nil {
			fmt.Printf("投递缓存消息失败 - 会话: %s, 错误: %v\n", sessionID, err)
			continue
		}
		successCount++
	}

	fmt.Printf("投递缓存消息完成 - 会话: %s, 总数: %d, 成功: %d\n",
		sessionID, len(cachedMessages), successCount)

	return nil
}

// writeMessageToSession 向会话写入消息数据
func (m *Manager) writeMessageToSession(session *Session, data []byte) error {
	if session.IsClosed() {
		return fmt.Errorf("会话已关闭")
	}

	_, err := session.Stream.Write(data)
	return err
}

// ValidateClientSequence 验证客户端消息序列号
func (m *Manager) ValidateClientSequence(sessionID string, clientSeq uint64) bool {
	session, exists := m.GetSession(sessionID)
	if !exists {
		return false
	}

	return session.ValidateClientSeq(clientSeq)
}

// GetExpectedClientSequence 获取期待的下一个客户端序列号
func (m *Manager) GetExpectedClientSequence(sessionID string) uint64 {
	session, exists := m.GetSession(sessionID)
	if !exists {
		return 1 // 如果会话不存在，期待的是第一个序列号
	}

	return session.MaxClientSeq() + 1
}

// CacheMessageForInactiveSession 为未激活的会话缓存消息
func (m *Manager) CacheMessageForInactiveSession(sessionID string, data []byte) error {
	session, exists := m.GetSession(sessionID)
	if !exists {
		return fmt.Errorf("会话不存在: %s", sessionID)
	}

	if session.IsNormal() {
		return fmt.Errorf("会话已激活，无需缓存: %s", sessionID)
	}

	session.AddCachedMessage(data)
	fmt.Printf("缓存消息 - 会话: %s, 当前缓存数量: %d\n",
		sessionID, session.GetCachedMessageCount())

	return nil
}

// GetSessionStats 获取会话统计信息
func (m *Manager) GetSessionStats(sessionID string) map[string]interface{} {
	session, exists := m.GetSession(sessionID)
	if !exists {
		return nil
	}

	stats := map[string]interface{}{
		"session_id":            session.ID,
		"state":                 session.State(),
		"zone":                  session.Zone(),
		"openid":                session.OpenID,
		"client_id":             session.ClientID,
		"create_time":           session.CreateTime,
		"last_activity":         session.LastActivity,
		"server_seq":            session.ServerSeq(),
		"max_client_seq":        session.MaxClientSeq(),
		"client_ack_server_seq": session.ClientAckServerSeq(),
		"cached_message_count":  session.GetCachedMessageCount(),
		"pending_message_count": m.GetPendingCount(sessionID),
		"is_closed":             session.IsClosed(),
	}

	return stats
}

// GetPendingCount 获取会话的待确认消息数量
func (m *Manager) GetPendingCount(sessionID string) int {
	session, exists := m.GetSession(sessionID)
	if !exists {
		return 0
	}

	orderedQueue := session.GetOrderedQueue()
	if orderedQueue == nil {
		return 0
	}

	return orderedQueue.GetPendingCount()
}

// GetSessionByOpenID 根据OpenID获取会话
func (m *Manager) GetSessionByOpenID(openID string) (*Session, bool) {
	if value, ok := m.sessionsByUID.Load(openID); ok {
		return value.(*Session), true
	}
	return nil, false
}

// BindSession 绑定会话到各种索引（连接建立后调用）
func (m *Manager) BindSession(session *Session) error {
	// 绑定到OpenID索引（如果有的话）
	if session.OpenID != "" {
		// 检查OpenID是否已被占用
		if existingValue, exists := m.sessionsByUID.Load(session.OpenID); exists {
			existingSession := existingValue.(*Session)
			if existingSession.ID != session.ID {
				return fmt.Errorf("OpenID %s 已被会话 %s 占用", session.OpenID, existingSession.ID)
			}
		}
		// 绑定到OpenID索引
		m.sessionsByUID.Store(session.OpenID, session)
	}

	return nil
}

// UnbindSession 解绑会话（登出或断开时调用）
func (m *Manager) UnbindSession(session *Session) {

	// 从OpenID索引中移除
	if session.OpenID != "" {
		m.sessionsByUID.Delete(session.OpenID)
	}
}

// RemoveSessionWithDelay 延迟移除会话（支持重连）
func (m *Manager) RemoveSessionWithDelay(sessionID string, delay bool, reason string) {
	value, exists := m.sessions.Load(sessionID)
	if !exists {
		return
	}

	session := value.(*Session)

	// 标记会话为关闭状态
	alreadyClosed := session.SetClosed()
	if alreadyClosed && !delay {
		// 已经关闭且不延迟，立即删除
		m.immediateRemoveSession(sessionID)
		return
	}

	if delay && session.OpenID != "" {
		// 延迟清理，支持重连
		fmt.Printf("会话 %s 将在 %v 后清理，原因: %s\n", sessionID, m.connectionDownDelay, reason)

		// 不立即从索引中移除，允许重连期间查找
		time.AfterFunc(m.connectionDownDelay, func() {
			m.immediateRemoveSession(sessionID)
		})
	} else {
		// 立即清理
		m.immediateRemoveSession(sessionID)
	}
}

// immediateRemoveSession 立即移除会话
func (m *Manager) immediateRemoveSession(sessionID string) {
	value, exists := m.sessions.Load(sessionID)
	if !exists {
		return
	}

	session := value.(*Session)

	// 从所有索引中原子移除
	m.sessions.Delete(sessionID)

	if session.OpenID != "" {
		m.sessionsByUID.Delete(session.OpenID)
	}

	session.Close()
	fmt.Printf("会话 %s 已被彻底清理\n", sessionID)
}

// PushToSession 单播推送消息到指定会话
func (m *Manager) PushToSession(sessionID string, msgType string, data []byte) error {
	session, exists := m.GetSession(sessionID)
	if !exists {
		return fmt.Errorf("会话不存在: %s", sessionID)
	}

	if session.IsClosed() {
		return fmt.Errorf("会话已关闭: %s", sessionID)
	}

	// 创建推送消息
	// 这里需要根据项目的消息协议来构造
	// 暂时返回成功，具体实现需要在gateway层

	fmt.Printf("单播推送到会话 %s: 类型=%s, 数据长度=%d\n", sessionID, msgType, len(data))
	return nil
}

// PushToOpenID 根据OpenID单播推送消息
func (m *Manager) PushToOpenID(openID string, msgType string, data []byte) error {
	session, exists := m.GetSessionByOpenID(openID)
	if !exists {
		return fmt.Errorf("OpenID对应的会话不存在: %s", openID)
	}

	return m.PushToSession(session.ID, msgType, data)
}

// GetOnlineUserCount 获取在线用户数量
func (m *Manager) GetOnlineUserCount() int {
	count := 0
	m.sessions.Range(func(key, value interface{}) bool {
		session := value.(*Session)
		if session.IsNormal() && !session.IsClosed() {
			count++
		}
		return true
	})
	return count
}

// EnableSync 启用同步功能
func (m *Manager) EnableSync(callback func(sessionID string, session *Session, event string)) {
	m.syncCallbackMux.Lock()
	defer m.syncCallbackMux.Unlock()

	m.syncEnabled = true
	m.syncCallback = callback
	fmt.Printf("会话管理器同步功能已启用\n")
}

// DisableSync 禁用同步功能
func (m *Manager) DisableSync() {
	m.syncCallbackMux.Lock()
	defer m.syncCallbackMux.Unlock()

	m.syncEnabled = false
	m.syncCallback = nil
	fmt.Printf("会话管理器同步功能已禁用\n")
}

// SetBackupMode 设置备份模式
func (m *Manager) SetBackupMode(backupMode bool) {
	m.backupMode = backupMode
	if backupMode {
		fmt.Printf("会话管理器进入备份模式（只读）\n")
	} else {
		fmt.Printf("会话管理器退出备份模式\n")
	}
}

// IsBackupMode 检查是否为备份模式
func (m *Manager) IsBackupMode() bool {
	return m.backupMode
}

// RestoreSession 从同步数据恢复会话（备份模式专用）
func (m *Manager) RestoreSession(sessionData map[string]interface{}) error {
	if !m.backupMode {
		return fmt.Errorf("只有备份模式才能恢复会话")
	}

	// 从同步数据创建会话对象
	sessionID, ok := sessionData["session_id"].(string)
	if !ok || sessionID == "" {
		return fmt.Errorf("无效的会话ID")
	}

	// 创建基础会话对象（不包含连接）
	restoredSession := &Session{
		ID:           sessionID,
		Connection:   nil, // 备份模式下无连接
		Stream:       nil, // 备份模式下无流
		CreateTime:   time.Now(),
		LastActivity: time.Now(),
		closeCh:      make(chan struct{}),
	}

	// 恢复基本信息
	if openID, ok := sessionData["open_id"].(string); ok {
		restoredSession.OpenID = openID
	}
	if clientID, ok := sessionData["client_id"].(string); ok {
		restoredSession.ClientID = clientID
	}
	if userIP, ok := sessionData["user_ip"].(string); ok {
		restoredSession.UserIP = userIP
	}

	// 恢复状态信息
	if state, ok := sessionData["state"].(float64); ok {
		restoredSession.state = int32(state)
	}
	if zone, ok := sessionData["zone"].(float64); ok {
		restoredSession.zone = int64(zone)
	}

	// 恢复序列号信息
	if serverSeq, ok := sessionData["server_seq"].(float64); ok {
		restoredSession.serverSeq = uint64(serverSeq)
	}
	if maxClientSeq, ok := sessionData["max_client_seq"].(float64); ok {
		restoredSession.maxClientSeq = uint64(maxClientSeq)
	}

	// 原子注册到各个索引
	m.sessions.Store(sessionID, restoredSession)
	if restoredSession.OpenID != "" {
		m.sessionsByUID.Store(restoredSession.OpenID, restoredSession)
	}

	fmt.Printf("恢复会话成功 - 会话: %s, OpenID: %s",
		sessionID, restoredSession.OpenID)

	return nil
}

// GetSessionSyncData 获取会话同步数据
func (m *Manager) GetSessionSyncData(sessionID string) (map[string]interface{}, error) {
	session, exists := m.GetSession(sessionID)
	if !exists {
		return nil, fmt.Errorf("会话不存在: %s", sessionID)
	}

	return map[string]interface{}{
		"session_id":     session.ID,
		"open_id":        session.OpenID,
		"client_id":      session.ClientID,
		"user_ip":        session.UserIP,
		"state":          session.State(),
		"zone":           session.Zone(),
		"server_seq":     session.ServerSeq(),
		"max_client_seq": session.MaxClientSeq(),
		"create_time":    session.CreateTime,
		"last_activity":  session.LastActivity,
	}, nil
}

// ValidateSessionData 校验会话数据完整性
func (m *Manager) ValidateSessionData() map[string]interface{} {
	// 计算各种统计数据
	totalSessions := 0
	gidIndexed := 0
	uidIndexed := 0

	// 统计总会话数
	m.sessions.Range(func(key, value interface{}) bool {
		totalSessions++
		return true
	})

	// 统计UID索引数
	m.sessionsByUID.Range(func(key, value interface{}) bool {
		uidIndexed++
		return true
	})

	stats := map[string]interface{}{
		"total_sessions":   totalSessions,
		"gid_indexed":      gidIndexed,
		"uid_indexed":      uidIndexed,
		"valid_sessions":   0,
		"invalid_sessions": 0,
		"closed_sessions":  0,
		"inconsistencies":  make([]string, 0),
	}

	validCount := 0
	invalidCount := 0
	closedCount := 0
	inconsistencies := make([]string, 0)

	// 遍历所有会话进行验证
	m.sessions.Range(func(key, value interface{}) bool {
		sessionID := key.(string)
		session := value.(*Session)

		if session == nil {
			invalidCount++
			inconsistencies = append(inconsistencies, fmt.Sprintf("空会话对象: %s", sessionID))
			return true
		}

		if session.IsClosed() {
			closedCount++
			return true
		}

		validCount++

		// 检查索引一致性
		if session.OpenID != "" {
			if indexedValue, exists := m.sessionsByUID.Load(session.OpenID); !exists {
				inconsistencies = append(inconsistencies, fmt.Sprintf("UID索引不一致: %s", sessionID))
			} else if indexedSession := indexedValue.(*Session); indexedSession.ID != sessionID {
				inconsistencies = append(inconsistencies, fmt.Sprintf("UID索引不一致: %s", sessionID))
			}
		}
		return true
	})

	stats["valid_sessions"] = validCount
	stats["invalid_sessions"] = invalidCount
	stats["closed_sessions"] = closedCount
	stats["inconsistencies"] = inconsistencies

	return stats
}
