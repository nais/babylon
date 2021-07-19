package deployment

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/nais/babylon/pkg/config"
	"github.com/nais/babylon/pkg/service"
	"github.com/nais/babylon/pkg/utils"
	log "github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	ErrPatchFailed           = errors.New("failed to apply patch")
	ErrFetchReplicasetFailed = errors.New("failed to fetch replicasets")
)

const (
	ImagePullBackOff           = "ImagePullBackOff"
	ErrImagePull               = "ErrImagePull"
	CrashLoopBackOff           = "CrashLoopBackOff"
	CreateContainerConfigError = "CreateContainerConfigError"
)

func GetFailingDeployments(
	ctx context.Context,
	s *service.Service,
	deployments *appsv1.DeploymentList) []*appsv1.Deployment {
	var fails []*appsv1.Deployment
	checkTime := time.Now()
	ageBarrier := checkTime.Add(-s.Config.ResourceAge)

DEPLOYMENTS:
	for i, deployment := range deployments.Items {
		rs, err := getReplicaSetsByDeployment(ctx, s, &deployments.Items[i])
		if err != nil {
			log.Errorf("Could not get replicaSet for deployment %s, %v", deployment.Name, err)

			continue
		}
		log.Infof("Checking deployment: %s", deployment.Name)

		for j, r := range rs.Items {
			if r.CreationTimestamp.After(ageBarrier) {
				log.Infof("deployment %s too young, skipping (%v)", r.Name, r.CreationTimestamp)

				continue
			}
			if allPodsFailingInReplicaSet(ctx, &rs.Items[j], s) {
				fails = append(fails, &deployments.Items[i])

				continue DEPLOYMENTS
			}
		}
	}

	return fails
}

func createContainerConfigError(containers []v1.ContainerStatus) bool {
	for _, containerStatus := range containers {
		waiting := containerStatus.State.Waiting
		if waiting != nil {
			log.Debugf("Waiting: %+v", waiting)
		}
		if waiting != nil && waiting.Reason == CreateContainerConfigError {
			return true
		}
	}

	return false
}

func containerImageCheckFail(containers []v1.ContainerStatus) bool {
	for _, containerStatus := range containers {
		waiting := containerStatus.State.Waiting
		if waiting != nil {
			log.Debugf("Waiting: %+v", waiting)
		}
		if waiting != nil && (waiting.Reason == ImagePullBackOff || waiting.Reason == ErrImagePull) {
			return true
		}
	}

	return false
}

func containerCrashLoopBackOff(config *config.Config, containers []v1.ContainerStatus) bool {
	for _, container := range containers {
		waiting := container.State.Waiting
		if waiting != nil {
			log.Debugf("Waiting: %+v", waiting)
		}

		if waiting != nil && waiting.Reason == CrashLoopBackOff && container.RestartCount > config.RestartThreshold {
			return true
		}
	}

	return false
}

func ShouldPodBeDeleted(config *config.Config, pod *v1.Pod) (bool, string) {
	switch {
	case pod.Status.Phase == v1.PodRunning:
		log.Debugf("Pod: %s running", pod.Name)
		if containerCrashLoopBackOff(config, pod.Status.ContainerStatuses) {
			return true, CrashLoopBackOff
		}

		return false, ""
	case pod.Status.Phase == v1.PodSucceeded:
		log.Debugf("Pod: %s succeeded", pod.Name)

		return false, ""
	case pod.Status.Phase == v1.PodPending:
		log.Debugf("Pod: %s pending", pod.Name)
		if containerImageCheckFail(pod.Status.ContainerStatuses) {
			return true, ImagePullBackOff
		}
		if createContainerConfigError(pod.Status.ContainerStatuses) {
			return true, CreateContainerConfigError
		}

		return false, ""
	case pod.Status.Phase == v1.PodFailed:
		log.Debugf("Pod: %s failed", pod.Name)

		return false, "" // should be true?
	case pod.Status.Phase == v1.PodUnknown:
		log.Debugf("Pod: %s unknown", pod.Name)

		return false, ""
	default:
		return false, ""
	}
}

func getReplicaSetsByDeployment(
	ctx context.Context,
	s *service.Service,
	deployment *appsv1.Deployment) (appsv1.ReplicaSetList, error) {
	labelSelector := labels.Set(deployment.Spec.Selector.MatchLabels)

	l := &client.ListOptions{LabelSelector: labelSelector.AsSelector(), Namespace: deployment.Namespace}
	var replicaSets appsv1.ReplicaSetList
	err := s.Client.List(ctx, &replicaSets, l)
	if err != nil {
		return appsv1.ReplicaSetList{}, fmt.Errorf("%w:%v", ErrFetchReplicasetFailed, err)
	}

	return replicaSets, nil
}

func allPodsFailingInReplicaSet(ctx context.Context, rs *appsv1.ReplicaSet, s *service.Service) bool {
	if *rs.Spec.Replicas == 0 {
		return false
	}

	labelSelector := labels.Set(rs.Spec.Selector.MatchLabels)
	pods := &v1.PodList{}
	err := s.Client.List(ctx, pods, &client.ListOptions{LabelSelector: labelSelector.AsSelector()})
	if err != nil {
		log.Errorf("finding pods for replicaSet %s failed", rs.Name)

		return false
	}
	failedPods := 0
	for i := range pods.Items {
		if fail, reason := ShouldPodBeDeleted(s.Config, &pods.Items[i]); fail {
			failedPods++
			s.Metrics.IncRuleActivations(rs, reason)
		}
	}
	log.Infof("%d/%d failing pods in replicaset %s", failedPods, len(pods.Items), rs.Name)

	return failedPods == len(pods.Items)
}

func RollbackDeployment(ctx context.Context, s *service.Service, deployment *appsv1.Deployment) error {
	rs, err := getReplicaSetsByDeployment(ctx, s, deployment)
	if err != nil {
		return err
	}
	// 0 replicaSets assumed to not be possible
	if len(rs.Items) == 1 {
		log.Infof("Deployment %s has only 1 replicaset", deployment.Name)
		patch := client.MergeFrom(deployment.DeepCopy())
		deployment.Spec.Replicas = utils.Int32ptr(0)
		deployment.Annotations[config.NotificationAnnotation] = time.Now().Format(time.RFC3339)
		err := s.Client.Patch(ctx, deployment, patch)
		if err != nil {
			return fmt.Errorf("failed to apply patch: %w", err)
		}

		return nil
	}
	// Most recent replicaSet assumed to be at index = 1
	log.Infof("Rolling back deployment %s to previous revision", deployment.Name)

	sort.Slice(rs.Items, func(i, j int) bool {
		return rs.Items[i].Annotations["deployment.kubernetes.io/revision"] >
			rs.Items[j].Annotations["deployment.kubernetes.io/revision"]
	})
	desiredReplicaSet := rs.Items[1]
	desiredReplicaSet.Spec.Replicas = utils.Int32ptr(0)
	patch := client.MergeFrom(deployment.DeepCopy())
	deployment.Annotations[config.NotificationAnnotation] = time.Now().Format(time.RFC3339)
	deployment.Spec.Template.Spec = desiredReplicaSet.Spec.Template.Spec
	err = s.Client.Patch(ctx, deployment, patch)
	if err != nil {
		log.Errorf("Failed to patch deployment: %+v", err)

		return ErrPatchFailed
	}
	log.Infof("Rolled back deployment %s to revision: %s",
		deployment.Name, desiredReplicaSet.Annotations["deployment.kubernetes.io/revision"])

	return nil
}

func PruneFailingDeployment(ctx context.Context, s *service.Service, deployment *appsv1.Deployment) {
	err := RollbackDeployment(ctx, s, deployment)
	if err != nil {
		log.Errorf("Rollback failed: %+v", err)

		return
	}
	s.Metrics.IncDeploymentRollbacks(deployment, s.Config.Armed)
}
