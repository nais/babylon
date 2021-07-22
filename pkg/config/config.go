package config

import (
	"os"
	"strings"
	"time"
)

const (
	DefaultTickRate            = 15 * time.Minute
	DefaultRestartThreshold    = 200
	DefaultAge                 = 10 * time.Minute
	DefaultNotificationTimeout = 24 * time.Hour
	NotificationAnnotation     = "nais.babylon/last_notified"
)

type Config struct {
	Armed                bool
	AlertChannels        bool
	LogLevel             string
	Port                 string
	TickRate             time.Duration
	RestartThreshold     int32
	ResourceAge          time.Duration
	NotificationTimeout  time.Duration
	UseAllowedNamespaces bool
	AllowedNamespaces    []string
}

func DefaultConfig() Config {
	return Config{
		LogLevel:             "info",
		Port:                 "8080",
		Armed:                false,
		AlertChannels:        false,
		TickRate:             DefaultTickRate,
		RestartThreshold:     DefaultRestartThreshold,
		ResourceAge:          DefaultAge,
		NotificationTimeout:  DefaultNotificationTimeout,
		UseAllowedNamespaces: false,
		AllowedNamespaces:    []string{},
	}
}

func GetEnv(name, fallback string) string {
	if value, ok := os.LookupEnv(name); ok {
		return value
	}

	return fallback
}

func (c *Config) IsNamespaceAllowed(namespace string) bool {
	if !c.UseAllowedNamespaces {
		return true
	}

	for i := range c.AllowedNamespaces {
		if strings.Contains(namespace, c.AllowedNamespaces[i]) || strings.Contains(c.AllowedNamespaces[i], namespace) {
			return true
		}
	}

	return false
}
