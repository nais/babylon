package service

import (
	"github.com/nais/babylon/pkg/config"
	"github.com/nais/babylon/pkg/metrics"
	"k8s.io/client-go/kubernetes"
)

type Service struct {
	Config  *config.Config
	Client  *kubernetes.Clientset
	Metrics *metrics.Metrics
}
