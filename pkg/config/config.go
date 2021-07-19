package config

import (
	"os"
	"time"
)

const (
	DefaultTickRate         = 15 * time.Minute
	DefaultRestartThreshold = 200
	DefaultAge              = 10 * time.Minute
)

type Config struct {
	Armed            bool
	LogLevel         string
	Port             string
	TickRate         time.Duration
	RestartThreshold int32
	ResourceAge      time.Duration
}

func DefaultConfig() Config {
	return Config{
		LogLevel:         "info",
		Port:             "8080",
		Armed:            false,
		TickRate:         DefaultTickRate,
		RestartThreshold: DefaultRestartThreshold,
		ResourceAge:      DefaultAge,
	}
}

func GetEnv(name, fallback string) string {
	if value, ok := os.LookupEnv(name); ok {
		return value
	}

	return fallback
}
