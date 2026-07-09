package e2e

import (
	"github.com/onsi/ginkgo/v2"
	"log"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/logger"
)

var (
	// suiteConfig is loaded once in cmd layer before tests start
	suiteConfig *config.Config
)

// SetSuiteConfig sets the global suite configuration for both e2e and helper packages
func SetSuiteConfig(cfg *config.Config) {
	suiteConfig = cfg
	helper.SetSuiteConfig(cfg)
}

// GetSuiteConfig returns the global suite configuration
func GetSuiteConfig() *config.Config {
	return suiteConfig
}

var _ = ginkgo.BeforeSuite(func() {
	cfg := GetSuiteConfig()
	if cfg == nil {
		log.Fatalf("Suite config not initialized")
	}

	if err := logger.Init(&cfg.Log, "dev"); err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}

	cfg.Display()
	logger.Info("starting hyperfleet-e2e test suite - creating resources with", "run-id", cfg.RunID)
})

var _ = ginkgo.AfterSuite(func() {
	helper.CleanupResources()
	helper.ClearSuiteConfig()
	logger.Info("test suite completed")
})
