package config

import (
	"os"
	"time"
)

const DefaultTickRate = 5 * time.Second

type Config struct {
	Armed    bool
	LogLevel string
	Port     string
	TickRate time.Duration
}

func DefaultConfig() Config {
	return Config{
		LogLevel: "info",
		Port:     "8080",
		Armed:    false,
		TickRate: DefaultTickRate,
	}
}

func GetEnv(name, fallback string) string {
	if value, ok := os.LookupEnv(name); ok {
		return value
	}

	return fallback
}
