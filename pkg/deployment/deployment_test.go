package deployment_test

import (
	"github.com/nais/babylon/pkg/deployment"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

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
					Reason: deployment.ImagePullBackOff,
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
			res := deployment.ShouldPodBeDeleted(&pod)

			if res != tt.Expected {
				t.Fatalf("Expected pod to be marked for deletion: %v, got: %v, pod: %+v", tt.Expected, res, pod)
			}
		})
	}
}
