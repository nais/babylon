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
	InfluxdbDatabase string
}

func NewHistory(influxClient influxdb2.Client, influxdbDatabase string) *History {
	return &History{influxClient: influxClient, InfluxdbDatabase: influxdbDatabase}
}

func (h *History) historize(measurement string, data map[string]string) {
	writeAPI := h.influxClient.WriteAPIBlocking("", h.InfluxdbDatabase+"/autogen")
	p := influxdb2.NewPoint(measurement, data, map[string]interface{}{}, time.Now())

	err := writeAPI.WritePoint(context.Background(), p)
	if err != nil {
		log.Errorf("InfluxClient write error: %v", err)
	}
}

func (h *History) HistorizeDeploymentFailing(reason, team, slackChannel, name string) {
	go h.historize("deployment_failing", map[string]string{
		"reason": reason, "team": team, "slack_channel": slackChannel, "name": name,
	})
}

func (h *History) HistorizeDeploymentKilled(method, team, slackChannel, name string, armed bool) {
	go h.historize("deployment_killed", map[string]string{
		"method": method, "team": team, "slack_channel": slackChannel, "name": name, "dry_run": fmt.Sprint(!armed),
	})
}
