package service

import (
	"time"

	"github.com/nais/babylon/pkg/config"
	"github.com/nais/babylon/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Service struct {
	Config  *config.Config
	Client  client.Client
	Metrics *metrics.Metrics
	// Stores the last time a deployment was pruned, primarily to avoid overly processing the same resource repeatedly
	PruneHistory map[string]time.Time
}
