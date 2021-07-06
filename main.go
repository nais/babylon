package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"time"

	"k8s.io/apimachinery/pkg/labels"

	"github.com/nais/babylon/pkg/config"
	logger2 "github.com/nais/babylon/pkg/logger"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

var cfg = config.DefaultConfig()

var podsDeleted = promauto.NewCounter(prometheus.CounterOpts{
	Name: "babylon_pods_deleted_total",
	Help: "Number of pods deleted in total",
})

var deploymentsDeleted = promauto.NewCounter(prometheus.CounterOpts{
	Name: "babylon_deployments_deleted_total",
	Help: "Number of deployments deleted in total",
})

func hello(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Hello world!")
}

func isAlive(w http.ResponseWriter, r *http.Request) {
	_, _ = fmt.Fprintf(w, "OK")
}

func isReady(w http.ResponseWriter, r *http.Request) {
	_, _ = fmt.Fprintf(w, "OK")
}

func main() {
	isArmed := config.GetEnv("ARMED", fmt.Sprintf("%v", cfg.Armed)) == "true"
	flag.StringVar(&cfg.LogLevel, "log-level", config.GetEnv("LOG_LEVEL", cfg.LogLevel), "set the log level of babylon")
	flag.BoolVar(&cfg.Armed, "armed", isArmed, "whether to start destruction")
	flag.StringVar(&cfg.Port, "port", config.GetEnv("PORT", cfg.Port), "set port number")

	flag.Parse()
	logger2.Setup(cfg.LogLevel)

	log.Infof("%+v", cfg)

	http.HandleFunc("/", hello)
	http.HandleFunc("/isready", isReady)
	http.HandleFunc("/isalive", isAlive)
	http.Handle("/metrics", promhttp.Handler())
	log.Infof("Listening on http://localhost:%v", cfg.Port)

	// creates the in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}
	// creates the clientset
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	if cfg.Armed {
		log.Info("starting gardener")
		go gardener(client)
	}

	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%v", cfg.Port), nil))
}

const ImagePullBackOff = "ImagePullBackOff"

func gardener(client kubernetes.Interface) {
	ticker := time.Tick(5 * time.Second)

	for {
		<-ticker
		deployments, err := client.AppsV1().Deployments("").List(context.TODO(), metav1.ListOptions{})
		logError(err)
		for _, deployment := range deployments.Items {
			labelSelector := labels.Set(deployment.Spec.Selector.MatchLabels)
			pods, err := client.CoreV1().Pods("").List(context.TODO(),
				metav1.ListOptions{LabelSelector: labelSelector.AsSelector().String()})
			if !logError(err) {
				for i, pod := range pods.Items {
					log.Debugf("%s: %s (%s)", pod.Name, pod.Status.Reason, pod.Status.Message)
					if shouldPodBeDeleted(&pods.Items[i]) {
						err = client.AppsV1().Deployments(deployment.Namespace).Delete(
							context.TODO(), deployment.Name, metav1.DeleteOptions{})
						if err != nil {
							log.Errorf("Could not delete deployment %s, %v", deployment.Name, err)
						} else {
							log.Infof("Deleting deployment %s", deployment.Name)
							deploymentsDeleted.Inc()
							podsDeleted.Add(float64(len(pods.Items)))
						}

						break
					}
				}
			}
		}
	}
}

func logError(err error) bool {
	var statusError *k8serrors.StatusError
	switch {
	case errors.As(err, &statusError):
		log.Errorf("Error getting deployment %v", statusError.ErrStatus.Message)

		return true
	case err != nil:
		log.Fatal(err.Error())

		return true
	default:
		return false
	}
}

func shouldPodBeDeleted(pod *v1.Pod) bool {
	for _, containerStatus := range pod.Status.ContainerStatuses {
		waiting := containerStatus.State.Waiting
		if waiting != nil && waiting.Reason == ImagePullBackOff {
			return true
		}
	}

	return false
}
