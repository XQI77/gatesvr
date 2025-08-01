// 网关服务器启动程序
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gatesvr/internal/gateway"
)

func main() {
	// 命令行参数
	var (
		quicAddr     = flag.String("quic", ":8443", "QUIC监听地址")
		httpAddr     = flag.String("http", ":8080", "HTTP API监听地址")
		grpcAddr     = flag.String("grpc", ":8082", "gRPC服务监听地址")
		metricsAddr  = flag.String("metrics", ":9090", "监控指标监听地址")
		upstreamAddr = flag.String("upstream", "localhost:9000", "上游gRPC服务地址")
		certFile     = flag.String("cert", "D:/25cxx/v3/certs/server.crt", "TLS证书文件路径")
		keyFile      = flag.String("key", "D:/25cxx/v3/certs/server.key", "TLS私钥文件路径")

		sessionTimeout = flag.Duration("session-timeout", 5*time.Minute, "会话超时时间")
		ackTimeout     = flag.Duration("ack-timeout", 10*time.Second, "ACK超时时间")
		maxRetries     = flag.Int("max-retries", 3, "最大重试次数")

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
		fmt.Println("  ./gatesvr -quic :8443 -http :8080 -upstream localhost:9000")
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

	// 从环境变量读取配置
	if addr := os.Getenv("GATESVR_QUIC_ADDR"); addr != "" {
		*quicAddr = addr
	}
	if addr := os.Getenv("GATESVR_HTTP_ADDR"); addr != "" {
		*httpAddr = addr
	}
	if addr := os.Getenv("GATESVR_GRPC_ADDR"); addr != "" {
		*grpcAddr = addr
	}
	if addr := os.Getenv("GATESVR_METRICS_ADDR"); addr != "" {
		*metricsAddr = addr
	}
	if addr := os.Getenv("GATESVR_UPSTREAM_ADDR"); addr != "" {
		*upstreamAddr = addr
	}
	if file := os.Getenv("GATESVR_CERT_FILE"); file != "" {
		*certFile = file
	}
	if file := os.Getenv("GATESVR_KEY_FILE"); file != "" {
		*keyFile = file
	}

	// 验证证书文件
	if err := validateCertFiles(*certFile, *keyFile); err != nil {
		log.Fatalf("证书文件验证失败: %v", err)
	}

	// 创建服务器配置
	config := &gateway.Config{
		QUICAddr:       *quicAddr,
		HTTPAddr:       *httpAddr,
		GRPCAddr:       *grpcAddr,
		MetricsAddr:    *metricsAddr,
		UpstreamAddr:   *upstreamAddr,
		TLSCertFile:    *certFile,
		TLSKeyFile:     *keyFile,
		SessionTimeout: *sessionTimeout,
		AckTimeout:     *ackTimeout,
		MaxRetries:     *maxRetries,
	}

	// 显示启动信息
	printStartupInfo(config)

	// 创建服务器
	server := gateway.NewServer(config)

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

// getBuildTime 获取构建时间
func getBuildTime() string {
	// 这里可以在构建时通过ldflags注入
	return "unknown"
}
