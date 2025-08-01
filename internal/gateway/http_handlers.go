// Package gateway 提供HTTP API处理功能
package gateway

import (
	"encoding/json"
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
			"session_id":      sess.ID,
			"create_time":     sess.CreateTime.Unix(),
			"last_activity":   sess.LastActivity.Unix(),
			"pending_count":   s.sessionManager.GetPendingCount(sess.ID),
			"ordered_queue":   s.orderedSender.GetQueueStats(sess),
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

// handleClientLatency 处理客户端时延查询 - 新增 (如果有客户端统计)
func (s *Server) handleClientLatency(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// 构建客户端时延响应格式（为future使用）
	response := map[string]interface{}{
		"client_latency": map[string]interface{}{
			"description":         "客户端到网关的往返时延统计",
			"note":                "此信息需要从客户端获取，当前显示服务端处理时延",
			"server_side_latency": s.performanceTracker.detailedLatencyTracker.GetDetailedStats()["total_latency"],
		},
		"timestamp": time.Now().Unix(),
	}

	json.NewEncoder(w).Encode(response)
}

// 注意：单播推送功能已移至gRPC接口 (internal/gateway/unicast_service.go)
// HTTP API已被移除，请使用gRPC客户端调用 GatewayService.PushToClient 和 GatewayService.BatchPushToClients
