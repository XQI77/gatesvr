// 网关服务器启动程序
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"gatesvr/internal/backup"
	"gatesvr/internal/config"
	"gatesvr/internal/gateway"
)

func main() {
	// 设置日志输出到文件，每次运行覆盖
	setupLogToFile()

	// 命令行参数
	var (
		configFile = flag.String("config", "config.yaml", "配置文件路径")

		// 可选的覆盖参数
		quicAddr     = flag.String("quic", "", "QUIC监听地址 (覆盖配置文件)")
		httpAddr     = flag.String("http", "", "HTTP API监听地址 (覆盖配置文件)")
		grpcAddr     = flag.String("grpc", "", "gRPC服务监听地址 (覆盖配置文件)")
		metricsAddr  = flag.String("metrics", "", "监控指标监听地址 (覆盖配置文件)")
		upstreamAddr = flag.String("upstream", "", "上游gRPC服务地址 (覆盖配置文件)")
		certFile     = flag.String("cert", "", "TLS证书文件路径 (覆盖配置文件)")
		keyFile      = flag.String("key", "", "TLS私钥文件路径 (覆盖配置文件)")

		sessionTimeout = flag.Duration("session-timeout", 0, "会话超时时间 (覆盖配置文件)")
		ackTimeout     = flag.Duration("ack-timeout", 0, "ACK超时时间 (覆盖配置文件)")
		maxRetries     = flag.Int("max-retries", 0, "最大重试次数 (覆盖配置文件)")

		// 备份相关参数
		enableBackup      = flag.Bool("backup-enable", false, "启用热备份功能 (覆盖配置文件)")
		backupMode        = flag.String("backup-mode", "", "备份模式: primary(主) 或 backup(备) (覆盖配置文件)")
		peerAddr          = flag.String("peer-addr", "", "对端服务器地址 (覆盖配置文件)")
		serverID          = flag.String("server-id", "", "服务器ID (覆盖配置文件)")
		heartbeatInterval = flag.Duration("heartbeat-interval", 0, "心跳间隔 (覆盖配置文件)")
		syncBatchSize     = flag.Int("sync-batch-size", 0, "同步批次大小 (覆盖配置文件)")
		syncTimeout       = flag.Duration("sync-timeout", 0, "同步超时 (覆盖配置文件)")
		readonly          = flag.Bool("readonly", false, "只读模式 (覆盖配置文件)")

		showVersion = flag.Bool("version", false, "显示版本信息")
		showHelp    = flag.Bool("help", false, "显示帮助信息")
	)

	flag.Parse()

	if *showVersion {
		fmt.Println("网关服务器 v1.0.0")
		fmt.Println("构建时间:", getBuildTime())
		os.Exit(0)
	}

	if *showHelp {
		fmt.Println("网关服务器 - 高性能QUIC网关")
		fmt.Println()
		fmt.Println("使用方法:")
		flag.PrintDefaults()
		fmt.Println()
		fmt.Println("示例:")
		fmt.Println("  # 使用默认配置文件")
		fmt.Println("  ./gatesvr")
		fmt.Println()
		fmt.Println("  # 使用指定配置文件")
		fmt.Println("  ./gatesvr -config config-backup.yaml")
		fmt.Println()
		fmt.Println("  # 覆盖配置文件中的端口设置")
		fmt.Println("  ./gatesvr -config config.yaml -quic :8453 -http :8090")
		fmt.Println()
		fmt.Println("  # 主备模式 - 主服务器")
		fmt.Println("  ./gatesvr -config config.yaml")
		fmt.Println()
		fmt.Println("  # 主备模式 - 备份服务器")
		fmt.Println("  ./gatesvr -config config-backup.yaml")
		fmt.Println()
		fmt.Println("环境变量:")
		fmt.Println("  GATESVR_QUIC_ADDR      QUIC监听地址")
		fmt.Println("  GATESVR_HTTP_ADDR      HTTP API监听地址")
		fmt.Println("  GATESVR_METRICS_ADDR   监控指标监听地址")
		fmt.Println("  GATESVR_UPSTREAM_ADDR  上游服务地址")
		fmt.Println("  GATESVR_CERT_FILE      TLS证书文件")
		fmt.Println("  GATESVR_KEY_FILE       TLS私钥文件")
		os.Exit(0)
	}

	// 加载配置文件
	cfg, err := config.Load(*configFile)
	if err != nil {
		log.Fatalf("加载配置文件失败: %v", err)
	}

	// 转换为网关配置
	gatewayConfig, err := cfg.ToGatewayConfig()
	if err != nil {
		log.Fatalf("配置转换失败: %v", err)
	}

	// 命令行参数覆盖配置文件
	if *quicAddr != "" {
		gatewayConfig.QUICAddr = *quicAddr
	}
	if *httpAddr != "" {
		gatewayConfig.HTTPAddr = *httpAddr
	}
	if *grpcAddr != "" {
		gatewayConfig.GRPCAddr = *grpcAddr
	}
	if *metricsAddr != "" {
		gatewayConfig.MetricsAddr = *metricsAddr
	}
	if *upstreamAddr != "" {
		gatewayConfig.UpstreamAddr = *upstreamAddr
	}
	if *certFile != "" {
		gatewayConfig.TLSCertFile = *certFile
	}
	if *keyFile != "" {
		gatewayConfig.TLSKeyFile = *keyFile
	}
	if *sessionTimeout != 0 {
		gatewayConfig.SessionTimeout = *sessionTimeout
	}
	if *ackTimeout != 0 {
		gatewayConfig.AckTimeout = *ackTimeout
	}
	if *maxRetries != 0 {
		gatewayConfig.MaxRetries = *maxRetries
	}
	if *serverID != "" {
		gatewayConfig.ServerID = *serverID
	}

	// 环境变量覆盖所有设置
	if addr := os.Getenv("GATESVR_QUIC_ADDR"); addr != "" {
		gatewayConfig.QUICAddr = addr
	}
	if addr := os.Getenv("GATESVR_HTTP_ADDR"); addr != "" {
		gatewayConfig.HTTPAddr = addr
	}
	if addr := os.Getenv("GATESVR_GRPC_ADDR"); addr != "" {
		gatewayConfig.GRPCAddr = addr
	}
	if addr := os.Getenv("GATESVR_METRICS_ADDR"); addr != "" {
		gatewayConfig.MetricsAddr = addr
	}
	if addr := os.Getenv("GATESVR_UPSTREAM_ADDR"); addr != "" {
		gatewayConfig.UpstreamAddr = addr
	}
	if file := os.Getenv("GATESVR_CERT_FILE"); file != "" {
		gatewayConfig.TLSCertFile = file
	}
	if file := os.Getenv("GATESVR_KEY_FILE"); file != "" {
		gatewayConfig.TLSKeyFile = file
	}

	// 备份配置的命令行覆盖
	if *enableBackup || cfg.Backup.Enabled {
		if gatewayConfig.BackupConfig == nil {
			// 如果配置文件中没有备份配置但命令行启用了，创建默认配置
			gatewayConfig.BackupConfig = &backup.BackupConfig{
				Sync: backup.SyncConfig{
					Enabled:           true,
					Mode:              backup.ModePrimary,
					HeartbeatInterval: 2 * time.Second,
					SyncBatchSize:     50,
					SyncTimeout:       200 * time.Millisecond,
					BufferSize:        1000,
				},
				Failover: backup.FailoverConfig{
					DetectionTimeout: 6 * time.Second,
					SwitchTimeout:    10 * time.Second,
					RecoveryTimeout:  30 * time.Second,
					MaxRetries:       3,
				},
			}
		}

		// 命令行参数覆盖备份配置
		if *backupMode != "" {
			switch *backupMode {
			case "primary":
				gatewayConfig.BackupConfig.Sync.Mode = backup.ModePrimary
			case "backup":
				gatewayConfig.BackupConfig.Sync.Mode = backup.ModeBackup
			default:
				log.Fatalf("无效的备份模式: %s, 支持: primary, backup", *backupMode)
			}
		}
		if *peerAddr != "" {
			gatewayConfig.BackupConfig.Sync.PeerAddr = *peerAddr
		}
		if *heartbeatInterval != 0 {
			gatewayConfig.BackupConfig.Sync.HeartbeatInterval = *heartbeatInterval
			gatewayConfig.BackupConfig.Failover.DetectionTimeout = *heartbeatInterval * 3
		}
		if *syncBatchSize != 0 {
			gatewayConfig.BackupConfig.Sync.SyncBatchSize = *syncBatchSize
		}
		if *syncTimeout != 0 {
			gatewayConfig.BackupConfig.Sync.SyncTimeout = *syncTimeout
		}
		if *readonly {
			gatewayConfig.BackupConfig.Sync.ReadOnly = *readonly
		}

		// 验证备份配置
		if gatewayConfig.BackupConfig.Sync.PeerAddr == "" {
			log.Fatalf("启用备份功能时必须指定对端地址")
		}
	}

	// 验证证书文件
	if err := validateCertFiles(gatewayConfig.TLSCertFile, gatewayConfig.TLSKeyFile); err != nil {
		log.Fatalf("证书文件验证失败: %v", err)
	}

	// 转换过载保护配置
	var overloadConfig *gateway.OverloadConfig
	if gatewayConfig.OverloadProtection != nil {
		overloadConfig = &gateway.OverloadConfig{
			Enabled:                      gatewayConfig.OverloadProtection.Enabled,
			MaxConnections:              gatewayConfig.OverloadProtection.MaxConnections,
			ConnectionWarningThreshold:   gatewayConfig.OverloadProtection.ConnectionWarningThreshold,
			MaxQPS:                      gatewayConfig.OverloadProtection.MaxQPS,
			QPSWarningThreshold:         gatewayConfig.OverloadProtection.QPSWarningThreshold,
			QPSWindowSeconds:            gatewayConfig.OverloadProtection.QPSWindowSeconds,
			MaxUpstreamConcurrent:       gatewayConfig.OverloadProtection.MaxUpstreamConcurrent,
			UpstreamTimeout:             gatewayConfig.OverloadProtection.UpstreamTimeout,
			UpstreamWarningThreshold:    gatewayConfig.OverloadProtection.UpstreamWarningThreshold,
		}
	}

	// 创建服务器配置
	serverConfig := &gateway.Config{
		QUICAddr:                 gatewayConfig.QUICAddr,
		HTTPAddr:                 gatewayConfig.HTTPAddr,
		GRPCAddr:                 gatewayConfig.GRPCAddr,
		MetricsAddr:              gatewayConfig.MetricsAddr,
		UpstreamAddr:             gatewayConfig.UpstreamAddr,
		UpstreamServices:         gatewayConfig.UpstreamServices,
		TLSCertFile:              gatewayConfig.TLSCertFile,
		TLSKeyFile:               gatewayConfig.TLSKeyFile,
		SessionTimeout:           gatewayConfig.SessionTimeout,
		AckTimeout:               gatewayConfig.AckTimeout,
		MaxRetries:               gatewayConfig.MaxRetries,
		BackupConfig:             gatewayConfig.BackupConfig,
		ServerID:                 gatewayConfig.ServerID,
		OverloadProtectionConfig: overloadConfig,
	}

	// 显示启动信息
	printStartupInfo(serverConfig)

	// 创建服务器
	server := gateway.NewServer(serverConfig)

	// 创建上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 启动服务器
	if err := server.Start(ctx); err != nil {
		log.Fatalf("启动服务器失败: %v", err)
	}

	// 等待中断信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	log.Printf("网关服务器正在运行... (按Ctrl+C停止)")
	<-sigCh

	log.Printf("收到停止信号，正在关闭服务器...")

	// 优雅关闭
	cancel()
	server.Stop()

	log.Printf("服务器已停止")
}

// validateCertFiles 验证证书文件
func validateCertFiles(certFile, keyFile string) error {
	// 检查证书文件
	if _, err := os.Stat(certFile); os.IsNotExist(err) {
		return fmt.Errorf("证书文件不存在: %s", certFile)
	}

	// 检查私钥文件
	if _, err := os.Stat(keyFile); os.IsNotExist(err) {
		return fmt.Errorf("私钥文件不存在: %s", keyFile)
	}

	return nil
}

// printStartupInfo 打印启动信息
func printStartupInfo(config *gateway.Config) {
	fmt.Println("========================================")
	fmt.Println("         网关服务器 v1.0.0")
	fmt.Println("========================================")
	fmt.Printf("QUIC地址:     %s\n", config.QUICAddr)
	fmt.Printf("HTTP API:     %s\n", config.HTTPAddr)
	fmt.Printf("监控指标:     %s\n", config.MetricsAddr)
	fmt.Printf("上游服务:     %s\n", config.UpstreamAddr)
	fmt.Printf("TLS证书:      %s\n", config.TLSCertFile)
	fmt.Printf("TLS私钥:      %s\n", config.TLSKeyFile)
	fmt.Printf("会话超时:     %v\n", config.SessionTimeout)
	fmt.Printf("ACK超时:      %v\n", config.AckTimeout)
	fmt.Printf("最大重试:     %d\n", config.MaxRetries)
	fmt.Println("========================================")
	fmt.Println()

	fmt.Println("可用的API端点:")
	fmt.Printf("  健康检查:   http://%s/health\n", config.HTTPAddr)
	fmt.Printf("  会话统计:   http://%s/stats\n", config.HTTPAddr)
	fmt.Printf("  性能监控:   http://%s/performance\n", config.HTTPAddr)
	fmt.Printf("  监控指标:   http://%s/metrics\n", config.MetricsAddr)
	fmt.Println()
}

// setupLogToFile 设置日志输出到文件，支持3MB大小限制和轮转
func setupLogToFile() {
	// 日志文件路径
	logFile := "gatesvr.log"

	// 创建轮转日志写入器
	writer := &rotatingWriter{
		filename:  logFile,
		maxSize:   3 * 1024 * 1024, // 3MB
		maxBackup: 5,               // 保留5个备份文件
	}

	// 打开当前日志文件
	if err := writer.openCurrent(); err != nil {
		fmt.Printf("无法创建日志文件 %s: %v\n", logFile, err)
		os.Exit(1)
	}

	// 设置日志输出到轮转写入器
	log.SetOutput(writer)

	// 设置日志格式：时间 + 文件:行号 + 消息
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// 输出到控制台告知日志文件位置
	fmt.Printf("日志已配置输出到文件: %s (最大3MB)\n", logFile)
}

// rotatingWriter 实现日志轮转的写入器
type rotatingWriter struct {
	filename    string
	maxSize     int64
	maxBackup   int
	file        *os.File
	currentSize int64
}

// openCurrent 打开当前日志文件
func (w *rotatingWriter) openCurrent() error {
	// 检查现有文件大小
	if info, err := os.Stat(w.filename); err == nil {
		w.currentSize = info.Size()
		// 如果现有文件太大，立即轮转
		if w.currentSize >= w.maxSize {
			if err := w.rotate(); err != nil {
				return err
			}
			w.currentSize = 0
		}
	}

	// 打开或创建文件
	file, err := os.OpenFile(w.filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return err
	}

	w.file = file
	return nil
}

// Write 实现io.Writer接口
func (w *rotatingWriter) Write(p []byte) (n int, err error) {
	// 检查是否需要轮转
	if w.currentSize+int64(len(p)) > w.maxSize {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}

	// 写入数据
	n, err = w.file.Write(p)
	if err != nil {
		return n, err
	}

	w.currentSize += int64(n)
	return n, nil
}

// rotate 执行日志轮转
func (w *rotatingWriter) rotate() error {
	// 关闭当前文件
	if w.file != nil {
		w.file.Close()
	}

	// 移动备份文件
	for i := w.maxBackup - 1; i >= 1; i-- {
		oldName := w.filename + "." + strconv.Itoa(i)
		newName := w.filename + "." + strconv.Itoa(i+1)

		if _, err := os.Stat(oldName); err == nil {
			// 如果是最后一个备份文件，删除它
			if i == w.maxBackup-1 {
				os.Remove(newName)
			}
			os.Rename(oldName, newName)
		}
	}

	// 移动当前文件为第一个备份
	if _, err := os.Stat(w.filename); err == nil {
		backupName := w.filename + ".1"
		if err := os.Rename(w.filename, backupName); err != nil {
			return err
		}
	}

	// 创建新的日志文件
	file, err := os.OpenFile(w.filename, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		return err
	}

	w.file = file
	w.currentSize = 0
	return nil
}

// Close 关闭日志文件
func (w *rotatingWriter) Close() error {
	if w.file != nil {
		return w.file.Close()
	}
	return nil
}

// getBuildTime 获取构建时间
func getBuildTime() string {
	// 这里可以在构建时通过ldflags注入
	return "unknown"
}
