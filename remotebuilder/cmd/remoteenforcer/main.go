package main

import (
	"fmt"
	"net/http"
	"time"

	"github.com/aporeto-inc/trireme-kubernetes/remotebuilder/configuration"
	"github.com/aporeto-inc/trireme-lib/controller"
	"go.uber.org/zap"

	_ "net/http/pprof"
)

func main() {

	cfg := configuration.NewConfiguration()
	fmt.Println(cfg)
	time.Local = time.UTC

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
