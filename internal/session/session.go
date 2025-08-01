package session

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/quic-go/quic-go"
)

// SessionState 会话状态枚举
type SessionState int32

const (
	SessionInited SessionState = iota // 初始化状态，等待登录
	SessionNormal                     // 正常状态，可以处理业务请求
	SessionClosed                     // 已关闭状态
)

// Session 表示一个客户端会话
type Session struct {
	ID           string       // 会话唯一标识符
	Connection   *quic.Conn   // QUIC连接
	Stream       *quic.Stream // 专用的双向流
	CreateTime   time.Time    // 创建时间
	LastActivity time.Time    // 最后活动时间

	// 客户端标识和重连相关
	ClientID    string // 客户端唯一标识符
	OpenID      string // 用户唯一标识符
	connIdx     int32  // 连接索引，0表示迁移过来的连接
	AccessToken string // 访问令牌
	UserIP      string // 用户IP地址

	// 会话状态
	state int32 // 原子操作的状态字段
	gid   int64 // 游戏内标识符
	zone  int64 // 分区标识符

	// 重连相关状态
	isSuccessor      bool      // 是否为继承会话
	isRelogin        bool      // 是否为重登录
	isRedirectable   bool      // 是否可重定向
	needReset        bool      // 是否需要重置序列号
	redirectableTime time.Time // 可重定向开始时间

	// ACK机制相关
	nextSeqID          uint64 // 下一个消息序列号（原子操作）
	serverSeq          uint64 // 服务器序列号
	maxClientSeq       uint64 // 最大客户端序列号
	clientAckServerSeq uint64 // 客户端确认的服务器序列号

	// 登录前缓存的消息
	cachedMessages    [][]byte     // 暂存的消息
	cachedMessagesMux sync.RWMutex // 保护暂存消息的锁

	// 有序消息队列
	orderedQueue *OrderedMessageQueue // 消息顺序保证队列

	// 状态管理
	closed   bool
	closeMux sync.Mutex

	// 用于通知会话关闭的channel
	closeCh chan struct{}

	// 扩展数据存储
	values    map[interface{}]interface{}
	valuesMux sync.RWMutex
}

// 注意：PendingMessage 已移至 cache.go 作为 CachedMessage

// NextSeqID 获取下一个序列号
func (s *Session) NextSeqID() uint64 {
	return atomic.AddUint64(&s.nextSeqID, 1)
}

// UpdateActivity 更新最后活动时间
func (s *Session) UpdateActivity() {
	s.LastActivity = time.Now()
}

// IsExpired 检查会话是否过期
func (s *Session) IsExpired(timeout time.Duration) bool {
	return time.Since(s.LastActivity) > timeout
}

// Close 关闭会话
func (s *Session) Close() {
	s.closeMux.Lock()
	defer s.closeMux.Unlock()

	if s.closed {
		return
	}

	s.closed = true

	// 停止有序消息队列
	if s.orderedQueue != nil {
		s.orderedQueue.Stop()
	}

	// 关闭流和连接
	s.Stream.Close()
	s.Connection.CloseWithError(0, "session closed")

	// 通知清理goroutine
	close(s.closeCh)
}

// IsClosed 检查会话是否已关闭
func (s *Session) IsClosed() bool {
	s.closeMux.Lock()
	defer s.closeMux.Unlock()
	return s.closed
}

// SetClosed 设置会话为关闭状态，返回是否已经关闭
func (s *Session) SetClosed() bool {
	s.closeMux.Lock()
	defer s.closeMux.Unlock()

	if s.closed {
		return true // 已经关闭
	}

	s.closed = true
	return false // 新关闭
}

// State 获取会话状态
func (s *Session) State() SessionState {
	return SessionState(atomic.LoadInt32(&s.state))
}

// SetState 设置会话状态
func (s *Session) SetState(state SessionState) {
	atomic.StoreInt32(&s.state, int32(state))
}

// IsInited 检查是否为初始化状态
func (s *Session) IsInited() bool {
	return s.State() == SessionInited
}

// IsNormal 检查是否为正常状态
func (s *Session) IsNormal() bool {
	return s.State() == SessionNormal
}

// SetNormal 设置为正常状态，返回是否设置成功
func (s *Session) SetNormal() bool {
	return atomic.CompareAndSwapInt32(&s.state, int32(SessionInited), int32(SessionNormal))
}

// Gid 获取游戏内标识符
func (s *Session) Gid() int64 {
	return atomic.LoadInt64(&s.gid)
}

// SetGid 设置游戏内标识符
func (s *Session) SetGid(gid int64) {
	atomic.StoreInt64(&s.gid, gid)
}

// Zone 获取分区标识符
func (s *Session) Zone() int64 {
	return atomic.LoadInt64(&s.zone)
}

// SetZone 设置分区标识符
func (s *Session) SetZone(zone int64) {
	atomic.StoreInt64(&s.zone, zone)
}

// ServerSeq 获取服务器序列号
func (s *Session) ServerSeq() uint64 {
	return atomic.LoadUint64(&s.serverSeq)
}

// SetServerSeq 设置服务器序列号
func (s *Session) SetServerSeq(seq uint64) {
	atomic.StoreUint64(&s.serverSeq, seq)
}

// NewServerSeq 生成新的服务器序列号
func (s *Session) NewServerSeq() uint64 {
	return atomic.AddUint64(&s.serverSeq, 1)
}

// ResetServerSeq 重置服务器序列号
func (s *Session) ResetServerSeq() {
	atomic.StoreUint64(&s.serverSeq, 1)
}

// MaxClientSeq 获取最大客户端序列号
func (s *Session) MaxClientSeq() uint64 {
	return atomic.LoadUint64(&s.maxClientSeq)
}

// UpdateMaxClientSeq 更新最大客户端序列号
func (s *Session) UpdateMaxClientSeq(seq uint64) error {
	for {
		current := atomic.LoadUint64(&s.maxClientSeq)
		if seq <= current {
			return nil // 序列号没有增加，正常情况
		}
		if atomic.CompareAndSwapUint64(&s.maxClientSeq, current, seq) {
			return nil
		}
	}
}

// ClientAckServerSeq 获取客户端确认的服务器序列号
func (s *Session) ClientAckServerSeq() uint64 {
	return atomic.LoadUint64(&s.clientAckServerSeq)
}

// UpdateAckServerSeq 更新客户端确认的服务器序列号
func (s *Session) UpdateAckServerSeq(seq uint64) error {
	atomic.StoreUint64(&s.clientAckServerSeq, seq)
	return nil
}

// IsSuccessor 检查是否为继承会话
func (s *Session) IsSuccessor() bool {
	return s.isSuccessor
}

// SetSuccessor 设置为继承会话
func (s *Session) SetSuccessor(isSuccessor bool) {
	s.isSuccessor = isSuccessor
}

// IsRelogin 检查是否为重登录
func (s *Session) IsRelogin() bool {
	return s.isRelogin
}

// SetRelogin 设置重登录状态
func (s *Session) SetRelogin(isRelogin bool) {
	s.isRelogin = isRelogin
}

// IsRedirectable 检查是否可重定向
func (s *Session) IsRedirectable() bool {
	return s.isRedirectable
}

// SetRedirectable 设置可重定向状态
func (s *Session) SetRedirectable(redirectable bool) {
	s.isRedirectable = redirectable
	if redirectable {
		s.redirectableTime = time.Now()
	}
}

// NeedReset 检查是否需要重置
func (s *Session) NeedReset() bool {
	return s.needReset
}

// SetNeedReset 设置需要重置
func (s *Session) SetNeedReset(needReset bool) {
	s.needReset = needReset
}

// ConnIdx 获取连接索引
func (s *Session) ConnIdx() int32 {
	return s.connIdx
}

// InheritFrom 从旧会话继承数据
func (s *Session) InheritFrom(oldSession *Session) {
	// 继承关键数据
	s.SetGid(oldSession.Gid())
	s.SetZone(oldSession.Zone())
	s.SetServerSeq(oldSession.ServerSeq())
	s.SetSuccessor(true)

	// 继承状态
	if oldSession.IsNormal() {
		s.SetState(SessionNormal)
	}

	// 继承序列号状态
	atomic.StoreUint64(&s.clientAckServerSeq, oldSession.ClientAckServerSeq())
	atomic.StoreUint64(&s.maxClientSeq, oldSession.MaxClientSeq())

	// 继承有序队列状态（如果存在）
	if oldSession.orderedQueue != nil && s.orderedQueue != nil {
		// 同步有序队列的序列号状态
		s.orderedQueue.ResyncSequence(oldSession.ClientAckServerSeq())
	}

	// 继承扩展数据
	oldSession.valuesMux.RLock()
	if oldSession.values != nil {
		s.valuesMux.Lock()
		if s.values == nil {
			s.values = make(map[interface{}]interface{})
		}
		for k, v := range oldSession.values {
			s.values[k] = v
		}
		s.valuesMux.Unlock()
	}
	oldSession.valuesMux.RUnlock()
}

// Value 获取扩展数据
func (s *Session) Value(key interface{}) interface{} {
	s.valuesMux.RLock()
	defer s.valuesMux.RUnlock()

	if s.values == nil {
		return nil
	}
	return s.values[key]
}

// SetValue 设置扩展数据
func (s *Session) SetValue(key, value interface{}) {
	s.valuesMux.Lock()
	defer s.valuesMux.Unlock()

	if s.values == nil {
		s.values = make(map[interface{}]interface{})
	}
	s.values[key] = value
}

// ValueOrSet 获取或设置扩展数据
func (s *Session) ValueOrSet(key, defaultValue interface{}) interface{} {
	s.valuesMux.Lock()
	defer s.valuesMux.Unlock()

	if s.values == nil {
		s.values = make(map[interface{}]interface{})
	}

	if value, exists := s.values[key]; exists {
		return value
	}

	s.values[key] = defaultValue
	return defaultValue
}

// Expired 检查会话是否过期（用于定时清理）
func (s *Session) Expired(idleTimeout, downTimeout time.Duration) bool {
	if s.IsClosed() {
		// 已关闭的会话，根据downTimeout判断是否清理
		return time.Since(s.LastActivity) > downTimeout
	}

	// 正常会话，根据idleTimeout判断
	return time.Since(s.LastActivity) > idleTimeout
}

// ValidateClientSeq 验证客户端序列号
func (s *Session) ValidateClientSeq(clientSeq uint64) bool {
	current := atomic.LoadUint64(&s.maxClientSeq)

	// 第一次收到消息，直接设置
	if current == 0 {
		atomic.StoreUint64(&s.maxClientSeq, clientSeq)
		return true
	}

	// 序列号必须递增且连续
	if clientSeq == current+1 {
		atomic.StoreUint64(&s.maxClientSeq, clientSeq)
		return true
	}

	// 拒绝过时或跳跃的序列号
	return false
}

// ActivateSession 激活会话（从Inited转换到Normal状态）
func (s *Session) ActivateSession(gid int64, zone int64) bool {
	// 原子性地从Inited状态转换到Normal状态
	if atomic.CompareAndSwapInt32(&s.state, int32(SessionInited), int32(SessionNormal)) {
		// 设置GID和Zone
		atomic.StoreInt64(&s.gid, gid)
		atomic.StoreInt64(&s.zone, zone)
		return true
	}
	return false
}

// CanProcessBusinessRequest 检查是否可以处理业务请求
func (s *Session) CanProcessBusinessRequest() bool {
	state := s.State()
	return state == SessionNormal
}

// CanProcessLoginRequest 检查是否可以处理登录请求
func (s *Session) CanProcessLoginRequest() bool {
	state := s.State()
	return state == SessionInited || state == SessionNormal
}

// AddCachedMessage 添加缓存消息（登录前暂存）
func (s *Session) AddCachedMessage(data []byte) {
	s.cachedMessagesMux.Lock()
	defer s.cachedMessagesMux.Unlock()

	// 复制消息数据避免外部修改
	msgCopy := make([]byte, len(data))
	copy(msgCopy, data)
	s.cachedMessages = append(s.cachedMessages, msgCopy)
}

// GetAndClearCachedMessages 获取并清空缓存的消息
func (s *Session) GetAndClearCachedMessages() [][]byte {
	s.cachedMessagesMux.Lock()
	defer s.cachedMessagesMux.Unlock()

	messages := s.cachedMessages
	s.cachedMessages = nil
	return messages
}

// GetCachedMessageCount 获取缓存消息数量
func (s *Session) GetCachedMessageCount() int {
	s.cachedMessagesMux.RLock()
	defer s.cachedMessagesMux.RUnlock()
	return len(s.cachedMessages)
}

// GetOrderedQueue 获取有序消息队列
func (s *Session) GetOrderedQueue() *OrderedMessageQueue {
	return s.orderedQueue
}

// InitOrderedQueue 初始化有序消息队列（用于重连时）
func (s *Session) InitOrderedQueue(maxQueueSize int) {
	if s.orderedQueue == nil {
		s.orderedQueue = NewOrderedMessageQueue(s.ID, maxQueueSize)
	}
}
