package deployment

import (
	"context"

	"github.com/nais/babylon/pkg/logger"
	"github.com/nais/babylon/pkg/service"
	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/apps/v1"
	v1core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

const ImagePullBackOff = "ImagePullBackOff"

func GetFailingDeployments(ctx context.Context, s *service.Service, deployments *v1.DeploymentList) []*v1.Deployment {
	var fails []*v1.Deployment
	for i, deployment := range deployments.Items {
		labelSelector := labels.Set(deployment.Spec.Selector.MatchLabels)
		pods, err := s.Client.CoreV1().Pods("").List(ctx,
			metav1.ListOptions{LabelSelector: labelSelector.AsSelector().String()})
		if logger.Logk8sError(err) {
			continue
		}

		for j, pod := range pods.Items {
			log.Debugf("%s: %s (%s)", pod.Name, pod.Status.Reason, pod.Status.Message)
			if ShouldPodBeDeleted(&pods.Items[j]) {
				fails = append(fails, &deployments.Items[i])
			}
		}
	}

	return fails
}

func ShouldPodBeDeleted(pod *v1core.Pod) bool {
	for _, containerStatus := range pod.Status.ContainerStatuses {
		waiting := containerStatus.State.Waiting
		if waiting != nil && waiting.Reason == ImagePullBackOff {
			return true
		}
	}

	return false
}

func PruneFailingDeployment(ctx context.Context, s *service.Service, deployment *v1.Deployment) {
	err := s.Client.AppsV1().Deployments(deployment.Namespace).Delete(ctx, deployment.Name, s.Config.DeleteOptions())
	if err != nil {
		log.Errorf("Could not delete deployment %s, %v", deployment.Name, err)
	} else {
		log.Infof("Deleting deployment %s", deployment.Name)
		s.Metrics.DeploymentsDeleted.Inc()
		// s.Metrics.PodsDeleted.Add(float64(len(pods.Items)))
	}
}
