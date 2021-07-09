package deployment

import (
	"context"

	"github.com/nais/babylon/pkg/logger"
	"github.com/nais/babylon/pkg/service"
	log "github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const ImagePullBackOff = "ImagePullBackOff"

func GetFailingDeployments(
	ctx context.Context,
	s *service.Service,
	deployments *appsv1.DeploymentList) []*appsv1.Deployment {
	var fails []*appsv1.Deployment
	for i, deployment := range deployments.Items {
		labelSelector := labels.Set(deployment.Spec.Selector.MatchLabels)
		pods := &v1.PodList{}
		err := s.Client.List(ctx, pods, &client.ListOptions{LabelSelector: labelSelector.AsSelector()})
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

func ShouldPodBeDeleted(pod *v1.Pod) bool {
	switch {
	case pod.Status.Phase == v1.PodRunning:
		return false
	case pod.Status.Phase == v1.PodSucceeded:
		return false
	case pod.Status.Phase == v1.PodPending:
		for _, containerStatus := range pod.Status.ContainerStatuses {
			waiting := containerStatus.State.Waiting
			if waiting != nil && waiting.Reason == ImagePullBackOff {
				return true
			}
		}

		return false
	case pod.Status.Phase == v1.PodFailed:
		return false // should be true?
	case pod.Status.Phase == v1.PodUnknown:
		return false
	default:
		return false
	}
}

func PruneFailingDeployment(ctx context.Context, s *service.Service, deployment *appsv1.Deployment) {
	err := s.Client.Delete(ctx, deployment)
	if err != nil {
		log.Errorf("Could not delete deployment %s, %v", deployment.Name, err)
	} else {
		log.Infof("Deleting deployment %s", deployment.Name)
		s.Metrics.DeploymentsDeleted.Inc()
		// s.Metrics.PodsDeleted.Add(float64(len(pods.Items)))
	}
}
