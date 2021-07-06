package config

import "os"

type Config struct {
	DryRun   bool
	LogLevel string
	Port     string
}

func DefaultConfig() Config {
	return Config{
		LogLevel: "info",
		Port:     "8080",
		DryRun:   false,
	}
}

func GetEnv(name, fallback string) string {
	if value, ok := os.LookupEnv(name); ok {
		return value
	}

	return fallback
}
