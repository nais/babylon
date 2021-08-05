package metrics

import (
	"context"
	"fmt"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	log "github.com/sirupsen/logrus"
)

type History struct {
	influxClient     influxdb2.Client
	influxdbDatabase string
	cluster          string
}

func NewHistory(influxClient influxdb2.Client, influxdbDatabase, cluster string) *History {
	return &History{influxClient: influxClient, influxdbDatabase: influxdbDatabase, cluster: cluster}
}

func (h *History) historize(measurement string, tags map[string]string, fields map[string]interface{}) {
	writeAPI := h.influxClient.WriteAPIBlocking("", h.influxdbDatabase+"/autogen")
	p := influxdb2.NewPoint(measurement, tags, fields, time.Now())

	err := writeAPI.WritePoint(context.Background(), p)
	if err != nil {
		log.Errorf("InfluxClient write error: %v", err)
	}
}

func (h *History) HistorizeDeploymentFailing(reason, team, slackChannel, name string) {
	go h.historize(
		"deployment_failing",
		map[string]string{
			"reason": reason, "team": team, "name": name, "cluster": h.cluster,
		},
		map[string]interface{}{
			"slack_channel": slackChannel,
		})
}

func (h *History) HistorizeDeploymentKilled(method, team, slackChannel, name string, armed bool) {
	go h.historize(
		"deployment_killed",
		map[string]string{
			"method": method, "team": team, "name": name, "dry_run": fmt.Sprint(!armed), "cluster": h.cluster,
		},
		map[string]interface{}{
			"slack_channel": slackChannel,
		},
	)
}
