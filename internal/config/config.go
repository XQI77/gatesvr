// Package config 提供配置文件读取功能
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
	"gatesvr/internal/backup"
)

// Config 应用程序配置
type Config struct {
	Server ServerConfig `yaml:"server"`
	Backup BackupConfig `yaml:"backup"`
}

// ServerConfig 服务器配置
type ServerConfig struct {
	QUICAddr       string `yaml:"quic_addr"`
	HTTPAddr       string `yaml:"http_addr"`
	GRPCAddr       string `yaml:"grpc_addr"`
	MetricsAddr    string `yaml:"metrics_addr"`
	UpstreamAddr   string `yaml:"upstream_addr"`
	CertFile       string `yaml:"cert_file"`
	KeyFile        string `yaml:"key_file"`
	SessionTimeout string `yaml:"session_timeout"`
	AckTimeout     string `yaml:"ack_timeout"`
	MaxRetries     int    `yaml:"max_retries"`
}

// HeartbeatConfig 心跳配置
type HeartbeatConfig struct {
	Interval  string `yaml:"interval"`    // 心跳间隔
	PeerAddr  string `yaml:"peer_addr"`   // 主服务器连接目标地址
	ListenAddr string `yaml:"listen_addr"` // 备份服务器监听地址
	Timeout   string `yaml:"timeout"`     // 心跳超时时间
}

// SyncConfig 同步配置
type SyncConfig struct {
	PeerAddr   string `yaml:"peer_addr"`    // 主服务器连接目标地址
	ListenAddr string `yaml:"listen_addr"`  // 备份服务器监听地址
	BatchSize  int    `yaml:"batch_size"`   // 同步批次大小
	Timeout    string `yaml:"timeout"`      // 同步超时
	BufferSize int    `yaml:"buffer_size"`  // 同步缓冲区大小
}

// BackupConfig 备份配置
type BackupConfig struct {
	Enabled   bool            `yaml:"enabled"`
	Mode      string          `yaml:"mode"`
	ServerID  string          `yaml:"server_id"`
	ReadOnly  bool            `yaml:"readonly"`
	Heartbeat HeartbeatConfig `yaml:"heartbeat"`
	Sync      SyncConfig      `yaml:"sync"`
}

// Load 从文件加载配置
func Load(configFile string) (*Config, error) {
	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	return &config, nil
}

// ParseDuration 解析时间字符串
func (c *Config) ParseDuration(durationStr string) (time.Duration, error) {
	return time.ParseDuration(durationStr)
}

// ToGatewayConfig 转换为网关配置
func (c *Config) ToGatewayConfig() (*GatewayConfig, error) {
	sessionTimeout, err := c.ParseDuration(c.Server.SessionTimeout)
	if err != nil {
		return nil, fmt.Errorf("解析会话超时时间失败: %w", err)
	}

	ackTimeout, err := c.ParseDuration(c.Server.AckTimeout)
	if err != nil {
		return nil, fmt.Errorf("解析ACK超时时间失败: %w", err)
	}

	config := &GatewayConfig{
		QUICAddr:       c.Server.QUICAddr,
		HTTPAddr:       c.Server.HTTPAddr,
		GRPCAddr:       c.Server.GRPCAddr,
		MetricsAddr:    c.Server.MetricsAddr,
		UpstreamAddr:   c.Server.UpstreamAddr,
		TLSCertFile:    c.Server.CertFile,
		TLSKeyFile:     c.Server.KeyFile,
		SessionTimeout: sessionTimeout,
		AckTimeout:     ackTimeout,
		MaxRetries:     c.Server.MaxRetries,
		ServerID:       c.Backup.ServerID,
	}

	// 处理备份配置
	if c.Backup.Enabled {
		// 解析心跳间隔
		heartbeatInterval, err := c.ParseDuration(c.Backup.Heartbeat.Interval)
		if err != nil {
			return nil, fmt.Errorf("解析心跳间隔失败: %w", err)
		}

		// 解析心跳超时
		heartbeatTimeout, err := c.ParseDuration(c.Backup.Heartbeat.Timeout)
		if err != nil {
			return nil, fmt.Errorf("解析心跳超时失败: %w", err)
		}

		// 解析同步超时
		syncTimeout, err := c.ParseDuration(c.Backup.Sync.Timeout)
		if err != nil {
			return nil, fmt.Errorf("解析同步超时失败: %w", err)
		}

		// 解析备份模式
		var mode backup.ServerMode
		switch c.Backup.Mode {
		case "primary":
			mode = backup.ModePrimary
		case "backup":
			mode = backup.ModeBackup
		default:
			return nil, fmt.Errorf("无效的备份模式: %s, 支持: primary, backup", c.Backup.Mode)
		}

		// 确定心跳和同步地址
		var heartbeatAddr, syncAddr string
		if mode == backup.ModePrimary {
			// 主服务器使用 peer_addr 连接到备份服务器
			heartbeatAddr = c.Backup.Heartbeat.PeerAddr
			syncAddr = c.Backup.Sync.PeerAddr
		} else {
			// 备份服务器使用 listen_addr 监听主服务器连接
			heartbeatAddr = c.Backup.Heartbeat.ListenAddr
			syncAddr = c.Backup.Sync.ListenAddr
		}

		config.BackupConfig = &backup.BackupConfig{
			Sync: backup.SyncConfig{
				Enabled:           c.Backup.Enabled,
				Mode:              mode,
				PeerAddr:          heartbeatAddr, // 心跳地址（临时兼容，后续会分离）
				HeartbeatInterval: heartbeatInterval,
				SyncBatchSize:     c.Backup.Sync.BatchSize,
				SyncTimeout:       syncTimeout,
				ReadOnly:          c.Backup.ReadOnly,
				BufferSize:        c.Backup.Sync.BufferSize,
			},
			Failover: backup.FailoverConfig{
				DetectionTimeout: heartbeatTimeout,
				SwitchTimeout:    10 * time.Second,
				RecoveryTimeout:  30 * time.Second,
				MaxRetries:       3,
			},
		}

		// 添加新的字段到配置中（为了向后兼容和新逻辑）
		config.BackupConfig.HeartbeatAddr = heartbeatAddr
		config.BackupConfig.SyncAddr = syncAddr
	}

	return config, nil
}

// GatewayConfig 网关配置（兼容现有接口）
type GatewayConfig struct {
	QUICAddr       string
	HTTPAddr       string
	GRPCAddr       string
	MetricsAddr    string
	UpstreamAddr   string
	TLSCertFile    string
	TLSKeyFile     string
	SessionTimeout time.Duration
	AckTimeout     time.Duration
	MaxRetries     int
	BackupConfig   *backup.BackupConfig
	ServerID       string
}