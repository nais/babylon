package service

import (
	"github.com/nais/babylon/pkg/config"
	"github.com/nais/babylon/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Service struct {
	Config  *config.Config
	Client  client.Client
	Metrics *metrics.Metrics
}
