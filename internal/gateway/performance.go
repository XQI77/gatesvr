// Package gateway 提供性能追踪功能
package gateway

import (
	"sync"
	"sync/atomic"
	"time"
)

// PerformanceTracker 性能追踪器
type PerformanceTracker struct {
	startTime         time.Time
	totalConnections  int64
	activeConnections int64
	totalRequests     int64
	totalResponses    int64
	totalErrors       int64
	totalBytes        int64
	requestLatencies  []time.Duration
	latencyMutex      sync.Mutex
	maxLatencyEntries int

	// 详细时延监控 - 新增
	detailedLatencyTracker *DetailedLatencyTracker
}

// DetailedLatencyTracker 详细时延跟踪器
type DetailedLatencyTracker struct {
	// 各阶段时延统计
	parseLatency    *StageLatencyStats // 消息解析时延
	upstreamLatency *StageLatencyStats // 上游调用时延
	encodeLatency   *StageLatencyStats // 响应编码时延
	sendLatency     *StageLatencyStats // 消息发送时延
	totalLatency    *StageLatencyStats // 总处理时延
	readLatency     *StageLatencyStats // 消息读取时延（新增）
	processLatency  *StageLatencyStats // 消息处理时延（纯处理，不含读取等待）
}

// StageLatencyStats 阶段时延统计
type StageLatencyStats struct {
	totalLatency  int64           // 总时延（纳秒）
	totalCount    int64           // 总次数
	minLatency    int64           // 最小时延（纳秒）
	maxLatency    int64           // 最大时延（纳秒）
	recentSamples []time.Duration // 最近的样本
	samplesMutex  sync.RWMutex
	maxSamples    int
}

// NewStageLatencyStats 创建新的阶段时延统计
func NewStageLatencyStats() *StageLatencyStats {
	return &StageLatencyStats{
		minLatency:    int64(^uint64(0) >> 1), // 初始化为最大值
		recentSamples: make([]time.Duration, 0, 100),
		maxSamples:    100,
	}
}

// Record 记录时延
func (s *StageLatencyStats) Record(latency time.Duration) {
	latencyNs := latency.Nanoseconds()

	atomic.AddInt64(&s.totalCount, 1)
	atomic.AddInt64(&s.totalLatency, latencyNs)

	// 更新最小最大值
	for {
		currentMin := atomic.LoadInt64(&s.minLatency)
		if latencyNs >= currentMin || atomic.CompareAndSwapInt64(&s.minLatency, currentMin, latencyNs) {
			break
		}
	}

	for {
		currentMax := atomic.LoadInt64(&s.maxLatency)
		if latencyNs <= currentMax || atomic.CompareAndSwapInt64(&s.maxLatency, currentMax, latencyNs) {
			break
		}
	}

	// 保存最近的样本
	s.samplesMutex.Lock()
	if len(s.recentSamples) >= s.maxSamples {
		s.recentSamples = s.recentSamples[1:]
	}
	s.recentSamples = append(s.recentSamples, latency)
	s.samplesMutex.Unlock()
}

// GetStats 获取统计信息
func (s *StageLatencyStats) GetStats() map[string]interface{} {
	totalCount := atomic.LoadInt64(&s.totalCount)
	if totalCount == 0 {
		return map[string]interface{}{
			"count":          0,
			"avg_latency_ms": 0.0,
			"min_latency_ms": 0.0,
			"max_latency_ms": 0.0,
			"p95_latency_ms": 0.0,
			"p99_latency_ms": 0.0,
		}
	}

	totalLatNs := atomic.LoadInt64(&s.totalLatency)
	minLatNs := atomic.LoadInt64(&s.minLatency)
	maxLatNs := atomic.LoadInt64(&s.maxLatency)

	avgLatencyMs := float64(totalLatNs) / float64(totalCount) / 1e6
	minLatencyMs := float64(minLatNs) / 1e6
	maxLatencyMs := float64(maxLatNs) / 1e6

	var p95LatencyMs, p99LatencyMs float64

	s.samplesMutex.RLock()
	if len(s.recentSamples) >= 20 {
		// 创建副本并排序
		samples := make([]time.Duration, len(s.recentSamples))
		copy(samples, s.recentSamples)

		// 排序
		for i := 0; i < len(samples)-1; i++ {
			for j := i + 1; j < len(samples); j++ {
				if samples[i] > samples[j] {
					samples[i], samples[j] = samples[j], samples[i]
				}
			}
		}

		p95Index := int(float64(len(samples)) * 0.95)
		p99Index := int(float64(len(samples)) * 0.99)

		if p95Index < len(samples) {
			p95LatencyMs = float64(samples[p95Index].Nanoseconds()) / 1e6
		}
		if p99Index < len(samples) {
			p99LatencyMs = float64(samples[p99Index].Nanoseconds()) / 1e6
		}
	}
	s.samplesMutex.RUnlock()

	return map[string]interface{}{
		"count":          totalCount,
		"avg_latency_ms": avgLatencyMs,
		"min_latency_ms": minLatencyMs,
		"max_latency_ms": maxLatencyMs,
		"p95_latency_ms": p95LatencyMs,
		"p99_latency_ms": p99LatencyMs,
	}
}

// NewDetailedLatencyTracker 创建新的详细时延跟踪器
func NewDetailedLatencyTracker() *DetailedLatencyTracker {
	return &DetailedLatencyTracker{
		parseLatency:    NewStageLatencyStats(),
		upstreamLatency: NewStageLatencyStats(),
		encodeLatency:   NewStageLatencyStats(),
		sendLatency:     NewStageLatencyStats(),
		totalLatency:    NewStageLatencyStats(),
		readLatency:     NewStageLatencyStats(), // 新增
		processLatency:  NewStageLatencyStats(), // 新增
	}
}

// RecordParseLatency 记录解析时延
func (dt *DetailedLatencyTracker) RecordParseLatency(latency time.Duration) {
	dt.parseLatency.Record(latency)
}

// RecordUpstreamLatency 记录上游调用时延
func (dt *DetailedLatencyTracker) RecordUpstreamLatency(latency time.Duration) {
	dt.upstreamLatency.Record(latency)
}

// RecordEncodeLatency 记录编码时延
func (dt *DetailedLatencyTracker) RecordEncodeLatency(latency time.Duration) {
	dt.encodeLatency.Record(latency)
}

// RecordSendLatency 记录发送时延
func (dt *DetailedLatencyTracker) RecordSendLatency(latency time.Duration) {
	dt.sendLatency.Record(latency)
}

// RecordTotalLatency 记录总处理时延
func (dt *DetailedLatencyTracker) RecordTotalLatency(latency time.Duration) {
	dt.totalLatency.Record(latency)
}

// RecordReadLatency 记录读取时延（新增）
func (dt *DetailedLatencyTracker) RecordReadLatency(latency time.Duration) {
	dt.readLatency.Record(latency)
}

// RecordProcessLatency 记录处理时延（纯处理，不含读取等待）
func (dt *DetailedLatencyTracker) RecordProcessLatency(latency time.Duration) {
	dt.processLatency.Record(latency)
}

// GetDetailedStats 获取详细统计
func (dt *DetailedLatencyTracker) GetDetailedStats() map[string]interface{} {
	return map[string]interface{}{
		"parse_latency":    dt.parseLatency.GetStats(),
		"upstream_latency": dt.upstreamLatency.GetStats(),
		"encode_latency":   dt.encodeLatency.GetStats(),
		"send_latency":     dt.sendLatency.GetStats(),
		"total_latency":    dt.totalLatency.GetStats(),
		"read_latency":     dt.readLatency.GetStats(),    // 新增
		"process_latency":  dt.processLatency.GetStats(), // 新增
	}
}

// NewPerformanceTracker 创建新的性能追踪器
func NewPerformanceTracker() *PerformanceTracker {
	return &PerformanceTracker{
		startTime:              time.Now(),
		maxLatencyEntries:      1000, // 保留最近1000个延迟记录
		requestLatencies:       make([]time.Duration, 0, 1000),
		detailedLatencyTracker: NewDetailedLatencyTracker(), // 新增
	}
}

// RecordConnection 记录新连接
func (pt *PerformanceTracker) RecordConnection() {
	atomic.AddInt64(&pt.totalConnections, 1)
	atomic.AddInt64(&pt.activeConnections, 1)
}

// RecordDisconnection 记录连接断开
func (pt *PerformanceTracker) RecordDisconnection() {
	atomic.AddInt64(&pt.activeConnections, -1)
}

// RecordRequest 记录请求
func (pt *PerformanceTracker) RecordRequest() {
	atomic.AddInt64(&pt.totalRequests, 1)
}

// RecordResponse 记录响应
func (pt *PerformanceTracker) RecordResponse() {
	atomic.AddInt64(&pt.totalResponses, 1)
}

// RecordError 记录错误
func (pt *PerformanceTracker) RecordError() {
	atomic.AddInt64(&pt.totalErrors, 1)
}

// RecordBytes 记录传输字节数
func (pt *PerformanceTracker) RecordBytes(bytes int64) {
	atomic.AddInt64(&pt.totalBytes, bytes)
}

// RecordLatency 记录延迟
func (pt *PerformanceTracker) RecordLatency(latency time.Duration) {
	pt.latencyMutex.Lock()
	defer pt.latencyMutex.Unlock()

	if len(pt.requestLatencies) >= pt.maxLatencyEntries {
		// 移除最老的记录
		pt.requestLatencies = pt.requestLatencies[1:]
	}
	pt.requestLatencies = append(pt.requestLatencies, latency)
}

// 新增详细时延记录方法
func (pt *PerformanceTracker) RecordParseLatency(latency time.Duration) {
	pt.detailedLatencyTracker.RecordParseLatency(latency)
}

func (pt *PerformanceTracker) RecordUpstreamLatency(latency time.Duration) {
	pt.detailedLatencyTracker.RecordUpstreamLatency(latency)
}

func (pt *PerformanceTracker) RecordEncodeLatency(latency time.Duration) {
	pt.detailedLatencyTracker.RecordEncodeLatency(latency)
}

func (pt *PerformanceTracker) RecordSendLatency(latency time.Duration) {
	pt.detailedLatencyTracker.RecordSendLatency(latency)
}

func (pt *PerformanceTracker) RecordTotalLatency(latency time.Duration) {
	pt.detailedLatencyTracker.RecordTotalLatency(latency)
}

// RecordReadLatency 记录读取时延（新增）
func (pt *PerformanceTracker) RecordReadLatency(latency time.Duration) {
	pt.detailedLatencyTracker.RecordReadLatency(latency)
}

// RecordProcessLatency 记录处理时延（纯处理，不含读取等待）
func (pt *PerformanceTracker) RecordProcessLatency(latency time.Duration) {
	pt.detailedLatencyTracker.RecordProcessLatency(latency)
}

// GetStats 获取性能统计数据
func (pt *PerformanceTracker) GetStats() map[string]interface{} {
	pt.latencyMutex.Lock()
	defer pt.latencyMutex.Unlock()

	uptime := time.Since(pt.startTime)
	totalReqs := atomic.LoadInt64(&pt.totalRequests)
	totalResps := atomic.LoadInt64(&pt.totalResponses)
	totalErrs := atomic.LoadInt64(&pt.totalErrors)
	activeConns := atomic.LoadInt64(&pt.activeConnections)
	totalConns := atomic.LoadInt64(&pt.totalConnections)
	totalBytes := atomic.LoadInt64(&pt.totalBytes)

	qps := float64(totalReqs) / uptime.Seconds()
	throughputMBps := float64(totalBytes) / (1024 * 1024) / uptime.Seconds()

	var avgLatency, minLatency, maxLatency, p95Latency, p99Latency time.Duration
	if len(pt.requestLatencies) > 0 {
		var total time.Duration
		minLatency = pt.requestLatencies[0]
		maxLatency = pt.requestLatencies[0]

		latencies := make([]time.Duration, len(pt.requestLatencies))
		copy(latencies, pt.requestLatencies)

		for _, latency := range latencies {
			total += latency
			if latency < minLatency {
				minLatency = latency
			}
			if latency > maxLatency {
				maxLatency = latency
			}
		}

		avgLatency = total / time.Duration(len(latencies))

		// 计算P95和P99
		if len(latencies) >= 20 {
			// 简单排序计算百分位数
			for i := 0; i < len(latencies)-1; i++ {
				for j := i + 1; j < len(latencies); j++ {
					if latencies[i] > latencies[j] {
						latencies[i], latencies[j] = latencies[j], latencies[i]
					}
				}
			}

			p95Index := int(float64(len(latencies)) * 0.95)
			p99Index := int(float64(len(latencies)) * 0.99)

			if p95Index < len(latencies) {
				p95Latency = latencies[p95Index]
			}
			if p99Index < len(latencies) {
				p99Latency = latencies[p99Index]
			}
		}
	}

	// 获取详细时延统计
	detailedStats := pt.detailedLatencyTracker.GetDetailedStats()

	return map[string]interface{}{
		"uptime_seconds":     uptime.Seconds(),
		"total_connections":  totalConns,
		"active_connections": activeConns,
		"total_requests":     totalReqs,
		"total_responses":    totalResps,
		"total_errors":       totalErrs,
		"success_rate":       float64(totalResps) / float64(totalReqs) * 100,
		"qps":                qps,
		"throughput_mbps":    throughputMBps,
		"total_bytes":        totalBytes,
		"avg_latency_ms":     float64(avgLatency.Nanoseconds()) / 1e6,
		"min_latency_ms":     float64(minLatency.Nanoseconds()) / 1e6,
		"max_latency_ms":     float64(maxLatency.Nanoseconds()) / 1e6,
		"p95_latency_ms":     float64(p95Latency.Nanoseconds()) / 1e6,
		"p99_latency_ms":     float64(p99Latency.Nanoseconds()) / 1e6,
		"latency_samples":    len(pt.requestLatencies),
		"detailed_latency":   detailedStats, // 新增详细时延统计
	}
}
