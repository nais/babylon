package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Metrics struct {
	PodsDeleted         prometheus.Counter
	DeploymentRollbacks *prometheus.CounterVec
}

func Init() Metrics {
	return Metrics{
		PodsDeleted: promauto.NewCounter(prometheus.CounterOpts{
			Name: "babylon_pods_deleted_total",
			Help: "Number of pods deleted in total",
		}),
		DeploymentRollbacks: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "babylon_deployment_rollbacks_total",
			Help: "Deployments rolled back",
		}, []string{"deployment", "affected_team"}),
	}
}
