package service

import (
	"context"
	"sort"

	"github.com/nais/babylon/pkg/config"
	"github.com/nais/babylon/pkg/metrics"
	nais_io_v1 "github.com/nais/liberator/pkg/apis/nais.io/v1"
	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Service struct {
	Config  *config.Config
	Client  client.Client
	Metrics *metrics.Metrics
}

const defaultChannel = "#babylon-alerts"

func (s *Service) SlackChannel(ctx context.Context, ns string) string {
	if !s.Config.AlertChannels {
		return defaultChannel
	}

	namespace := &v1.Namespace{}
	key := client.ObjectKey{Name: ns}
	err := s.Client.Get(ctx, key, namespace)
	if err != nil {
		log.Errorf("Failed to get namespace %v, got error %v", ns, err)

		return defaultChannel
	}

	ch, ok := namespace.Annotations["platform-alerts-channel"]
	if ok {
		return ch
	}

	ch, ok = s.existingAlertChannel(ctx, ns)
	if ok {
		return ch
	}

	ch, ok = namespace.Annotations["slack-channel"]
	if ok {
		return ch
	}

	log.Warnf("Namespace %s does not have a slack-channel-annotation", ns)

	return defaultChannel
}

// Get an existing alert channel in use by looking at NAIS alerts.
func (s *Service) existingAlertChannel(ctx context.Context, ns string) (string, bool) {
	alerts := &nais_io_v1.AlertList{}
	err := s.Client.List(ctx, alerts, &client.ListOptions{Namespace: ns})
	if err != nil {
		log.Errorf("Failed to list alerts in namespace %s, got error %v", ns, err)

		return "", false
	}
	// Sort alerts to avoid random channel picks
	sort.Slice(alerts.Items, func(i, j int) bool {
		return alerts.Items[i].Spec.Receivers.Slack.Channel < alerts.Items[j].Spec.Receivers.Slack.Channel
	})
	for _, alert := range alerts.Items {
		ch := alert.Spec.Receivers.Slack.Channel
		if ch == "" {
			continue
		}

		return ch, true
	}

	return "", false
}
