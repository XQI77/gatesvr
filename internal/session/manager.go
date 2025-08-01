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
	sessions      map[string]*Session // 所有活跃会话（按sessionID索引）
	sessionsByGID map[int64]*Session  // 按GID索引的会话
	sessionsByUID map[string]*Session // 按用户ID索引的会话（支持重连检测）
	sessionsMux   sync.RWMutex        // 保护sessions的读写锁

	// 消息缓存管理器（已废弃，由OrderedMessageQueue替代）
	// messageCache *MessageCache

	// 超时配置
	sessionTimeout      time.Duration // 会话超时时间
	ackTimeout          time.Duration // ACK超时时间
	maxRetries          int           // 最大重试次数
	connectionDownDelay time.Duration // 连接断开延迟清理时间

	// 用于停止清理goroutine
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewManager 创建新的会话管理器
func NewManager(sessionTimeout, ackTimeout time.Duration, maxRetries int) *Manager {
	return &Manager{
		sessions:      make(map[string]*Session),
		sessionsByGID: make(map[int64]*Session),
		sessionsByUID: make(map[string]*Session),
		// messageCache:        NewMessageCache(DefaultTotalMemLimit, DefaultSessionMemLimit), // 已废弃
		sessionTimeout:      sessionTimeout,
		ackTimeout:          ackTimeout,
		maxRetries:          maxRetries,
		connectionDownDelay: 30 * time.Second, // 默认30秒延迟清理
		stopCh:              make(chan struct{}),
	}
}

// CreateOrReconnectSession 创建或重连会话
func (m *Manager) CreateOrReconnectSession(conn *quic.Conn, stream *quic.Stream, clientID, openID, accessToken, userIP string) (*Session, bool) {
	m.sessionsMux.Lock()
	defer m.sessionsMux.Unlock()

	// 检查是否有现有的会话（重连检测）
	var oldSession *Session
	var isReconnect bool

	// 1. 优先根据客户端ID检查重连
	if clientID != "" {
		for _, sess := range m.sessions {
			if sess.ClientID == clientID && sess.OpenID == openID {
				oldSession = sess
				isReconnect = true
				break
			}
		}
	}

	// 2. 如果没有找到，再根据OpenID检查重连
	if oldSession == nil && openID != "" {
		if existingSession, exists := m.sessionsByUID[openID]; exists {
			// 检查是否可以重连（比如时间间隔不太长）
			if time.Since(existingSession.LastActivity) < m.connectionDownDelay*2 {
				oldSession = existingSession
				isReconnect = true
			}
		}
	}

	// 创建新会话
	sessionID := uuid.New().String()
	newSession := &Session{
		ID:           sessionID,
		Connection:   conn,
		Stream:       stream,
		CreateTime:   time.Now(),
		LastActivity: time.Now(),

		// 客户端信息
		ClientID:    clientID,
		OpenID:      openID,
		AccessToken: accessToken,
		UserIP:      userIP,
		connIdx:     1, // 正常连接

		// 初始状态
		nextSeqID: 1,
		state:     int32(SessionInited),

		closeCh: make(chan struct{}),
	}

	// 初始化有序消息队列
	newSession.orderedQueue = NewOrderedMessageQueue(sessionID, 1000) // 最大队列长度1000

	if isReconnect && oldSession != nil {
		// 重连处理：继承旧会话数据
		newSession.InheritFrom(oldSession)
		newSession.SetSuccessor(true)

		// 处理旧会话
		m.handleOldSessionOnReconnect(oldSession, newSession)

		fmt.Printf("检测到重连 - 客户端: %s, 用户: %s, 旧会话: %s, 新会话: %s\n",
			clientID, openID, oldSession.ID, newSession.ID)
	}

	// 注册新会话
	m.sessions[sessionID] = newSession
	if openID != "" {
		m.sessionsByUID[openID] = newSession
	}

	// 启动会话清理协程
	m.wg.Add(1)
	go m.sessionCleanupRoutine(newSession)

	return newSession, isReconnect
}

// handleOldSessionOnReconnect 处理重连时的旧会话
func (m *Manager) handleOldSessionOnReconnect(oldSession, newSession *Session) {
	// 标记旧会话为关闭但不立即删除
	oldSession.SetClosed()

	// 从GID索引中移除旧会话
	if gid := oldSession.Gid(); gid != 0 {
		delete(m.sessionsByGID, gid)
	}

	// 从主索引中移除旧会话
	delete(m.sessions, oldSession.ID)

	// 如果新会话继承了GID，需要防止旧会话清理时删除缓存
	if newSession.Gid() == oldSession.Gid() && oldSession.Gid() != 0 {
		oldSession.SetGid(0) // 重置旧会话的GID
	}

	// 延迟清理旧会话
	go func() {
		time.Sleep(5 * time.Second) // 给重传一些时间
		oldSession.Close()
	}()
}

// GetSession 获取指定会话
func (m *Manager) GetSession(sessionID string) (*Session, bool) {
	m.sessionsMux.RLock()
	defer m.sessionsMux.RUnlock()

	session, exists := m.sessions[sessionID]
	return session, exists
}

// GetAllSessions 获取所有活跃会话
func (m *Manager) GetAllSessions() []*Session {
	m.sessionsMux.RLock()
	defer m.sessionsMux.RUnlock()

	sessions := make([]*Session, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}
	return sessions
}

// RemoveSession 移除会话（立即删除，用于兼容原有调用）
func (m *Manager) RemoveSession(sessionID string) {
	m.RemoveSessionWithDelay(sessionID, false, "immediate removal")
}

// GetSessionCount 获取会话总数
func (m *Manager) GetSessionCount() int {
	m.sessionsMux.RLock()
	defer m.sessionsMux.RUnlock()

	return len(m.sessions)
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

	// 停止消息缓存管理器（已废弃）
	// if m.messageCache != nil {
	//     m.messageCache.Stop()
	// }

	// 关闭所有会话
	m.sessionsMux.Lock()
	for _, session := range m.sessions {
		session.Close()
	}
	m.sessions = make(map[string]*Session)
	m.sessionsMux.Unlock()
}

// sessionCleanupRoutine 会话清理协程，处理ACK超时
func (m *Manager) sessionCleanupRoutine(session *Session) {
	defer m.wg.Done()

	ticker := time.NewTicker(time.Second) // 每秒检查一次
	defer ticker.Stop()

	for {
		select {
		case <-session.closeCh:
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			// 检查待确认消息是否超时
			m.checkAckTimeouts(session)
		}
	}
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

// checkAckTimeouts 检查ACK超时（现在由OrderedMessageQueue自动处理）
func (m *Manager) checkAckTimeouts(session *Session) {
	// ACK超时检查和重试现在由OrderedMessageQueue的cleanupLoop自动处理
	// 这里只需要检查会话是否正常
	if session.IsClosed() {
		return
	}

	// 可以添加一些会话级别的健康检查
	orderedQueue := session.GetOrderedQueue()
	if orderedQueue != nil && orderedQueue.IsStopped() {
		fmt.Printf("会话 %s 的有序队列已停止\n", session.ID)
	}
}

// removeExpiredSessions 移除过期会话
func (m *Manager) removeExpiredSessions() {
	m.sessionsMux.Lock()
	defer m.sessionsMux.Unlock()

	expiredSessions := make([]string, 0)

	for sessionID, session := range m.sessions {
		if session.IsExpired(m.sessionTimeout) {
			expiredSessions = append(expiredSessions, sessionID)
		}
	}

	// 移除过期会话
	for _, sessionID := range expiredSessions {
		if session, exists := m.sessions[sessionID]; exists {
			delete(m.sessions, sessionID)
			session.Close()
			fmt.Printf("会话 %s 已过期并被移除\n", sessionID)
		}
	}
}

// AddPendingMessage 添加待确认消息到缓存（已由OrderedMessageQueue管理）
func (m *Manager) AddPendingMessage(sessionID string, seqID uint64, data []byte) {
	// 消息已在OrderedMessageQueue中管理，不需要单独添加
	// m.messageCache.AddMessage(sessionID, seqID, data)
}

// AckMessage 确认消息（从缓存中移除）
func (m *Manager) AckMessage(sessionID string, seqID uint64) bool {
	session, exists := m.GetSession(sessionID)
	if !exists {
		return false
	}

	orderedQueue := session.GetOrderedQueue()
	if orderedQueue == nil {
		return false
	}

	return orderedQueue.AckMessage(seqID)
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
func (m *Manager) ActivateSession(sessionID string, gid int64, zone int64) error {
	m.sessionsMux.Lock()
	defer m.sessionsMux.Unlock()

	session, exists := m.sessions[sessionID]
	if !exists {
		return fmt.Errorf("会话不存在: %s", sessionID)
	}

	// 激活会话状态
	if !session.ActivateSession(gid, zone) {
		return fmt.Errorf("会话激活失败，当前状态不是Inited: %s", sessionID)
	}

	// 绑定到GID索引
	if gid != 0 {
		// 检查GID是否已被占用
		if existingSession, exists := m.sessionsByGID[gid]; exists && existingSession.ID != session.ID {
			// 如果有旧会话占用相同GID，移除旧会话
			m.immediateRemoveSession(existingSession.ID)
		}
		m.sessionsByGID[gid] = session
	}

	fmt.Printf("会话激活成功 - 会话: %s, GID: %d, Zone: %d\n", sessionID, gid, zone)
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
		"gid":                   session.Gid(),
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

// GetTotalCacheMemUsed 获取缓存使用的总内存（已废弃）
func (m *Manager) GetTotalCacheMemUsed() int64 {
	// 现在由各个会话的OrderedMessageQueue管理内存，返回0
	return 0
}

// GetSessionByGID 根据GID获取会话
func (m *Manager) GetSessionByGID(gid int64) (*Session, bool) {
	m.sessionsMux.RLock()
	defer m.sessionsMux.RUnlock()

	session, exists := m.sessionsByGID[gid]
	return session, exists
}

// GetSessionByOpenID 根据OpenID获取会话
func (m *Manager) GetSessionByOpenID(openID string) (*Session, bool) {
	m.sessionsMux.RLock()
	defer m.sessionsMux.RUnlock()

	session, exists := m.sessionsByUID[openID]
	return session, exists
}

// BindSession 绑定会话到各种索引（连接建立后调用）
func (m *Manager) BindSession(session *Session) error {
	m.sessionsMux.Lock()
	defer m.sessionsMux.Unlock()

	// 绑定到OpenID索引（如果有的话）
	if session.OpenID != "" {
		// 检查OpenID是否已被占用
		if existingSession, exists := m.sessionsByUID[session.OpenID]; exists && existingSession.ID != session.ID {
			return fmt.Errorf("OpenID %s 已被会话 %s 占用", session.OpenID, existingSession.ID)
		}
		// 绑定到OpenID索引
		m.sessionsByUID[session.OpenID] = session
	}

	// 绑定到GID索引（如果有GID的话）
	if gid := session.Gid(); gid != 0 {
		// 检查GID是否已被占用
		if existingSession, exists := m.sessionsByGID[gid]; exists && existingSession.ID != session.ID {
			return fmt.Errorf("GID %d 已被会话 %s 占用", gid, existingSession.ID)
		}
		// 绑定到GID索引
		m.sessionsByGID[gid] = session
	}

	return nil
}

// UnbindSession 解绑会话（登出或断开时调用）
func (m *Manager) UnbindSession(session *Session) {
	m.sessionsMux.Lock()
	defer m.sessionsMux.Unlock()

	// 从GID索引中移除
	if gid := session.Gid(); gid != 0 {
		delete(m.sessionsByGID, gid)
	}

	// 从OpenID索引中移除
	if session.OpenID != "" {
		delete(m.sessionsByUID, session.OpenID)
	}
}

// RemoveSessionWithDelay 延迟移除会话（支持重连）
func (m *Manager) RemoveSessionWithDelay(sessionID string, delay bool, reason string) {
	m.sessionsMux.RLock()
	session, exists := m.sessions[sessionID]
	m.sessionsMux.RUnlock()

	if !exists {
		return
	}

	// 标记会话为关闭状态
	alreadyClosed := session.SetClosed()
	if alreadyClosed && !delay {
		// 已经关闭且不延迟，立即删除
		m.immediateRemoveSession(sessionID)
		return
	}

	if delay && session.Gid() != 0 {
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
	m.sessionsMux.Lock()
	session, exists := m.sessions[sessionID]
	if exists {
		delete(m.sessions, sessionID)

		// 从其他索引中移除
		if gid := session.Gid(); gid != 0 {
			delete(m.sessionsByGID, gid)
		}
		if session.OpenID != "" {
			delete(m.sessionsByUID, session.OpenID)
		}
	}
	m.sessionsMux.Unlock()

	if exists {
		// 清理该会话的所有缓存消息（现在由OrderedMessageQueue处理）
		// m.messageCache.RemoveSession(sessionID)
		session.Close()
		fmt.Printf("会话 %s 已被彻底清理\n", sessionID)
	}
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

// PushToGID 根据GID单播推送消息
func (m *Manager) PushToGID(gid int64, msgType string, data []byte) error {
	session, exists := m.GetSessionByGID(gid)
	if !exists {
		return fmt.Errorf("GID对应的会话不存在: %d", gid)
	}

	return m.PushToSession(session.ID, msgType, data)
}

// PushToOpenID 根据OpenID单播推送消息
func (m *Manager) PushToOpenID(openID string, msgType string, data []byte) error {
	session, exists := m.GetSessionByOpenID(openID)
	if !exists {
		return fmt.Errorf("OpenID对应的会话不存在: %s", openID)
	}

	return m.PushToSession(session.ID, msgType, data)
}

// GetSessionsByGIDs 根据GID列表获取会话列表
func (m *Manager) GetSessionsByGIDs(gids []int64) []*Session {
	m.sessionsMux.RLock()
	defer m.sessionsMux.RUnlock()

	sessions := make([]*Session, 0, len(gids))
	for _, gid := range gids {
		if session, exists := m.sessionsByGID[gid]; exists {
			sessions = append(sessions, session)
		}
	}
	return sessions
}

// GetOnlineUserCount 获取在线用户数量
func (m *Manager) GetOnlineUserCount() int {
	m.sessionsMux.RLock()
	defer m.sessionsMux.RUnlock()

	count := 0
	for _, session := range m.sessions {
		if session.IsNormal() && !session.IsClosed() {
			count++
		}
	}
	return count
}
