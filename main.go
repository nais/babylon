package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/nais/babylon/pkg/config"
	"github.com/nais/babylon/pkg/deployment"
	logger2 "github.com/nais/babylon/pkg/logger"
	"github.com/nais/babylon/pkg/metrics"
	"github.com/nais/babylon/pkg/service"
	log "github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlMetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

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

	m := metrics.Init()
	ctrlMetrics.Registry.MustRegister(m.PodsDeleted, m.DeploymentsDeleted)
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 clientgoscheme.Scheme,
		MetricsBindAddress:     ":8080",
		HealthProbeBindAddress: ":8081",
	})
	if err != nil {
		log.Fatal(err.Error())

		return
	}

	log.Infof("%+v", cfg)
	log.Infof("Metrics: http://localhost:%v/m", cfg.Port)

	c := mgr.GetClient()
	if !cfg.Armed {
		log.Info("Not armed and dangerous! :(")
		c = client.NewDryRunClient(c)
	} else {
		log.Info("Armed and dangerous! 🪖")
	}

	s := service.Service{Config: &cfg, Client: c, Metrics: &m}

	log.Info("starting gardener")
	go gardener(ctx, &s)

	log.Fatal(mgr.Start(ctx))
}

func gardener(ctx context.Context, service *service.Service) {
	ticker := time.Tick(service.Config.TickRate)

	for {
		<-ticker
		deployments := &appsv1.DeploymentList{}
		err := service.Client.List(ctx, deployments)
		if logger2.Logk8sError(err) {
			continue
		}
		deploymentFails := deployment.GetFailingDeployments(ctx, service, deployments)
		for _, deploy := range deploymentFails {
			deployment.PruneFailingDeployment(ctx, service, deploy)
		}
	}
}
