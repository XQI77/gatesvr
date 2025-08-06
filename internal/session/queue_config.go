package session

import (
	"os"
	"strconv"
)

// QueueConfig 队列配置
type QueueConfig struct {
	MaxQueueSize      int  `json:"max_queue_size"`      // 最大队列大小
	MaxRetries        int  `json:"max_retries"`          // 最大重试次数
	CleanupInterval   int  `json:"cleanup_interval_ms"`  // 清理间隔(毫秒)
}

// DefaultQueueConfig 默认队列配置
func DefaultQueueConfig() *QueueConfig {
	return &QueueConfig{
		MaxQueueSize:      1000,
		MaxRetries:        MaxRetries,
		CleanupInterval:   10000, // 10s
	}
}

// LoadQueueConfigFromEnv 从环境变量加载队列配置
func LoadQueueConfigFromEnv() *QueueConfig {
	config := DefaultQueueConfig()
	
	// 最大队列大小
	if val := os.Getenv("QUEUE_MAX_QUEUE_SIZE"); val != "" {
		if size, err := strconv.Atoi(val); err == nil && size > 0 {
			config.MaxQueueSize = size
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
	
	return config
}

// Validate 验证配置参数
func (c *QueueConfig) Validate() error {
	if c.MaxQueueSize <= 0 {
		c.MaxQueueSize = 1000
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
	
	return NewOrderedMessageQueue(sessionID, config.MaxQueueSize)
}