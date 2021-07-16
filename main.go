package main

import (
	"context"
	"flag"
	"fmt"
	"strconv"
	"time"

	"github.com/nais/babylon/pkg/config"
	"github.com/nais/babylon/pkg/deployment"
	logger2 "github.com/nais/babylon/pkg/logger"
	"github.com/nais/babylon/pkg/metrics"
	"github.com/nais/babylon/pkg/service"
	log "github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlMetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

func parseFlags() config.Config {
	cfg := config.DefaultConfig()
	isArmed := config.GetEnv("ARMED", fmt.Sprintf("%v", cfg.Armed)) == "true"
	flag.StringVar(&cfg.LogLevel, "log-level", config.GetEnv("LOG_LEVEL", cfg.LogLevel), "set the log level of babylon")
	flag.BoolVar(&cfg.Armed, "armed", isArmed, "whether to start destruction")
	flag.StringVar(&cfg.Port, "port", config.GetEnv("PORT", cfg.Port), "set port number")

	var tickrate string
	flag.StringVar(&tickrate, "timeout", config.GetEnv("TICKRATE", "5s"), "tickrate of main loop")

	var restartThreshold string
	defaultRestartThreshold := config.GetEnv("RESTART_THRESHOLD", fmt.Sprintf("%d", cfg.RestartThreshold))
	flag.StringVar(&restartThreshold, "restart-threshold", defaultRestartThreshold, "set restart threshold")

	var resourceAge string
	flag.StringVar(&resourceAge, "resource-age", config.GetEnv("RESOURCE_AGE", "10m"),
		"resource age needed before rollback")

	flag.Parse()
	duration, err := time.ParseDuration(tickrate)
	if err == nil {
		cfg.TickRate = duration
	}
	age, err := time.ParseDuration(resourceAge)
	if err == nil {
		cfg.ResourceAge = age
	}

	rt, err := strconv.ParseInt(restartThreshold, 10, 32)
	if err == nil {
		cfg.RestartThreshold = int32(rt)
	}

	return cfg
}

func main() {
	cfg := parseFlags()
	logger2.Setup(cfg.LogLevel)

	// TODO: perhaps timeout between each tick?
	ctx := context.Background()

	port, err := strconv.Atoi(cfg.Port)
	if err != nil {
		log.Fatal(err.Error())
	}

	m := metrics.Init()
	ctrlMetrics.Registry.MustRegister(m.PodsDeleted, m.DeploymentRollbacks)
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme.Scheme,
		MetricsBindAddress:     fmt.Sprintf(":%d", port),
		HealthProbeBindAddress: fmt.Sprintf(":%d", port+1),
	})
	if err != nil {
		log.Fatalf("error creating manager: %v", err)

		return
	}

	log.Infof("%+v", cfg)
	log.Infof("Metrics: http://localhost:%v/metrics", cfg.Port)

	c := mgr.GetClient()
	if !cfg.Armed {
		log.Info("Not armed and dangerous! :(")
		c = client.NewDryRunClient(c)
	} else {
		log.Info("Armed and dangerous! 🪖")
	}

	s := service.Service{Config: &cfg, Client: c, Metrics: &m, PruneHistory: make(map[string]time.Time)}

	go gardener(ctx, &s)

	log.Fatal(mgr.Start(ctx))
}

func gardener(ctx context.Context, s *service.Service) {
	log.Info("starting gardener")
	ticker := time.Tick(s.Config.TickRate)

	for {
		<-ticker
		deployments := &appsv1.DeploymentList{}
		err := s.Client.List(ctx, deployments)
		if logger2.Logk8sError(err) {
			continue
		}
		deploymentFails := deployment.GetFailingDeployments(ctx, s, deployments)
		for _, deploy := range deploymentFails {
			name := deploy.Namespace + deploy.Name
			if time.Since(s.PruneHistory[name]) < time.Minute*5 {
				// TODO: use annotation instead of in-memory map
				continue
			}

			deployment.PruneFailingDeployment(ctx, s, deploy)
		}
	}
}
