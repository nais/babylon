package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Unleash/unleash-client-go/v3"
	"github.com/nais/babylon/pkg/logger"
	log "github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
)

const (
	DefaultTickRate            = 15 * time.Minute
	DefaultRestartThreshold    = 200
	DefaultAge                 = 10 * time.Minute
	DefaultNotificationTimeout = 24 * time.Hour
	DefaultGracePeriod         = 24 * time.Hour
	NotificationAnnotation     = "babylon.nais.io/last-notified"
	GracePeriodAnnotation      = "babylon.nais.io/grace-period"
	RollbackAnnotation         = "babylon.nais.io/rollback"
	EnabledAnnotation          = "babylon.nais.io/enabled"
)

type Config struct {
	Armed                bool
	LogLevel             string
	Port                 string
	TickRate             time.Duration
	RestartThreshold     int32
	ResourceAge          time.Duration
	NotificationTimeout  time.Duration
	UseAllowedNamespaces bool
	AllowedNamespaces    []string
	GracePeriod          time.Duration
}

func DefaultConfig() Config {
	return Config{
		LogLevel:             "info",
		Port:                 "8080",
		Armed:                false,
		TickRate:             DefaultTickRate,
		RestartThreshold:     DefaultRestartThreshold,
		ResourceAge:          DefaultAge,
		NotificationTimeout:  DefaultNotificationTimeout,
		UseAllowedNamespaces: false,
		AllowedNamespaces:    []string{},
		GracePeriod:          DefaultGracePeriod,
	}
}

func ConfigureUnleash() (*unleash.Client, error) {
	val, ok := os.LookupEnv("UNLEASH_URL")
	if !ok {
		log.Info("No environment variable for Unleashed, skipped creating client")

		return nil, nil
	}

	unleashClient, err := unleash.NewClient(
		unleash.WithListener(logger.UnleashListener{}),
		unleash.WithAppName("babylon"),
		unleash.WithUrl(val),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create unleash client: %w", err)
	}

	unleashClient.WaitForReady()

	return unleashClient, nil
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

func (c *Config) GraceDuration(deployment *appsv1.Deployment) time.Duration {
	gracePeriod, err := time.ParseDuration(deployment.Labels[GracePeriodAnnotation])
	if err != nil {
		return c.GracePeriod
	}

	return gracePeriod
}

func (c *Config) GraceCutoff(deployment *appsv1.Deployment) time.Time {
	return time.Now().Add(-c.GraceDuration(deployment))
}
