package criteria

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nais/babylon/pkg/config"
	"github.com/nais/babylon/pkg/deployment"
	"github.com/nais/babylon/pkg/metrics"
	"github.com/nais/babylon/pkg/utils"
	"github.com/prometheus/alertmanager/timeinterval"
	log "github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/utils/strings/slices"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Executioner struct {
	client              client.Client
	history             *metrics.History
	metrics             *metrics.Metrics
	armed               bool
	activeTimeIntervals map[string][]timeinterval.TimeInterval
}

const (
	Downscale            = "downscale"
	Rollback             = "rollback"
	DownscaleStrategy    = "downscale"
	RolloutAbortStrategy = "abort-rollout"
)

var ErrNoAvailableStrategies = errors.New("no cleanup strategies suitable for this deployment")

func NewExecutioner(
	config *config.Config,
	client client.Client,
	metrics *metrics.Metrics,
	history *metrics.History) *Executioner {
	return &Executioner{
		client:              client,
		history:             history,
		armed:               config.Armed,
		activeTimeIntervals: config.ActiveTimeIntervals,
		metrics:             metrics,
	}
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
			method, err := e.pruneFailingDeployment(ctx, deploy)
			if err != nil {
				log.Errorf("Failed to prune deployment %s: %v", deploy.Name, err)
			} else {
				e.history.HistorizeDeploymentKilled(
					method, deployment.SafeGetLabel(deploy, "team"),
					e.metrics.SlackChannel(ctx, deploy.Namespace), deploy.Name, e.armed)
			}
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

func (e *Executioner) pruneFailingDeployment(ctx context.Context, deploy *appsv1.Deployment) (string, error) {
	strategies := strings.Split(deploy.Annotations[config.StrategyAnnotation], ",")

	if len(strategies) == 0 {
		return "", ErrNoAvailableStrategies
	}

	candidate, err := e.getRollbackCandidate(ctx, deploy)
	switch {
	case slices.Contains(strategies, RolloutAbortStrategy) && err == nil:
		err = e.rollbackDeployment(ctx, deploy, candidate)
		if err != nil {
			return "", err
		}
		e.metrics.IncDeploymentCleanup(deploy, e.armed, e.metrics.SlackChannel(ctx, deploy.Namespace), metrics.RollbackLabel)

		return Rollback, nil
	case slices.Contains(strategies, DownscaleStrategy):
		err = e.downscaleDeployment(ctx, deploy)
		if err != nil {
			return "", err
		}
		e.metrics.IncDeploymentCleanup(deploy, e.armed, e.metrics.SlackChannel(ctx, deploy.Namespace), metrics.DownscaleLabel)

		return Downscale, nil
	case err != nil && !errors.Is(err, deployment.ErrNoRollbackCandidateFound):
		return "", err
	default:
		log.Infof("Attempted to kill deployment %s, but no strategies available", deploy.Name)

		return "", ErrNoAvailableStrategies
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
