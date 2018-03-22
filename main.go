package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aporeto-inc/trireme-kubernetes/auth"
	kubecollector "github.com/aporeto-inc/trireme-kubernetes/collector"
	"github.com/aporeto-inc/trireme-kubernetes/config"
	"github.com/aporeto-inc/trireme-kubernetes/resolver"
	"github.com/aporeto-inc/trireme-kubernetes/utils"
	"github.com/aporeto-inc/trireme-kubernetes/version"

	"github.com/aporeto-inc/trireme-lib/collector"
	"github.com/aporeto-inc/trireme-lib/controller"
	"github.com/aporeto-inc/trireme-lib/controller/pkg/fqconfig"
	"github.com/aporeto-inc/trireme-lib/controller/pkg/secrets"
	"github.com/aporeto-inc/trireme-lib/monitor"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func banner(version, revision string) {
	fmt.Printf(`


	  _____     _
	 |_   _| __(_)_ __ ___ _ __ ___   ___
	   | || '__| | '__/ _ \ '_'' _ \ / _ \
	   | || |  | | | |  __/ | | | | |  __/
	   |_||_|  |_|_|  \___|_| |_| |_|\___|


_______________________________________________________________
             %s - %s
                                                 🚀  by Aporeto

`, version, revision)
}

// launch is used when this trireme-kubernetes process is launched as the main Trireme-Kubernetes
// process on the node. This Trireme-Kubernetes process will set everything up and orchestrate the launch
// of the other Trireme-Kubernetes process on the node (Container specific)
func launch(config *config.Configuration) {
	banner(version.VERSION, version.REVISION)

	zap.L().Debug("Config used", zap.Any("Config", config))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Generate a unique NodeName used internally to Trireme.
	triremeNodeName := utils.GenerateNodeName(config.KubeNodeName)

	// Setting up the EventCollector based on the user Config
	var collectorInstance collector.EventCollector
	if config.CollectorEndpoint != "" {
		zap.L().Info("Initializing Trireme with InfluxDBCollector")
		collectorInstance = kubecollector.NewInfluxDBCollector(config.CollectorUser, config.CollectorPass, config.CollectorEndpoint, config.CollectorDB, config.CollectorInsecureSkipVerify)
	} else {
		zap.L().Info("Initializing Trireme with Default collector")
		collectorInstance = kubecollector.NewDefaultCollector()
	}

	// Setting up Auth type based on user config.
	var triremesecret secrets.Secrets
	if config.AuthType == "PSK" {
		zap.L().Info("Initializing Trireme with PSK Auth. Should NOT be used in production")

		triremesecret = secrets.NewPSKSecrets([]byte(config.PSK))

	}
	if config.AuthType == "PKI" {
		zap.L().Info("Initializing Trireme with PKI Auth")

		// Load the PKI Certs/Keys based on config.
		pki, err := auth.LoadPKI(config.KubeNodeName, config.KubeconfigPath)
		if err != nil {
			zap.L().Fatal("error loading Certificates for PKI Trireme", zap.Error(err))
		}

		triremesecret, err = secrets.NewCompactPKIWithTokenCA(pki.KeyPEM, pki.CertPEM, pki.CaCertPEM, [][]byte{[]byte(pki.CaCertPEM)}, pki.SmartToken)
		if err != nil {
			zap.L().Fatal("error creating PKI Secret for Trireme", zap.Error(err))
		}
	}

	// Creating the controller
	controllerOptions := []controller.Option{
		controller.OptionSecret(triremesecret),
		controller.OptionCollector(collectorInstance),
		controller.OptionEnforceFqConfig(fqconfig.NewFilterQueueWithDefaults()),
		//controller.OptionTargetNetworks(config.ParsedTriremeNetworks),
	}
	if config.LogLevel == "trace" {
		controllerOptions = append(controllerOptions, controller.OptionPacketLogs())
	}

	// Initialize the controllers
	ctrl := controller.New(triremeNodeName, controllerOptions...)
	if ctrl == nil {
		zap.L().Fatal("Unable to initialize trireme")
	}

	// Create New PolicyEngine based on Kubernetes rules.
	kubernetesPolicyResolver, err := resolver.NewKubernetesPolicy(ctx, ctrl, config.KubeconfigPath, config.KubeNodeName, config.ParsedTriremeNetworks)
	if err != nil {
		zap.L().Fatal("Error initializing KubernetesPolicy: ", zap.Error(err))
	}

	// Monitor configuration
	monitorOptions := []monitor.Options{
		monitor.OptionMonitorKubernetes(
			monitor.SubOptionMonitorKubernetesKubeconfig(config.KubeconfigPath),
			monitor.SubOptionMonitorKubernetesNodename(config.KubeNodeName),
		),
		monitor.OptionPolicyResolver(kubernetesPolicyResolver),
		monitor.OptionCollector(collectorInstance),
	}

	m, err := monitor.NewMonitors(monitorOptions...)
	if err != nil {
		zap.L().Fatal("Unable to initialize monitor", zap.Error(err))
	}

	// Launching Trireme-Kubernetes and the Policy resolver and waiting for thw associarted initial sync to finish theough the chanel
	// TODO: Use context here instead ?
	syncChan := make(chan struct{})
	kubernetesPolicyResolver.Run(syncChan)
	<-syncChan

	if err := ctrl.Run(ctx); err != nil {
		zap.L().Fatal("Failed to start controller", zap.Error(err))
	}

	// Start all the go routines.
	if err := m.Run(ctx); err != nil {
		zap.L().Fatal("Failed to start monitor", zap.Error(err))
	}

	zap.L().Debug("Trireme started")

	zap.L().Debug("PolicyResolver started")

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
	zap.L().Info("Everything started. Waiting for Stop signal")
	// Waiting for a Sig
	<-c

	// Cancel al routines that support ctx
	cancel()

	zap.L().Debug("Stop signal received")
	kubernetesPolicyResolver.Stop()
	zap.L().Debug("KubernetesPolicy stopped")

	zap.L().Info("Everything stopped. Bye Kubernetes!")
}

// enforce is used when this trireme-kubernetes process is launched in "Enforce" mode.
// In this mode, the process is typically launched specifically for one single container
// in a specific Container namespace.
func enforce() {
	zap.L().Info("Launching in enforcer mode")

	if err := controller.LaunchRemoteEnforcer(nil); err != nil {
		zap.L().Fatal("Unable to start enforcer", zap.Error(err))
	}

	return
}

// setLogs setups Zap to the correct log level and correct output format.
func setLogs(logFormat, logLevel string) error {
	var zapConfig zap.Config

	switch logFormat {
	case "json":
		zapConfig = zap.NewProductionConfig()
		zapConfig.DisableStacktrace = true
	default:
		zapConfig = zap.NewDevelopmentConfig()
		zapConfig.DisableStacktrace = true
		zapConfig.DisableCaller = true
		zapConfig.EncoderConfig.EncodeTime = func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {}
		zapConfig.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	// Set the logger
	switch logLevel {
	case "trace":
		zapConfig.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	case "debug":
		zapConfig.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	case "info":
		zapConfig.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	case "warn":
		zapConfig.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
	case "error":
		zapConfig.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
	case "fatal":
		zapConfig.Level = zap.NewAtomicLevelAt(zap.FatalLevel)
	default:
		zapConfig.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	}

	logger, err := zapConfig.Build()
	if err != nil {
		return err
	}

	zap.ReplaceGlobals(logger)
	return nil
}

// main is setting up the basics and check if this process is launched
// as Enforce or as the Main launcher
func main() {
	config, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Error loading config: %s", err)
	}

	if config.Enforce {
		_, _, config.LogLevel, config.LogFormat = controller.GetLogParameters()
	}

	err = setLogs(config.LogFormat, config.LogLevel)
	if err != nil {
		log.Fatalf("Error setting up logs: %s", err)
	}

	if config.Enforce {
		enforce()
	} else {
		launch(config)
	}
}
