// Package gateway 提供异步请求处理功能
package gateway

import (
	"context"
	"fmt"
	"log"
	"runtime"
	"sync"
	"time"

	"gatesvr/internal/session"
	pb "gatesvr/proto"
)

// AsyncTask 异步任务结构
type AsyncTask struct {
	TaskID      string               // 任务唯一ID
	SessionID   string               // 会话ID
	Session     *session.Session     // 会话对象
	Request     *pb.ClientRequest    // 原始客户端请求
	BusinessReq *pb.BusinessRequest  // 解析后的业务请求
	UpstreamReq *pb.UpstreamRequest  // 上游请求
	Context     context.Context      // 请求上下文
	StartTime   time.Time            // 任务开始时间
	IsLogin     bool                 // 是否为登录请求
}

// AsyncConfig 异步处理配置
type AsyncConfig struct {
	MaxWorkers   int           `json:"max_workers"`    // 最大工作协程数
	MaxQueueSize int           `json:"max_queue_size"` // 最大队列长度
	TaskTimeout  time.Duration `json:"task_timeout"`   // 任务超时时间
}

// DefaultAsyncConfig 默认异步配置
var DefaultAsyncConfig = &AsyncConfig{
	MaxWorkers:   runtime.NumCPU() * 4, // CPU核数 * 4
	MaxQueueSize: 10000,                // 队列长度10000
	TaskTimeout:  30 * time.Second,     // 30秒超时
}

// AsyncRequestProcessor 异步请求处理器
type AsyncRequestProcessor struct {
	config     *AsyncConfig    // 配置
	taskQueue  chan *AsyncTask // 任务队列
	server     *Server         // 服务器引用
	stopCh     chan struct{}   // 停止信号
	wg         sync.WaitGroup  // 等待组
	started    bool            // 是否已启动
	startMutex sync.Mutex      // 启动保护锁
}

// NewAsyncRequestProcessor 创建异步请求处理器
func NewAsyncRequestProcessor(server *Server, config *AsyncConfig) *AsyncRequestProcessor {
	if config == nil {
		config = DefaultAsyncConfig
	}

	return &AsyncRequestProcessor{
		config:    config,
		taskQueue: make(chan *AsyncTask, config.MaxQueueSize),
		server:    server,
		stopCh:    make(chan struct{}),
		started:   false,
	}
}

// Start 启动异步处理器
func (ap *AsyncRequestProcessor) Start() error {
	ap.startMutex.Lock()
	defer ap.startMutex.Unlock()

	if ap.started {
		return fmt.Errorf("异步处理器已启动")
	}

	// 启动工作协程池
	for i := 0; i < ap.config.MaxWorkers; i++ {
		ap.wg.Add(1)
		go ap.worker(i)
	}

	ap.started = true
	log.Printf("异步请求处理器已启动 - 工作协程数: %d, 队列大小: %d",
		ap.config.MaxWorkers, ap.config.MaxQueueSize)

	return nil
}

// Stop 停止异步处理器
func (ap *AsyncRequestProcessor) Stop() error {
	ap.startMutex.Lock()
	defer ap.startMutex.Unlock()

	if !ap.started {
		return nil
	}

	close(ap.stopCh)
	ap.wg.Wait()

	ap.started = false
	log.Printf("异步请求处理器已停止")

	return nil
}

// SubmitTask 提交异步任务
func (ap *AsyncRequestProcessor) SubmitTask(task *AsyncTask) bool {
	if !ap.started {
		log.Printf("异步处理器未启动，拒绝任务: %s", task.TaskID)
		return false
	}

	select {
	case ap.taskQueue <- task:
		log.Printf("异步任务已提交 - 任务ID: %s, 会话: %s, 动作: %s",
			task.TaskID, task.SessionID, task.BusinessReq.Action)
		return true
	default:
		// 队列已满，拒绝任务
		log.Printf("异步队列已满，拒绝任务 - 任务ID: %s, 会话: %s",
			task.TaskID, task.SessionID)
		return false
	}
}

// GetQueueLength 获取队列长度
func (ap *AsyncRequestProcessor) GetQueueLength() int {
	return len(ap.taskQueue)
}

// GetStats 获取统计信息
func (ap *AsyncRequestProcessor) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"max_workers":    ap.config.MaxWorkers,
		"max_queue_size": ap.config.MaxQueueSize,
		"queue_length":   ap.GetQueueLength(),
		"started":        ap.started,
	}
}

// worker 工作协程
func (ap *AsyncRequestProcessor) worker(workerID int) {
	defer ap.wg.Done()
	log.Printf("异步工作协程启动 - ID: %d", workerID)

	for {
		select {
		case task := <-ap.taskQueue:
			ap.processTask(workerID, task)
		case <-ap.stopCh:
			log.Printf("异步工作协程停止 - ID: %d", workerID)
			return
		}
	}
}

// processTask 处理异步任务
func (ap *AsyncRequestProcessor) processTask(workerID int, task *AsyncTask) {
	startTime := time.Now()
	log.Printf("开始处理异步任务 - 工作协程: %d, 任务: %s, 会话: %s, 动作: %s",
		workerID, task.TaskID, task.SessionID, task.BusinessReq.Action)

	defer func() {
		processingTime := time.Since(startTime)
		log.Printf("异步任务处理完成 - 工作协程: %d, 任务: %s, 耗时: %.2fms",
			workerID, task.TaskID, processingTime.Seconds()*1000)

		// 记录总处理时延
		ap.server.performanceTracker.RecordTotalLatency(time.Since(task.StartTime))
	}()

	// 检查会话是否仍然有效
	if !ap.isSessionValid(task.Session) {
		log.Printf("会话已失效，跳过任务处理 - 任务: %s, 会话: %s", task.TaskID, task.SessionID)
		return
	}

	// 直接使用传入的上下文（已包含上游超时设置）
	ctx := task.Context

	// 调用上游服务（使用OpenID路由）
	upstreamStart := time.Now()
	upstreamResp, err := ap.server.callUpstreamService(ctx, task.Session.OpenID, task.UpstreamReq)
	upstreamLatency := time.Since(upstreamStart)
	ap.server.performanceTracker.RecordUpstreamLatency(upstreamLatency)

	if err != nil {
		serviceInfo := ap.server.getUpstreamServiceInfo(task.Session.OpenID)
		log.Printf("异步调用上游服务失败 - 任务: %s, 服务: %s, 错误: %v",
			task.TaskID, serviceInfo, err)
		ap.server.metrics.IncError("upstream_error")
		ap.server.sendErrorResponse(task.Session, task.Request.MsgId, 500, "上游服务错误", err.Error())
		return
	}

	// 发送业务响应
	sendRespStart := time.Now()
	if err := ap.server.orderedSender.SendBusinessResponse(task.Session, task.Request.MsgId,
		upstreamResp.Code, upstreamResp.Message, upstreamResp.Data, upstreamResp.Headers); err != nil {
		log.Printf("异步发送业务响应失败 - 任务: %s, 错误: %v", task.TaskID, err)
		return
	}
	sendRespLatency := time.Since(sendRespStart)
	ap.server.performanceTracker.RecordSendRespLatency(sendRespLatency)

	// 处理绑定notify消息
	grid := uint32(task.Request.SeqId)
	if err := ap.server.processBoundNotifies(task.Session, grid); err != nil {
		log.Printf("异步处理绑定notify消息失败 - 任务: %s, 错误: %v", task.TaskID, err)
	}

	log.Printf("异步任务处理成功 - 任务: %s, 响应码: %d, 上游时延: %.2fms",
		task.TaskID, upstreamResp.Code, upstreamLatency.Seconds()*1000)
}

// isSessionValid 检查会话是否仍然有效
func (ap *AsyncRequestProcessor) isSessionValid(sess *session.Session) bool {
	if sess == nil {
		return false
	}

	// 检查会话状态是否已关闭
	if int32(sess.State()) == int32(session.SessionClosed) {
		return false
	}

	// 检查连接是否仍然有效
	if sess.Connection == nil {
		return false
	}

	return true
}
