package criteria

import (
	"context"
	"fmt"
	"time"

	"github.com/nais/babylon/pkg/config"
	"github.com/nais/babylon/pkg/deployment"
	"github.com/nais/babylon/pkg/metrics"
	log "github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type CoreCriteriaJudge struct {
	client           client.Client
	metrics          *metrics.Metrics
	restartThreshold int32
	resourceAge      time.Duration
}

func NewCoreCriteriaJudge(config *config.Config, client client.Client, metric *metrics.Metrics) *CoreCriteriaJudge {
	return &CoreCriteriaJudge{
		client:           client,
		metrics:          metric,
		restartThreshold: config.RestartThreshold,
		resourceAge:      config.ResourceAge,
	}
}

//nolint:nestif
func (d *CoreCriteriaJudge) Failing(ctx context.Context, deployments *appsv1.DeploymentList) []*appsv1.Deployment {
	var fails []*appsv1.Deployment
	for i := range deployments.Items {
		deploy := &deployments.Items[i]
		if d.isFailing(ctx, deploy) {
			err := d.flagFailingDeployment(ctx, deploy)
			if err != nil {
				log.Errorf("failed to add notification annotation, err: %v", err)

				continue
			}

			d.metrics.SetDeploymentStatus(deploy, d.metrics.SlackChannel(ctx, deploy.Namespace), metrics.FAILING)
			fails = append(fails, deploy)
		} else {
			if deploy.Annotations[config.FailureDetectedAnnotation] != "" {
				patch := client.MergeFrom(deploy.DeepCopy())
				delete(deploy.Annotations, config.FailureDetectedAnnotation)
				err := d.client.Patch(ctx, deploy, patch)
				if err != nil {
					log.Errorf("Error removing %s annotation from deployment %s since it is healthy. Error: %v",
						config.FailureDetectedAnnotation, deploy.Name, err)
				} else {
					log.Infof("Removed %s annotation from deployment %s since it is healthy",
						config.FailureDetectedAnnotation, deploy.Name)
				}
			}
			d.metrics.SetDeploymentStatus(deploy, d.metrics.SlackChannel(ctx, deploy.Namespace), metrics.OK)
		}
	}

	return fails
}

func (d *CoreCriteriaJudge) isFailing(ctx context.Context, deploy *appsv1.Deployment) bool {
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

func (d *CoreCriteriaJudge) judge(ctx context.Context, set *appsv1.ReplicaSet) bool {
	return d.allPodsFailingInReplicaset(ctx, set) || d.initPodsFailing(ctx, set)
}

func (d *CoreCriteriaJudge) flagFailingDeployment(ctx context.Context, deployment *appsv1.Deployment) error {
	if deployment.Annotations[config.FailureDetectedAnnotation] == "" {
		patch := client.MergeFrom(deployment.DeepCopy())
		deployment.Annotations[config.FailureDetectedAnnotation] = time.Now().Format(time.RFC3339)
		err := d.client.Patch(ctx, deployment, patch)
		if err != nil {
			return fmt.Errorf("%w", err)
		}

		log.Infof("Marking deployment %s as failing", deployment.Name)

		return nil
	}

	return nil
}

func (d *CoreCriteriaJudge) allPodsFailingInReplicaset(ctx context.Context, set *appsv1.ReplicaSet) bool {
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
			d.metrics.IncRuleActivations(&pods.Items[i], reason)
			reasons = append(reasons, reason)
		}
	}

	if failedPods > 0 {
		log.Debugf("%d/%d failing pods in replicaset %s due to %v", failedPods, len(pods.Items), set.Name, reasons)
	}

	return failedPods == len(pods.Items)
}

func (d *CoreCriteriaJudge) initPodsFailing(ctx context.Context, set *appsv1.ReplicaSet) bool {
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

func (d *CoreCriteriaJudge) shouldPodBeDeleted(pod *v1.Pod) (bool, string) {
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
