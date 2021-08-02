package metrics

import (
	"context"
	"strconv"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/nais/babylon/pkg/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	log "github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
)

const Unknown = "unknown"

type Metrics struct {
	DeploymentRollback  *prometheus.CounterVec
	DeploymentDownscale *prometheus.CounterVec
	RuleActivations     *prometheus.CounterVec
	TeamNotifications   *prometheus.CounterVec
	InfluxdbDatabase    string
}

func Init(database string) Metrics {
	return Metrics{
		DeploymentRollback: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "babylon_deployment_rollback_total",
			Help: "Deployments rolled back",
		}, []string{
			"cluster", "deployment", "namespace", "affected_team", "dry_run",
			"slack_channel", "previous_docker_hash", "current_docker_hash",
		}),
		DeploymentDownscale: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "babylon_deployment_downscale_total",
			Help: "Deployments downscaled",
		}, []string{"cluster", "deployment", "namespace", "affected_team", "dry_run", "slack_channel", "resource_age"}),
		RuleActivations: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "babylon_rule_activations_total",
			Help: "Rules triggered",
		}, []string{"cluster", "deployment", "namespace", "affected_team", "reason"}),
		TeamNotifications: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "babylon_team_notifications_total",
			Help: "Notifications sent to team regarding failing deployments",
		}, []string{"cluster", "deployment", "namespace", "affected_team", "slack_channel", "grace_cutoff"}),
		InfluxdbDatabase: database,
	}
}

func (m *Metrics) IncDownscaledDeployments(
	deployment *appsv1.Deployment,
	armed bool,
	channel string,
	resourceAge string) {
	team, ok := deployment.Labels["team"]
	if !ok {
		team = Unknown
	}

	cluster := config.GetEnv("CLUSTER", "unknown")
	metric, err := m.DeploymentDownscale.GetMetricWith(prometheus.Labels{
		"cluster": cluster, "deployment": deployment.Name, "namespace": deployment.Namespace, "affected_team": team,
		"dry_run": strconv.FormatBool(!armed), "slack_channel": channel, "resource_age": resourceAge,
	})
	if err != nil {
		log.Errorf("Metric failed: %+v", err)

		return
	}
	log.Debugf("Team %s notified in %s about downscaling", team, channel)

	metric.Inc()
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

	cluster := config.GetEnv("CLUSTER", "unknown")
	metric, err := m.DeploymentRollback.GetMetricWith(prometheus.Labels{
		"cluster": cluster, "deployment": deployment.Name, "namespace": deployment.Namespace,
		"affected_team": team, "dry_run": strconv.FormatBool(!armed), "slack_channel": channel,
		"previous_docker_hash": previousDockerHash, "current_docker_hash": currentDockerHash,
	})
	if err != nil {
		log.Errorf("Metric failed: %+v", err)

		return
	}
	log.Debugf("Team %s notified in %s about rollback", team, channel)

	metric.Inc()
}

func (m *Metrics) IncRuleActivations(
	influxC influxdb2.Client,
	rs *appsv1.ReplicaSet,
	reason string) {
	team, ok := rs.Labels["team"]

	if !ok {
		team = Unknown
	}
	deployment, ok := rs.Spec.Selector.MatchLabels["app"]

	if !ok {
		deployment = Unknown
	}

	cluster := config.GetEnv("CLUSTER", "unknown")
	metric, err := m.RuleActivations.GetMetricWith(prometheus.Labels{
		"cluster": cluster, "deployment": deployment, "namespace": rs.Namespace, "affected_team": team, "reason": reason,
	})
	if err != nil {
		log.Errorf("Metric failed: %+v", err)

		return
	}
	log.Debugf("RuleActivationsMetric incremented by team: %s", team)

	metric.Inc()

	writeAPI := influxC.WriteAPIBlocking("", m.InfluxdbDatabase+"/autogen")
	p := influxdb2.NewPoint("rule-activation",
		map[string]string{},
		map[string]interface{}{"deployment": deployment, "team": team, "reason": reason},
		time.Now())

	err = writeAPI.WritePoint(context.Background(), p)
	if err != nil {
		log.Errorf("InfluxClient write error: %v", err)
	}

	queryAPI := influxC.QueryAPI("")
	// get QueryTableResult
	result, err := queryAPI.Query(context.Background(), `SELECT * FROM foo`)
	log.Infof("Result: %v", result)
	if err == nil {
		log.Infof("Result record value: %v", result.Record().Value())
	} else {
		log.Error(err)
	}
}

func (m *Metrics) IncTeamNotification(deployment *appsv1.Deployment, channel string, graceCutoff time.Time) {
	team, ok := deployment.Labels["team"]

	if !ok {
		team = Unknown
	}

	cluster := config.GetEnv("CLUSTER", "unknown")
	metric, err := m.TeamNotifications.GetMetricWith(prometheus.Labels{
		"cluster": cluster, "deployment": deployment.Name, "namespace": deployment.Namespace,
		"affected_team": team, "slack_channel": channel, "grace_cutoff": graceCutoff.Format("2006-01-02 15:04:05 -0700 MST"),
	})
	if err != nil {
		log.Errorf("Metric failed: %+v", err)

		return
	}
	log.Debugf("Team %s notified in %s about failing deployment", team, channel)

	metric.Inc()
}
