package criteria

import (
	"github.com/nais/babylon/pkg/config"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
	"time"
)

func TestCleanUpJudge_filterByNamespace(t *testing.T) {
	createJudge := func(namespaces []string, useAllowedNamespaces bool) CleanUpJudge {
		return CleanUpJudge{
			useAllowedNamespaces: useAllowedNamespaces,
			allowedNamespaces:    namespaces,
			gracePeriod:          0,
			notificationDelay:    0,
		}
	}

	deploymentTest := []struct {
		Name                 string
		AllowedNamespaces    []string
		UseAllowedNamespaces bool
		Namespace            string
		Expected             bool
	}{
		{
			Name:                 "By default everything is allowed",
			Namespace:            "testdefault",
			AllowedNamespaces:    []string{},
			UseAllowedNamespaces: false,
			Expected:             true,
		},
		{
			Name:                 "Works on single namespace",
			Namespace:            "test",
			AllowedNamespaces:    []string{"test"},
			UseAllowedNamespaces: true,
			Expected:             true,
		},
		{
			Name:                 "Works on multiple allowed namespaces",
			Namespace:            "guri",
			AllowedNamespaces:    []string{"guri", "tor", "marianne"},
			UseAllowedNamespaces: true,
			Expected:             true,
		},
		{
			Name:                 "Works when name is contained in allowed namespace",
			Namespace:            "odd",
			AllowedNamespaces:    []string{"oddrane"},
			UseAllowedNamespaces: true,
			Expected:             true,
		},
		{
			Name:                 "Not working namespace",
			Namespace:            "notworking",
			AllowedNamespaces:    []string{"allowed"},
			UseAllowedNamespaces: true,
			Expected:             false,
		},
		{
			Name:                 "Empty allowed namespaces",
			Namespace:            "test",
			AllowedNamespaces:    []string{},
			UseAllowedNamespaces: true,
			Expected:             false,
		},
		{
			Name:                 "Sanity check",
			Namespace:            "kuttl-test-able-molly",
			AllowedNamespaces:    []string{"babylon-test", "kuttl-test"},
			UseAllowedNamespaces: true,
			Expected:             true,
		},
	}

	for _, tt := range deploymentTest {
		tt := tt
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()

			judge := createJudge(tt.AllowedNamespaces, tt.UseAllowedNamespaces)
			deployment := createDeployment(tt.Namespace, nil)
			actual := judge.filterByAllowedNamespace(&deployment)

			if actual != tt.Expected {
				t.Fatalf("Expected namespace %s to be %t was %t", tt.Namespace, tt.Expected, actual)
			}
		})
	}
}

func TestCleanUpJudge_Judge_not_allowed_namespace(t *testing.T) {
	var deploymentList []*appsv1.Deployment
	deployment := createDeployment("", nil)
	deploymentList = append(deploymentList, &deployment)

	judge := CleanUpJudge{
		useAllowedNamespaces: false,
		allowedNamespaces:    []string{"default"},
		gracePeriod:          0,
		notificationDelay:    0,
	}

	actual := judge.Judge(deploymentList)

	if len(actual) < 0 {
		t.Fatalf("Expected actual length of to be > 0, actual = %v", actual)
	}
}

func TestCleanUpJudge_Judge_allowed_namespaces(t *testing.T) {
	timeout := 30 * time.Second
	var deploymentList []*appsv1.Deployment
	deployment := createDeployment("not", map[string]string{
		config.FailureDetectedAnnotation: time.Now().Format(time.RFC3339),
		config.GracePeriodAnnotation:     "30s"})
	deployment2 := createDeployment("default", map[string]string{
		config.FailureDetectedAnnotation: time.Now().Add(-timeout).Format(time.RFC3339),
		config.GracePeriodAnnotation:     "0s"})
	deploymentList = append(deploymentList, &deployment, &deployment2)

	judge := CleanUpJudge{
		useAllowedNamespaces: true,
		allowedNamespaces:    []string{"default"},
		gracePeriod:          timeout,
		notificationDelay:    timeout,
	}

	actual := judge.Judge(deploymentList)

	if len(actual) != 1 {
		t.Fatalf("Expected actual length of to be 1, actual = %v", actual)
	}
}

func TestCleanUpJudge_Judge_grace_period(t *testing.T) {
	var deploymentList []*appsv1.Deployment
	deployment := createDeployment("", map[string]string{
		config.GracePeriodAnnotation:     "10s",
		config.FailureDetectedAnnotation: time.Now().Format(time.RFC3339)},
	)
	deploymentList = append(deploymentList, &deployment)

	judge := CleanUpJudge{
		useAllowedNamespaces: false,
		allowedNamespaces:    []string{"default"},
		gracePeriod:          10 * time.Second,
		notificationDelay:    0,
	}

	actual := judge.Judge(deploymentList)

	if len(actual) > 0 {
		t.Fatalf("Expected actual length of to be 0, actual = %v", actual)
	}
}

func TestCleanUpJudge_Judge_notification_timeout(t *testing.T) {
	var deploymentList []*appsv1.Deployment
	deployment := createDeployment("", map[string]string{
		config.FailureDetectedAnnotation: time.Now().Format(time.RFC3339),
		config.GracePeriodAnnotation:     "10s"})
	deploymentList = append(deploymentList, &deployment)

	notificationTimeout, _ := time.ParseDuration("10s")
	judge := CleanUpJudge{
		useAllowedNamespaces: false,
		allowedNamespaces:    []string{"default"},
		gracePeriod:          0,
		notificationDelay:    notificationTimeout,
	}

	actual := judge.Judge(deploymentList)

	if len(actual) > 0 {
		t.Fatalf("Expected actual length of to be 0, actual = %v", actual)
	}
}

func createDeployment(namespace string, annotations map[string]string) appsv1.Deployment {
	return appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
		Namespace:   namespace,
		Annotations: annotations}}
}
