// Package upstream 会话管理和序列号生成
package upstream

import (
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// UserSession 上游服务的用户会话
type UserSession struct {
	OpenID       string    // 5位数字的OpenID
	LastActivity time.Time // 最后活动时间
	ServerSeq    uint64    // 服务端序列号（原子操作）
	CreateTime   time.Time // 会话创建时间
	mutex        sync.RWMutex
}

// SessionManager 会话管理器
type SessionManager struct {
	sessions    map[string]*UserSession // openid -> session
	mutex       sync.RWMutex
	sessionTTL  time.Duration // 会话过期时间
	cleanupTick *time.Ticker  // 清理定时器
	stopCh      chan struct{} // 停止信号
	wg          sync.WaitGroup
}

// NewSessionManager 创建新的会话管理器
func NewSessionManager() *SessionManager {
	sm := &SessionManager{
		sessions:    make(map[string]*UserSession),
		sessionTTL:  2 * time.Minute,                  // 2分钟过期时间
		cleanupTick: time.NewTicker(30 * time.Second), // 30秒清理一次
		stopCh:      make(chan struct{}),
	}

	// 预生成100个用户的OpenID
	sm.initializePredefinedUsers()

	// 启动清理goroutine
	sm.wg.Add(1)
	go sm.cleanupExpiredSessions()

	return sm
}

// initializePredefinedUsers 初始化100个预定义用户
func (sm *SessionManager) initializePredefinedUsers() {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	// 生成100个5位数字的OpenID (10000-10099)
	for i := 0; i < 100; i++ {
		openID := fmt.Sprintf("%05d", 10000+i)
		session := &UserSession{
			OpenID:       openID,
			LastActivity: time.Now(),
			ServerSeq:    0, // 初始序列号为0
			CreateTime:   time.Now(),
		}
		sm.sessions[openID] = session
	}
}

// GetOrCreateSession 获取或创建会话
func (sm *SessionManager) GetOrCreateSession(openID string) *UserSession {
	sm.mutex.RLock()
	session, exists := sm.sessions[openID]
	sm.mutex.RUnlock()

	if exists {
		// 更新最后活动时间
		session.mutex.Lock()
		session.LastActivity = time.Now()
		session.mutex.Unlock()
		return session
	}

	// 如果不是预定义的OpenID，返回nil
	return nil
}

// GetRandomOpenID 获取一个随机的有效OpenID（用于客户端测试）
func (sm *SessionManager) GetRandomOpenID() string {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	// 从预定义的100个用户中随机选择一个
	openIDs := make([]string, 0, len(sm.sessions))
	for openID := range sm.sessions {
		openIDs = append(openIDs, openID)
	}

	if len(openIDs) == 0 {
		return "10000" // 默认返回第一个
	}

	return openIDs[rand.Intn(len(openIDs))]
}

// GetAllValidOpenIDs 获取所有有效的OpenID列表
func (sm *SessionManager) GetAllValidOpenIDs() []string {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	openIDs := make([]string, 0, len(sm.sessions))
	for openID := range sm.sessions {
		openIDs = append(openIDs, openID)
	}
	return openIDs
}

// GenerateServerSeq 为指定用户生成下一个服务端序列号
func (sm *SessionManager) GenerateServerSeq(openID string) uint64 {
	session := sm.GetOrCreateSession(openID)
	if session == nil {
		return 0 // 无效的OpenID
	}

	return atomic.AddUint64(&session.ServerSeq, 1)
}

// GetSessionStats 获取会话统计信息
func (sm *SessionManager) GetSessionStats() map[string]interface{} {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	stats := make(map[string]interface{})
	stats["total_sessions"] = len(sm.sessions)
	stats["session_ttl_minutes"] = sm.sessionTTL.Minutes()

	// 统计活跃会话（最近2分钟内有活动）
	activeCount := 0
	now := time.Now()
	for _, session := range sm.sessions {
		session.mutex.RLock()
		if now.Sub(session.LastActivity) < sm.sessionTTL {
			activeCount++
		}
		session.mutex.RUnlock()
	}
	stats["active_sessions"] = activeCount

	return stats
}

// GetSessionInfo 获取特定会话信息
func (sm *SessionManager) GetSessionInfo(openID string) map[string]interface{} {
	sm.mutex.RLock()
	session, exists := sm.sessions[openID]
	sm.mutex.RUnlock()

	if !exists {
		return map[string]interface{}{
			"exists": false,
		}
	}

	session.mutex.RLock()
	defer session.mutex.RUnlock()

	return map[string]interface{}{
		"exists":        true,
		"openid":        session.OpenID,
		"server_seq":    atomic.LoadUint64(&session.ServerSeq),
		"create_time":   session.CreateTime.Format(time.RFC3339),
		"last_activity": session.LastActivity.Format(time.RFC3339),
		"age_seconds":   time.Since(session.CreateTime).Seconds(),
		"idle_seconds":  time.Since(session.LastActivity).Seconds(),
	}
}

// cleanupExpiredSessions 清理过期会话
func (sm *SessionManager) cleanupExpiredSessions() {
	defer sm.wg.Done()

	for {
		select {
		case <-sm.stopCh:
			return
		case <-sm.cleanupTick.C:
			sm.performCleanup()
		}
	}
}

// performCleanup 执行清理操作
func (sm *SessionManager) performCleanup() {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	now := time.Now()
	expiredCount := 0

	for _, session := range sm.sessions {
		session.mutex.RLock()
		isExpired := now.Sub(session.LastActivity) > sm.sessionTTL
		session.mutex.RUnlock()

		if isExpired {
			// 重置过期会话的序列号，但保留会话对象
			atomic.StoreUint64(&session.ServerSeq, 0)
			session.mutex.Lock()
			session.LastActivity = now
			session.CreateTime = now
			session.mutex.Unlock()
			expiredCount++
		}
	}

	if expiredCount > 0 {
		fmt.Printf("清理了 %d 个过期会话（重置序列号）\n", expiredCount)
	}
}

// Stop 停止会话管理器
func (sm *SessionManager) Stop() {
	close(sm.stopCh)
	sm.cleanupTick.Stop()
	sm.wg.Wait()
}

// NextServerSeq 为会话生成下一个服务端序列号（线程安全）
func (s *UserSession) NextServerSeq() uint64 {
	return atomic.AddUint64(&s.ServerSeq, 1)
}

// GetServerSeq 获取当前服务端序列号
func (s *UserSession) GetServerSeq() uint64 {
	return atomic.LoadUint64(&s.ServerSeq)
}

// UpdateActivity 更新会话活动时间
func (s *UserSession) UpdateActivity() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.LastActivity = time.Now()
}

// IsExpired 检查会话是否过期
func (s *UserSession) IsExpired(ttl time.Duration) bool {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return time.Since(s.LastActivity) > ttl
}
