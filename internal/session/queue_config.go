package session

// QueueConfig 队列配置
type QueueConfig struct {
	MaxQueueSize    int `json:"max_queue_size"`      // 最大队列大小
	MaxRetries      int `json:"max_retries"`         // 最大重试次数
	CleanupInterval int `json:"cleanup_interval_ms"` // 清理间隔(毫秒)
}

// LoadQueueConfigFromEnv 从环境变量加载队列配置
func LoadQueueConfigFromEnv() *QueueConfig {
	return &QueueConfig{
		MaxQueueSize:    1000,
		MaxRetries:      MaxRetries,
		CleanupInterval: 10000, // 10s
	}
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
