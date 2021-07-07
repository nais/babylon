package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"time"

	"github.com/nais/babylon/pkg/config"
	"github.com/nais/babylon/pkg/deployment"
	logger2 "github.com/nais/babylon/pkg/logger"
	"github.com/nais/babylon/pkg/metrics"
	"github.com/nais/babylon/pkg/service"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

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
	cfg := config.DefaultConfig()
	isArmed := config.GetEnv("ARMED", fmt.Sprintf("%v", cfg.Armed)) == "true"
	flag.StringVar(&cfg.LogLevel, "log-level", config.GetEnv("LOG_LEVEL", cfg.LogLevel), "set the log level of babylon")
	flag.BoolVar(&cfg.Armed, "armed", isArmed, "whether to start destruction")
	flag.StringVar(&cfg.Port, "port", config.GetEnv("PORT", cfg.Port), "set port number")
	var tickrate string

	flag.StringVar(&tickrate, "timeout", config.GetEnv("TICKRATE", "5s"), "tickrate of main loop")
	flag.Parse()
	duration, err := time.ParseDuration(tickrate)
	if err == nil {
		cfg.TickRate = duration
	}
	logger2.Setup(cfg.LogLevel)

	// TODO: perhaps timeout between each tick?
	ctx := context.Background()

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
	metrics := metrics.Init()
	service := service.Service{Config: &cfg, Client: client, Metrics: &metrics}

	log.Info("starting gardener")
	go gardener(ctx, &service)

	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%v", cfg.Port), nil))
}

func gardener(ctx context.Context, service *service.Service) {
	ticker := time.Tick(service.Config.TickRate)

	for {
		<-ticker
		deployments, err := service.Client.AppsV1().Deployments("").List(context.TODO(), metav1.ListOptions{})
		if logger2.Logk8sError(err) {
			continue
		}
		deploymentFails := deployment.GetFailingDeployments(ctx, service, deployments)
		for _, deploy := range deploymentFails {
			deployment.PruneFailingDeployment(ctx, service, deploy)
		}
	}
}
