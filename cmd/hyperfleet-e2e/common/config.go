package common

import (
	"fmt"
	"os"
	"reflect"
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

	// 4. Automatically bind all config paths to environment variables
	// This uses reflection to traverse the Config struct and bind all paths
	// Binding AFTER reading config ensures env vars override config file values
	bindAllEnvVars()

	// 5. Add custom environment variable bindings
	// These allow alternative env var names for specific config paths
	_ = viper.BindEnv("adapters.cluster", "API_ADAPTERS_CLUSTER")
	_ = viper.BindEnv("adapters.nodepool", "API_ADAPTERS_NODEPOOL")

	return nil
}

// bindAllEnvVars automatically binds all config struct fields to environment variables
// using reflection to traverse the config.Config structure
func bindAllEnvVars() {
	// Get the Config struct type
	configType := reflect.TypeOf(config.Config{})

	// Recursively bind all fields
	bindStructFields(configType, "")
}

// bindStructFields recursively binds struct fields to environment variables
func bindStructFields(t reflect.Type, prefix string) {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		tag := field.Tag.Get(config.TagMapstructure)
		if tag == "" {
			continue
		}

		var configPath string
		if prefix == "" {
			configPath = tag
		} else {
			configPath = prefix + "." + tag
		}

		// Convert dots to underscores: api.url -> HYPERFLEET_API_URL
		envVar := config.EnvVar(strings.ToUpper(strings.ReplaceAll(configPath, ".", "_")))

		_ = viper.BindEnv(configPath, envVar)

		fieldType := field.Type
		if fieldType.Kind() == reflect.Struct {
			bindStructFields(fieldType, configPath)
		}
	}
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
