package core

import (
	"context"
	"time"

	"github.com/nais/babylon/pkg/config"
	"github.com/nais/babylon/pkg/deployment"
	log "github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type DeploymentJudge struct {
	client           client.Client
	restartThreshold int32
	resourceAge      time.Duration
}

func NewDeploymentJudge(config *config.Config, client client.Client) *DeploymentJudge {
	return &DeploymentJudge{client: client, restartThreshold: config.RestartThreshold, resourceAge: config.ResourceAge}
}

func (d *DeploymentJudge) Failing(ctx context.Context, deployments *appsv1.DeploymentList) []*appsv1.Deployment {
	var fails []*appsv1.Deployment
	for i := range deployments.Items {
		if d.isFailing(ctx, &deployments.Items[i]) {
			fails = append(fails, &deployments.Items[i])
		}
	}

	return fails
}

func (d *DeploymentJudge) isFailing(ctx context.Context, deploy *appsv1.Deployment) bool {
	minDeploymentAge := time.Now().Add(-d.resourceAge)
	if deploy.CreationTimestamp.After(minDeploymentAge) {
		log.Debugf("deployment %s too young, skipping (%v)", deploy.Name, deploy.CreationTimestamp)

		return false
	}

	rs, err := deployment.GetReplicaSetsByDeployment(ctx, d.client, deploy)
	if err != nil {
		log.Errorf("Could not get replicasets for deployment %s: %v", deploy.Name, err)

		return false
	}

	log.Tracef("Checking deployment: %s", deploy.Name)

	for j := range rs.Items {
		if d.judge(ctx, &rs.Items[j]) {
			log.Infof("Found errors in deployment %s", deploy.Name)

			return true
		}
	}

	return false
}

func (d *DeploymentJudge) judge(ctx context.Context, set *appsv1.ReplicaSet) bool {
	return d.allPodsFailingInReplicaset(ctx, set) || d.initPodsFailing(ctx, set)
}

func (d *DeploymentJudge) allPodsFailingInReplicaset(ctx context.Context, set *appsv1.ReplicaSet) bool {
	if *set.Spec.Replicas == 0 {
		return false
	}

	pods, err := deployment.GetPodsFromReplicaSet(ctx, d.client, set)
	if err != nil {
		log.Errorf("finding pods for replicaSet %s failed", set.Name)

		return false
	}

	failedPods := 0
	var reasons []string
	for i := range pods.Items {
		if fail, reason := d.shouldPodBeDeleted(&pods.Items[i]); fail {
			failedPods++
			reasons = append(reasons, reason)
		}
	}

	if failedPods > 0 {
		log.Debugf("%d/%d failing pods in replicaset %s due to %v", failedPods, len(pods.Items), set.Name, reasons)
	}

	return failedPods == len(pods.Items)
}

func (d *DeploymentJudge) initPodsFailing(ctx context.Context, set *appsv1.ReplicaSet) bool {
	pods, err := deployment.GetPodsFromReplicaSet(ctx, d.client, set)
	if err != nil {
		return false
	}

	for i := range pods.Items {
		if deployment.IsInitContainerFailed(d.restartThreshold, pods.Items[i].Status.InitContainerStatuses) {
			log.Infof("Init container failing for rs %s", set.Name)

			return true
		}
	}

	return false
}

func (d *DeploymentJudge) shouldPodBeDeleted(pod *v1.Pod) (bool, string) {
	switch {
	case pod.Status.Phase == v1.PodRunning:
		log.Tracef("Pod: %s running", pod.Name)
		if deployment.IsContainerCrashLoopBackOff(d.restartThreshold, pod.Status.ContainerStatuses) {
			return true, deployment.CrashLoopBackOff
		}

		return false, ""
	case pod.Status.Phase == v1.PodSucceeded:
		log.Tracef("Pod: %s succeeded", pod.Name)

		return false, ""
	case pod.Status.Phase == v1.PodPending:
		log.Tracef("Pod: %s pending", pod.Name)
		if deployment.IsContainerImageCheckFail(pod.Status.ContainerStatuses) {
			return true, deployment.ImagePullBackOff
		}
		if deployment.IsCreateContainerConfigError(pod.Status.ContainerStatuses) {
			return true, deployment.CreateContainerConfigError
		}

		return false, ""
	case pod.Status.Phase == v1.PodFailed:
		log.Tracef("Pod: %s failed", pod.Name)

		return false, "" // should be true?
	case pod.Status.Phase == v1.PodUnknown:
		log.Tracef("Pod: %s unknown", pod.Name)

		return false, ""
	default:
		return false, ""
	}
}
