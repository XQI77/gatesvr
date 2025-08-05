// Package gateway 提供START消息异步处理功能
package gateway

import (
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"

	"gatesvr/internal/session"
	pb "gatesvr/proto"
)

// StartTask START消息处理任务
type StartTask struct {
	Session    *session.Session  // 会话对象
	Request    *pb.ClientRequest // 客户端请求
	ResponseCh chan StartResult  // 响应通道
	StartTime  time.Time         // 任务开始时间
	Timeout    time.Duration     // 超时时间
}

// StartResult START消息处理结果
type StartResult struct {
	Success     bool              // 是否成功
	Error       error             // 错误信息
	Response    *pb.StartResponse // 响应消息
	Session     *session.Session  // 更新后的会话
	ProcessTime time.Duration     // 处理时间
}

// StartMessageProcessor START消息异步处理器
type StartMessageProcessor struct {
	server     *Server         // 网关服务器引用
	workers    chan struct{}   // worker信号量，控制并发数
	taskQueue  chan *StartTask // 任务队列
	maxWorkers int             // 最大worker数量
	queueSize  int             // 队列大小
	timeout    time.Duration   // 处理超时时间

	// 统计信息
	stats struct {
		totalTasks     int64 // 总任务数
		successTasks   int64 // 成功任务数
		failedTasks    int64 // 失败任务数
		timeoutTasks   int64 // 超时任务数
		queueFullTasks int64 // 队列满任务数
		avgProcessTime int64 // 平均处理时间(纳秒)
	}

	stopCh chan struct{}
	wg     sync.WaitGroup
	mu     sync.RWMutex
}

// NewStartMessageProcessor 创建START消息处理器
func NewStartMessageProcessor(server *Server, maxWorkers, queueSize int, timeout time.Duration) *StartMessageProcessor {
	if maxWorkers <= 0 {
		maxWorkers = 100 // 默认100个worker
	}
	if queueSize <= 0 {
		queueSize = 1000 // 默认1000个任务队列
	}
	if timeout <= 0 {
		timeout = 30 * time.Second // 默认30秒超时
	}

	processor := &StartMessageProcessor{
		server:     server,
		workers:    make(chan struct{}, maxWorkers),
		taskQueue:  make(chan *StartTask, queueSize),
		maxWorkers: maxWorkers,
		queueSize:  queueSize,
		timeout:    timeout,
		stopCh:     make(chan struct{}),
	}

	// 预先分配worker信号量
	for i := 0; i < maxWorkers; i++ {
		processor.workers <- struct{}{}
	}

	return processor
}

// Start 启动处理器
func (p *StartMessageProcessor) Start() {
	log.Printf("启动START消息异步处理器 - Workers: %d, 队列大小: %d, 超时: %v",
		p.maxWorkers, p.queueSize, p.timeout)

	// 启动worker调度器
	p.wg.Add(1)
	go p.workerDispatcher()
}

// Stop 停止处理器
func (p *StartMessageProcessor) Stop() {
	log.Printf("停止START消息异步处理器...")
	close(p.stopCh)
	p.wg.Wait()
	log.Printf("START消息异步处理器已停止")
}

// ProcessStartMessage 异步处理START消息
func (p *StartMessageProcessor) ProcessStartMessage(sess *session.Session, req *pb.ClientRequest) (*StartResult, error) {
	atomic.AddInt64(&p.stats.totalTasks, 1)

	// 创建任务
	task := &StartTask{
		Session:    sess,
		Request:    req,
		ResponseCh: make(chan StartResult, 1),
		StartTime:  time.Now(),
		Timeout:    p.timeout,
	}

	// 尝试将任务加入队列
	select {
	case p.taskQueue <- task:
		// 成功加入队列，等待处理结果
		select {
		case result := <-task.ResponseCh:
			return &result, nil
		case <-time.After(p.timeout):
			atomic.AddInt64(&p.stats.timeoutTasks, 1)
			return &StartResult{
				Success: false,
				Error:   fmt.Errorf("START消息处理超时"),
			}, nil
		}
	default:
		// 队列已满
		atomic.AddInt64(&p.stats.queueFullTasks, 1)
		return &StartResult{
			Success: false,
			Error:   fmt.Errorf("START消息处理队列已满，请稍后重试"),
		}, nil
	}
}

// workerDispatcher worker调度器
func (p *StartMessageProcessor) workerDispatcher() {
	defer p.wg.Done()

	for {
		select {
		case <-p.stopCh:
			// 处理剩余任务
			p.drainTasks()
			return

		case task := <-p.taskQueue:
			// 获取worker信号量
			select {
			case <-p.workers:
				// 启动worker处理任务
				p.wg.Add(1)
				go p.processTask(task)
			case <-p.stopCh:
				// 如果正在停止，拒绝新任务
				p.rejectTask(task, fmt.Errorf("处理器正在停止"))
				return
			}
		}
	}
}

// processTask 处理单个任务
func (p *StartMessageProcessor) processTask(task *StartTask) {
	defer p.wg.Done()
	defer func() {
		// 归还worker信号量
		p.workers <- struct{}{}
	}()

	startTime := time.Now()
	result := p.handleStartMessage(task.Session, task.Request)
	processTime := time.Since(startTime)
	result.ProcessTime = processTime

	// 更新统计信息
	if result.Success {
		atomic.AddInt64(&p.stats.successTasks, 1)
	} else {
		atomic.AddInt64(&p.stats.failedTasks, 1)
	}

	// 更新平均处理时间
	currentAvg := atomic.LoadInt64(&p.stats.avgProcessTime)
	newAvg := (currentAvg + processTime.Nanoseconds()) / 2
	atomic.StoreInt64(&p.stats.avgProcessTime, newAvg)

	// 发送结果
	select {
	case task.ResponseCh <- result:
		// 成功发送结果
	default:
		// 接收方已经不再等待（可能超时了）
		log.Printf("任务结果无法发送，接收方可能已超时 - 会话: %s", task.Session.ID)
	}
}

// handleStartMessage 具体的START消息处理逻辑（从原handleStart方法移植）
func (p *StartMessageProcessor) handleStartMessage(sess *session.Session, req *pb.ClientRequest) StartResult {
	log.Printf("异步处理START请求 - 会话: %s, 消息ID: %d", sess.ID, req.MsgId)

	// START消息不验证序列号（序列号应为0）
	if req.SeqId != 0 {
		log.Printf("START消息序列号应为0 - 会话: %s, 实际序列号: %d", sess.ID, req.SeqId)
		return StartResult{
			Success: false,
			Error:   fmt.Errorf("无效的序列号"),
			Response: &pb.StartResponse{
				Success: false,
				Error: &pb.ErrorMessage{
					ErrorCode:    400,
					ErrorMessage: "无效的序列号",
					Detail:       "START消息序列号必须为0",
				},
			},
		}
	}

	// 解析连接建立请求
	parseStart := time.Now()
	startReq := &pb.StartRequest{}
	if err := proto.Unmarshal(req.Payload, startReq); err != nil {
		log.Printf("解析连接建立请求失败: %v", err)
		p.server.performanceTracker.RecordError()
		return StartResult{
			Success: false,
			Error:   err,
			Response: &pb.StartResponse{
				Success: false,
				Error: &pb.ErrorMessage{
					ErrorCode:    400,
					ErrorMessage: "解析请求失败",
					Detail:       err.Error(),
				},
			},
		}
	}
	p.server.performanceTracker.RecordParseLatency(time.Since(parseStart))

	// 验证必要字段
	if startReq.Openid == "" {
		log.Printf("连接建立请求缺少OpenID - 会话: %s", sess.ID)
		return StartResult{
			Success: false,
			Error:   fmt.Errorf("缺少OpenID"),
			Response: &pb.StartResponse{
				Success: false,
				Error: &pb.ErrorMessage{
					ErrorCode:    400,
					ErrorMessage: "缺少OpenID",
					Detail:       "OpenID不能为空",
				},
			},
		}
	}

	// 更新session的客户端信息
	sess.OpenID = startReq.Openid
	sess.ClientID = req.Openid // 从请求头中获取openid作为备份
	sess.AccessToken = startReq.AuthToken

	// 基础认证验证（可选）
	if startReq.AuthToken != "" {
		if !p.server.validateAuthToken(startReq.AuthToken, startReq.Openid) {
			log.Printf("认证令牌验证失败 - OpenID: %s, 会话: %s", startReq.Openid, sess.ID)
			return StartResult{
				Success: false,
				Error:   fmt.Errorf("认证失败"),
				Response: &pb.StartResponse{
					Success: false,
					Error: &pb.ErrorMessage{
						ErrorCode:    401,
						ErrorMessage: "认证失败",
						Detail:       "无效的认证令牌",
					},
				},
			}
		}
	}

	// 将session注册到openid索引中
	if err := p.server.sessionManager.BindSession(sess); err != nil {
		log.Printf("绑定session到openid失败: %v", err)
		return StartResult{
			Success: false,
			Error:   err,
			Response: &pb.StartResponse{
				Success: false,
				Error: &pb.ErrorMessage{
					ErrorCode:    500,
					ErrorMessage: "会话绑定失败",
					Detail:       err.Error(),
				},
			},
		}
	}

	// 处理重连时的序列号同步
	if startReq.LastAckedSeqId > 0 {
		// 清除已确认的消息
		ackedCount := p.server.sessionManager.AckMessagesUpTo(sess.ID, startReq.LastAckedSeqId)

		// 同步有序队列的序列号
		p.server.orderedSender.ResyncSessionSequence(sess, startReq.LastAckedSeqId)

		log.Printf("重连时清除已确认消息 - 会话: %s, 最后确认序列号: %d, 清除数量: %d",
			sess.ID, startReq.LastAckedSeqId, ackedCount)
	}

	// 创建成功响应
	connectionID := sess.ID // 使用session ID作为连接ID
	successResponse := &pb.StartResponse{
		Success:           true,
		Error:             nil,
		HeartbeatInterval: 30, // 30秒心跳间隔
		ConnectionId:      connectionID,
	}

	log.Printf("异步处理连接建立请求成功 - OpenID: %s, 客户端: %s, 会话: %s, 状态: %d",
		startReq.Openid, sess.ClientID, sess.ID, sess.State())

	return StartResult{
		Success:  true,
		Error:    nil,
		Response: successResponse,
		Session:  sess,
	}
}

// rejectTask 拒绝任务
func (p *StartMessageProcessor) rejectTask(task *StartTask, err error) {
	result := StartResult{
		Success: false,
		Error:   err,
		Response: &pb.StartResponse{
			Success: false,
			Error: &pb.ErrorMessage{
				ErrorCode:    503,
				ErrorMessage: "服务繁忙",
				Detail:       err.Error(),
			},
		},
	}

	select {
	case task.ResponseCh <- result:
	default:
		// 无法发送结果，任务可能已经超时
	}
}

// drainTasks 处理剩余任务
func (p *StartMessageProcessor) drainTasks() {
	for {
		select {
		case task := <-p.taskQueue:
			p.rejectTask(task, fmt.Errorf("处理器已停止"))
		default:
			return
		}
	}
}

// GetStats 获取统计信息
func (p *StartMessageProcessor) GetStats() map[string]interface{} {
	p.mu.RLock()
	defer p.mu.RUnlock()

	total := atomic.LoadInt64(&p.stats.totalTasks)
	success := atomic.LoadInt64(&p.stats.successTasks)
	failed := atomic.LoadInt64(&p.stats.failedTasks)
	timeout := atomic.LoadInt64(&p.stats.timeoutTasks)
	queueFull := atomic.LoadInt64(&p.stats.queueFullTasks)
	avgTime := atomic.LoadInt64(&p.stats.avgProcessTime)

	var successRate float64
	if total > 0 {
		successRate = float64(success) / float64(total) * 100
	}

	return map[string]interface{}{
		"max_workers":         p.maxWorkers,
		"queue_size":          p.queueSize,
		"queue_length":        len(p.taskQueue),
		"available_workers":   len(p.workers),
		"total_tasks":         total,
		"success_tasks":       success,
		"failed_tasks":        failed,
		"timeout_tasks":       timeout,
		"queue_full_tasks":    queueFull,
		"success_rate":        successRate,
		"avg_process_time_ms": float64(avgTime) / 1e6,
		"timeout_duration":    p.timeout.String(),
	}
}
