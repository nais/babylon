package criteria

import (
	"strings"
	"time"

	"github.com/nais/babylon/pkg/config"
	log "github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
)

type CleanUpJudge struct {
	useAllowedNamespaces bool
	allowedNamespaces    []string
	gracePeriod          time.Duration
	notificationTimeout  time.Duration
}

func NewCleanUpJudge(config *config.Config) *CleanUpJudge {
	return &CleanUpJudge{
		useAllowedNamespaces: config.UseAllowedNamespaces,
		allowedNamespaces:    config.AllowedNamespaces,
		gracePeriod:          config.GracePeriod,
		notificationTimeout:  config.NotificationTimeout,
	}
}

func (j *CleanUpJudge) Judge(deployments []*appsv1.Deployment) []*appsv1.Deployment {
	var filteredDeployments []*appsv1.Deployment
	for i := range deployments {
		if j.filterByAllowedNamespace(deployments[i]) &&
			j.filterByNotified(deployments[i]) {
			filteredDeployments = append(filteredDeployments, deployments[i])
		}
	}

	return filteredDeployments
}

func (j *CleanUpJudge) filterByAllowedNamespace(deployment *appsv1.Deployment) bool {
	if !j.useAllowedNamespaces {
		return true
	}

	namespace := deployment.Namespace
	for i := range j.allowedNamespaces {
		if j.allowedNamespaces[i] == "" {
			continue
		}
		if strings.Contains(namespace, j.allowedNamespaces[i]) || strings.Contains(j.allowedNamespaces[i], namespace) {
			log.Tracef("namespace %s allowed", namespace)

			return true
		}
	}
	log.Tracef("namespace %s not allowed", namespace)

	return false
}

func (j *CleanUpJudge) filterByNotified(deployment *appsv1.Deployment) bool {
	if deployment.Annotations[config.NotificationAnnotation] != "" {
		lastNotified, err := time.Parse(time.RFC3339, deployment.Annotations[config.NotificationAnnotation])
		switch {
		case err != nil:
			log.Warnf("Could not parse %s for %s: %v", config.NotificationAnnotation, deployment.Name, err)

			return false
		case time.Since(lastNotified) < j.graceDuration(deployment):
			log.Infof(
				"not yet ready to prune deployment %s, too early since last notification: %s",
				deployment.Name, lastNotified.String())

			return false
		case time.Since(lastNotified) < j.notificationTimeout:
			log.Infof("Team already notified at %s, skipping deploy %s", lastNotified.String(), deployment.Name)

			return false
		}

		return true
	}

	return false
}

func (j *CleanUpJudge) graceDuration(deployment *appsv1.Deployment) time.Duration {
	gracePeriod, err := time.ParseDuration(deployment.Labels[config.GracePeriodLabel])
	if err != nil {
		log.Infof("Failed to parse duration for deployment %s: %s",
			deployment.Name, deployment.Labels[config.GracePeriodLabel])

		return j.gracePeriod
	}

	return gracePeriod
}
