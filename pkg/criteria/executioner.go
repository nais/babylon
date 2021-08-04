package criteria

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nais/babylon/pkg/config"
	"github.com/nais/babylon/pkg/deployment"
	"github.com/nais/babylon/pkg/utils"
	"github.com/prometheus/alertmanager/timeinterval"
	log "github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Executioner struct {
	client              client.Client
	activeTimeIntervals map[string][]timeinterval.TimeInterval
}

func NewExecutioner(config *config.Config, client client.Client) *Executioner {
	return &Executioner{client: client, activeTimeIntervals: config.ActiveTimeIntervals}
}

func (e *Executioner) Kill(ctx context.Context, deployments []*appsv1.Deployment) {
	if !e.inActivePeriod(time.Now()) {
		log.Debug("sleeping due to inactive period")

		return
	}

	for _, deploy := range deployments {
		// TODO: Move to rollbackDeployment?
		if deploy.Annotations[deployment.ChangeCauseAnnotationKey] == deployment.RollbackCauseAnnotation {
			log.Infof("Deployment %s already rolled back, ignoring", deploy.Name)

			continue
		}
		if !deployment.IsDeploymentDisabled(deploy) {
			e.pruneFailingDeployment(ctx, deploy)
		}
	}
}

func (e *Executioner) inActivePeriod(time time.Time) bool {
	for _, t := range e.activeTimeIntervals {
		for _, i := range t {
			if i.ContainsTime(time) {
				return true
			}
		}
	}

	return false
}

func (e *Executioner) pruneFailingDeployment(ctx context.Context, deploy *appsv1.Deployment) {
	rollbacksDisabled := deploy.Labels[config.RollbackLabel] == "false"
	candidate, err := e.getRollbackCandidate(ctx, deploy)
	switch {
	case errors.Is(err, deployment.ErrNoRollbackCandidateFound) || rollbacksDisabled:
		err = e.downscaleDeployment(ctx, deploy)
		if err != nil {
			log.Errorf("Downscale failed, %v", err)
		}
	case err != nil:
		log.Errorf("getting candidate, %v", err)
	default:
		err = e.rollbackDeployment(ctx, deploy, candidate)
		if err != nil {
			log.Errorf("Rollback failed: %v", err)

			return
		}
	}
}

func (e *Executioner) downscaleDeployment(ctx context.Context, deploy *appsv1.Deployment) error {
	patch := client.MergeFrom(deploy.DeepCopy())
	deploy.Spec.Replicas = utils.Int32ptr(0)
	deploy.Annotations[deployment.ChangeCauseAnnotationKey] = deployment.DownscaleCauseAnnotation
	err := e.client.Patch(ctx, deploy, patch)
	if err != nil {
		return fmt.Errorf("failed to apply patch: %w", err)
	}
	log.Infof("Downscaled deployment %s", deploy.Name)

	return nil
}

func (e *Executioner) rollbackDeployment(
	ctx context.Context,
	deploy *appsv1.Deployment,
	replicaSet *appsv1.ReplicaSet) error {
	patch := client.MergeFrom(deploy.DeepCopy())
	deploy.Annotations[deployment.ChangeCauseAnnotationKey] = deployment.RollbackCauseAnnotation
	deploy.Spec.Template.Spec = replicaSet.Spec.Template.Spec
	err := e.client.Patch(ctx, deploy, patch)
	if err != nil {
		log.Errorf("Failed to patch deployment: %+v", err)

		return deployment.ErrPatchFailed
	}
	log.Infof("Rolled back deployment %s to revision: %s",
		deploy.Name, replicaSet.Annotations["deployment.kubernetes.io/revision"])

	return nil
}

func (e *Executioner) getRollbackCandidate(
	ctx context.Context,
	deploy *appsv1.Deployment) (*appsv1.ReplicaSet, error) {
	rs, err := deployment.GetReplicaSetsByDeployment(ctx, e.client, deploy)
	if err != nil {
		log.Errorf("Could not find replicasets for deploy %s", deploy.Name)

		return nil, fmt.Errorf("no replica set found: %w", err)
	}

	if len(rs.Items) == 0 {
		// 0 replicaSets assumed to not be possible
		log.Fatal("encountered deployment without replicaset")
	}

	for _, replicaSet := range rs.Items {
		if replicaSet.Annotations["deployment.kubernetes.io/revision"] ==
			deploy.Annotations["deployment.kubernetes.io/revision"] {
			continue
		}

		// if replicaset has running pods (good state)
		if *replicaSet.Spec.Replicas > 0 {
			return &replicaSet, nil
		}
	}

	return nil, deployment.ErrNoRollbackCandidateFound
}
