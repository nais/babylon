package metrics

import (
	"context"
	"strconv"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	log "github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
)

const Unknown = "unknown"

type DeploymentStatus float64

// OK Deployment is detected as ok.
const OK DeploymentStatus = 0

// FAILING Deployment is detected as failing and currently in grace period.
const FAILING DeploymentStatus = 1

// CLEANUP Deployment is in the process of rolling back or downscaling.
const CLEANUP DeploymentStatus = 2

const (
	RollbackLabel  = "rollback"
	DownscaleLabel = "downscale"
)

type Metrics struct {
	DeploymentCleanup     *prometheus.CounterVec
	RuleActivations       *prometheus.CounterVec
	TeamNotifications     *prometheus.CounterVec
	DeploymentStatus      *prometheus.GaugeVec
	DeploymentUpdated     *prometheus.GaugeVec
	DeploymentGraceCutoff *prometheus.GaugeVec
	SlackChannelMapping   *prometheus.GaugeVec
	InfluxdbDatabase      string
}

func Init(database string) Metrics {
	return Metrics{
		DeploymentCleanup: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "babylon_deployment_cleanup_total",
			Help: "Deployments cleaned up (downscaled or rolled back)",
		}, []string{"deployment", "namespace", "affected_team", "dry_run", "reason", "slack_channel"}),
		RuleActivations: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "babylon_rule_activations_total",
			Help: "Rules triggered",
		}, []string{"deployment", "namespace", "affected_team", "reason"}),
		DeploymentStatus: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "babylon_deployment_status",
			Help: "Deployment status marked",
		}, []string{"deployment", "namespace", "affected_team"}),
		DeploymentUpdated: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "babylon_deployment_last_updated",
			Help: "When babylon last observed the deployment and updated it's status",
		}, []string{"deployment", "namespace", "affected_team"}),
		DeploymentGraceCutoff: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "babylon_deployment_grace_cutoff",
			Help: "When babylon will become potentially volatile against the deployment, otherwise 0",
		}, []string{"deployment", "namespace", "affected_team"}),
		SlackChannelMapping: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "babylon_slack_channel",
			Help: "Latest observed slack channel by team",
		}, []string{"deployment", "namespace", "affected_team", "slack_channel"}),
		InfluxdbDatabase: database,
	}
}

func (m Metrics) SetGraceCutoff(deployment *appsv1.Deployment, graceCutoff time.Time) {
	team, ok := deployment.Labels["team"]

	if !ok {
		team = Unknown
	}

	m.DeploymentGraceCutoff.With(prometheus.Labels{
		"deployment": deployment.Name, "namespace": deployment.Namespace,
		"affected_team": team,
	}).Set(float64(graceCutoff.Unix()))
}

func (m Metrics) SetDeploymentStatus(deployment *appsv1.Deployment, channel string, status DeploymentStatus) {
	team, ok := deployment.Labels["team"]

	if !ok {
		team = Unknown
	}

	if status == OK {
		// if status != OK, graceCutoff is either already set, og will be set during flagging
		m.SetGraceCutoff(deployment, time.Unix(0, 0))
	}

	m.SlackChannelMapping.With(prometheus.Labels{
		"deployment": deployment.Name, "namespace": deployment.Namespace,
		"affected_team": team, "slack_channel": channel,
	}).SetToCurrentTime()

	m.DeploymentUpdated.With(prometheus.Labels{
		"deployment": deployment.Name, "namespace": deployment.Namespace,
		"affected_team": team,
	}).SetToCurrentTime()

	m.DeploymentStatus.With(prometheus.Labels{
		"deployment": deployment.Name, "namespace": deployment.Namespace,
		"affected_team": team,
	}).Set(float64(status))
}

func (m *Metrics) IncDeploymentCleanup(
	deployment *appsv1.Deployment,
	armed bool,
	channel string,
	reason string) {
	team, ok := deployment.Labels["team"]
	if !ok {
		team = Unknown
	}

	m.DeploymentCleanup.With(prometheus.Labels{
		"deployment": deployment.Name, "namespace": deployment.Namespace,
		"affected_team": team, "dry_run": strconv.FormatBool(!armed), "reason": reason,
		"slack_channel": channel,
	}).Inc()

	log.Debugf("Team %s notified in %s about rollback", team, channel)
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

	m.RuleActivations.With(prometheus.Labels{
		"deployment": deployment, "namespace": rs.Namespace, "affected_team": team, "reason": reason,
	}).Inc()
	log.Debugf("RuleActivationsMetric incremented by team: %s", team)

	writeAPI := influxC.WriteAPIBlocking("", m.InfluxdbDatabase+"/autogen")
	p := influxdb2.NewPoint("rule-activation",
		map[string]string{},
		map[string]interface{}{"deployment": deployment, "team": team, "reason": reason},
		time.Now())

	err := writeAPI.WritePoint(context.Background(), p)
	if err != nil {
		log.Errorf("InfluxClient write error: %v", err)
	}
}
