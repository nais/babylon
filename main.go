package main

import (
	"context"
	"fmt"
	"strconv"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/nais/babylon/pkg/config"
	"github.com/nais/babylon/pkg/deployment"
	"github.com/nais/babylon/pkg/logger"
	"github.com/nais/babylon/pkg/metrics"
	"github.com/nais/babylon/pkg/service"
	nais_io_v1 "github.com/nais/liberator/pkg/apis/nais.io/v1"
	log "github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlMetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

func main() {
	logger.Setup(config.GetEnv("LOG_LEVEL", "debug"))
	cfg := config.ParseConfig()

	// TODO: perhaps timeout between each tick?
	ctx := context.Background()

	port, err := strconv.Atoi(cfg.Port)
	if err != nil {
		log.Fatal(err.Error())
	}

	m := metrics.Init(cfg.InfluxdbDatabase)
	ctrlMetrics.Registry.MustRegister(m.RuleActivations, m.DeploymentRollback, m.DeploymentDownscale, m.TeamNotifications)

	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = nais_io_v1.AddToScheme(scheme)

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
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

	influxC := influxdb2.NewClient(
		cfg.InfluxdbURI,
		fmt.Sprintf("%s:%s",
			cfg.InfluxdbUsername.SecretString(),
			cfg.InfluxdbPassword.SecretString()))

	health, err := influxC.Health(ctx)
	log.Infof("InfluxDB health: %+v, Error: %+v", health, err)

	s := service.Service{Config: &cfg, Client: c, Metrics: &m, UnleashClient: unleash, InfluxClient: influxC}

	go gardener(ctx, &s)

	log.Fatal(mgr.Start(ctx))
}

//nolint:cyclop
func gardener(ctx context.Context, s *service.Service) {
	log.Info("starting gardener")
	ticker := time.Tick(s.Config.TickRate)

	for {
		<-ticker
		deployments := &appsv1.DeploymentList{}
		err := s.Client.List(ctx, deployments)
		if logger.Logk8sError(err) {
			continue
		}

		if !s.Config.InActivePeriod(time.Now()) {
			log.Debug("sleeping due to inactive period")

			continue
		}

		fails := deployment.GetFailingDeployments(ctx, s, deployments)
		var deploymentFails []*appsv1.Deployment
		for _, f := range fails {
			if s.Config.IsNamespaceAllowed(f.Namespace) {
				deploymentFails = append(deploymentFails, f)
			}
		}

		for _, deploy := range deploymentFails {
			if deploy.Annotations[config.NotificationAnnotation] != "" {
				lastNotified, err := time.Parse(time.RFC3339, deploy.Annotations[config.NotificationAnnotation])
				switch {
				case err != nil:
					log.Warnf("Could not parse %s for %s: %v", config.NotificationAnnotation, deploy.Name, err)

					continue
				case time.Since(lastNotified) < s.Config.GraceDuration(deploy):
					log.Infof(
						"not yet ready to prune deployment %s, too early since last notification: %s",
						deploy.Name, lastNotified.String())

					continue
				case time.Since(lastNotified) < s.Config.NotificationTimeout:
					log.Infof("Team already notified at %s, skipping deploy %s", lastNotified.String(), deploy.Name)

					continue
				}
			} else {
				err := deployment.FlagFailingDeployment(ctx, s, deploy)
				if err != nil {
					log.Errorf("failed to add notification annotation, err: %v", err)
				}

				continue
			}

			if deploy.Annotations[deployment.ChangeCauseAnnotationKey] != deployment.RollbackCauseAnnotation {
				deployment.PruneFailingDeployment(ctx, s, deploy)
			}
		}
	}
}
