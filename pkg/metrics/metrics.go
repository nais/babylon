package metrics

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	log "github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
)

const Unknown = "unknown"

type Metrics struct {
	PodsDeleted         prometheus.Counter
	DeploymentRollbacks *prometheus.CounterVec
	RuleActivations     *prometheus.CounterVec
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
		}, []string{"deployment", "affected_team", "dryrun"}),
		RuleActivations: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "babylon_rule_activations_total",
			Help: "Rules triggered",
		}, []string{"deployment", "affected_team", "reason"}),
	}
}

func (m *Metrics) IncDeploymentRollbacks(deployment *appsv1.Deployment, armed bool) {
	team, ok := deployment.Labels["team"]

	if !ok {
		team = Unknown
	}

	metric, err := m.DeploymentRollbacks.GetMetricWithLabelValues(
		deployment.Name, team, strconv.FormatBool(!armed))
	if err != nil {
		log.Errorf("Metric failed: %+v", err)

		return
	}
	metric.Inc()
}

func (m *Metrics) IncRuleActivations(rs *appsv1.ReplicaSet, reason string) {
	team, ok := rs.Labels["team"]

	if !ok {
		team = Unknown
	}
	app, ok := rs.Spec.Selector.MatchLabels["app"]

	if !ok {
		app = Unknown
	}

	metric, err := m.RuleActivations.GetMetricWithLabelValues(
		app, team, reason)
	if err != nil {
		log.Errorf("Metric failed: %+v", err)

		return
	}
	metric.Inc()
}
