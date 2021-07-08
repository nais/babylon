package deployment_test

import (
	"context"
	"testing"
	"time"

	"github.com/nais/babylon/pkg/config"
	deployment2 "github.com/nais/babylon/pkg/deployment"
	metrics2 "github.com/nais/babylon/pkg/metrics"
	"github.com/nais/babylon/pkg/service"
	log "github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
)

// nolint:funlen,wrapcheck
func TestPruneFailingDeployment(t *testing.T) {
	t.Parallel()
	t.Run("DryRun does not delete", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Create fake client with deployment
		deployment := appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "torrLoping"}}
		fk := fake.NewSimpleClientset(&deployment)

		cfg := config.DefaultConfig()
		cfg.Armed = false

		watcherStarted := make(chan struct{})

		fk.PrependWatchReactor("*", func(action clienttesting.Action) (handled bool, ret watch.Interface, err error) {
			gvr := action.GetResource()
			ns := action.GetNamespace()
			watcher, err := fk.Tracker().Watch(gvr, ns)
			if err != nil {
				return false, nil, err
			}
			close(watcherStarted)

			return true, watcher, nil
		})

		// We will create an informer that writes added deployments to a channel.
		deployments := make(chan *appsv1.Deployment, 1)
		informersFactory := informers.NewSharedInformerFactory(fk, 0)
		deploymentInformer := informersFactory.Apps().V1().Deployments().Informer()
		deploymentInformer.AddEventHandler(&cache.ResourceEventHandlerFuncs{
			DeleteFunc: func(obj interface{}) {
				d, _ := obj.(*appsv1.Deployment)
				t.Logf("deployment deleted: %s/%s", d.Namespace, d.Name)
				deployments <- d
			},
		})

		// Make sure informersFactory are running.
		informersFactory.Start(ctx.Done())

		// The fake client doesn't support resource version. Any writes to the client
		// after the informer's initial LIST and before the informer establishing the
		// watcher will be missed by the informer. Therefore we wait until the watcher
		// starts.
		// Note that the fake client isn't designed to work with informer. It
		// doesn't support resource version. It's encouraged to use a real client
		// in an integration/E2E test if you need to test complex behavior with
		// informer/controllers.
		<-watcherStarted

		metrics := metrics2.Init()

		l, err := fk.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
		if err != nil {
			t.Fatal(err.Error())
		}

		if len(l.Items) != 1 {
			t.Fatalf("Deployment actually deleted during dry run, length of list of deployments: %d", len(l.Items))
		}

		deployment2.PruneFailingDeployment(ctx, &service.Service{Metrics: &metrics, Client: fk, Config: &cfg}, &deployment)

		select {
		case d := <-deployments:
			t.Logf("Got d from channel: %s/%s", d.Namespace, d.Name)
		case <-time.After(wait.ForeverTestTimeout):
			t.Error("Informer did not get the added d")
		}

		actions := fk.Actions()
		log.Infof("actions: %+v", actions)
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
