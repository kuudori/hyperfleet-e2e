package e2e

import (
	"context"
	"fmt"
	"log"
	"log/slog"

	"github.com/onsi/ginkgo/v2"

	k8sclient "github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client/kubernetes"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/identity"
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

var _ = ginkgo.SynchronizedBeforeSuite(
	// Process 1 only: run the read-only startup checks once.
	func(ctx ginkgo.SpecContext) []byte {
		cfg := GetSuiteConfig()
		if cfg == nil {
			log.Fatalf("Suite config not initialized")
		}

		// Fail before any spec runs when the deployed API does not serve the
		// entity configuration this suite's descriptor table expects. Entity
		// routing is independent of authentication, so this also runs for
		// AUTH_MODE=NONE. Ginkgo does not execute setup nodes in dry-run, so
		// listing specs stays cluster-free even though this hook is registered.
		if err := helper.ValidateAPIEntityConfig(ctx, cfg.Namespace, config.EntityDescriptors()); err != nil {
			ginkgo.Fail(err.Error())
		}
		return nil
	},
	// Runs on every process: set up this process's human tokens and finish suite setup.
	func(ctx ginkgo.SpecContext, _ []byte) {
		cfg := GetSuiteConfig()
		if cfg == nil {
			log.Fatalf("Suite config not initialized")
		}

		if err := initLogging(&cfg.Log); err != nil {
			log.Fatalf("Failed to initialize logger: %v", err)
		}

		if ginkgo.GinkgoParallelProcess() == 1 {
			cfg.Display()
		}
		slog.Info("starting hyperfleet-e2e test suite - creating resources with", "run-id", cfg.RunID)

		if !config.IsDryRun() && cfg.AuthMode != config.AuthModeNone {
			setUpHumanAuth(ctx, cfg)
		}

		// Initialize adapter deployment list - for test tiers that deploy temporary adapters.
		adapterDeploymentList := helper.InitAdapterDeploymentList()
		helper.SetAdapterDeploymentList(adapterDeploymentList)

		slog.Info("starting hyperfleet-e2e test suite - each test creates temporary resources")
	},
)

// setUpHumanAuth mints this process's default human token and checks that the
// deployment accepts it, so a missing mock or a config mismatch fails the suite
// before any spec runs.
func setUpHumanAuth(ctx context.Context, cfg *config.Config) {
	k8s, err := k8sclient.NewClient()
	if err != nil {
		log.Fatalf("Failed to create K8s client for human tokens: %v", err)
	}
	issuer := identity.NewMockIssuer(k8s, cfg.Namespace)
	human := cfg.DefaultHuman()
	src := issuer.TokenSource(human)
	if _, err := src.Token(); err != nil {
		ginkgo.Fail(fmt.Sprintf(
			"mint the default human token from release %q in namespace %q: %v\n"+
				"See docs/debugging.md#human-token-mint-failed", identity.MockReleaseName, cfg.Namespace, err))
	}
	if err := helper.CheckDefaultHuman(ctx, cfg, src); err != nil {
		ginkgo.Fail(fmt.Sprintf("the deployment does not accept the default human %s: %v\n"+
			"See docs/debugging.md#default-human-rejected", human.Subject, err))
	}
	helper.SetHumanAuth(src)
}

var _ = ginkgo.SynchronizedAfterSuite(
	// Per-process: sweep Pub/Sub resources. Safe to call from every process
	// since each tracks its own AdapterDeploymentList in memory.
	func() {
		helper.CleanupPubSubResources()
	},
	// Process 1 only, after all processes finish: sweep Helm releases and labeled K8s
	// resources. Running this once prevents sweeping resources that belong to specs still
	// executing on other processes.
	func() {
		helper.CleanupKubeResources()
		helper.ClearSuiteConfig()
		slog.Info("test suite completed")
	},
)
