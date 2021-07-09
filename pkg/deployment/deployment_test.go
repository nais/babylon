package deployment_test

import (
	"context"
	"testing"

	"github.com/nais/babylon/pkg/config"
	deployment2 "github.com/nais/babylon/pkg/deployment"
	metrics2 "github.com/nais/babylon/pkg/metrics"
	"github.com/nais/babylon/pkg/service"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	fake2 "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestPruneFailingDeployment(t *testing.T) {
	t.Parallel()
	t.Run("Deletes when armed", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Create fake client with deployment
		deployment := appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "torrLoping"}}
		fk := fake2.NewClientBuilder().WithRuntimeObjects(&deployment).WithScheme(clientgoscheme.Scheme).Build()

		cfg := config.DefaultConfig()

		l := &appsv1.DeploymentList{}
		err := fk.List(ctx, l)
		if err != nil {
			t.Fatal(err.Error())
		}

		if len(l.Items) != 1 {
			t.Fatalf("Wrong number of deployments, should be %d, got %d", 1, len(l.Items))
		}

		metrics := metrics2.Init()
		deployment2.PruneFailingDeployment(ctx, &service.Service{Metrics: &metrics, Client: fk, Config: &cfg}, &deployment)

		l = &appsv1.DeploymentList{}
		err = fk.List(ctx, l)
		if err != nil {
			t.Fatal(err.Error())
		}

		if len(l.Items) != 0 {
			t.Fatalf("Deployment actually deleted during dry run, length of list of deployments: %d", len(l.Items))
		}
	})
}

func TestShouldPodBeDeleted(t *testing.T) {
	t.Parallel()
	makePodWithState := func(state v1.ContainerState, phase v1.PodPhase) v1.Pod {
		return v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "failingpod",
			}, Status: v1.PodStatus{
				Phase: phase,
				ContainerStatuses: []v1.ContainerStatus{
					{State: state},
				},
			},
		}
	}

	shouldBeDeletedTest := []struct {
		Name     string
		State    v1.ContainerState
		Phase    v1.PodPhase
		Expected bool
	}{
		{
			Name: "ImagePullBackOff marks for deletion",
			State: v1.ContainerState{
				Waiting: &v1.ContainerStateWaiting{
					Reason: deployment2.ImagePullBackOff,
				},
			},
			Phase:    v1.PodPending,
			Expected: true,
		},
		{
			Name:     "No mark for deletion",
			State:    v1.ContainerState{},
			Phase:    v1.PodRunning,
			Expected: false,
		},
	}

	for _, tt := range shouldBeDeletedTest {
		tt := tt
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			pod := makePodWithState(tt.State, tt.Phase)
			res := deployment2.ShouldPodBeDeleted(&pod)

			if res != tt.Expected {
				t.Fatalf("Expected pod to be marked for deletion: %v, got: %v, pod: %+v", tt.Expected, res, pod)
			}
		})
	}
}
