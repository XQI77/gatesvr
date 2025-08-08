// Package gateway 提供HTTP API处理功能
package gateway

import (
	"encoding/json"
	"gatesvr/internal/session"
	"net/http"
	"time"
)

// handleHealth 处理健康检查
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":             "healthy",
		"active_connections": s.sessionManager.GetSessionCount(),
		"timestamp":          time.Now().Unix(),
	})
}

// handleStats 处理统计信息
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	sessions := s.sessionManager.GetAllSessions()

	stats := make([]map[string]interface{}, len(sessions))
	for i, sess := range sessions {
		sessionStats := map[string]interface{}{
			"session_id":    sess.ID,
			"create_time":   sess.CreateTime.Unix(),
			"last_activity": sess.LastActivity.Unix(),
			"pending_count": s.sessionManager.GetPendingCount(sess.ID),
			"ordered_queue": s.orderedSender.GetQueueStats(sess),
			"remote_addr":   sess.Connection.RemoteAddr().String(),
		}

		// 添加有序队列统计信息
		if queueStats := s.orderedSender.GetQueueStats(sess); queueStats != nil {
			sessionStats["ordered_queue"] = queueStats
		}

		stats[i] = sessionStats
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"total_sessions": len(sessions),
		"sessions":       stats,
		"timestamp":      time.Now().Unix(),
	})
}

// handlePerformance 处理性能监控请求
func (s *Server) handlePerformance(w http.ResponseWriter, r *http.Request) {
	stats := s.performanceTracker.GetStats()

	// 添加START处理器统计信息
	if s.startProcessor != nil {
		stats["start_processor"] = s.startProcessor.GetStats()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleQueueStatus 处理队列状态查询
func (s *Server) handleQueueStatus(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")

	if sessionID == "" {
		// 返回所有会话的队列状态
		sessions := s.sessionManager.GetAllSessions()
		queueStats := make(map[string]interface{})

		for _, sess := range sessions {
			if stats := s.orderedSender.GetQueueStats(sess); stats != nil {
				queueStats[sess.ID] = stats
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"queue_stats": queueStats,
			"timestamp":   time.Now().Unix(),
		})
		return
	}

	// 返回指定会话的队列状态
	sess, exists := s.sessionManager.GetSession(sessionID)
	if !exists {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	stats := s.orderedSender.GetQueueStats(sess)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

/*
// handleDetailedLatency 处理详细时延查询请求 - 新增
func (s *Server) handleDetailedLatency(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// 获取详细时延统计
	detailedStats := s.performanceTracker.detailedLatencyTracker.GetDetailedStats()

	// 添加汇总信息
	response := map[string]interface{}{
		"detailed_latency": detailedStats,
		"summary": map[string]interface{}{
			"description": "各处理阶段的详细时延统计",
			"stages": map[string]string{
				"read_latency":     "消息读取时延（包含网络等待和阻塞时间）",
				"process_latency":  "消息处理时延（纯处理时间，不含读取等待）",
				"parse_latency":    "消息解析时延",
				"upstream_latency": "上游服务调用时延",
				"encode_latency":   "响应编码时延",
				"send_latency":     "消息发送时延",
				"total_latency":    "总处理时延",
			},
		},
		"timestamp": time.Now().Unix(),
	}

	json.NewEncoder(w).Encode(response)
}

// handleBusinessRequestLatency 处理业务请求详细时延分析 - 新增
func (s *Server) handleBusinessRequestLatency(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	detailedStats := s.performanceTracker.detailedLatencyTracker.GetDetailedStats()

	// 构建业务请求处理时延分析
	response := map[string]interface{}{
		"business_request_latency_analysis": map[string]interface{}{
			"validation_phase": map[string]interface{}{
				"seq_validate_latency":   detailedStats["seq_validate_latency"],
				"state_validate_latency": detailedStats["state_validate_latency"],
				"description":            "请求验证阶段：序列号验证和会话状态验证",
			},
			"processing_phase": map[string]interface{}{
				"parse_latency":     detailedStats["parse_latency"],
				"build_req_latency": detailedStats["build_req_latency"],
				"description":       "请求处理阶段：消息解析和构造上游请求",
			},
			"upstream_phase": map[string]interface{}{
				"upstream_latency": detailedStats["upstream_latency"],
				"description":      "上游调用阶段：gRPC调用上游服务处理业务逻辑",
			},
			"response_phase": map[string]interface{}{
				"login_process_latency":  detailedStats["login_process_latency"],
				"send_resp_latency":      detailedStats["send_resp_latency"],
				"process_notify_latency": detailedStats["process_notify_latency"],
				"description":            "响应处理阶段：登录处理、发送响应和处理通知消息",
			},
			"total_processing": map[string]interface{}{
				"total_latency": detailedStats["total_latency"],
				"description":   "端到端业务请求处理时延",
			},
		},
		"optimization_hints": map[string]interface{}{
			"validation_bottleneck": "如果validation_phase时延过高，考虑优化序列号验证逻辑或会话状态检查",
			"processing_bottleneck": "如果processing_phase时延过高，考虑优化protobuf解析或请求构造逻辑",
			"upstream_bottleneck":   "如果upstream_phase时延过高，主要瓶颈在上游服务，需要优化上游处理逻辑",
			"response_bottleneck":   "如果response_phase时延过高，考虑优化消息发送机制或notify处理逻辑",
		},
		"performance_thresholds": map[string]interface{}{
			"good_total_latency_ms":     "< 50ms",
			"warn_total_latency_ms":     "50-200ms",
			"critical_total_latency_ms": "> 200ms",
			"expected_upstream_ratio":   "70-80% of total latency",
		},
		"timestamp": time.Now().Unix(),
	}

	json.NewEncoder(w).Encode(response)
}

// handleSendOrderedMessageLatency 处理SendOrderedMessage详细时延分析 - 新增
func (s *Server) handleSendOrderedMessageLatency(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	detailedStats := s.performanceTracker.detailedLatencyTracker.GetDetailedStats()

	// 构建SendOrderedMessage处理时延分析
	response := map[string]interface{}{
		"send_ordered_message_latency_analysis": map[string]interface{}{
			"preparation_phase": map[string]interface{}{
				"seq_alloc_latency":    detailedStats["seq_alloc_latency"],
				"msg_encode_latency":   detailedStats["msg_encode_latency"],
				"queue_get_latency":    detailedStats["queue_get_latency"],
				"callback_set_latency": detailedStats["callback_set_latency"],
				"description":          "消息准备阶段：序列号分配、消息编码、队列获取和回调设置",
			},
			"queuing_phase": map[string]interface{}{
				"enqueue_latency": detailedStats["enqueue_latency"],
				"description":     "消息入队阶段：将消息加入有序队列等待发送",
			},
			"sending_phase": map[string]interface{}{
				"direct_send_latency":    detailedStats["direct_send_latency"],
				"write_message_latency":  detailedStats["write_message_latency"],
				"metrics_update_latency": detailedStats["metrics_update_latency"],
				"description":            "消息发送阶段：直接发送、网络写入、指标更新",
			},
			"comparison_with_legacy": map[string]interface{}{
				"encode_latency": detailedStats["encode_latency"],
				"send_latency":   detailedStats["send_latency"],
				"description":    "原有统计（编码和发送时延，用于对比）",
			},
		},
		"bottleneck_analysis": map[string]interface{}{
			"preparation_bottlenecks": map[string]string{
				"seq_alloc_high":    "序列号分配时延过高：可能存在锁竞争或原子操作瓶颈",
				"msg_encode_high":   "消息编码时延过高：protobuf编码性能问题或消息过大",
				"queue_get_high":    "队列获取时延过高：队列初始化或访问锁竞争",
				"callback_set_high": "回调设置时延过高：首次设置回调函数的开销",
			},
			"queuing_bottlenecks": map[string]string{
				"enqueue_high": "入队时延过高：队列满或队列锁竞争，可能需要调整队列大小",
			},
			"sending_bottlenecks": map[string]string{
				"direct_send_high":    "直接发送时延过高：整体发送流程存在瓶颈",
				"write_message_high":  "网络IO瓶颈或连接问题，这是最可能的200ms时延根源",
				"metrics_update_high": "指标更新时延过高：指标收集系统性能问题",
			},
		},
		"optimization_recommendations": map[string]interface{}{
			"if_seq_alloc_high":      "优化序列号分配：考虑使用无锁算法或预分配序列号池",
			"if_msg_encode_high":     "优化消息编码：使用protobuf编译优化选项，考虑消息压缩",
			"if_enqueue_high":        "优化消息队列：增加队列大小，使用无锁队列实现",
			"if_write_message_high":  "优化网络写入：使用批量写入，检查网络连接质量，考虑连接池",
			"if_metrics_update_high": "优化指标更新：异步更新指标，减少同步操作",
		},
		"performance_thresholds": map[string]interface{}{
			"good_total_send_ms":     "< 10ms",
			"warn_total_send_ms":     "10-50ms",
			"critical_total_send_ms": "> 50ms",
			"expected_write_ratio":   "60-80% of total send latency",
			"expected_encode_ratio":  "10-20% of total send latency",
		},
		"timestamp": time.Now().Unix(),
	}

	json.NewEncoder(w).Encode(response)
}

*/

// handleAsyncQueueStats 处理异步队列统计信息查询 - 新增
func (s *Server) handleAsyncQueueStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// 收集所有会话的异步队列统计
	sessions := s.sessionManager.GetAllSessions()

	response := map[string]interface{}{
		"async_queue_analysis": map[string]interface{}{
			"total_sessions": len(sessions),
			"sessions_data":  make([]map[string]interface{}, 0, len(sessions)),
			"summary":        map[string]interface{}{},
		},
		"performance_comparison": map[string]interface{}{
			"async_vs_sync": "异步发送vs同步发送性能对比",
			"expected_improvements": map[string]string{
				"enqueue_latency": "从100ms降低到1-5ms",
				"lock_contention": "大幅减少锁竞争",
				"throughput":      "提升5-10倍",
			},
		},
		"timestamp": time.Now().Unix(),
	}

	// 统计汇总数据
	var totalAsyncSessions, totalSyncSessions int
	var totalEnqueued, totalSent, totalFailed, totalDropped int64
	var totalWorkers int
	var avgSuccessRate float64

	// 收集每个会话的详细统计
	sessionsData := response["async_queue_analysis"].(map[string]interface{})["sessions_data"].([]map[string]interface{})

	for _, sess := range sessions {
		orderedQueue := sess.GetOrderedQueue()
		if orderedQueue == nil {
			continue
		}

		detailedStats := orderedQueue.GetQueueStats()

		sessionData := map[string]interface{}{
			"session_id":    sess.ID,
			"create_time":   sess.CreateTime.Format("2006-01-02 15:04:05"),
			"last_activity": sess.LastActivity.Format("2006-01-02 15:04:05"),
			"queue_stats":   detailedStats,
		}

		// 检查是否启用了异步发送
		if asyncEnabled, ok := detailedStats["async_enabled"].(bool); ok && asyncEnabled {
			totalAsyncSessions++

			// 提取异步发送统计
			if asyncSend, ok := detailedStats["async_send"].(map[string]interface{}); ok {
				if sendQueueStats, ok := asyncSend["send_queue_stats"].(map[string]interface{}); ok {
					if val, ok := sendQueueStats["total_enqueued"].(int64); ok {
						totalEnqueued += val
					}
					if val, ok := sendQueueStats["total_sent"].(int64); ok {
						totalSent += val
					}
					if val, ok := sendQueueStats["total_failed"].(int64); ok {
						totalFailed += val
					}
					if val, ok := sendQueueStats["dropped_tasks"].(int64); ok {
						totalDropped += val
					}
					if val, ok := sendQueueStats["success_rate"].(float64); ok {
						avgSuccessRate += val
					}
				}

				if workerCount, ok := asyncSend["worker_count"].(int); ok {
					totalWorkers += workerCount
				}
			}
		} else {
			totalSyncSessions++
		}

		sessionsData = append(sessionsData, sessionData)
	}

	// 计算平均成功率
	if totalAsyncSessions > 0 {
		avgSuccessRate /= float64(totalAsyncSessions)
	}

	// 更新汇总统计
	response["async_queue_analysis"].(map[string]interface{})["sessions_data"] = sessionsData
	response["async_queue_analysis"].(map[string]interface{})["summary"] = map[string]interface{}{
		"async_sessions":      totalAsyncSessions,
		"sync_sessions":       totalSyncSessions,
		"total_workers":       totalWorkers,
		"total_enqueued":      totalEnqueued,
		"total_sent":          totalSent,
		"total_failed":        totalFailed,
		"total_dropped":       totalDropped,
		"avg_success_rate":    avgSuccessRate,
		"async_adoption_rate": float64(totalAsyncSessions) / float64(len(sessions)) * 100,
	}

	json.NewEncoder(w).Encode(response)
}

/*
// handleQueueOptimizationAnalysis 处理队列优化分析 - 新增
func (s *Server) handleQueueOptimizationAnalysis(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// 获取详细的队列性能分析
	detailedStats := s.performanceTracker.detailedLatencyTracker.GetDetailedStats()

	response := map[string]interface{}{
		"queue_optimization_analysis": map[string]interface{}{
			"problem_identification": map[string]interface{}{
				"original_bottleneck": "enqueue_latency达到100ms，主要由同步发送和锁竞争导致",
				"root_cause":          "在持锁状态下执行网络IO操作",
				"impact":              "1000并发客户端被强制串行化处理",
			},
			"solution_implemented": map[string]interface{}{
				"approach": "生产者-消费者异步发送模式",
				"key_changes": []string{
					"将入队和发送操作解耦",
					"使用工作协程池处理发送任务",
					"最小化锁持有时间",
					"支持优雅降级到同步模式",
				},
			},
			"performance_metrics": map[string]interface{}{
				"current_latencies": map[string]interface{}{
					"enqueue_latency":        detailedStats["enqueue_latency"],
					"seq_alloc_latency":      detailedStats["seq_alloc_latency"],
					"msg_encode_latency":     detailedStats["msg_encode_latency"],
					"queue_get_latency":      detailedStats["queue_get_latency"],
					"callback_set_latency":   detailedStats["callback_set_latency"],
					"direct_send_latency":    detailedStats["direct_send_latency"],
					"write_message_latency":  detailedStats["write_message_latency"],
					"metrics_update_latency": detailedStats["metrics_update_latency"],
				},
				"expected_improvements": map[string]string{
					"enqueue_latency":       "100ms → 1-5ms (20-100倍改进)",
					"lock_contention":       "严重竞争 → 最小竞争 (95%减少)",
					"concurrent_processing": "串行 → 并行 (1000倍改进)",
					"overall_throughput":    "当前 → 5-10倍提升",
				},
			},
			"monitoring_recommendations": []string{
				"监控 enqueue_latency 应显著降低",
				"观察 async_send_queue 工作协程利用率",
				"检查 dropped_tasks 确保队列容量足够",
				"对比 async_enabled=true vs false 的会话性能",
			},
		},
		"next_optimization_steps": map[string]interface{}{
			"if_still_slow": []string{
				"检查网络IO性能 (write_message_latency)",
				"调整工作协程数量",
				"实现批量发送减少网络调用",
				"考虑消息压缩降低传输时间",
			},
			"advanced_optimizations": []string{
				"实现无锁队列进一步减少竞争",
				"使用内存池减少GC压力",
				"实现自适应队列大小",
				"添加背压控制机制",
			},
		},
		"timestamp": time.Now().Unix(),
	}

	json.NewEncoder(w).Encode(response)
}
*/

// handleQueueConfig 处理队列配置查询和设置 - 新增
func (s *Server) handleQueueConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case "GET":
		s.handleGetQueueConfig(w, r)
	case "POST":
		s.handleSetQueueConfig(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleGetQueueConfig 获取当前队列配置
func (s *Server) handleGetQueueConfig(w http.ResponseWriter, r *http.Request) {
	// 获取当前环境变量配置
	config := session.LoadQueueConfigFromEnv()

	response := map[string]interface{}{
		"queue_configuration": map[string]interface{}{
			"current_config": config,
			"environment_variables": map[string]string{
				"QUEUE_ENABLE_ASYNC_SEND":   "启用/禁用异步发送 (true/false)",
				"QUEUE_SEND_WORKER_COUNT":   "发送工作协程数量 (1-100)",
				"QUEUE_SEND_QUEUE_SIZE":     "发送队列大小 (100-10000)",
				"QUEUE_MAX_QUEUE_SIZE":      "最大队列大小 (100-10000)",
				"QUEUE_BATCH_TIMEOUT_MS":    "批量发送超时毫秒数 (1-1000)",
				"QUEUE_MAX_RETRIES":         "最大重试次数 (0-10)",
				"QUEUE_CLEANUP_INTERVAL_MS": "清理间隔毫秒数 (1000-60000)",
				"QUEUE_ENABLE_DEBUG_LOG":    "启用调试日志 (true/false)",
				"QUEUE_ENABLE_METRICS":      "启用指标监控 (true/false)",
			},
			"configuration_examples": map[string]interface{}{
				"high_performance": map[string]string{
					"QUEUE_ENABLE_ASYNC_SEND": "true",
					"QUEUE_SEND_WORKER_COUNT": "8",
					"QUEUE_SEND_QUEUE_SIZE":   "2000",
					"QUEUE_MAX_QUEUE_SIZE":    "2000",
					"description":             "高性能配置，适用于大并发场景",
				},
				"conservative": map[string]string{
					"QUEUE_ENABLE_ASYNC_SEND": "true",
					"QUEUE_SEND_WORKER_COUNT": "2",
					"QUEUE_SEND_QUEUE_SIZE":   "500",
					"QUEUE_MAX_QUEUE_SIZE":    "500",
					"description":             "保守配置，适用于内存受限环境",
				},
				"sync_fallback": map[string]string{
					"QUEUE_ENABLE_ASYNC_SEND": "false",
					"description":             "同步模式，用于调试或兼容性",
				},
			},
		},
		"current_sessions_status": s.getSessionsAsyncStatus(),
		"timestamp":               time.Now().Unix(),
	}

	json.NewEncoder(w).Encode(response)
}

// handleSetQueueConfig 设置队列配置（仅影响新创建的会话）
func (s *Server) handleSetQueueConfig(w http.ResponseWriter, r *http.Request) {
	var configUpdate map[string]interface{}

	if err := json.NewDecoder(r.Body).Decode(&configUpdate); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	response := map[string]interface{}{
		"message":         "配置更新请求已接收",
		"note":            "环境变量更改需要重启服务才能生效，或仅影响新创建的会话",
		"received_config": configUpdate,
		"recommendations": []string{
			"生产环境建议使用环境变量而非API设置配置",
			"配置更改后监控 /queue/async-stats 确认效果",
			"如需立即生效，请重启服务",
		},
		"timestamp": time.Now().Unix(),
	}

	json.NewEncoder(w).Encode(response)
}

// getSessionsAsyncStatus 获取当前会话的异步发送状态
func (s *Server) getSessionsAsyncStatus() map[string]interface{} {
	sessions := s.sessionManager.GetAllSessions()

	var asyncCount, syncCount int

	for _, sess := range sessions {
		if orderedQueue := sess.GetOrderedQueue(); orderedQueue != nil {
			stats := orderedQueue.GetQueueStats()
			if asyncEnabled, ok := stats["async_enabled"].(bool); ok && asyncEnabled {
				asyncCount++
			} else {
				syncCount++
			}
		}
	}

	return map[string]interface{}{
		"total_sessions":      len(sessions),
		"async_enabled_count": asyncCount,
		"sync_enabled_count":  syncCount,
		"async_adoption_rate": func() float64 {
			if len(sessions) == 0 {
				return 0
			}
			return float64(asyncCount) / float64(len(sessions)) * 100
		}(),
	}
}

/*

// handleLatencyBreakdown 处理时延分解查询 - 新增
func (s *Server) handleLatencyBreakdown(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	detailedStats := s.performanceTracker.detailedLatencyTracker.GetDetailedStats()

	// 构建分解统计
	breakdown := map[string]interface{}{
		"latency_breakdown": map[string]interface{}{
			"io_processing": map[string]interface{}{
				"read_latency": detailedStats["read_latency"],
				"description":  "IO读取时延（包含网络等待和阻塞时间）",
			},
			"pure_processing": map[string]interface{}{
				"process_latency": detailedStats["process_latency"],
				"description":     "纯消息处理时延（不含IO等待）",
			},
			"message_processing": map[string]interface{}{
				"parse_latency":  detailedStats["parse_latency"],
				"encode_latency": detailedStats["encode_latency"],
				"description":    "消息解析和编码时延",
			},
			"upstream_processing": map[string]interface{}{
				"upstream_latency": detailedStats["upstream_latency"],
				"description":      "上游服务处理时延",
			},
			"network_processing": map[string]interface{}{
				"send_latency": detailedStats["send_latency"],
				"description":  "网络发送时延",
			},
			"total_processing": map[string]interface{}{
				"total_latency": detailedStats["total_latency"],
				"description":   "端到端处理时延",
			},
		},
		"timestamp": time.Now().Unix(),
	}

	json.NewEncoder(w).Encode(breakdown)
}
*/
// handleClientLatency 处理客户端时延查询 - 新增 (如果有客户端统计)
func (s *Server) handleClientLatency(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// 构建客户端时延响应格式（为future使用）
	response := map[string]interface{}{
		"client_latency": map[string]interface{}{
			"description": "客户端到网关的往返时延统计",
			"note":        "此信息需要从客户端获取，当前显示服务端处理时延",
			//"server_side_latency": s.performanceTracker.detailedLatencyTracker.GetDetailedStats()["total_latency"],
		},
		"timestamp": time.Now().Unix(),
	}

	json.NewEncoder(w).Encode(response)
}

// handleStartProcessor 处理START消息处理器统计查询
func (s *Server) handleStartProcessor(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if s.startProcessor == nil {
		response := map[string]interface{}{
			"error":       "START异步处理器未启用",
			"description": "系统使用同步处理模式",
			"timestamp":   time.Now().Unix(),
		}
		json.NewEncoder(w).Encode(response)
		return
	}

	stats := s.startProcessor.GetStats()

	// 添加详细描述信息
	response := map[string]interface{}{
		"start_processor_stats": stats,
		"description": map[string]string{
			"max_workers":         "最大工作线程数",
			"queue_size":          "任务队列大小",
			"queue_length":        "当前队列长度",
			"available_workers":   "可用工作线程数",
			"total_tasks":         "总任务数",
			"success_tasks":       "成功任务数",
			"failed_tasks":        "失败任务数",
			"timeout_tasks":       "超时任务数",
			"queue_full_tasks":    "队列满拒绝任务数",
			"success_rate":        "成功率（%）",
			"avg_process_time_ms": "平均处理时间（毫秒）",
		},
		"health_indicators": map[string]interface{}{
			"is_healthy":     stats["success_rate"].(float64) > 95.0 && stats["queue_length"].(float64) < stats["queue_size"].(float64)*0.8,
			"queue_pressure": float64(stats["queue_length"].(int)) / float64(stats["queue_size"].(int)) * 100,
			"worker_usage":   float64(stats["max_workers"].(int)-stats["available_workers"].(int)) / float64(stats["max_workers"].(int)) * 100,
		},
		"timestamp": time.Now().Unix(),
	}

	json.NewEncoder(w).Encode(response)
}

// handleOverloadProtection 处理过载保护状态查询 - 新增
func (s *Server) handleOverloadProtection(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if s.overloadProtector == nil {
		response := map[string]interface{}{
			"error":       "过载保护器未初始化",
			"description": "系统未启用过载保护功能",
			"timestamp":   time.Now().Unix(),
		}
		json.NewEncoder(w).Encode(response)
		return
	}

	stats := s.overloadProtector.GetStats()

	// 添加详细状态分析
	response := map[string]interface{}{
		"overload_protection_status": stats,
		"protection_levels": map[string]interface{}{
			"connection_level": map[string]interface{}{
				"current_connections":      stats["current_connections"],
				"max_connections":          stats["max_connections"],
				"connection_usage_percent": float64(stats["current_connections"].(int64)) / float64(stats["max_connections"].(int)) * 100,
				"connections_rejected":     stats["connections_rejected"],
				"status": func() string {
					usage := float64(stats["current_connections"].(int64)) / float64(stats["max_connections"].(int)) * 100
					if usage > 95 {
						return "critical"
					} else if usage > 80 {
						return "warning"
					}
					return "normal"
				}(),
			},
			"qps_level": map[string]interface{}{
				"current_qps": stats["current_qps"],
				"max_qps":     stats["max_qps"],
				"qps_usage_percent": func() float64 {
					if stats["max_qps"].(int) == 0 {
						return 0
					}
					return stats["current_qps"].(float64) / float64(stats["max_qps"].(int)) * 100
				}(),
				"requests_rejected": stats["requests_rejected"],
				"status": func() string {
					if stats["max_qps"].(int) == 0 {
						return "disabled"
					}
					usage := stats["current_qps"].(float64) / float64(stats["max_qps"].(int)) * 100
					if usage > 95 {
						return "critical"
					} else if usage > 80 {
						return "warning"
					}
					return "normal"
				}(),
			},
			"upstream_level": map[string]interface{}{
				"upstream_active":        stats["upstream_active"],
				"max_upstream":           stats["max_upstream"],
				"upstream_usage_percent": float64(stats["upstream_active"].(int64)) / float64(stats["max_upstream"].(int)) * 100,
				"upstream_rejected":      stats["upstream_rejected"],
				"status": func() string {
					usage := float64(stats["upstream_active"].(int64)) / float64(stats["max_upstream"].(int)) * 100
					if usage > 95 {
						return "critical"
					} else if usage > 80 {
						return "warning"
					}
					return "normal"
				}(),
			},
		},
		"overall_status": func() string {
			connectionUsage := float64(stats["current_connections"].(int64)) / float64(stats["max_connections"].(int)) * 100
			var qpsUsage float64
			if stats["max_qps"].(int) > 0 {
				qpsUsage = stats["current_qps"].(float64) / float64(stats["max_qps"].(int)) * 100
			}
			upstreamUsage := float64(stats["upstream_active"].(int64)) / float64(stats["max_upstream"].(int)) * 100

			maxUsage := connectionUsage
			if qpsUsage > maxUsage {
				maxUsage = qpsUsage
			}
			if upstreamUsage > maxUsage {
				maxUsage = upstreamUsage
			}

			if maxUsage > 95 {
				return "critical"
			} else if maxUsage > 80 {
				return "warning"
			}
			return "healthy"
		}(),
		"recommendations": func() []string {
			var recommendations []string
			connectionUsage := float64(stats["current_connections"].(int64)) / float64(stats["max_connections"].(int)) * 100
			var qpsUsage float64
			if stats["max_qps"].(int) > 0 {
				qpsUsage = stats["current_qps"].(float64) / float64(stats["max_qps"].(int)) * 100
			}
			upstreamUsage := float64(stats["upstream_active"].(int64)) / float64(stats["max_upstream"].(int)) * 100

			if connectionUsage > 90 {
				recommendations = append(recommendations, "连接数接近上限，考虑增加最大连接数或优化连接管理")
			}
			if qpsUsage > 90 {
				recommendations = append(recommendations, "QPS接近上限，考虑增加最大QPS或优化请求处理性能")
			}
			if upstreamUsage > 90 {
				recommendations = append(recommendations, "上游并发接近上限，考虑增加上游并发数或优化上游服务性能")
			}
			if stats["connections_rejected"].(int64) > 0 {
				recommendations = append(recommendations, "有连接被拒绝，考虑调整连接保护策略")
			}
			if stats["requests_rejected"].(int64) > 0 {
				recommendations = append(recommendations, "有请求被拒绝，考虑调整QPS保护策略")
			}
			if stats["upstream_rejected"].(int64) > 0 {
				recommendations = append(recommendations, "有上游请求被拒绝，考虑调整上游保护策略")
			}
			if len(recommendations) == 0 {
				recommendations = append(recommendations, "过载保护运行正常，无需调整")
			}
			return recommendations
		}(),
		"configuration": map[string]interface{}{
			"enabled":                      stats["enabled"],
			"uptime_seconds":               stats["uptime_seconds"],
			"connection_warning_threshold": "80% of max_connections",
			"qps_warning_threshold":        "80% of max_qps",
			"upstream_warning_threshold":   "80% of max_upstream",
		},
		"timestamp": time.Now().Unix(),
	}

	json.NewEncoder(w).Encode(response)
}
