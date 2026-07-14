package helper

import (
	"context"
	"log"
	"net/http"
	"sync"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	k8sclient "github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client/kubernetes"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/config"
)

var (
	// suiteConfig is loaded once in cmd layer before tests start
	suiteConfig           *config.Config
	configMutex           sync.RWMutex
	adapterDeploymentList *AdapterDeploymentList
)

// SetSuiteConfig sets the global suite configuration for the test suite
func SetSuiteConfig(cfg *config.Config) {
	configMutex.Lock()
	defer configMutex.Unlock()
	suiteConfig = cfg
}

// GetSuiteConfig returns the global suite configuration
func GetSuiteConfig() *config.Config {
	configMutex.RLock()
	defer configMutex.RUnlock()
	return suiteConfig
}

// ClearSuiteConfig clears the global suite configuration
func ClearSuiteConfig() {
	configMutex.Lock()
	defer configMutex.Unlock()
	suiteConfig = nil
}

func SetAdapterDeploymentList(list *AdapterDeploymentList) {
	configMutex.Lock()
	defer configMutex.Unlock()
	adapterDeploymentList = list
}

func GetAdapterDeploymentList() *AdapterDeploymentList {
	configMutex.RLock()
	defer configMutex.RUnlock()
	return adapterDeploymentList
}

// New creates a helper instance for testing
// Creates a new helper per test
func New() *Helper {
	cfg := GetSuiteConfig()
	if cfg == nil {
		log.Fatalf("Suite config not initialized")
	}
	adapterDeploymentList := GetAdapterDeploymentList()
	if adapterDeploymentList == nil {
		log.Fatalf("Adapter deployment list not initialized")
	}

	k8sClient, err := k8sclient.NewClient()
	if err != nil {
		log.Fatalf("Failed to create K8s client: %v", err)
	}

	// Acquire JWT via K8s TokenRequest API if configured
	if cfg.Identity.TokenRequest.IsEnabled() {
		token, err := k8sClient.CreateToken(
			context.Background(),
			cfg.Identity.TokenRequest.Namespace,
			cfg.Identity.TokenRequest.ServiceAccountName,
			cfg.Identity.TokenRequest.Audience,
			cfg.Identity.TokenRequest.ExpirationSeconds,
		)
		if err != nil {
			log.Fatalf("Failed to acquire JWT via TokenRequest: %v", err)
		}
		cfg.Identity.SetToken(token)
		log.Printf("Acquired JWT for SA %s/%s (audience: %s, expires: %ds)",
			cfg.Identity.TokenRequest.Namespace,
			cfg.Identity.TokenRequest.ServiceAccountName,
			cfg.Identity.TokenRequest.Audience,
			cfg.Identity.TokenRequest.ExpirationSeconds)
	}

	var opts []openapi.ClientOption
	if token := cfg.Identity.Token(); token != "" {
		opts = append(opts, openapi.WithRequestEditorFn(
			func(_ context.Context, req *http.Request) error {
				req.Header.Set("Authorization", "Bearer "+token)
				return nil
			}))
	}

	cl, err := client.NewHyperFleetClient(cfg.API.URL, nil, opts...)
	if err != nil {
		log.Fatalf("Failed to create HyperFleet client: %v", err)
	}

	return &Helper{
		Cfg:                   cfg,
		Client:                cl,
		K8sClient:             k8sClient,
		AdapterDeploymentList: adapterDeploymentList,
	}
}
