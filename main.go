package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/nais/babylon/pkg/config"
	"github.com/nais/babylon/pkg/deployment"
	logger2 "github.com/nais/babylon/pkg/logger"
	"github.com/nais/babylon/pkg/metrics"
	"github.com/nais/babylon/pkg/service"
	naisiov1alpha1 "github.com/nais/liberator/pkg/apis/nais.io/v1alpha1"
	log "github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlMetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

const StringTrue = "true"

func parseFlags() config.Config {
	cfg := config.DefaultConfig()
	// Whether to start destruction
	cfg.Armed = config.GetEnv("ARMED", fmt.Sprintf("%v", cfg.Armed)) == StringTrue

	cfg.LogLevel = config.GetEnv("LOG_LEVEL", cfg.LogLevel)
	cfg.Port = config.GetEnv("PORT", cfg.Port)

	tickRate := config.GetEnv("TICKRATE", cfg.TickRate.String())
	restartThreshold := config.GetEnv("RESTART_THRESHOLD", fmt.Sprintf("%d", cfg.RestartThreshold))

	// Resource age needed before rollback
	resourceAge := config.GetEnv("RESOURCE_AGE", "10m")

	// Timeout between notifying teams
	notificationTimeout := config.GetEnv("NOTIFICATION_TIMEOUT", fmt.Sprintf("%d", cfg.NotificationTimeout))

	cfg.UseAllowedNamespaces = config.GetEnv("USE_ALLOWED_NAMESPACES",
		fmt.Sprintf("%t", cfg.UseAllowedNamespaces)) == StringTrue

	namespacesFromEnv := config.GetEnv("ALLOWED_NAMESPACES", "")
	cfg.AllowedNamespaces = strings.Split(namespacesFromEnv, ",")

	duration, err := time.ParseDuration(tickRate)
	if err == nil {
		cfg.TickRate = duration
	}
	age, err := time.ParseDuration(resourceAge)
	if err == nil {
		cfg.ResourceAge = age
	}
	nt, err := time.ParseDuration(notificationTimeout)
	if err == nil {
		cfg.NotificationTimeout = nt
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
	ctrlMetrics.Registry.MustRegister(m.RuleActivations, m.DeploymentRollbacks)

	sch := scheme.Scheme
	err = naisiov1alpha1.AddToScheme(sch)
	if err != nil {
		log.Fatal(err.Error())
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 sch,
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

	unleash, err := config.ConfigureUnleash()
	if err != nil {
		log.Fatal(err.Error())
	}
	s := service.Service{Config: &cfg, Client: c, Metrics: &m, Unleash: unleash}

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
			if deploy.Annotations[config.NotificationAnnotation] != "" {
				lastNotified, err := time.Parse(time.RFC3339, deploy.Annotations[config.NotificationAnnotation])
				if err != nil {
					log.Warnf("Could not parse %s for %s: %v", config.NotificationAnnotation, deploy.Name, err)

					continue
				}

				if time.Since(lastNotified) < s.Config.NotificationTimeout {
					log.Debugf("Team already notified at %s, skipping deploy %s", lastNotified.String(), deploy.Name)

					continue
				}
			}

			deployment.PruneFailingDeployment(ctx, s, deploy)
		}
	}
}
