package config

import (
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/aporeto-inc/trireme-lib/controller"
	"github.com/spf13/viper"

	flag "github.com/spf13/pflag"
)

// DefaultKubeConfigLocation is the default location of the KubeConfig file.
const DefaultKubeConfigLocation = "/.kube/config"

// TriremeEnvPrefix is the prefix used to provide configuration through env variables.
const TriremeEnvPrefix = "TRIREME"

// Configuration contains all the User Parameter for Trireme-Kubernetes.
type Configuration struct {
	// AuthType defines if Trireme uses PSK or PKI
	AuthType string
	// KubeNodeName is the identifier used for this Trireme instance
	KubeNodeName string
	// PSK is the PSK used for Trireme (if using PSK)
	PSK string
	// RemoteEnforcer defines if the enforcer is spawned into each POD namespace
	// or into the host default namespace.
	RemoteEnforcer bool
	// BetaNetPolicies defines if Trireme Kubernetes should follow the beta model
	// or the GA model for Network Policies. Default is GA
	BetaNetPolicies bool
	// EgressNetPolicies defines if Trireme Kubernetes should follow the 1.8 model for
	// NetworkPolicies.
	EgressNetPolicies bool

	TriremeNetworks       string
	ParsedTriremeNetworks []string

	KubeconfigPath string

	LogFormat string
	LogLevel  string

	// Credentials info for InfluxDB Collector interface
	CollectorEndpoint           string
	CollectorUser               string
	CollectorPass               string
	CollectorDB                 string
	CollectorInsecureSkipVerify bool

	// Enforce defines if this process is an enforcer process (spawned into POD namespaces)
	Enforce bool `mapstructure:"Enforce"`
}

func usage() {
	flag.PrintDefaults()
	os.Exit(2)
}

// LoadConfig loads a Configuration struct:
// 1) If presents flags are used
// 2) If no flags, Env Variables are used
// 3) If no Env Variables, defaults are used when possible.
func LoadConfig() (*Configuration, error) {
	flag.Usage = usage
	flag.String("AuthType", "", "Authentication type: PKI/PSK")
	flag.String("KubeNodeName", "", "Node name in Kubernetes")
	flag.String("Cacert", "", "Path to the CACert root of trust.")
	flag.String("PSK", "", "PSK to use")
	flag.Bool("RemoteEnforcer", true, "Use the Trireme Remote Enforcer.")
	flag.Bool("BetaNetPolicies", false, "Use old deprecated Beta Network policy model (default: use GA).")
	flag.Bool("EgressNetPolicies", true, "Use new Egress Network policy model (default: use Egress).")
	flag.String("TriremeNetworks", "", "TriremeNetworks")
	flag.String("KubeconfigPath", "", "KubeConfig used to connect to Kubernetes")
	flag.String("LogLevel", "", "Log level. Default to info (trace//debug//info//warn//error//fatal)")
	flag.String("LogFormat", "", "Log Format. Default to human")
	flag.String("CollectorEndpoint", "", "Endpoint for InfluxDB customer collector")
	flag.String("CollectorUser", "", "User info for InfluxDB")
	flag.String("CollectorPass", "", "Pass for InfluxDB")
	flag.String("CollectorDB", "", "DB for InfluxDB")
	flag.Bool("CollectorInsecureSkipVerify", false, "InsecureSkipVerify for InfluxDB")
	flag.Bool("Enforce", false, "Run Trireme-Kubernetes in Enforce mode.")

	// Setting up default configuration
	viper.SetDefault("AuthType", "PSK")
	viper.SetDefault("KubeNodeName", "")
	viper.SetDefault("PKIDirectory", "")
	viper.SetDefault("PSK", "PSK")
	viper.SetDefault("RemoteEnforcer", true)
	viper.SetDefault("BetaNetPolicies", false)
	viper.SetDefault("EgressNetPolicies", true)
	viper.SetDefault("TriremeNetworks", "")
	viper.SetDefault("KubeconfigPath", "")
	viper.SetDefault("LogLevel", "info")
	viper.SetDefault("LogFormat", "human")
	viper.SetDefault("CollectorEndpoint", "")
	viper.SetDefault("CollectorUser", "")
	viper.SetDefault("CollectorPass", "")
	viper.SetDefault("CollectorDB", "")
	viper.SetDefault("CollectorInsecureSkipVerify", "")
	viper.SetDefault("Enforce", false)

	// Binding ENV variables
	// Each config will be of format TRIREME_XYZ as env variable, where XYZ
	// is the upper case config.
	viper.SetEnvPrefix(TriremeEnvPrefix)
	viper.AutomaticEnv()

	// Binding CLI flags.
	flag.Parse()
	viper.BindPFlags(flag.CommandLine)

	var config Configuration

	// Manual check for Enforce mode as this is given as a simple argument
	if len(os.Args) > 1 {
		if os.Args[1] == "enforce" {
			config.Enforce = true
			config.LogLevel = viper.GetString("LogLevel")
			return &config, nil
		}
	}

	err := viper.Unmarshal(&config)
	if err != nil {
		return nil, fmt.Errorf("Error unmarshalling:%s", err)
	}

	err = validateConfig(&config)
	if err != nil {
		return nil, err
	}

	// unset current Trireme Env variables as to keep a clean state for the remote enforcer process.
	unsetEnvVar(TriremeEnvPrefix)

	setupTriremeSubProcessArgs(&config)

	return &config, nil
}

// setupTriremeSubProcessArgs setups the logs for the remote Enforcer
func setupTriremeSubProcessArgs(config *Configuration) {
	logToConsole := true
	logWithID := false

	controller.SetLogParameters(logToConsole, logWithID, config.LogLevel, config.LogFormat)
}

// validateConfig is validating the Configuration struct.
func validateConfig(config *Configuration) error {
	// Validating KUBECONFIG
	// In case not running as InCluster, we try to infer a possible KubeConfig location
	if os.Getenv("KUBERNETES_PORT") == "" {
		if config.KubeconfigPath == "" {
			config.KubeconfigPath = os.Getenv("HOME") + DefaultKubeConfigLocation
		}
	} else {
		config.KubeconfigPath = ""
	}

	// Validating KUBE NODENAME
	if !config.Enforce && config.KubeNodeName == "" {
		return fmt.Errorf("Couldn't load NodeName. Ensure Kubernetes Nodename is given as a parameter")
	}

	// Validating AUTHTYPE
	if config.AuthType != "PSK" && config.AuthType != "PKI" {
		return fmt.Errorf("AuthType should be PSK or PKI")
	}

	// Validating PSK
	if config.AuthType == "PSK" && config.PSK == "" {
		return fmt.Errorf("PSK should be provided")
	}

	parsedTriremeNetworks, err := parseTriremeNets(config.TriremeNetworks)
	if err != nil {
		return fmt.Errorf("TargetNetwork is invalid: %s", err)
	}
	config.ParsedTriremeNetworks = parsedTriremeNetworks

	return nil
}

// parseTriremeNets returns a parsed array of strings parsed based on white spaces between CIDR entries.
// An error is returned if any of the entries is not a valid IP CIDR.
func parseTriremeNets(nets string) ([]string, error) {
	resultNets := strings.Fields(nets)

	// Validation of each networks.
	for _, network := range resultNets {
		_, _, err := net.ParseCIDR(network)
		if err != nil {
			return nil, fmt.Errorf("Invalid CIDR: %s", err)
		}
	}
	return resultNets, nil
}

// unsetEnvVar unsets all env variables with a specific prefix.
// Usage inside Trireme is to unset all Trireme env variables so
// that the remote doesn't get confused.
func unsetEnvVar(prefix string) {
	env := os.Environ()
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			kv := strings.Split(e, "=")
			if len(kv) > 0 {
				if err := os.Unsetenv(kv[0]); err != nil {
					continue
				}
			}
		}
	}
}
