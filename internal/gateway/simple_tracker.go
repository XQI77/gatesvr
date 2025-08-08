// Package gateway 提供简化的性能跟踪器，替代复杂的performance.go
package gateway

import (
	"sync/atomic"
	"time"
)

// SimpleTracker 简化的性能跟踪器（替代复杂的PerformanceTracker）
type SimpleTracker struct {
	startTime         time.Time
	totalConnections  int64
	activeConnections int64
	totalRequests     int64
	totalErrors       int64
}

// NewSimpleTracker 创建简化的性能跟踪器
func NewSimpleTracker() *SimpleTracker {
	return &SimpleTracker{
		startTime: time.Now(),
	}
}

// RecordConnection 记录新连接
func (st *SimpleTracker) RecordConnection() {
	atomic.AddInt64(&st.totalConnections, 1)
	atomic.AddInt64(&st.activeConnections, 1)
}

// RecordDisconnection 记录连接断开
func (st *SimpleTracker) RecordDisconnection() {
	atomic.AddInt64(&st.activeConnections, -1)
}

// RecordRequest 记录请求
func (st *SimpleTracker) RecordRequest() {
	atomic.AddInt64(&st.totalRequests, 1)
}

// RecordError 记录错误
func (st *SimpleTracker) RecordError() {
	atomic.AddInt64(&st.totalErrors, 1)
}

// GetBasicStats 获取基础统计信息
func (st *SimpleTracker) GetBasicStats() map[string]interface{} {
	uptime := time.Since(st.startTime)
	totalReqs := atomic.LoadInt64(&st.totalRequests)
	totalErrs := atomic.LoadInt64(&st.totalErrors)
	activeConns := atomic.LoadInt64(&st.activeConnections)
	totalConns := atomic.LoadInt64(&st.totalConnections)

	qps := float64(totalReqs) / uptime.Seconds()
	successRate := float64(totalReqs-totalErrs) / float64(totalReqs) * 100
	if totalReqs == 0 {
		successRate = 100
	}

	return map[string]interface{}{
		"uptime_seconds":     uptime.Seconds(),
		"total_connections":  totalConns,
		"active_connections": activeConns,
		"total_requests":     totalReqs,
		"total_errors":       totalErrs,
		"qps":               qps,
		"success_rate":      successRate,
	}
}

// 为了兼容现有代码，提供空实现的详细监控方法
func (st *SimpleTracker) RecordLatency(latency time.Duration)          {}
func (st *SimpleTracker) RecordParseLatency(latency time.Duration)     {}
func (st *SimpleTracker) RecordUpstreamLatency(latency time.Duration)  {}
func (st *SimpleTracker) RecordSendLatency(latency time.Duration)      {}
func (st *SimpleTracker) RecordTotalLatency(latency time.Duration)     {}
func (st *SimpleTracker) RecordReadLatency(latency time.Duration)      {}
func (st *SimpleTracker) RecordProcessLatency(latency time.Duration)   {}
func (st *SimpleTracker) RecordResponse()                              {}
func (st *SimpleTracker) RecordBytes(bytes int64)                      {}

// 兼容业务请求详细阶段方法（空实现）
func (st *SimpleTracker) RecordSeqValidateLatency(latency time.Duration)   {}
func (st *SimpleTracker) RecordStateValidateLatency(latency time.Duration) {}
func (st *SimpleTracker) RecordBuildReqLatency(latency time.Duration)      {}
func (st *SimpleTracker) RecordLoginProcessLatency(latency time.Duration)  {}
func (st *SimpleTracker) RecordSendRespLatency(latency time.Duration)      {}
func (st *SimpleTracker) RecordProcessNotifyLatency(latency time.Duration) {}

// 兼容发送消息详细阶段方法（空实现）
func (st *SimpleTracker) RecordSeqAllocLatency(latency time.Duration)      {}
func (st *SimpleTracker) RecordMsgEncodeLatency(latency time.Duration)     {}
func (st *SimpleTracker) RecordQueueGetLatency(latency time.Duration)      {}
func (st *SimpleTracker) RecordCallbackSetLatency(latency time.Duration)   {}
func (st *SimpleTracker) RecordEnqueueLatency(latency time.Duration)       {}
func (st *SimpleTracker) RecordDirectSendLatency(latency time.Duration)    {}
func (st *SimpleTracker) RecordWriteMessageLatency(latency time.Duration)  {}
func (st *SimpleTracker) RecordMetricsUpdateLatency(latency time.Duration) {}

// GetStats 兼容原有接口，返回基础统计
func (st *SimpleTracker) GetStats() map[string]interface{} {
	return st.GetBasicStats()
}