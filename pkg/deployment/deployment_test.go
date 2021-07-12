package deployment_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nais/babylon/pkg/config"
	"github.com/nais/babylon/pkg/deployment"
	"github.com/nais/babylon/pkg/metrics"
	"github.com/nais/babylon/pkg/service"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/scheme"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fake2 "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	k8sClient client.Client
	testEnv   *envtest.Environment
	m         = metrics.Init()
)

func TestPruneFailingDeployment(t *testing.T) {
	t.Parallel()
	t.Run("Deletes when armed", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Create fake client with d
		d := appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "torrLoping"}}
		fk := fake2.NewClientBuilder().WithRuntimeObjects(&d).WithScheme(clientgoscheme.Scheme).Build()

		cfg := config.DefaultConfig()

		l := &appsv1.DeploymentList{}
		err := fk.List(ctx, l)
		if err != nil {
			t.Fatal(err.Error())
		}

		if len(l.Items) != 1 {
			t.Fatalf("Wrong number of deployments, should be %d, got %d", 1, len(l.Items))
		}

		deployment.PruneFailingDeployment(ctx, &service.Service{Metrics: &m, Client: fk, Config: &cfg}, &d)

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

var _ = Describe("gardener prune ImagePullBackOff", func() {
	const timeout = time.Second * 10
	const interval = time.Second * 1

	deploymentFixture := func(deploymentName string) appsv1.Deployment {
		return appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      deploymentName,
				Namespace: "default",
				Labels:    map[string]string{"app": "babylon"},
			},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "babylon"},
				},
				Template: v1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"app": "babylon"},
					},
					Spec: v1.PodSpec{
						Containers: []v1.Container{
							{
								Name:  "babylon",
								Image: "haiebfawioef",
							},
						},
					},
				},
			},
		}
	}

	Context("pruneFailingDeployment", func() {
		When("deployment with ImagePullBackOff is processed", func() {
			It("does not delete when using DryRun", func() {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				DeploymentName := "test.torr.loping"

				dep := deploymentFixture(DeploymentName)
				err := k8sClient.Create(ctx, &dep)
				Expect(err).NotTo(HaveOccurred())

				cfg := config.DefaultConfig()

				deployment.PruneFailingDeployment(ctx,
					&service.Service{
						Metrics: &m,
						Client:  client.NewDryRunClient(k8sClient),
						Config:  &cfg,
					}, &dep)

				dl := &appsv1.DeploymentList{}

				Consistently(func() (string, error) {
					err := k8sClient.List(context.Background(), dl)
					Expect(err).NotTo(HaveOccurred())

					if len(dl.Items) != 1 {
						return "", errors.New("wrong amount of deployments")
					}

					return dl.Items[0].Name, nil
				}, timeout, interval).Should(Equal(DeploymentName))
				Expect(k8sClient.Delete(ctx, &dep)).Should(Succeed())
			})
			It("does delete", func() {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				DeploymentName := "test.loping"

				dep := deploymentFixture(DeploymentName)
				err := k8sClient.Create(ctx, &dep)
				Expect(err).NotTo(HaveOccurred())

				cfg := config.DefaultConfig()

				deployment.PruneFailingDeployment(ctx, &service.Service{Metrics: &m, Client: k8sClient, Config: &cfg}, &dep)

				dl := &appsv1.DeploymentList{}
				Eventually(func() (int, error) {
					err := k8sClient.List(context.Background(), dl)
					Expect(err).NotTo(HaveOccurred())

					return len(dl.Items), nil
				}, timeout, interval).Should(Equal(0))
			})
		})
	})
	/*
		Context("deployment with ImagePullBackOff is not deleted with DryRun", func() {
			deployment :=
		})*/
})

func TestApis(t *testing.T) {
	t.Parallel()
	RegisterFailHandler(Fail)

	RunSpecs(t, "deployment tests")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{}

	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = v1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = appsv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())
}, 60)

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})
