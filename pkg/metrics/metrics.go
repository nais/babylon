package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Metrics struct {
	PodsDeleted           prometheus.Counter
	DeploymentsDownscaled *prometheus.CounterVec
}

func Init() Metrics {
	return Metrics{
		PodsDeleted: promauto.NewCounter(prometheus.CounterOpts{
			Name: "babylon_pods_deleted_total",
			Help: "Number of pods deleted in total",
		}),
		DeploymentsDownscaled: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "babylon_deployments_downscaled_total",
			Help: "Deployments downscaled",
		}, []string{"deployment", "team"}),
	}
}
