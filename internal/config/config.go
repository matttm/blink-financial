package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Port         string
	RedisAddr    string
	RedisListKey string
	InstanceID   string
}

func Load() (Config, error) {
	cfg := Config{
		Port:         envOrDefault("PORT", "8080"),
		RedisAddr:    envOrDefault("REDIS_ADDR", "redis:6379"),
		RedisListKey: envOrDefault("REDIS_LIST_KEY", "blink:transactions"),
		InstanceID:   envOrDefault("HOSTNAME", "local-app"),
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.Port) == "" {
		return fmt.Errorf("PORT must not be empty")
	}
	if strings.TrimSpace(c.RedisAddr) == "" {
		return fmt.Errorf("REDIS_ADDR must not be empty")
	}
	if strings.TrimSpace(c.RedisListKey) == "" {
		return fmt.Errorf("REDIS_LIST_KEY must not be empty")
	}
	if strings.TrimSpace(c.InstanceID) == "" {
		return fmt.Errorf("HOSTNAME must not be empty")
	}

	return nil
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
