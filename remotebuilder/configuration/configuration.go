package configuration

import (
	"fmt"
	"os"
	"strings"

	docopt "github.com/docopt/docopt-go"
	"go.aporeto.io/addedeffect/envopt"
)

const (
	usage = `Aporeto RemoteEnforcer Daemon.

Usage: enforcerd -h | --help
       enforcerd -v | --version
       enforcerd enforce
            [--profiling-enable]
            [--profiling-listen=<address>]
            [--log-level=<log-level>]
            [--log-level-remote=<log-level>]
            [--log-format=<log-format>]
            [--log-to-console]
            [--enable-opentracing]
            [--opentracing-server=<opentracing-server>]

Arguments:
  log-level: trace | debug | info | warn | error | fatal
  log-format: text | json

Options:
    -h --help                                    Show this screen.
    -v --version                                 Show the version.

Profiling Options:
    --profiling-enable                           Enable the CPU profiling [default: true].
    --profiling-listen=<address>                 Profiling server [default: localhost:6061].

Opentracing Options:
    --enable-opentracing                         Enable opentracing [default: false].
    --opentracing-server=<opentracing-server>    Specify a custom opentracing server, use Meister by default [default: ].

Logging Options:
    --log-level=<log-level>                      Log level [default: debug].
    --log-level-remote=<log-level>               Log level for remote enforcers [default: info].
    --log-format=<log-format>                    Log format [default: json].
    --log-id=<log-id>                            Log format.
    --log-to-console                             Log to console [default: true].
`
)

// Configuration is a struct used to load the config file
type Configuration struct {
	Register bool
	Enforce  bool

	EnableProfiling        bool
	ProfilingListenAddress string

	MachineMetadata []string

	EnableOpenTracing bool
	OpenTracingServer string

	LogFormat    string
	LogLevel     string
	LogID        string
	LogToConsole bool
}

// NewConfiguration returns a new Configuration.
//
// It will use the command line arguments to parse the various
// values it holds.
func NewConfiguration() *Configuration {

	if err := envopt.Parse("ENFORCERD", usage); err != nil {
		panic("Cannot run envopt")
	}

	// Remove all env variables with ENFORECRD prefix so remote enforcer doesnt get confused.
	env := os.Environ()
	for _, e := range env {
		if strings.HasPrefix(e, "ENFORCERD") {
			kv := strings.Split(e, "=")
			if len(kv) > 0 {
				if err := os.Unsetenv(kv[0]); err != nil {
					continue
				}
			}
		}
	}

	arguments, err := docopt.Parse(usage, nil, true, "", false)
	if err != nil {
		panic(fmt.Sprintf("Can not parse usage: Error %s\n", err.Error()))
	}

	var cfg *Configuration

	if arguments["enforce"].(bool) {
		LogID, _ := arguments["--log-id"].(string)
		cfg = &Configuration{
			Register:          false,
			Enforce:           true,
			EnableProfiling:   true,
			LogFormat:         arguments["--log-format"].(string),
			LogLevel:          arguments["--log-level"].(string),
			LogID:             LogID,
			LogToConsole:      arguments["--log-to-console"].(bool),
			MachineMetadata:   []string{},
			EnableOpenTracing: arguments["--enable-opentracing"].(bool),
			OpenTracingServer: arguments["--opentracing-server"].(string),
		}
	}

	return cfg
}
