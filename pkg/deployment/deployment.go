package deployment

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
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

DEPLOYMENTS:
	for i, deployment := range deployments.Items {
		// graceCutoff := s.Config.GraceCutoff(&deployments.Items[i])
		minDeploymentAge := time.Now().Add(-s.Config.ResourceAge)
		enabled := deployment.Labels[config.EnabledAnnotation]
		switch {
		case !s.Config.IsNamespaceAllowed(deployment.Namespace):
			log.Debugf("Namespace %s is not allowed, skipping", deployment.Namespace)

			continue
		case deployment.CreationTimestamp.After(minDeploymentAge):
			log.Debugf("deployment %s too young, skipping (%v)", deployment.Name, deployment.CreationTimestamp)

			continue
		case strings.ToLower(enabled) == "false":
			log.Debugf("deployment %s has disabled Babylon, skipping", deployment.Name)

			continue
		}

		rs, err := getReplicaSetsByDeployment(ctx, s, &deployments.Items[i])
		if err != nil {
			log.Errorf("Could not get replicaSet for deployment %s, %v", deployment.Name, err)

			continue
		}
		log.Debugf("Checking deployment: %s", deployment.Name)

		for j := range rs.Items {
			if allPodsFailingInReplicaSet(ctx, &rs.Items[j], s) || checkFailingInitContainers(ctx, s, &rs.Items[j]) {
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

func checkFailingInitContainers(ctx context.Context, s *service.Service, rs *appsv1.ReplicaSet) bool {
	pods, err := getPodsFromReplicaSet(ctx, s, rs)
	if err != nil {
		return false
	}

	for i := range pods.Items {
		if IsInitContainerFailed(s.Config, pods.Items[i].Status.InitContainerStatuses) {
			log.Debugf("Init container failing for rs %s", rs.Name)

			return true
		}
	}

	return false
}

func IsInitContainerFailed(config *config.Config, initContainers []v1.ContainerStatus) bool {
	return containerCrashLoopBackOff(config, initContainers) || containerImageCheckFail(initContainers)
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

func getPodsFromReplicaSet(ctx context.Context, s *service.Service, rs *appsv1.ReplicaSet) (*v1.PodList, error) {
	labelSelector := labels.Set(rs.Spec.Selector.MatchLabels)
	pods := &v1.PodList{}
	err := s.Client.List(ctx, pods, &client.ListOptions{LabelSelector: labelSelector.AsSelector()})
	if err != nil {
		return nil, fmt.Errorf("could not get pods from replica set: %w", err)
	}

	return pods, nil
}

func allPodsFailingInReplicaSet(ctx context.Context, rs *appsv1.ReplicaSet, s *service.Service) bool {
	if *rs.Spec.Replicas == 0 {
		return false
	}

	pods, err := getPodsFromReplicaSet(ctx, s, rs)
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
	log.Debugf("%d/%d failing pods in replicaset %s", failedPods, len(pods.Items), rs.Name)

	return failedPods == len(pods.Items)
}

func DownscaleDeployment(ctx context.Context, s *service.Service, deployment *appsv1.Deployment) error {
	patch := client.MergeFrom(deployment.DeepCopy())
	deployment.Spec.Replicas = utils.Int32ptr(0)
	err := s.Client.Patch(ctx, deployment, patch)
	if err != nil {
		return fmt.Errorf("failed to apply patch: %w", err)
	}

	return nil
}

func RollbackDeployment(
	ctx context.Context,
	s *service.Service,
	deployment *appsv1.Deployment) (*appsv1.ReplicaSet, error) {
	rs, err := getReplicaSetsByDeployment(ctx, s, deployment)
	if err != nil {
		log.Errorf("Could not find replicasets for deploy %s", deployment.Name)

		return nil, err
	}

	if len(rs.Items) == 0 {
		// 0 replicaSets assumed to not be possible
		log.Fatal("encountered deployment without replicaset")
	}

	if len(rs.Items) == 1 || strings.ToLower(deployment.Labels[config.RollbackAnnotation]) == "false" {
		err = DownscaleDeployment(ctx, s, deployment)

		return nil, err
	}

	sort.Slice(rs.Items, func(i, j int) bool {
		iRev, err := strconv.Atoi(rs.Items[i].Annotations["deployment.kubernetes.io/revision"])
		if err != nil {
			log.Fatalf("invalid revision on replicaset %s, got error: %v", rs.Items[i].Name, err)
		}
		jRev, err := strconv.Atoi(rs.Items[j].Annotations["deployment.kubernetes.io/revision"])
		if err != nil {
			log.Fatalf("invalid revision on replicaset %s, got error: %v", rs.Items[j].Name, err)
		}

		return iRev > jRev
	})

	// Most recent (previous) replicaSet assumed to be at index = 1
	desiredReplicaSet := rs.Items[1]
	patch := client.MergeFrom(deployment.DeepCopy())
	deployment.Spec.Template.Spec = desiredReplicaSet.Spec.Template.Spec
	err = s.Client.Patch(ctx, deployment, patch)
	if err != nil {
		log.Errorf("Failed to patch deployment: %+v", err)

		return nil, ErrPatchFailed
	}
	log.Infof("Rolled back deployment %s to revision: %s",
		deployment.Name, desiredReplicaSet.Annotations["deployment.kubernetes.io/revision"])

	return &desiredReplicaSet, nil
}

func PruneFailingDeployment(ctx context.Context, s *service.Service, deployment *appsv1.Deployment) {
	rs, err := RollbackDeployment(ctx, s, deployment)
	if err != nil {
		log.Errorf("Rollback failed: %+v", err)

		return
	}
	s.Metrics.IncDeploymentRollbacks(deployment, s.Config.Armed, s.SlackChannel(ctx, deployment.Namespace), rs)
}

func FlagFailingDeployment(ctx context.Context, s *service.Service, deployment *appsv1.Deployment) error {
	patch := client.MergeFrom(deployment.DeepCopy())
	deployment.Annotations[config.NotificationAnnotation] = time.Now().Format(time.RFC3339)
	err := s.Client.Patch(ctx, deployment, patch)
	if err != nil {
		return fmt.Errorf("%w", err)
	}

	s.Metrics.IncTeamNotification(deployment, s.SlackChannel(ctx, deployment.Namespace), s.Config.GraceCutoff(deployment))

	return nil
}
