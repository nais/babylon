package metrics

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	log "github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
)

const Unknown = "unknown"

type Metrics struct {
	DeploymentRollbacks *prometheus.CounterVec
	RuleActivations     *prometheus.CounterVec
	TeamNotifications   *prometheus.CounterVec
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
		TeamNotifications: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "babylon_team_notifications_total",
			Help: "Notifiactions sent to team regarding failing deployments",
		}, []string{"deployment", "affected_team", "slack_channel", "grace_cutoff"}),
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

	previousDockerHash := ""
	currentDockerHash := ""

	if len(deployment.Spec.Template.Spec.Containers) > 0 {
		previousDockerHash = deployment.Spec.Template.Spec.Containers[0].Image
	}
	if currentRs != nil && len(currentRs.Spec.Template.Spec.Containers) > 0 {
		currentDockerHash = currentRs.Spec.Template.Spec.Containers[0].Image
	}

	metric, err := m.DeploymentRollbacks.GetMetricWithLabelValues(
		deployment.Name, team, strconv.FormatBool(!armed), channel, previousDockerHash, currentDockerHash)
	if err != nil {
		log.Errorf("Metric failed: %+v", err)

		return
	}
	log.Debugf("Team %s notified about rollback", team)

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
	log.Debugf("Team %s notified about rollback", team)

	metric.Inc()
}

func (m *Metrics) IncTeamNotification(deployment *appsv1.Deployment, channel string, graceCutoff time.Time) {
	team, ok := deployment.Labels["team"]

	if !ok {
		team = Unknown
	}
	metric, err := m.TeamNotifications.GetMetricWithLabelValues(
		deployment.Name, team, channel, graceCutoff.Format("2006-01-02 15:04:05 -0700 MST"))
	if err != nil {
		log.Errorf("Metric failed: %+v", err)

		return
	}
	log.Debugf("Team %s notified about failing deployment", team)

	metric.Inc()
}
