package session

import (
	"fmt"
	"gatesvr/proto"
	"sync"
	"sync/atomic"
	"time"

	"github.com/quic-go/quic-go"
	protobuf "google.golang.org/protobuf/proto"
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
	ClientID string // 客户端唯一标识符
	OpenID   string // 用户唯一标识符
	connIdx  int32  // 连接索引，0表示迁移过来的连接
	UserIP   string // 用户IP地址

	// 会话状态
	state int32 // 原子操作的状态字段
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

	// 消息保序机制 - 新增
	orderingManager *MessageOrderingManager // 消息排序管理器

	// 状态管理
	closed   bool
	closeMux sync.Mutex

	// 用于通知会话关闭的channel
	closeCh chan struct{}
}

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

// SetClosed 设置会话为关闭状态，返回是否已经关闭  setclose和close有什么区别
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

// MaxClientSeq 获取最大客户端序列号
func (s *Session) MaxClientSeq() uint64 {
	return atomic.LoadUint64(&s.maxClientSeq)
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

// ConnIdx 获取连接索引
func (s *Session) ConnIdx() int32 {
	return s.connIdx
}

// InheritFrom 从旧会话继承数据
func (s *Session) InheritFrom(oldSession *Session) {
	// 继承关键数据
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
func (s *Session) ActivateSession(zone int64) bool {
	// 原子性地从Inited状态转换到Normal状态
	if atomic.CompareAndSwapInt32(&s.state, int32(SessionInited), int32(SessionNormal)) {
		// 设置GID和Zone
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

// ======= 消息保序机制相关方法 =======

// AddNotifyBindBeforeRsp 将notify消息添加到beforeRspNotifies映射中
// 这个方法将notify消息绑定到指定的response之前发送
func (s *Session) AddNotifyBindBeforeRsp(grid uint32, notify *NotifyBindMsgItem) bool {
	if s.orderingManager == nil {
		return false
	}
	return s.orderingManager.AddNotifyBindBeforeRsp(grid, notify)
}

// AddNotifyBindAfterRsp 将notify消息添加到afterRspNotifies映射中
// 这个方法将notify消息绑定到指定的response之后发送
func (s *Session) AddNotifyBindAfterRsp(grid uint32, notify *NotifyBindMsgItem) bool {
	if s.orderingManager == nil {
		return false
	}
	return s.orderingManager.AddNotifyBindAfterRsp(grid, notify)
}

// IncrementAndGetSeq 原子性地将serverSeq加一并返回新值
// 这是为下行消息分配递增序号的核心方法
func (s *Session) IncrementAndGetSeq() uint64 {
	return s.NewServerSeq()
}

// GetOrderingManager 获取消息排序管理器
func (s *Session) GetOrderingManager() *MessageOrderingManager {
	return s.orderingManager
}

// CleanupExpiredBindNotifies 清理过期的绑定通知消息
func (s *Session) CleanupExpiredBindNotifies(maxAge int64) int {
	if s.orderingManager == nil {
		return 0
	}
	return s.orderingManager.CleanupExpiredNotifies(time.Now().Unix(), maxAge)
}

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
