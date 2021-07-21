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
	DeploymentRollbacks *prometheus.CounterVec
	RuleActivations     *prometheus.CounterVec
}

func Init() Metrics {
	return Metrics{
		DeploymentRollbacks: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "babylon_deployment_rollbacks_total",
			Help: "Deployments rolled back",
		}, []string{"deployment", "affected_team", "dryrun", "slack_channel", "previousDockerHash", "currentDockerHash"}),
		RuleActivations: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "babylon_rule_activations_total",
			Help: "Rules triggered",
		}, []string{"deployment", "affected_team", "reason"}),
	}
}

func (m *Metrics) IncDeploymentRollbacks(
	deployment *appsv1.Deployment,
	armed bool,
	channel string,
	currentRs *appsv1.ReplicaSet) {
	team, ok := deployment.Labels["team"]
	if !ok {
		team = Unknown
	}

	pContainers := deployment.Spec.Template.Spec.Containers
	cContainers := currentRs.Spec.Template.Spec.Containers

	previousDockerHash := ""
	currentDockerHash := ""
	if len(pContainers) > 0 {
		previousDockerHash = deployment.Spec.Template.Spec.Containers[0].Image
	}
	if len(cContainers) > 0 {
		currentDockerHash = currentRs.Spec.Template.Spec.Containers[0].Image
	}

	log.Debugf("Deployment %v", deployment)
	log.Debugf("ReplicaSet %v", currentRs)

	metric, err := m.DeploymentRollbacks.GetMetricWithLabelValues(
		deployment.Name, team, strconv.FormatBool(!armed), channel, previousDockerHash, currentDockerHash)
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
	deployment, ok := rs.Spec.Selector.MatchLabels["app"]

	if !ok {
		deployment = Unknown
	}

	metric, err := m.RuleActivations.GetMetricWithLabelValues(deployment, team, reason)
	if err != nil {
		log.Errorf("Metric failed: %+v", err)

		return
	}
	metric.Inc()
}
