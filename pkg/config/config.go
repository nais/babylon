package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

const (
	DefaultTickRate         = 5 * time.Second
	DefaultRestartThreshold = 200
)

type Config struct {
	Armed            bool
	LogLevel         string
	Port             string
	TickRate         time.Duration
	RestartThreshold string
}

func DefaultConfig() Config {
	return Config{
		LogLevel:         "info",
		Port:             "8080",
		Armed:            false,
		TickRate:         DefaultTickRate,
		RestartThreshold: fmt.Sprintf("%d", DefaultRestartThreshold),
	}
}

func (c *Config) GetRestartThreshold() int32 {
	num, err := strconv.ParseInt(c.RestartThreshold, 10, 32)
	if err != nil {
		return DefaultRestartThreshold
	}

	return int32(num)
}

func GetEnv(name, fallback string) string {
	if value, ok := os.LookupEnv(name); ok {
		return value
	}

	return fallback
}
