// 客户端启动程序
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"gatesvr/internal/client"
)

// PerformanceStats 性能统计
type PerformanceStats struct {
	totalRequests     int64
	successRequests   int64
	failedRequests    int64
	totalResponseTime int64
	startTime         time.Time
	mutex             sync.RWMutex
}

func (s *PerformanceStats) AddRequest(success bool, responseTime time.Duration) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	atomic.AddInt64(&s.totalRequests, 1)
	if success {
		atomic.AddInt64(&s.successRequests, 1)
	} else {
		atomic.AddInt64(&s.failedRequests, 1)
	}
	atomic.AddInt64(&s.totalResponseTime, responseTime.Nanoseconds())
}

func (s *PerformanceStats) GetStats() map[string]interface{} {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	total := atomic.LoadInt64(&s.totalRequests)
	success := atomic.LoadInt64(&s.successRequests)
	failed := atomic.LoadInt64(&s.failedRequests)
	totalTime := atomic.LoadInt64(&s.totalResponseTime)

	elapsed := time.Since(s.startTime)

	var avgResponseTime float64
	var qps float64

	if total > 0 {
		avgResponseTime = float64(totalTime) / float64(total) / 1e6 // 转换为毫秒
		qps = float64(total) / elapsed.Seconds()
	}

	return map[string]interface{}{
		"total_requests":    total,
		"success_requests":  success,
		"failed_requests":   failed,
		"success_rate":      float64(success) / float64(total) * 100,
		"avg_response_time": avgResponseTime,
		"qps":               qps,
		"elapsed_seconds":   elapsed.Seconds(),
	}
}

func main() {
	// 命令行参数
	var (
		serverAddr        = flag.String("server", "localhost:8443", "网关服务器地址")
		clientID          = flag.String("id", "", "客户端ID (空则自动生成)")
		openID            = flag.String("openid", "", "客户端OpenID (用户唯一标识)")
		skipTLSVerify     = flag.Bool("skip-tls-verify", true, "跳过TLS证书验证")
		connectTimeout    = flag.Duration("connect-timeout", 10*time.Second, "连接超时时间")
		heartbeatInterval = flag.Duration("heartbeat", 30*time.Second, "心跳间隔 (0禁用)")

		// 性能测试参数
		performanceMode = flag.Bool("performance", false, "性能测试模式")
		requestInterval = flag.Duration("request-interval", time.Second, "请求发送间隔")
		multipleClients = flag.Int("clients", 1, "并发客户端数量")
		maxRequests     = flag.Int("max-requests", 0, "最大请求数量 (0表示无限)")
		enableReconnect = flag.Bool("enable-reconnect", false, "性能模式下启用重连测试 (默认关闭)")

		// 测试参数
		interactive  = flag.Bool("interactive", false, "交互模式")
		testAction   = flag.String("test", "", "测试动作 (echo,time,hello,calculate,status)")
		testData     = flag.String("data", "Hello from client", "测试数据")
		testCount    = flag.Int("count", 1, "测试次数")
		testInterval = flag.Duration("interval", time.Second, "测试间隔")

		showVersion = flag.Bool("version", false, "显示版本信息")
		showHelp    = flag.Bool("help", false, "显示帮助信息")
	)

	flag.Parse()

	if *showVersion {
		fmt.Println("网关客户端 v1.0.0")
		fmt.Println("构建时间:", getBuildTime())
		os.Exit(0)
	}

	if *showHelp {
		fmt.Println("网关客户端 - QUIC客户端工具")
		fmt.Println()
		fmt.Println("使用方法:")
		flag.PrintDefaults()
		fmt.Println()
		fmt.Println("示例:")
		fmt.Println("  # 单客户端测试")
		fmt.Println("  ./client -server localhost:8443 -openid 10001 -test echo -data \"Hello World\"")
		fmt.Println("  # 交互模式")
		fmt.Println("  ./client -server localhost:8443 -openid 10005 -interactive")
		fmt.Println("  # 性能测试模式 - 5个并发客户端")
		fmt.Println("  ./client -server localhost:8443 -performance -clients 5 -request-interval 500ms")
		fmt.Println("  # 性能测试(启用重连测试)")
		fmt.Println("  ./client -server localhost:8443 -performance -clients 3 -enable-reconnect")
		fmt.Println("  # 持续测试模式")
		fmt.Println("  ./client -server localhost:8443 -openid 10010 -performance -max-requests 1000")
		fmt.Println()
		fmt.Println("注意:")
		fmt.Println("  OpenID必须是5位数字格式(10000-99999)，如未指定将自动生成")
		os.Exit(0)
	}

	// 从环境变量读取配置
	if addr := os.Getenv("CLIENT_SERVER_ADDR"); addr != "" {
		*serverAddr = addr
	}
	if id := os.Getenv("CLIENT_ID"); id != "" {
		*clientID = id
	}
	if openid := os.Getenv("CLIENT_OPENID"); openid != "" {
		*openID = openid
	}
	if skip := os.Getenv("CLIENT_SKIP_TLS_VERIFY"); skip != "" {
		if val, err := strconv.ParseBool(skip); err == nil {
			*skipTLSVerify = val
		}
	}

	// 设置默认值 - 生成5位数字OpenID
	if *openID == "" {
		// 生成10000-99999范围内的随机OpenID，支持更多并发连接
		randomNum := 10000 + (time.Now().UnixNano() % 90000)
		*openID = fmt.Sprintf("%05d", randomNum)
		log.Printf("未指定OpenID，自动生成5位数字OpenID: %s", *openID)
	}

	// 创建上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 设置中断信号处理
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	if *performanceMode {
		// 性能测试模式
		runPerformanceTest(ctx, *serverAddr, *openID, *skipTLSVerify, *connectTimeout, *heartbeatInterval,
			*multipleClients, *requestInterval, *maxRequests, *enableReconnect, sigCh)
	} else if *interactive {
		// 交互模式
		runSingleClient(ctx, *serverAddr, *clientID, *openID, *skipTLSVerify, *connectTimeout, *heartbeatInterval, true, sigCh)
	} else if *testAction != "" {
		// 测试模式
		runTestMode(ctx, *serverAddr, *clientID, *openID, *skipTLSVerify, *connectTimeout, *heartbeatInterval,
			*testAction, *testData, *testCount, *testInterval, sigCh)
	} else {
		// 默认模式：长连接持续测试
		runSingleClient(ctx, *serverAddr, *clientID, *openID, *skipTLSVerify, *connectTimeout, *heartbeatInterval, false, sigCh)
	}
}

// runPerformanceTest 运行性能测试
func runPerformanceTest(ctx context.Context, serverAddr, openID string, skipTLSVerify bool,
	connectTimeout, heartbeatInterval time.Duration, clientCount int, requestInterval time.Duration,
	maxRequests int, enableReconnect bool, sigCh <-chan os.Signal) {

	fmt.Println("========================================")
	fmt.Println("         性能测试模式")
	fmt.Println("========================================")
	fmt.Printf("服务器地址: %s\n", serverAddr)
	fmt.Printf("并发客户端: %d\n", clientCount)
	fmt.Printf("请求间隔:   %v\n", requestInterval)
	fmt.Printf("最大请求:   %d (0=无限)\n", maxRequests)
	fmt.Printf("重连测试:   %v\n", enableReconnect)
	fmt.Println("========================================")

	var wg sync.WaitGroup
	globalStats := &PerformanceStats{startTime: time.Now()}

	// 启动性能统计显示
	go showPerformanceStats(globalStats, sigCh)

	// 启动多个客户端
	for i := 0; i < clientCount; i++ {
		wg.Add(1)
		go func(clientIndex int) {
			defer wg.Done()
			runPerformanceClient(ctx, clientIndex, serverAddr, openID, skipTLSVerify, connectTimeout,
				heartbeatInterval, requestInterval, maxRequests, enableReconnect, globalStats, sigCh)
		}(i)

		// 错开启动时间
		time.Sleep(100 * time.Millisecond)
	}

	// 等待中断信号
	<-sigCh
	cancel := ctx.Value("cancel")
	if cancel != nil {
		cancel.(context.CancelFunc)()
	}
	fmt.Println("\n收到停止信号，正在关闭所有客户端...")
	wg.Wait()

	// 显示最终统计
	finalStats := globalStats.GetStats()
	fmt.Println("\n========================================")
	fmt.Println("         最终性能统计")
	fmt.Println("========================================")
	data, _ := json.MarshalIndent(finalStats, "", "  ")
	fmt.Println(string(data))
}

// runPerformanceClient 运行单个性能测试客户端
func runPerformanceClient(ctx context.Context, clientIndex int, serverAddr, openID string,
	skipTLSVerify bool, connectTimeout, heartbeatInterval, requestInterval time.Duration,
	maxRequests int, enableReconnect bool, stats *PerformanceStats, sigCh <-chan os.Signal) {

	clientID := fmt.Sprintf("perf-client-%d", clientIndex)
	// 为性能测试生成不同的有效OpenID，支持更大范围避免冲突
	baseNum := 10000 + clientIndex
	performanceOpenID := fmt.Sprintf("%05d", baseNum)

	config := &client.Config{
		ServerAddr:        serverAddr,
		ClientID:          clientID,
		OpenID:            performanceOpenID,
		TLSSkipVerify:     skipTLSVerify,
		ConnectTimeout:    connectTimeout,
		HeartbeatInterval: heartbeatInterval,
	}

	c := client.NewClient(config)

	// 连接到服务器
	if err := c.Connect(ctx); err != nil {
		log.Printf("客户端 %d 连接失败: %v", clientIndex, err)
		return
	}
	defer c.Disconnect()

	log.Printf("客户端 %d 已连接", clientIndex)

	// 首次发送hello消息进行登录
	log.Printf("客户端 %d 正在进行登录...", clientIndex)
	loginSuccess := sendLoginRequest(c, clientIndex)
	if !loginSuccess {
		log.Printf("客户端 %d 登录失败，退出", clientIndex)
		return
	}
	log.Printf("客户端 %d 登录成功", clientIndex)

	// Performance test main loop
	runPerformanceLoop(ctx, c, clientIndex, requestInterval, maxRequests, enableReconnect, stats, sigCh)
}

// runPerformanceLoop 运行性能测试主循环
func runPerformanceLoop(ctx context.Context, c *client.Client, clientIndex int,
	requestInterval time.Duration, maxRequests int, enableReconnect bool, stats *PerformanceStats, sigCh <-chan os.Signal) {

	// 扩展的测试动作列表
	actions := []string{
		"echo", "time", "calculate", "status", "session_info",
		"business_msg", "heartbeat", "data_sync", "notification",
	}
	requestCount := 0
	reconnectCount := 0

	ticker := time.NewTicker(requestInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-sigCh:
			return
		case <-ticker.C:
			if maxRequests > 0 && requestCount >= maxRequests {
				log.Printf("客户端 %d 达到最大请求数 %d", clientIndex, maxRequests)
				return
			}

			// 只有在启用重连测试时才进行 (默认关闭)
			if enableReconnect && rand.Intn(100) < 10 && reconnectCount < 3 {
				log.Printf("客户端 %d 执行短线重连测试", clientIndex)
				if performReconnectTest(ctx, c, clientIndex, stats) {
					reconnectCount++
					log.Printf("客户端 %d 重连成功", clientIndex)
				} else {
					log.Printf("客户端 %d 重连失败", clientIndex)
					return
				}
				continue
			}

			// 随机选择一个动作
			action := actions[rand.Intn(len(actions))]

			startTime := time.Now()
			success := sendRandomRequest(c, action, clientIndex, requestCount)
			responseTime := time.Since(startTime)

			stats.AddRequest(success, responseTime)
			requestCount++
		}
	}
}

// sendRandomRequest 发送随机请求
func sendRandomRequest(c *client.Client, action string, clientIndex, requestCount int) bool {
	var params map[string]string
	var data []byte

	switch action {
	case "echo":
		data = []byte(fmt.Sprintf("Hello from client %d - request %d", clientIndex, requestCount))

	case "time":
		// 获取服务器时间
		params = map[string]string{"format": "2006-01-02 15:04:05"}

	case "calculate":
		operations := []string{"add", "subtract", "multiply", "divide"}
		op := operations[rand.Intn(len(operations))]
		a := rand.Intn(100) + 1
		b := rand.Intn(100) + 1
		if op == "divide" && b == 0 {
			b = 1
		}
		params = map[string]string{
			"operation": op,
			"a":         fmt.Sprintf("%d", a),
			"b":         fmt.Sprintf("%d", b),
		}

	case "status":
		// 获取服务器状态
		params = map[string]string{"type": "system"}

	case "session_info":
		// 获取会话信息
		params = map[string]string{"detail": "true"}

	case "business_msg":
		// 发送业务消息
		params = map[string]string{
			"type":     "test",
			"content":  fmt.Sprintf("Business message from client %d", clientIndex),
			"priority": fmt.Sprintf("%d", rand.Intn(5)+1),
		}
		data = []byte(fmt.Sprintf("Business data %d-%d", clientIndex, requestCount))

	case "heartbeat":
		// 手动心跳测试
		params = map[string]string{
			"client_time": fmt.Sprintf("%d", time.Now().Unix()),
			"sequence":    fmt.Sprintf("%d", requestCount),
		}

	case "data_sync":
		// 数据同步测试
		params = map[string]string{
			"sync_type": "incremental",
			"last_sync": fmt.Sprintf("%d", time.Now().Add(-time.Hour).Unix()),
		}
		data = []byte(fmt.Sprintf(`{"client_id":%d,"data":"sync_test_%d"}`, clientIndex, requestCount))

	case "notification":
		// 通知测试
		params = map[string]string{
			"notify_type": "test",
			"target":      "all",
			"message":     fmt.Sprintf("Test notification from client %d", clientIndex),
		}

	default:
		// 默认发送echo
		data = []byte(fmt.Sprintf("Unknown action %s from client %d - request %d", action, clientIndex, requestCount))
		action = "echo"
	}

	_, err := c.SendBusinessRequestWithTimeout(action, params, data, 5*time.Second)
	return err == nil
}

// showPerformanceStats 显示性能统计
func showPerformanceStats(stats *PerformanceStats, sigCh <-chan os.Signal) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-sigCh:
			return
		case <-ticker.C:
			currentStats := stats.GetStats()
			fmt.Printf("\n[性能统计] QPS: %.2f, 成功率: %.1f%%, 平均响应时间: %.2fms, 总请求: %d\n",
				currentStats["qps"],
				currentStats["success_rate"],
				currentStats["avg_response_time"],
				currentStats["total_requests"])
		}
	}
}

// runSingleClient 运行单个客户端
func runSingleClient(ctx context.Context, serverAddr, clientID, openID string, skipTLSVerify bool,
	connectTimeout, heartbeatInterval time.Duration, interactive bool, sigCh <-chan os.Signal) {

	config := &client.Config{
		ServerAddr:        serverAddr,
		ClientID:          clientID,
		OpenID:            openID,
		TLSSkipVerify:     skipTLSVerify,
		ConnectTimeout:    connectTimeout,
		HeartbeatInterval: heartbeatInterval,
	}

	// 显示启动信息
	printStartupInfo(config)

	// 创建客户端
	c := client.NewClient(config)

	// 连接到服务器
	log.Printf("正在连接到服务器...")
	if err := c.Connect(ctx); err != nil {
		log.Fatalf("连接服务器失败: %v", err)
	}
	defer c.Disconnect()

	log.Printf("已连接到服务器，客户端ID: %s", config.ClientID)

	if interactive {
		// 交互模式
		runInteractiveMode(c, sigCh)
	} else {
		// 持续测试模式
		runContinuousTest(c, sigCh)
	}

	log.Printf("正在断开连接...")
}

// runContinuousTest 运行持续测试
func runContinuousTest(c *client.Client, sigCh <-chan os.Signal) {
	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("           持续测试模式")
	fmt.Println("========================================")
	fmt.Println("每秒发送一个随机请求，按Ctrl+C停止")
	fmt.Println("========================================")

	actions := []string{"echo", "time", "hello", "calculate", "status", "session_info"}
	requestCount := 0

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-sigCh:
			fmt.Println("\n收到中断信号")
			return
		case <-ticker.C:
			action := actions[rand.Intn(len(actions))]

			var params map[string]string
			var data []byte

			switch action {
			case "echo":
				data = []byte(fmt.Sprintf("持续测试消息 #%d", requestCount))
			case "hello":
				params = map[string]string{"name": fmt.Sprintf("用户%d", requestCount)}
			case "calculate":
				params = map[string]string{
					"operation": "add",
					"a":         fmt.Sprintf("%d", rand.Intn(100)),
					"b":         fmt.Sprintf("%d", rand.Intn(100)),
				}
			}

			startTime := time.Now()
			resp, err := c.SendBusinessRequestWithTimeout(action, params, data, 5*time.Second)
			duration := time.Since(startTime)

			if err != nil {
				fmt.Printf("[%03d] ❌ %s 请求失败: %v (耗时: %v)\n", requestCount, action, err, duration)
			} else {
				fmt.Printf("[%03d] ✅ %s 请求成功: [%d] %s (耗时: %v)\n",
					requestCount, action, resp.Code, resp.Message, duration)
			}

			requestCount++
		}
	}
}

// runTestMode 运行测试模式
func runTestMode(ctx context.Context, serverAddr, clientID, openID string, skipTLSVerify bool,
	connectTimeout, heartbeatInterval time.Duration, action, data string, count int,
	interval time.Duration, sigCh <-chan os.Signal) {

	config := &client.Config{
		ServerAddr:        serverAddr,
		ClientID:          clientID,
		OpenID:            openID,
		TLSSkipVerify:     skipTLSVerify,
		ConnectTimeout:    connectTimeout,
		HeartbeatInterval: heartbeatInterval,
	}

	c := client.NewClient(config)

	if err := c.Connect(ctx); err != nil {
		log.Fatalf("连接服务器失败: %v", err)
	}
	defer c.Disconnect()

	// 首先发送hello消息进行登录
	log.Printf("正在进行登录...")
	loginParams := map[string]string{
		"name":      "测试客户端",
		"version":   "1.0",
		"timestamp": fmt.Sprintf("%d", time.Now().Unix()),
	}
	
	loginResp, err := c.SendBusinessRequestWithTimeout("hello", loginParams, nil, 10*time.Second)
	if err != nil {
		log.Fatalf("登录失败: %v", err)
	}
	log.Printf("登录成功: [%d] %s", loginResp.Code, loginResp.Message)

	log.Printf("开始测试 - 动作: %s, 次数: %d, 间隔: %v", action, count, interval)

	successCount := 0
	errorCount := 0

	for i := 0; i < count; i++ {
		select {
		case <-sigCh:
			log.Printf("收到中断信号，停止测试")
			goto summary
		default:
		}

		log.Printf("测试 %d/%d", i+1, count)

		var params map[string]string
		var requestData []byte

		switch action {
		case "echo":
			requestData = []byte(data)
		case "hello":
			params = map[string]string{"name": "测试客户端"}
		case "calculate":
			var calcParams map[string]string
			if err := json.Unmarshal([]byte(data), &calcParams); err != nil {
				params = map[string]string{
					"operation": "add",
					"a":         "10",
					"b":         "20",
				}
			} else {
				params = calcParams
			}
		}

		resp, err := c.SendBusinessRequest(action, params, requestData)
		if err != nil {
			log.Printf("请求失败: %v", err)
			errorCount++
		} else {
			log.Printf("响应: [%d] %s - %s", resp.Code, resp.Message, string(resp.Data))
			successCount++
		}

		if i < count-1 && interval > 0 {
			time.Sleep(interval)
		}
	}

summary:
	log.Printf("测试完成 - 成功: %d, 失败: %d", successCount, errorCount)
}

// runInteractiveMode 运行交互模式
func runInteractiveMode(c *client.Client, sigCh <-chan os.Signal) {
	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("           交互模式")
	fmt.Println("========================================")
	fmt.Println("可用命令:")
	fmt.Println("  echo <message>           - 回显消息")
	fmt.Println("  time                     - 获取时间")
	fmt.Println("  hello [name]             - 问候")
	fmt.Println("  calc <op> <a> <b>        - 计算 (add/sub/mul/div)")
	fmt.Println("  status                   - 服务状态")
	fmt.Println("  session_info             - 会话管理器状态")
	fmt.Println("  stats                    - 客户端状态")
	fmt.Println("  disconnect               - 断开连接（保持重连状态）")
	fmt.Println("  reconnect                - 重新连接（保持序列号连续性）")
	fmt.Println("  test_reconnect           - 测试短线重连功能")
	fmt.Println("  before [message]         - 测试notify在response之前返回")
	fmt.Println("  after [message]          - 测试notify在response之后返回")
	fmt.Println("  quit/exit                - 退出")
	fmt.Println("========================================")
	fmt.Println()

	for {
		fmt.Print("> ")

		select {
		case <-sigCh:
			fmt.Println("\n收到中断信号")
			return
		default:
		}

		var input string
		if _, err := fmt.Scanln(&input); err != nil {
			continue
		}

		parts := strings.Fields(input)
		if len(parts) == 0 {
			continue
		}

		cmd := strings.ToLower(parts[0])
		switch cmd {
		case "quit", "exit":
			return
		case "echo":
			data := strings.Join(parts[1:], " ")
			if data == "" {
				data = "Hello from interactive client"
			}
			sendBusinessRequest(c, "echo", nil, []byte(data))
		case "time":
			sendBusinessRequest(c, "time", nil, nil)
		case "hello":
			name := "世界"
			if len(parts) > 1 {
				name = strings.Join(parts[1:], " ")
			}
			params := map[string]string{"name": name}
			sendBusinessRequest(c, "hello", params, nil)
		case "calc":
			if len(parts) < 4 {
				fmt.Println("用法: calc <operation> <a> <b>")
				continue
			}
			params := map[string]string{
				"operation": parts[1],
				"a":         parts[2],
				"b":         parts[3],
			}
			sendBusinessRequest(c, "calculate", params, nil)
		case "status":
			sendBusinessRequest(c, "status", nil, nil)
		case "session_info":
			sendBusinessRequest(c, "session_info", nil, nil)
		case "stats":
			stats := c.GetStats()
			data, _ := json.MarshalIndent(stats, "", "  ")
			fmt.Printf("客户端统计:\n%s\n", data)
		case "disconnect":
			fmt.Println("正在强制断开连接(保持重连状态)...")
			if err := c.ForceDisconnect(); err != nil {
				fmt.Printf("断开连接失败: %v\n", err)
			} else {
				fmt.Println("已断开连接，重连状态已保存")
			}
		case "reconnect":
			fmt.Println("正在重新连接(保持序列号连续性)...")
			ctx := context.Background()
			if err := c.Reconnect(ctx); err != nil {
				fmt.Printf("重连失败: %v\n", err)
			} else {
				fmt.Println("重连成功，序列号已恢复")
			}
		case "test_reconnect":
			testReconnectFunction(c)
		case "before":
			message := "测试notify在response之前"
			if len(parts) > 1 {
				message = strings.Join(parts[1:], " ")
			}
			params := map[string]string{"message": message}
			fmt.Println("发送before指令 - notify应该在response之前到达...")
			sendBusinessRequest(c, "before", params, nil)
		case "after":
			message := "测试notify在response之后"
			if len(parts) > 1 {
				message = strings.Join(parts[1:], " ")
			}
			params := map[string]string{"message": message}
			fmt.Println("发送after指令 - notify应该在response之后到达...")
			sendBusinessRequest(c, "after", params, nil)
		default:
			fmt.Printf("未知命令: %s\n", cmd)
		}
	}
}

// sendBusinessRequest 发送业务请求
func sendBusinessRequest(c *client.Client, action string, params map[string]string, data []byte) {
	resp, err := c.SendBusinessRequest(action, params, data)
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		return
	}

	if resp.Error != nil {
		fmt.Printf("业务错误: %v\n", resp.Error)
		return
	}

	fmt.Printf("📬 收到RESPONSE [%s]: [%d] %s\n", time.Now().Format("15:04:05.000"), resp.Code, resp.Message)
	if len(resp.Data) > 0 {
		fmt.Printf("数据: %s\n", string(resp.Data))
	}

	if len(resp.Headers) > 0 {
		fmt.Printf("头部:\n")
		for k, v := range resp.Headers {
			fmt.Printf("  %s: %s\n", k, v)
		}
	}
}

// printStartupInfo 打印启动信息
func printStartupInfo(config *client.Config) {
	fmt.Println("========================================")
	fmt.Println("         网关客户端 v1.0.0")
	fmt.Println("========================================")
	fmt.Printf("服务器地址:   %s\n", config.ServerAddr)
	fmt.Printf("客户端ID:     %s\n", config.ClientID)
	fmt.Printf("跳过TLS验证:  %v\n", config.TLSSkipVerify)
	fmt.Printf("连接超时:     %v\n", config.ConnectTimeout)
	fmt.Printf("心跳间隔:     %v\n", config.HeartbeatInterval)
	fmt.Println("========================================")
	fmt.Println()
}

// testReconnectFunction 测试短线重连功能
func testReconnectFunction(c *client.Client) {
	fmt.Println("\n========================================")
	fmt.Println("         短线重连测试")
	fmt.Println("========================================")

	// 步骤1: 显示初始状态
	stats := c.GetStats()
	fmt.Printf("初始状态 - 业务序列号: %v\n", stats["next_business_seq"])

	// 步骤2: 发送几个业务请求
	fmt.Println("步骤1: 发送初始业务请求...")
	for i := 1; i <= 3; i++ {
		message := fmt.Sprintf("断线前的消息 #%d", i)
		resp, err := c.SendBusinessRequestWithTimeout("echo", nil, []byte(message), 5*time.Second)
		if err != nil {
			fmt.Printf("  请求 %d 失败: %v\n", i, err)
		} else {
			fmt.Printf("  请求 %d 成功: [%d] %s\n", i, resp.Code, resp.Message)
		}
		time.Sleep(200 * time.Millisecond)
	}

	// 显示断线前状态
	stats = c.GetStats()
	fmt.Printf("断线前状态 - 业务序列号: %v\n", stats["next_business_seq"])

	// 步骤3: 强制断开连接保持状态
	fmt.Println("\n步骤2: 强制断开连接(保持重连状态)...")
	if err := c.ForceDisconnect(); err != nil {
		fmt.Printf("断开连接失败: %v\n", err)
		return
	}
	fmt.Println("连接已断开，重连状态已保存")

	// 等待一下
	time.Sleep(2 * time.Second)

	// 步骤4: 重新连接
	fmt.Println("\n步骤3: 重新连接(保持序列号连续性)...")
	ctx := context.Background()
	if err := c.Reconnect(ctx); err != nil {
		fmt.Printf("重连失败: %v\n", err)
		return
	}
	fmt.Println("重连成功")

	// 显示重连后状态
	stats = c.GetStats()
	fmt.Printf("重连后状态 - 业务序列号: %v\n", stats["next_business_seq"])

	// 步骤5: 发送重连后的业务请求（序列号应该继续）
	fmt.Println("\n步骤4: 发送重连后的业务请求(序列号应该继续)...")
	for i := 4; i <= 6; i++ {
		message := fmt.Sprintf("重连后的消息 #%d", i)
		resp, err := c.SendBusinessRequestWithTimeout("echo", nil, []byte(message), 5*time.Second)
		if err != nil {
			fmt.Printf("  请求 %d 失败: %v\n", i, err)
		} else {
			fmt.Printf("  请求 %d 成功: [%d] %s\n", i, resp.Code, resp.Message)
		}
		time.Sleep(200 * time.Millisecond)
	}

	// 显示最终状态
	stats = c.GetStats()
	fmt.Printf("\n最终状态 - 业务序列号: %v\n", stats["next_business_seq"])

	fmt.Println("========================================")
	fmt.Println("短线重连测试完成")
	fmt.Println("如果业务序列号保持连续，说明重连功能正常")
	fmt.Println("========================================\n")
}

// sendLoginRequest 发送登录请求
func sendLoginRequest(c *client.Client, clientIndex int) bool {
	params := map[string]string{
		"name":      fmt.Sprintf("PerfClient-%d", clientIndex),
		"version":   "1.0",
		"timestamp": fmt.Sprintf("%d", time.Now().Unix()),
	}

	_, err := c.SendBusinessRequest("hello", params, nil)
	if err != nil {
		log.Printf("客户端 %d 登录请求失败: %v", clientIndex, err)
		return false
	}

	return true
}

// performReconnectTest 执行短线重连测试
func performReconnectTest(ctx context.Context, c *client.Client, clientIndex int, stats *PerformanceStats) bool {
	startTime := time.Now()

	// 强制断开连接
	if err := c.ForceDisconnect(); err != nil {
		log.Printf("客户端 %d 强制断开失败: %v", clientIndex, err)
		stats.AddRequest(false, time.Since(startTime))
		return false
	}

	// 短暂等待
	time.Sleep(500 * time.Millisecond)

	// 重新连接
	if err := c.Reconnect(ctx); err != nil {
		log.Printf("客户端 %d 重连失败: %v", clientIndex, err)
		stats.AddRequest(false, time.Since(startTime))
		return false
	}

	// 重新登录
	if !sendLoginRequest(c, clientIndex) {
		log.Printf("客户端 %d 重连后登录失败", clientIndex)
		stats.AddRequest(false, time.Since(startTime))
		return false
	}

	stats.AddRequest(true, time.Since(startTime))
	return true
}

// getBuildTime 获取构建时间
func getBuildTime() string {
	return "unknown"
}
