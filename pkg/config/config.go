package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Unleash/unleash-client-go/v3"
	"github.com/nais/babylon/pkg/logger"
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/timeinterval"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

const (
	DefaultTickRate           = 15 * time.Minute
	DefaultRestartThreshold   = 200
	DefaultAge                = 10 * time.Minute
	DefaultNotificationDelay  = 24 * time.Hour
	DefaultGracePeriod        = 24 * time.Hour
	StringTrue                = "true"
	FailureDetectedAnnotation = "babylon.nais.io/failure-detected"
	GracePeriodAnnotation     = "babylon.nais.io/grace-period"
	StrategyAnnotation        = "babylon.nais.io/strategy"
	EnabledAnnotation         = "babylon.nais.io/enabled"
)

type Config struct {
	Armed                bool
	LogLevel             string
	Port                 string
	TickRate             time.Duration
	RestartThreshold     int32
	ResourceAge          time.Duration
	NotificationDelay    time.Duration
	UseAllowedNamespaces bool
	AllowedNamespaces    []string
	GracePeriod          time.Duration
	ActiveTimeIntervals  map[string][]timeinterval.TimeInterval
	InfluxdbURI          string
	InfluxdbUsername     SecretToken
	InfluxdbPassword     SecretToken
	InfluxdbDatabase     string
	Cluster              string
}

type SecretToken string

func (ssu SecretToken) String() string {
	return "[REDACTED]" // TODO: Make more sophisticated(sensor credentials)
}

func (ssu SecretToken) SecretString() string {
	return string(ssu)
}

func DefaultConfig() Config {
	return Config{
		LogLevel:             "info",
		Port:                 "8080",
		Armed:                false,
		TickRate:             DefaultTickRate,
		RestartThreshold:     DefaultRestartThreshold,
		ResourceAge:          DefaultAge,
		NotificationDelay:    DefaultNotificationDelay,
		UseAllowedNamespaces: false,
		AllowedNamespaces:    []string{},
		GracePeriod:          DefaultGracePeriod,
		ActiveTimeIntervals: map[string][]timeinterval.TimeInterval{
			"defaultAlways": {
				{Times: []timeinterval.TimeRange{{StartMinute: 0, EndMinute: 1440}}},
			},
		},
		Cluster: "unknown",
	}
}

//nolint:funlen
func ParseConfig() Config {
	cfg := DefaultConfig()
	// Whether to start destruction
	cfg.Armed = GetEnv("ARMED", fmt.Sprintf("%v", cfg.Armed)) == StringTrue

	cfg.LogLevel = GetEnv("LOG_LEVEL", cfg.LogLevel)
	cfg.Port = GetEnv("PORT", cfg.Port)

	tickRate := GetEnv("TICKRATE", cfg.TickRate.String())
	restartThreshold := GetEnv("RESTART_THRESHOLD", fmt.Sprintf("%d", cfg.RestartThreshold))

	// Resource age needed before rollback
	resourceAge := GetEnv("RESOURCE_AGE", "1h")

	// Timeout between notifying teams
	notificationTimeout := GetEnv("NOTIFICATION_DELAY", fmt.Sprintf("%d", cfg.NotificationDelay))

	gracePeriod := GetEnv("GRACE_PERIOD", fmt.Sprintf("%d", cfg.GracePeriod))

	cfg.UseAllowedNamespaces = GetEnv("USE_ALLOWED_NAMESPACES",
		fmt.Sprintf("%t", cfg.UseAllowedNamespaces)) == StringTrue

	namespacesFromEnv := GetEnv("ALLOWED_NAMESPACES", "")
	cfg.AllowedNamespaces = strings.Split(namespacesFromEnv, ",")

	influxdbServiceURI := GetEnv("AIVEN_INFLUXDB_SERVICE_URI", "")
	cfg.InfluxdbURI = influxdbServiceURI

	influxdbPassword := GetEnv("AIVEN_INFLUXDB_PASSWORD", "")
	cfg.InfluxdbPassword = SecretToken(influxdbPassword)

	influxdbUsername := GetEnv("AIVEN_INFLUXDB_USERNAME", "")
	cfg.InfluxdbUsername = SecretToken(influxdbUsername)

	influxdbDatabase := GetEnv("AIVEN_INFLUXDB_DATABASE", "")
	cfg.InfluxdbDatabase = influxdbDatabase

	cfg.Cluster = GetEnv("CLUSTER", cfg.Cluster)

	duration, err := time.ParseDuration(tickRate)
	if err == nil {
		cfg.TickRate = duration
	}
	age, err := time.ParseDuration(resourceAge)
	if err == nil {
		cfg.ResourceAge = age
	}
	nt, err := time.ParseDuration(notificationTimeout)
	if err == nil {
		cfg.NotificationDelay = nt
	}
	gp, err := time.ParseDuration(gracePeriod)
	if err == nil {
		cfg.GracePeriod = gp
	}

	rt, err := strconv.ParseInt(restartThreshold, 10, 32)
	if err == nil {
		cfg.RestartThreshold = int32(rt)
	}

	var intervals []config.MuteTimeInterval
	file, err := os.ReadFile("/etc/config/working-hours.yaml")
	if err != nil {
		log.Infof("error reading working hours: %v", err)

		return cfg
	}

	err = yaml.Unmarshal(file, &intervals)
	if err != nil {
		log.Infof("error parsing working hours: %v", err)

		return cfg
	}

	cfg.ActiveTimeIntervals = map[string][]timeinterval.TimeInterval{}
	for _, mti := range intervals {
		cfg.ActiveTimeIntervals[mti.Name] = mti.TimeIntervals
	}
	log.Infof("working hours: %v", cfg.ActiveTimeIntervals)

	return cfg
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
