package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Unleash/unleash-client-go/v3"
	"github.com/nais/babylon/pkg/logger"
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

func (c *Config) ConfigureUnleash() error {
	unleashClient, err := unleash.NewClient(
		unleash.WithListener(logger.UnleashListener{}),
		unleash.WithAppName("babylon"),
		unleash.WithUrl("https://unleash.nais.io/api/"),
	)
	if err != nil {
		return fmt.Errorf("failed to create unleash client: %w", err)
	}

	unleashClient.WaitForReady()

	if unleashClient != nil {
		c.AlertChannels = unleashClient.IsEnabled("babylon_alerts")
	}

	return nil
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
