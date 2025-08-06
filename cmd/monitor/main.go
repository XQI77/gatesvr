// 性能监控工具
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

type Stats struct {
	UptimeSeconds     float64 `json:"uptime_seconds"`
	TotalConnections  int64   `json:"total_connections"`
	ActiveConnections int64   `json:"active_connections"`
	TotalRequests     int64   `json:"total_requests"`
	TotalResponses    int64   `json:"total_responses"`
	TotalErrors       int64   `json:"total_errors"`
	SuccessRate       float64 `json:"success_rate"`
	QPS               float64 `json:"qps"`
	ThroughputMbps    float64 `json:"throughput_mbps"`
	TotalBytes        int64   `json:"total_bytes"`
	AvgLatencyMs      float64 `json:"avg_latency_ms"`
	MinLatencyMs      float64 `json:"min_latency_ms"`
	MaxLatencyMs      float64 `json:"max_latency_ms"`
	LatencySamples    int     `json:"latency_samples"`

	// 系统资源监控
	CpuUsagePercent    float64 `json:"cpu_usage_percent"`
	MemoryUsagePercent float64 `json:"memory_usage_percent"`
	MemoryUsageMB      int64   `json:"memory_usage_mb"`

	// 新增详细时延统计
	DetailedLatency map[string]interface{} `json:"detailed_latency"`
}

type LatencyStageStats struct {
	Count        int64   `json:"count"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
	MinLatencyMs float64 `json:"min_latency_ms"`
	MaxLatencyMs float64 `json:"max_latency_ms"`
	P95LatencyMs float64 `json:"p95_latency_ms"`
	P99LatencyMs float64 `json:"p99_latency_ms"`
}

type DetailedLatencyResponse struct {
	DetailedLatency map[string]LatencyStageStats `json:"detailed_latency"`
	Summary         map[string]interface{}       `json:"summary"`
	Timestamp       int64                        `json:"timestamp"`
}

func main() {
	var (
		serverAddr    = flag.String("server", "localhost:8080", "网关服务器HTTP地址")
		interval      = flag.Duration("interval", 5*time.Second, "监控间隔")
		continuous    = flag.Bool("continuous", false, "持续监控模式")
		outputFormat  = flag.String("format", "table", "输出格式 (table/json)")
		showDetailed  = flag.Bool("detailed", false, "显示详细时延分解")
		showBreakdown = flag.Bool("breakdown", false, "显示时延分类统计")
		showHelp      = flag.Bool("help", false, "显示帮助信息")
	)

	flag.Parse()

	if *showHelp {
		fmt.Println("网关性能监控工具")
		fmt.Println()
		fmt.Println("使用方法:")
		flag.PrintDefaults()
		fmt.Println()
		fmt.Println("示例:")
		fmt.Println("  # 单次查询")
		fmt.Println("  ./monitor -server localhost:8080")
		fmt.Println("  # 持续监控")
		fmt.Println("  ./monitor -server localhost:8080 -continuous -interval 3s")
		fmt.Println("  # JSON格式输出")
		fmt.Println("  ./monitor -server localhost:8080 -format json")
		fmt.Println("  # 显示详细时延")
		fmt.Println("  ./monitor -server localhost:8080 -detailed")
		fmt.Println("  # 显示时延分解")
		fmt.Println("  ./monitor -server localhost:8080 -breakdown")
		os.Exit(0)
	}

	if *showDetailed {
		runDetailedLatencyQuery(*serverAddr, *outputFormat)
		return
	}

	if *showBreakdown {
		runLatencyBreakdownQuery(*serverAddr, *outputFormat)
		return
	}

	if *continuous {
		runContinuousMonitor(*serverAddr, *interval, *outputFormat)
	} else {
		runSingleQuery(*serverAddr, *outputFormat)
	}
}

func runSingleQuery(serverAddr, format string) {
	stats, err := fetchStats(serverAddr)
	if err != nil {
		fmt.Printf("获取统计信息失败: %v\n", err)
		os.Exit(1)
	}

	displayStats(stats, format)
}

func runDetailedLatencyQuery(serverAddr, format string) {
	latency, err := fetchDetailedLatency(serverAddr)
	if err != nil {
		fmt.Printf("获取详细时延信息失败: %v\n", err)
		os.Exit(1)
	}

	displayDetailedLatency(latency, format)
}

func runLatencyBreakdownQuery(serverAddr, format string) {
	breakdown, err := fetchLatencyBreakdown(serverAddr)
	if err != nil {
		fmt.Printf("获取时延分解信息失败: %v\n", err)
		os.Exit(1)
	}

	displayLatencyBreakdown(breakdown, format)
}

func runContinuousMonitor(serverAddr string, interval time.Duration, format string) {
	fmt.Printf("开始监控网关服务器 %s (间隔: %v)\n", serverAddr, interval)
	fmt.Println("按 Ctrl+C 停止监控")
	fmt.Println()

	if format == "table" {
		printTableHeader()
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		stats, err := fetchStats(serverAddr)
		if err != nil {
			fmt.Printf("获取统计信息失败: %v\n", err)
			time.Sleep(interval)
			continue
		}

		if format == "table" {
			printTableRow(stats)
		} else {
			displayStats(stats, format)
			fmt.Println("---")
		}

		<-ticker.C
	}
}

func fetchStats(serverAddr string) (*Stats, error) {
	url := fmt.Sprintf("http://%s/performance", serverAddr)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("HTTP请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP错误 %d: %s", resp.StatusCode, string(body))
	}

	var stats Stats
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return nil, fmt.Errorf("解析JSON失败: %w", err)
	}

	return &stats, nil
}

func fetchDetailedLatency(serverAddr string) (*DetailedLatencyResponse, error) {
	url := fmt.Sprintf("http://%s/latency/detailed", serverAddr)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("HTTP请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP错误 %d: %s", resp.StatusCode, string(body))
	}

	var latency DetailedLatencyResponse
	if err := json.NewDecoder(resp.Body).Decode(&latency); err != nil {
		return nil, fmt.Errorf("解析JSON失败: %w", err)
	}

	return &latency, nil
}

func fetchLatencyBreakdown(serverAddr string) (map[string]interface{}, error) {
	url := fmt.Sprintf("http://%s/latency/breakdown", serverAddr)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("HTTP请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP错误 %d: %s", resp.StatusCode, string(body))
	}

	var breakdown map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&breakdown); err != nil {
		return nil, fmt.Errorf("解析JSON失败: %w", err)
	}

	return breakdown, nil
}

func displayStats(stats *Stats, format string) {
	switch format {
	case "json":
		data, _ := json.MarshalIndent(stats, "", "  ")
		fmt.Println(string(data))
	case "table":
	default:
		printDetailedStats(stats)
	}
}

func displayDetailedLatency(latency *DetailedLatencyResponse, format string) {
	switch format {
	case "json":
		data, _ := json.MarshalIndent(latency, "", "  ")
		fmt.Println(string(data))
	default:
		printDetailedLatencyStats(latency)
	}
}

func displayLatencyBreakdown(breakdown map[string]interface{}, format string) {
	switch format {
	case "json":
		data, _ := json.MarshalIndent(breakdown, "", "  ")
		fmt.Println(string(data))
	default:
		printLatencyBreakdownStats(breakdown)
	}
}

func printDetailedStats(stats *Stats) {
	fmt.Println("========================================")
	fmt.Println("         网关服务器性能监控")
	fmt.Println("========================================")

	// 基本信息
	fmt.Printf("运行时间:         %.1f 秒 (%.1f 小时)\n",
		stats.UptimeSeconds, stats.UptimeSeconds/3600)

	// 连接统计
	fmt.Printf("总连接数:         %d\n", stats.TotalConnections)
	fmt.Printf("活跃连接数:       %d\n", stats.ActiveConnections)

	// 请求统计
	fmt.Printf("总请求数:         %d\n", stats.TotalRequests)
	fmt.Printf("总响应数:         %d\n", stats.TotalResponses)
	fmt.Printf("总错误数:         %d\n", stats.TotalErrors)
	fmt.Printf("成功率:           %.2f%%\n", stats.SuccessRate)

	// 性能指标
	fmt.Printf("QPS:              %.2f 请求/秒\n", stats.QPS)
	fmt.Printf("吞吐量:           %.2f MB/s\n", stats.ThroughputMbps)
	fmt.Printf("总字节数:         %s\n", formatBytes(stats.TotalBytes))

	// 延迟统计
	if stats.LatencySamples > 0 {
		fmt.Printf("平均延迟:         %.2f ms\n", stats.AvgLatencyMs)
		fmt.Printf("最小延迟:         %.2f ms\n", stats.MinLatencyMs)
		fmt.Printf("最大延迟:         %.2f ms\n", stats.MaxLatencyMs)
		fmt.Printf("延迟样本数:       %d\n", stats.LatencySamples)
	} else {
		fmt.Printf("延迟统计:         暂无数据\n")
	}

	// 系统资源统计
	fmt.Printf("CPU使用率:        %.1f%%\n", stats.CpuUsagePercent)
	fmt.Printf("内存使用率:       %.1f%%\n", stats.MemoryUsagePercent)
	fmt.Printf("内存使用量:       %d MB\n", stats.MemoryUsageMB)

	// 显示详细时延统计 - 新增
	if stats.DetailedLatency != nil {
		fmt.Println("----------------------------------------")
		fmt.Println("         详细时延分解统计")
		fmt.Println("----------------------------------------")
		printSimpleDetailedLatency(stats.DetailedLatency)
	}

	fmt.Printf("查询时间:         %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println("========================================")
}

func printSimpleDetailedLatency(detailedLatency map[string]interface{}) {
	stages := []struct {
		key  string
		name string
	}{
		{"read_latency", "消息读取(含等待)"},
		{"process_latency", "纯处理时间"},
		{"parse_latency", "消息解析"},
		{"upstream_latency", "上游调用"},
		{"encode_latency", "响应编码"},
		{"send_latency", "消息发送"},
		{"total_latency", "总处理"},
	}

	for _, stage := range stages {
		if stageData, ok := detailedLatency[stage.key].(map[string]interface{}); ok {
			if count, ok := stageData["count"].(float64); ok && count > 0 {
				avgLatency := stageData["avg_latency_ms"].(float64)
				fmt.Printf("%-18s 平均: %.2fms, 次数: %.0f\n",
					stage.name, avgLatency, count)
			}
		}
	}
}

func printDetailedLatencyStats(latency *DetailedLatencyResponse) {
	fmt.Println("========================================")
	fmt.Println("         详细时延分解统计")
	fmt.Println("========================================")

	stages := []struct {
		key  string
		name string
	}{
		{"read_latency", "消息读取时延（包含网络等待和阻塞）"},
		{"process_latency", "消息处理时延（纯处理时间，不含读取等待）"},
		{"parse_latency", "消息解析时延"},
		{"upstream_latency", "上游调用时延"},
		{"encode_latency", "响应编码时延"},
		{"send_latency", "消息发送时延"},
		{"total_latency", "总处理时延"},
	}

	for _, stage := range stages {
		if stageStats, ok := latency.DetailedLatency[stage.key]; ok {
			fmt.Printf("\n%s:\n", stage.name)
			fmt.Printf("  处理次数:       %d\n", int64(stageStats.Count))
			fmt.Printf("  平均时延:       %.2f ms\n", stageStats.AvgLatencyMs)
			fmt.Printf("  最小时延:       %.2f ms\n", stageStats.MinLatencyMs)
			fmt.Printf("  最大时延:       %.2f ms\n", stageStats.MaxLatencyMs)
		}
	}

	fmt.Printf("\n查询时间:         %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println("========================================")
}

func printLatencyBreakdownStats(breakdown map[string]interface{}) {
	fmt.Println("========================================")
	fmt.Println("         时延分类统计")
	fmt.Println("========================================")

	if latencyBreakdown, ok := breakdown["latency_breakdown"].(map[string]interface{}); ok {
		categories := []struct {
			key  string
			name string
		}{
			{"io_processing", "IO处理"},
			{"pure_processing", "纯处理"},
			{"message_processing", "消息处理"},
			{"upstream_processing", "上游处理"},
			{"network_processing", "网络处理"},
			{"total_processing", "总体处理"},
		}

		for _, category := range categories {
			if catData, ok := latencyBreakdown[category.key].(map[string]interface{}); ok {
				fmt.Printf("\n%s:\n", category.name)
				if desc, ok := catData["description"].(string); ok {
					fmt.Printf("  说明: %s\n", desc)
				}

				// 显示该分类下的各项统计
				for key, value := range catData {
					if key != "description" && key != "name" {
						if stageData, ok := value.(map[string]interface{}); ok {
							if count, ok := stageData["count"].(float64); ok && count > 0 {
								avgLatency := stageData["avg_latency_ms"].(float64)
								fmt.Printf("    %-15s 平均: %.2fms, 次数: %.0f\n",
									key, avgLatency, count)
							}
						}
					}
				}
			}
		}
	}

	fmt.Printf("\n查询时间:         %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println("========================================")
}

func printTableHeader() {
	fmt.Printf("%-19s %8s %8s %8s %10s %8s %8s %8s %8s %8s %8s\n",
		"时间", "活跃连接", "总请求", "QPS", "成功率%", "平均延迟", "吞吐量", "错误数", "CPU%", "内存%", "内存MB")
	fmt.Println("----------------------------------------------------------------------------------------------------------------------------------------")
}

func printTableRow(stats *Stats) {
	now := time.Now().Format("15:04:05")

	fmt.Printf("%-19s %8d %8d %8.1f %9.1f%% %7.1fms %6.1fMB/s %8d %6.1f %6.1f %7d\n",
		now,
		stats.ActiveConnections,
		stats.TotalRequests,
		stats.QPS,
		stats.SuccessRate,
		stats.AvgLatencyMs,
		stats.ThroughputMbps,
		stats.TotalErrors,
		stats.CpuUsagePercent,
		stats.MemoryUsagePercent,
		stats.MemoryUsageMB)
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
