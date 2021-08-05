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
	history          *metrics.History
	restartThreshold int32
	resourceAge      time.Duration
}

func NewCoreCriteriaJudge(
	config *config.Config,
	client client.Client,
	metric *metrics.Metrics,
	history *metrics.History) *CoreCriteriaJudge {
	return &CoreCriteriaJudge{
		client:           client,
		metrics:          metric,
		history:          history,
		restartThreshold: config.RestartThreshold,
		resourceAge:      config.ResourceAge,
	}
}

func (d *CoreCriteriaJudge) Failing(ctx context.Context, deployments *appsv1.DeploymentList) []*appsv1.Deployment {
	var fails []*appsv1.Deployment
	for i := range deployments.Items {
		deploy := &deployments.Items[i]
		if failing, reasons := d.isFailing(ctx, deploy); failing {
			_, err := d.flagFailingDeployment(ctx, deploy)
			if err != nil {
				log.Errorf("failed to add notification annotation, err: %v", err)

				continue
			}

			d.historizeDeployment(ctx, reasons, deploy)
			d.metrics.SetDeploymentStatus(deploy, d.metrics.SlackChannel(ctx, deploy.Namespace), metrics.FAILING)
			fails = append(fails, deploy)
		} else {
			d.flagHealthyDeployment(ctx, deploy)
			d.metrics.SetDeploymentStatus(deploy, d.metrics.SlackChannel(ctx, deploy.Namespace), metrics.OK)
		}
	}

	return fails
}

func (d *CoreCriteriaJudge) isFailing(ctx context.Context, deploy *appsv1.Deployment) (bool, []string) {
	minDeploymentAge := time.Now().Add(-d.resourceAge)
	if deploy.CreationTimestamp.After(minDeploymentAge) {
		log.Debugf("deployment %s too young, skipping (%v)", deploy.Name, deploy.CreationTimestamp)

		return false, nil
	}

	rs, err := deployment.GetReplicaSetsByDeployment(ctx, d.client, deploy)
	if err != nil {
		log.Errorf("Could not get replicasets for deployment %s: %v", deploy.Name, err)

		return false, nil
	}

	log.Tracef("Checking deployment: %s", deploy.Name)

	for j := range rs.Items {
		if failing, reasons := d.judge(ctx, &rs.Items[j]); failing {
			log.Infof("Found errors in deployment %s", deploy.Name)

			return true, reasons
		}
	}

	return false, nil
}

func (d *CoreCriteriaJudge) judge(ctx context.Context, set *appsv1.ReplicaSet) (bool, []string) {
	initPodsFailing, initReasons := d.initPodsFailing(ctx, set)
	if podsFailing, podReasons := d.allPodsFailingInReplicaset(ctx, set); podsFailing || initPodsFailing {
		return true, append(podReasons, initReasons)
	}

	return false, nil
}

func (d *CoreCriteriaJudge) flagFailingDeployment(ctx context.Context, deployment *appsv1.Deployment) (bool, error) {
	if deployment.Annotations[config.FailureDetectedAnnotation] == "" {
		patch := client.MergeFrom(deployment.DeepCopy())
		deployment.Annotations[config.FailureDetectedAnnotation] = time.Now().Format(time.RFC3339)
		err := d.client.Patch(ctx, deployment, patch)
		if err != nil {
			return false, fmt.Errorf("%w", err)
		}

		log.Infof("Marking deployment %s as failing", deployment.Name)

		return true, nil
	}

	return false, nil
}

func (d *CoreCriteriaJudge) flagHealthyDeployment(ctx context.Context, deploy *appsv1.Deployment) {
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
}

func (d *CoreCriteriaJudge) allPodsFailingInReplicaset(ctx context.Context, set *appsv1.ReplicaSet) (bool, []string) {
	if *set.Spec.Replicas == 0 {
		return false, nil
	}

	pods, err := deployment.GetPodsFromReplicaSet(ctx, d.client, set)
	if err != nil {
		log.Errorf("finding pods for replicaSet %s failed", set.Name)

		return false, nil
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

	return failedPods == len(pods.Items), reasons
}

func (d *CoreCriteriaJudge) initPodsFailing(ctx context.Context, set *appsv1.ReplicaSet) (bool, string) {
	pods, err := deployment.GetPodsFromReplicaSet(ctx, d.client, set)
	if err != nil {
		return false, ""
	}

	for i := range pods.Items {
		if failing, reason := deployment.IsInitContainerFailed(
			d.restartThreshold,
			pods.Items[i].Status.InitContainerStatuses); failing {
			log.Infof("Init container failing for rs %s due to %s", set.Name, reason)

			return true, reason
		}
	}

	return false, ""
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

func (d *CoreCriteriaJudge) warnIfMultipleUniqueReasons(deploy *appsv1.Deployment, reasons []string) {
	m := make(map[string]struct{})

	for _, reason := range reasons {
		if reason == "" {
			continue
		}
		m[reason] = struct{}{}
	}

	if len(m) > 1 {
		log.Warnf("Deployment %s has multiple distinct reasons for failing: %v", deploy.Name, reasons)
	}
}

func (d *CoreCriteriaJudge) historizeDeployment(ctx context.Context, reasons []string, deploy *appsv1.Deployment) {
	if len(reasons) > 0 {
		d.warnIfMultipleUniqueReasons(deploy, reasons)
		d.history.HistorizeDeploymentFailing(
			reasons[0], deployment.SafeGetLabel(deploy, "team"),
			d.metrics.SlackChannel(ctx, deploy.Namespace), deploy.Name)
	} else {
		log.Warnf("Deployment %s marked as failing but without failing reasons: %v", deploy.Name, reasons)
	}
}
