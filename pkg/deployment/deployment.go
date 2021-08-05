package deployment

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/nais/babylon/pkg/config"
	log "github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	ErrPatchFailed              = errors.New("failed to apply patch")
	ErrFetchReplicasetFailed    = errors.New("failed to fetch replicasets")
	ErrNoRollbackCandidateFound = errors.New("no rollback candidate found")
)

const (
	Unknown                    = "unknown"
	ImagePullBackOff           = "ImagePullBackOff"
	ErrImagePull               = "ErrImagePull"
	CrashLoopBackOff           = "CrashLoopBackOff"
	CreateContainerConfigError = "CreateContainerConfigError"
	RollbackCauseAnnotation    = "rolled back by babylon"
	DownscaleCauseAnnotation   = "scaled down by babylon"
	ChangeCauseAnnotationKey   = "kubernetes.io/change-cause"
)

func IsCreateContainerConfigError(containers []v1.ContainerStatus) bool {
	for _, containerStatus := range containers {
		waiting := containerStatus.State.Waiting
		if waiting != nil {
			log.Tracef("Waiting (CreateContainerConfigError): %+v", waiting)
		}
		if waiting != nil && waiting.Reason == CreateContainerConfigError {
			return true
		}
	}

	return false
}

func IsContainerImageCheckFail(containers []v1.ContainerStatus) bool {
	for _, containerStatus := range containers {
		waiting := containerStatus.State.Waiting
		if waiting != nil {
			log.Tracef("Waiting (IsContainerImageCheckFail): %+v", waiting)
		}
		if waiting != nil && (waiting.Reason == ImagePullBackOff || waiting.Reason == ErrImagePull) {
			return true
		}
	}

	return false
}

func IsContainerCrashLoopBackOff(restartThreshold int32, containers []v1.ContainerStatus) bool {
	for _, container := range containers {
		waiting := container.State.Waiting
		if waiting != nil {
			log.Tracef("Waiting (IsContainerCrashLoopBackOff): %+v", waiting)
		}

		if waiting != nil && waiting.Reason == CrashLoopBackOff && container.RestartCount > restartThreshold {
			return true
		}
	}

	return false
}

func IsInitContainerFailed(restartThreshold int32, initContainers []v1.ContainerStatus) (bool, string) {
	if IsContainerCrashLoopBackOff(restartThreshold, initContainers) {
		return true, CrashLoopBackOff
	} else if IsContainerImageCheckFail(initContainers) {
		return true, ImagePullBackOff
	}

	return false, ""
}

func GetReplicaSetsByDeployment(ctx context.Context,
	c client.Client,
	deployment *appsv1.Deployment) (appsv1.ReplicaSetList, error) {
	labelSelector := labels.Set(deployment.Spec.Selector.MatchLabels)

	l := &client.ListOptions{LabelSelector: labelSelector.AsSelector(), Namespace: deployment.Namespace}
	var replicaSets appsv1.ReplicaSetList
	err := c.List(ctx, &replicaSets, l)
	if err != nil {
		return appsv1.ReplicaSetList{}, fmt.Errorf("%w:%v", ErrFetchReplicasetFailed, err)
	}

	return replicaSets, nil
}

func GetPodsFromReplicaSet(ctx context.Context, c client.Client, rs *appsv1.ReplicaSet) (*v1.PodList, error) {
	labelSelector := labels.Set(rs.Spec.Selector.MatchLabels)
	pods := &v1.PodList{}
	err := c.List(ctx, pods, &client.ListOptions{LabelSelector: labelSelector.AsSelector()})
	if err != nil {
		return nil, fmt.Errorf("could not get pods from replica set: %w", err)
	}

	return pods, nil
}

func IsDeploymentDisabled(deployment *appsv1.Deployment) bool {
	enabled := deployment.Labels[config.EnabledLabel]
	if strings.ToLower(enabled) == "false" {
		log.Debugf("deployment %s has disabled Babylon, ignoring", deployment.Name)

		return true
	}

	return false
}

func SafeGetLabel(deploy *appsv1.Deployment, label string) string {
	value, ok := deploy.Labels[label]

	if !ok {
		value = Unknown
	}

	return value
}
