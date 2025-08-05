package session

import (
	"os"
	"strconv"
)

// QueueConfig 队列配置
type QueueConfig struct {
	// 异步发送配置
	EnableAsyncSend   bool `json:"enable_async_send"`
	SendWorkerCount   int  `json:"send_worker_count"`
	SendQueueSize     int  `json:"send_queue_size"`
	MaxQueueSize      int  `json:"max_queue_size"`
	
	// 性能调优配置
	BatchTimeout      int  `json:"batch_timeout_ms"`     // 批量发送超时(毫秒)
	MaxRetries        int  `json:"max_retries"`          // 最大重试次数
	CleanupInterval   int  `json:"cleanup_interval_ms"`  // 清理间隔(毫秒)
	
	// 调试和监控
	EnableDebugLog    bool `json:"enable_debug_log"`
	EnableMetrics     bool `json:"enable_metrics"`
}

// DefaultQueueConfig 默认队列配置
func DefaultQueueConfig() *QueueConfig {
	return &QueueConfig{
		EnableAsyncSend:   true,
		SendWorkerCount:   DefaultSendWorkerCount,
		SendQueueSize:     DefaultSendQueueSize,
		MaxQueueSize:      1000,
		BatchTimeout:      5,    // 5ms
		MaxRetries:        MaxRetries,
		CleanupInterval:   10000, // 10s
		EnableDebugLog:    false,
		EnableMetrics:     true,
	}
}

// LoadQueueConfigFromEnv 从环境变量加载队列配置
func LoadQueueConfigFromEnv() *QueueConfig {
	config := DefaultQueueConfig()
	
	// 异步发送开关
	if val := os.Getenv("QUEUE_ENABLE_ASYNC_SEND"); val != "" {
		if enabled, err := strconv.ParseBool(val); err == nil {
			config.EnableAsyncSend = enabled
		}
	}
	
	// 工作协程数量
	if val := os.Getenv("QUEUE_SEND_WORKER_COUNT"); val != "" {
		if count, err := strconv.Atoi(val); err == nil && count > 0 {
			config.SendWorkerCount = count
		}
	}
	
	// 发送队列大小
	if val := os.Getenv("QUEUE_SEND_QUEUE_SIZE"); val != "" {
		if size, err := strconv.Atoi(val); err == nil && size > 0 {
			config.SendQueueSize = size
		}
	}
	
	// 最大队列大小
	if val := os.Getenv("QUEUE_MAX_QUEUE_SIZE"); val != "" {
		if size, err := strconv.Atoi(val); err == nil && size > 0 {
			config.MaxQueueSize = size
		}
	}
	
	// 批量超时
	if val := os.Getenv("QUEUE_BATCH_TIMEOUT_MS"); val != "" {
		if timeout, err := strconv.Atoi(val); err == nil && timeout > 0 {
			config.BatchTimeout = timeout
		}
	}
	
	// 最大重试次数
	if val := os.Getenv("QUEUE_MAX_RETRIES"); val != "" {
		if retries, err := strconv.Atoi(val); err == nil && retries >= 0 {
			config.MaxRetries = retries
		}
	}
	
	// 清理间隔
	if val := os.Getenv("QUEUE_CLEANUP_INTERVAL_MS"); val != "" {
		if interval, err := strconv.Atoi(val); err == nil && interval > 0 {
			config.CleanupInterval = interval
		}
	}
	
	// 调试日志
	if val := os.Getenv("QUEUE_ENABLE_DEBUG_LOG"); val != "" {
		if enabled, err := strconv.ParseBool(val); err == nil {
			config.EnableDebugLog = enabled
		}
	}
	
	// 指标监控
	if val := os.Getenv("QUEUE_ENABLE_METRICS"); val != "" {
		if enabled, err := strconv.ParseBool(val); err == nil {
			config.EnableMetrics = enabled
		}
	}
	
	return config
}

// Validate 验证配置参数
func (c *QueueConfig) Validate() error {
	if c.SendWorkerCount <= 0 {
		c.SendWorkerCount = DefaultSendWorkerCount
	}
	
	if c.SendQueueSize <= 0 {
		c.SendQueueSize = DefaultSendQueueSize
	}
	
	if c.MaxQueueSize <= 0 {
		c.MaxQueueSize = 1000
	}
	
	if c.BatchTimeout <= 0 {
		c.BatchTimeout = 5
	}
	
	if c.MaxRetries < 0 {
		c.MaxRetries = MaxRetries
	}
	
	if c.CleanupInterval <= 0 {
		c.CleanupInterval = 10000
	}
	
	return nil
}

// NewOrderedMessageQueueWithEnvConfig 使用环境变量配置创建队列
func NewOrderedMessageQueueWithEnvConfig(sessionID string) *OrderedMessageQueue {
	config := LoadQueueConfigFromEnv()
	config.Validate()
	
	return NewOrderedMessageQueueWithConfig(
		sessionID,
		config.MaxQueueSize,
		config.EnableAsyncSend,
		config.SendWorkerCount,
		config.SendQueueSize,
	)
}