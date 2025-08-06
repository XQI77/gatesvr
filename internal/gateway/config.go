// Package gateway 提供网关服务器配置管理
package gateway

import (
	"time"

	"gatesvr/internal/backup"
)

// Config 网关服务器配置
type Config struct {
	// 服务地址配置
	QUICAddr         string              // QUIC监听地址
	HTTPAddr         string              // HTTP API地址
	GRPCAddr         string              // gRPC服务地址（新增）
	MetricsAddr      string              // 监控地址
	UpstreamAddr     string              // 上游服务地址（保留向后兼容）
	UpstreamServices map[string][]string // 多上游服务配置

	// TLS配置
	TLSCertFile string // TLS证书文件
	TLSKeyFile  string // TLS私钥文件

	// 会话配置
	SessionTimeout time.Duration // 会话超时时间
	AckTimeout     time.Duration // ACK超时时间
	MaxRetries     int           // 最大重试次数

	// START消息异步处理配置
	StartProcessorConfig *StartProcessorConfig // START消息处理器配置

	// 备份配置
	BackupConfig *backup.BackupConfig // 热备份配置
	ServerID     string               // 服务器ID

	// 过载保护配置
	OverloadProtectionConfig *OverloadConfig // 过载保护配置
}

// StartProcessorConfig START消息处理器配置
type StartProcessorConfig struct {
	Enabled    bool          // 是否启用异步处理（默认true）
	MaxWorkers int           // 最大工作线程数（默认100）
	QueueSize  int           // 任务队列大小（默认1000）
	Timeout    time.Duration // 处理超时时间（默认30秒）
}
