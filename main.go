package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aporeto-inc/trireme-kubernetes/auth"
	"github.com/aporeto-inc/trireme-kubernetes/collector"
	"github.com/aporeto-inc/trireme-kubernetes/config"
	"github.com/aporeto-inc/trireme-kubernetes/resolver"
	"github.com/aporeto-inc/trireme-kubernetes/utils"
	"github.com/aporeto-inc/trireme-kubernetes/version"

	trireme "github.com/aporeto-inc/trireme-lib"
	"github.com/aporeto-inc/trireme-lib/configurator"
	tlog "github.com/aporeto-inc/trireme-lib/log"
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

func main() {

	config, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Error loading config: %s", err)
	}

	if !config.Enforce {
		banner(version.VERSION, version.REVISION)
	}

	err = setLogs(config.LogFormat, config.LogLevel)
	if err != nil {
		log.Fatalf("Error setting up logs: %s", err)
	}

	zap.L().Debug("Config used", zap.Any("Config", config))

	if config.Enforce {
		zap.L().Info("Launching in enforcer mode")

		if err := trireme.LaunchRemoteEnforcer(nil); err != nil {
			zap.L().Fatal("Unable to start enforcer", zap.Error(err))
		}

		return
	}

	// Create New PolicyEngine based on Kubernetes rules.
	kubernetesPolicy, err := resolver.NewKubernetesPolicy(config.KubeconfigPath, config.KubeNodeName, config.ParsedTriremeNetworks, config.BetaNetPolicies, config.EgressNetPolicies)
	if err != nil {
		zap.L().Fatal("Error initializing KubernetesPolicy: ", zap.Error(err))
	}

	var trireme trireme.Trireme
	var monitor monitor.Monitor

	triremeNodeName := utils.GenerateNodeName(config.KubeNodeName)

	// Instantiating LibTrireme
	options := configurator.DefaultTriremeOptions()
	options.ServerID = triremeNodeName
	options.TargetNetworks = config.ParsedTriremeNetworks
	options.RemoteContainer = true
	options.LocalContainer = false
	options.LocalProcess = false
	options.SyncAtStart = true
	options.KillContainerError = false
	options.Resolver = kubernetesPolicy

	// Setting up the EventCollector based on the user Config
	if config.CollectorEndpoint != "" {
		options.EventCollector = collector.NewInfluxDBCollector(config.CollectorUser, config.CollectorPass, config.CollectorEndpoint, config.CollectorDB, config.CollectorInsecureSkipVerify)
	} else {
		options.EventCollector = collector.NewDefaultCollector()
	}

	if config.AuthType == "PSK" {
		zap.L().Info("Initializing Trireme with PSK Auth")

		options.PKI = false
		options.PSK = []byte(config.PSK)

		triremeResult, err := configurator.NewTriremeWithOptions(options)
		if err != nil {
			zap.L().Fatal("Error instantiating libtrireme", zap.Error(err))
		}

		trireme = triremeResult.Trireme
		monitor = triremeResult.DockerMonitor

	}

	if config.AuthType == "PKI" {
		zap.L().Info("Initializing Trireme with PKI Auth")

		// Load the PKI Certs/Keys based on config.
		pki, err := auth.LoadPKI(config.KubeNodeName, config.KubeconfigPath)
		if err != nil {
			zap.L().Fatal("Error loading Certificates for PKI Trireme", zap.Error(err))
		}

		options.PKI = true
		options.KeyPEM = pki.KeyPEM
		options.CertPEM = pki.CertPEM
		options.CaCertPEM = pki.CaCertPEM
		options.SmartToken = pki.SmartToken

		zap.L().Debug("CryptoCert used: ", zap.Any("options.CertPEM", options.CertPEM), zap.Any("options.CaCertPEM", options.CaCertPEM), zap.Any("options.SmartToken", options.SmartToken))

		triremeResult, err := configurator.NewTriremeWithOptions(options)
		if err != nil {
			zap.L().Fatal("Error instantiating libtrireme", zap.Error(err))
		}

		trireme = triremeResult.Trireme
		monitor = triremeResult.DockerMonitor
	}

	// Register Trireme to the Kubernetes policy resolver
	kubernetesPolicy.SetPolicyUpdater(trireme)

	// Start all the go routines.
	trireme.Start()
	zap.L().Debug("Trireme started")
	monitor.Start()
	zap.L().Debug("Monitor started")
	kubernetesPolicy.Run()
	zap.L().Debug("PolicyResolver started")

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
	zap.L().Info("Everything started. Waiting for Stop signal")
	// Waiting for a Sig
	<-c

	zap.L().Debug("Stop signal received")
	kubernetesPolicy.Stop()
	zap.L().Debug("KubernetesPolicy stopped")
	monitor.Stop()
	zap.L().Debug("Monitor stopped")
	trireme.Stop()
	zap.L().Debug("Trireme stopped")

	zap.L().Info("Everything stopped. Bye Kubernetes!")
}

// setLogs setups Zap to
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
		tlog.Trace = true
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
