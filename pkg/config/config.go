package config

import (
	"os"
	"time"

	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func (c *Config) DeleteOptions() metav1.DeleteOptions {
	if c.Armed {
		log.Info("Armed and dangerous! 🪖")

		return metav1.DeleteOptions{}
	}
	log.Info("Running in dry run-mode, nothing will be deleted.")

	return metav1.DeleteOptions{DryRun: []string{metav1.DryRunAll}}
}
