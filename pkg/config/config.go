package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds all application configuration loaded from environment variables.
type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Redis    RedisConfig
	Webhook  WebhookConfig
	Worker   WorkerConfig
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Port         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// DatabaseConfig holds PostgreSQL connection settings.
type DatabaseConfig struct {
	DSN             string
	MaxConns        int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// RedisConfig holds Redis connection settings.
type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

// WebhookConfig holds the outbound webhook provider settings.
type WebhookConfig struct {
	URL     string
	Timeout time.Duration
}

// WorkerConfig holds async worker pool settings.
type WorkerConfig struct {
	WorkersPerChannel int
	RetryBaseDelay    time.Duration
	MaxRetries        int
}

// Load reads all configuration from environment variables with sane defaults.
func Load() *Config {
	return &Config{
		Server: ServerConfig{
			Port:         getEnv("SERVER_PORT", "8080"),
			ReadTimeout:  getDuration("SERVER_READ_TIMEOUT", 30*time.Second),
			WriteTimeout: getDuration("SERVER_WRITE_TIMEOUT", 30*time.Second),
		},
		Database: DatabaseConfig{
			DSN:             getEnv("DATABASE_DSN", "postgres://postgres:postgres@localhost:5432/notifications?sslmode=disable"),
			MaxConns:        getInt("DATABASE_MAX_CONNS", 25),
			MaxIdleConns:    getInt("DATABASE_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: getDuration("DATABASE_CONN_MAX_LIFETIME", 5*time.Minute),
		},
		Redis: RedisConfig{
			Addr:     getEnv("REDIS_ADDR", "localhost:6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getInt("REDIS_DB", 0),
		},
		Webhook: WebhookConfig{
			URL:     getEnv("WEBHOOK_URL", "https://webhook.site/your-uuid-here"),
			Timeout: getDuration("WEBHOOK_TIMEOUT", 10*time.Second),
		},
		Worker: WorkerConfig{
			WorkersPerChannel: getInt("WORKERS_PER_CHANNEL", 5),
			RetryBaseDelay:    getDuration("RETRY_BASE_DELAY", time.Second),
			MaxRetries:        getInt("MAX_RETRIES", 3),
		},
	}
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func getInt(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return defaultVal
}

func getDuration(key string, defaultVal time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return defaultVal
}
