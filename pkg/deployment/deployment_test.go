package deployment_test

import (
	"github.com/nais/babylon/pkg/config"
	"github.com/nais/babylon/pkg/deployment"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func makePodWithState(meta metav1.ObjectMeta, status v1.PodStatus) v1.Pod {
	return v1.Pod{
		ObjectMeta: meta,
		Status:     status,
	}
}

func TestContainersInCrashLoopBackOff(t *testing.T) {
	t.Parallel()

	createPod := func(state v1.ContainerState, restartCount int32) v1.Pod {
		return makePodWithState(metav1.ObjectMeta{
			Name: "failingpod",
		}, v1.PodStatus{
			Phase:             v1.PodRunning,
			ContainerStatuses: []v1.ContainerStatus{{State: state, RestartCount: restartCount}},
		})
	}

	shouldBeDeletedTest := []struct {
		Name             string
		State            v1.ContainerState
		RestartCount     int32
		RestartThreshold int32
		Expected         bool
	}{
		{
			Name: "CrashLoopBackOff with enough restarts",
			State: v1.ContainerState{
				Waiting: &v1.ContainerStateWaiting{
					Reason: deployment.CrashLoopBackOff,
				},
			},
			RestartCount:     1000,
			RestartThreshold: 500,
			Expected:         true,
		},
		{
			Name: "CrashLoopBackOff with not enough restarts",
			State: v1.ContainerState{
				Waiting: &v1.ContainerStateWaiting{
					Reason: deployment.CrashLoopBackOff,
				},
			},
			RestartCount:     100,
			RestartThreshold: 500,
			Expected:         false,
		},
		{
			Name:             "No mark for deletion",
			State:            v1.ContainerState{},
			RestartCount:     0,
			RestartThreshold: 500,
			Expected:         false,
		},
	}

	for _, tt := range shouldBeDeletedTest {
		tt := tt
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			cfg := config.DefaultConfig()
			pod := createPod(tt.State, tt.RestartCount)
			cfg.RestartThreshold = tt.RestartThreshold
			res := deployment.ShouldPodBeDeleted(&cfg, &pod)

			if res != tt.Expected {
				t.Fatalf("Expected pod to be marked for deletion: %v, got: %v, pod: %+v", tt.Expected, res, pod)
			}
		})
	}

}

func TestContainersWithImageCheckFailed(t *testing.T) {
	t.Parallel()
	createPod := func(state v1.ContainerState, phase v1.PodPhase) v1.Pod {
		return makePodWithState(metav1.ObjectMeta{
			Name: "failingpod",
		}, v1.PodStatus{
			Phase:             phase,
			ContainerStatuses: []v1.ContainerStatus{{State: state}},
		})
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
			pod := createPod(tt.State, tt.Phase)
			res := deployment.ShouldPodBeDeleted(nil, &pod)

			if res != tt.Expected {
				t.Fatalf("Expected pod to be marked for deletion: %v, got: %v, pod: %+v", tt.Expected, res, pod)
			}
		})
	}
}
