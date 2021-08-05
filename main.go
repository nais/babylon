package main

import (
	"context"
	"fmt"
	"strconv"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/nais/babylon/pkg/config"
	"github.com/nais/babylon/pkg/criteria"
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

//nolint:funlen
func main() {
	logger.Setup(config.GetEnv("LOG_LEVEL", "debug"))
	cfg := config.ParseConfig()

	// TODO: perhaps timeout between each tick?
	ctx := context.Background()

	port, err := strconv.Atoi(cfg.Port)
	if err != nil {
		log.Fatal(err.Error())
	}

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
	if err != nil {
		log.Errorf("Influx health error: %+v", err)
	}
	log.Infof("InfluxDB health: %+v", health)

	m := metrics.Init(unleash, c)
	ctrlMetrics.Registry.MustRegister(m.RuleActivations, m.DeploymentCleanup, m.DeploymentGraceCutoff,
		m.DeploymentUpdated, m.DeploymentStatus, m.SlackChannelMapping)

	h := metrics.NewHistory(influxC, cfg.InfluxdbDatabase)
	s := service.Service{Config: &cfg, Client: c, Metrics: &m, UnleashClient: unleash, InfluxClient: influxC, History: h}

	go gardener(ctx, &s)

	log.Fatal(mgr.Start(ctx))
}

func gardener(ctx context.Context, s *service.Service) {
	log.Info("starting gardener")
	ticker := time.Tick(s.Config.TickRate)
	coreCriteriaJudge := criteria.NewCoreCriteriaJudge(s.Config, s.Client, s.Metrics, s.History)
	cleanUpJudge := criteria.NewCleanUpJudge(s.Config)
	executioner := criteria.NewExecutioner(s.Config, s.Client, s.Metrics)

	for {
		<-ticker
		deployments := &appsv1.DeploymentList{}
		err := s.Client.List(ctx, deployments)
		if logger.Logk8sError(err) {
			continue
		}

		fails := coreCriteriaJudge.Failing(ctx, deployments)
		deploymentFails := cleanUpJudge.Judge(fails)
		executioner.Kill(ctx, deploymentFails)
	}
}
