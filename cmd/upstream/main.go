// 上游服务启动程序
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"gatesvr/internal/upstream"
)

func main() {
	// 命令行参数
	var (
		addr        = flag.String("addr", ":9000", "gRPC服务监听地址")
		showVersion = flag.Bool("version", false, "显示版本信息")
		showHelp    = flag.Bool("help", false, "显示帮助信息")
	)

	flag.Parse()

	if *showVersion {
		fmt.Println("上游服务 v1.0.0")
		fmt.Println("构建时间:", getBuildTime())
		os.Exit(0)
	}

	if *showHelp {
		fmt.Println("上游服务 - gRPC业务逻辑服务器")
		fmt.Println()
		fmt.Println("使用方法:")
		flag.PrintDefaults()
		fmt.Println()
		fmt.Println("示例:")
		fmt.Println("  ./upstream -addr :9000")
		fmt.Println()
		fmt.Println("环境变量:")
		fmt.Println("  UPSTREAM_ADDR    gRPC监听地址")
		fmt.Println()
		fmt.Println("支持的业务操作:")
		fmt.Println("  echo       - 回显消息")
		fmt.Println("  time       - 获取当前时间")
		fmt.Println("  hello      - 问候消息")
		fmt.Println("  calculate  - 数学计算")
		fmt.Println("  status     - 服务状态查询")
		os.Exit(0)
	}

	// 从环境变量读取配置
	if envAddr := os.Getenv("UPSTREAM_ADDR"); envAddr != "" {
		*addr = envAddr
	}

	// 显示启动信息
	printStartupInfo(*addr)

	// 创建服务器
	server := upstream.NewServer(*addr)

	// 启动服务器
	if err := server.Start(); err != nil {
		log.Fatalf("启动上游服务失败: %v", err)
	}

	// 等待中断信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	log.Printf("上游服务正在运行... (按Ctrl+C停止)")
	<-sigCh

	log.Printf("收到停止信号，正在关闭服务...")

	// 停止服务器
	server.Stop()

	log.Printf("上游服务已停止")
}

// printStartupInfo 打印启动信息
func printStartupInfo(addr string) {
	fmt.Println("========================================")
	fmt.Println("         上游服务 v1.0.0")
	fmt.Println("========================================")
	fmt.Printf("gRPC地址:     %s\n", addr)
	fmt.Println("========================================")
	fmt.Println()

	fmt.Println("支持的业务操作:")
	fmt.Printf("  echo       - 回显客户端发送的数据\n")
	fmt.Printf("  time       - 返回当前服务器时间\n")
	fmt.Printf("  hello      - 返回问候消息 (参数: name)\n")
	fmt.Printf("  calculate  - 数学计算 (参数: operation, a, b)\n")
	fmt.Printf("             支持: add, subtract, multiply, divide\n")
	fmt.Printf("  status     - 返回服务器状态信息\n")
	fmt.Println()

	fmt.Println("gRPC服务接口:")
	fmt.Printf("  ProcessRequest - 处理业务请求\n")
	fmt.Printf("  GetStatus      - 获取服务状态\n")
	fmt.Println()
}

// getBuildTime 获取构建时间
func getBuildTime() string {
	// 这里可以在构建时通过ldflags注入
	return "unknown"
}
