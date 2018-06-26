package main

import (
	"fmt"
	"net/http"
	"time"

	_ "net/http/pprof"

	"github.com/aporeto-inc/trireme-example/remotebuilder/configuration"
	"github.com/aporeto-inc/trireme-lib/controller"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

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

func main() {

	cfg := configuration.NewConfiguration()
	fmt.Println(cfg)
	time.Local = time.UTC

	if cfg.Enforce {
		_, _, cfg.LogLevel, cfg.LogFormat = controller.GetLogParameters()

		if err := setLogs(cfg.LogFormat, cfg.LogLevel); err != nil {
			zap.L().Fatal("Error setting up logs:", zap.Error(err))
		}
	}

	if cfg.EnableProfiling {
		go func() {
			fmt.Println(http.ListenAndServe("localhost:6061", nil))
		}()
	}

	if cfg.Enforce {
		if err := controller.LaunchRemoteEnforcer(nil); err != nil {
			zap.L().Fatal("Unable to start enforcer", zap.Error(err))
		}
	}

	zap.L().Debug("Enforcerd stopped")
}
