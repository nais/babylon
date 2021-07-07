package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Metrics struct {
	PodsDeleted        prometheus.Counter
	DeploymentsDeleted prometheus.Counter
}

func Init() Metrics {
	return Metrics{
		PodsDeleted: promauto.NewCounter(prometheus.CounterOpts{
			Name: "babylon_pods_deleted_total",
			Help: "Number of pods deleted in total",
		}),
		DeploymentsDeleted: promauto.NewCounter(prometheus.CounterOpts{
			Name: "babylon_deployments_deleted_total",
			Help: "Number of deployments deleted in total",
		}),
	}
}
