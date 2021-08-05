package service

import (
	"github.com/Unleash/unleash-client-go/v3"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/nais/babylon/pkg/config"
	"github.com/nais/babylon/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Service struct {
	Config        *config.Config
	Client        client.Client
	Metrics       *metrics.Metrics
	UnleashClient *unleash.Client
	InfluxClient  influxdb2.Client
	History       *metrics.History
}
