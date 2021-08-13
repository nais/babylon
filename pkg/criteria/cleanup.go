package criteria

import (
	"strings"
	"time"

	"github.com/nais/babylon/pkg/config"
	log "github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
)

type CleanUpJudge struct {
	deniedNamespaces  []string
	gracePeriod       time.Duration
	notificationDelay time.Duration
}

func NewCleanUpJudge(config *config.Config) *CleanUpJudge {
	return &CleanUpJudge{
		deniedNamespaces:  config.DeniedNamespaces,
		gracePeriod:       config.GracePeriod,
		notificationDelay: config.NotificationDelay,
	}
}

func (j *CleanUpJudge) Judge(deployments []*appsv1.Deployment) []*appsv1.Deployment {
	var filteredDeployments []*appsv1.Deployment
	for i := range deployments {
		if j.filterByDeniedNamespace(deployments[i]) &&
			j.filterByNotified(deployments[i]) {
			filteredDeployments = append(filteredDeployments, deployments[i])
		}
	}

	return filteredDeployments
}

func (j *CleanUpJudge) filterByDeniedNamespace(deployment *appsv1.Deployment) bool {
	namespace := deployment.Namespace
	for i := range j.deniedNamespaces {
		if j.deniedNamespaces[i] == "" {
			continue
		}
		if strings.Contains(namespace, j.deniedNamespaces[i]) || strings.Contains(j.deniedNamespaces[i], namespace) {
			log.Tracef("namespace %s denied", namespace)

			return false
		}
	}
	log.Tracef("namespace %s allowed", namespace)

	return true
}

func (j *CleanUpJudge) filterByNotified(deployment *appsv1.Deployment) bool {
	if failTime, ok := deployment.Annotations[config.FailureDetectedAnnotation]; ok {
		firstDetectedAsFailing, err := time.Parse(time.RFC3339, failTime)
		switch {
		case err != nil:
			log.Warnf("Could not parse %s for %s: %v", config.FailureDetectedAnnotation, deployment.Name, err)

			return false
		case time.Since(firstDetectedAsFailing) < j.graceDuration(deployment)+j.notificationDelay:
			log.Infof(
				"not yet ready to prune deployment %s, too early since last notification: %s",
				deployment.Name, firstDetectedAsFailing.String())

			return false
		}

		return true
	}

	return false
}

func (j *CleanUpJudge) graceDuration(deployment *appsv1.Deployment) time.Duration {
	gracePeriod, err := time.ParseDuration(deployment.Annotations[config.GracePeriodAnnotation])
	if err != nil {
		log.Infof("Failed to parse duration for deployment %s: %s",
			deployment.Name, deployment.Annotations[config.GracePeriodAnnotation])

		return j.gracePeriod
	}

	return gracePeriod
}
