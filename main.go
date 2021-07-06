package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"time"

	appconfig "github.com/nais/babylon/config"

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

var cfg = appconfig.DefaultConfig()

var podsDeleted = promauto.NewCounter(prometheus.CounterOpts{
	Name: "babylon_pods_deleted_total",
	Help: "Number of pods deleted in total",
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

func Setup(level string) {
	log.SetFormatter(&log.JSONFormatter{FieldMap: log.FieldMap{
		log.FieldKeyMsg: "message",
	}})

	l, err := log.ParseLevel(level)
	if err != nil {
		log.Fatal(err)
	}

	log.SetLevel(l)
}

func main() {
	dryRun := appconfig.GetEnv("DRY_RUN", fmt.Sprintf("%v", cfg.DryRun)) == "true"
	flag.StringVar(&cfg.LogLevel, "log-level", appconfig.GetEnv("LOG_LEVEL", cfg.LogLevel), "set the log level of babylon")
	flag.BoolVar(&cfg.DryRun, "dry-run", dryRun, "whether to dry run babylon")
	flag.StringVar(&cfg.Port, "port", appconfig.GetEnv("PORT", cfg.Port), "set port number")

	flag.Parse()

	Setup(cfg.LogLevel)
	http.HandleFunc("/", hello)
	http.HandleFunc("/isReady", isReady)
	http.HandleFunc("/isAlive", isAlive)
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

	go gardener(client)

	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%v", cfg.Port), nil))
}

const ImagePullBackOff = "ImagePullBackOff"

func gardener(client kubernetes.Interface) {
	ticker := time.Tick(5 * time.Second)

	for {
		<-ticker
		pods, err := client.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})
		var statusError *k8serrors.StatusError
		if isStatus := errors.As(err, &statusError); isStatus {
			log.Errorf("Error getting pod %v", statusError.ErrStatus.Message)
		} else if err != nil {
			log.Fatal(err.Error())
		}

		for i, pod := range pods.Items {
			log.Debugf("%s: %s (%s)", pod.Name, pod.Status.Reason, pod.Status.Message)
			if shouldPodBeDeleted(&pods.Items[i]) {
				err = client.CoreV1().Pods(pod.Namespace).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{})
				if err != nil {
					log.Errorf("Could not delete pod %s, %v", pod.Name, err)
				} else {
					log.Infof("Deleting pod %s", pod.Name)
					podsDeleted.Inc()
				}
			}
		}
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
