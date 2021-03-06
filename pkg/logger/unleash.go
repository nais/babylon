package logger

import (
	"github.com/Unleash/unleash-client-go/v3"
	log "github.com/sirupsen/logrus"
)

type UnleashListener struct{}

func (l UnleashListener) OnCount(name string, enabled bool) {
	log.Tracef("Unleash toggle '%s': %t", name, enabled)
}

func (l UnleashListener) OnError(err error) {
	log.Errorf("ERROR: %s", err.Error())
}

func (l UnleashListener) OnReady() {
	log.Info("Unleash client ready")
}

func (l UnleashListener) OnRegistered(payload unleash.ClientData) {
	log.Infof("Unleash client registered: %+v", payload)
}

func (l UnleashListener) OnSent(payload unleash.MetricsData) {
	log.Infof("Sent: %+v", payload)
}

func (l UnleashListener) OnWarning(warning error) {
	log.Warnf("WARNING: %s", warning.Error())
}
