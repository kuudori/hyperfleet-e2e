package common

import (
	"fmt"
	"os"
	"strings"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/config"
	"github.com/spf13/viper"
)

// ConfigFile holds the --config flag value set by main package
var ConfigFile string

// LoadConfig loads configuration following HyperFleet standard
// Priority: flags > env vars > config file > defaults
func LoadConfig(configFlagValue string) error {
	// 1. Determine config file path
	configFile := getConfigFilePath(configFlagValue)

	// 2. Load config file FIRST if found
	if configFile != "" {
		viper.SetConfigFile(configFile)
		viper.SetConfigType("yaml")

		if err := viper.ReadInConfig(); err != nil {
			return fmt.Errorf("failed to read config file %s: %w", configFile, err)
		}
	}

	// 3. Enable environment variable support (AFTER reading config file)
	viper.SetEnvPrefix(config.EnvPrefix)
	viper.AutomaticEnv() // Enable automatic environment variable checking

	// Replace dots with underscores in env var names
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// 4. Automatically bind all config paths to environment variables, including
	// the API_ADAPTERS_CLUSTER/API_ADAPTERS_NODEPOOL and AUTH_MODE aliases
	// This uses reflection to traverse the Config struct and bind all paths
	// Binding AFTER reading config ensures env vars override config file values
	config.BindEnvVars()

	return nil
}

// getConfigFilePath determines the config file path following standard priority
// 1. --config flag value
// 2. HYPERFLEET_CONFIG env var
// 3. ./configs/config.yaml (default for development)
func getConfigFilePath(configFlagValue string) string {
	// Priority 1: --config flag
	if configFlagValue != "" {
		return configFlagValue
	}

	// Priority 2: HYPERFLEET_CONFIG env var
	if configPath := os.Getenv(config.EnvPrefix + "_CONFIG"); configPath != "" {
		return configPath
	}

	// Priority 3: Default path (development)
	defaultPath := "./configs/config.yaml"
	if _, err := os.Stat(defaultPath); err == nil {
		return defaultPath
	}

	// No config file found - continue with flags and env vars only
	return ""
}
