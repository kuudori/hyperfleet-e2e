package helper

import (
    "context"
    "fmt"
    "path/filepath"

    "github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/api/openapi"
    "github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
    k8sclient "github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client/kubernetes"
    "github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client/maestro"
    "github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/config"
    "github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/logger"
)

// Helper provides utility functions for e2e tests
type Helper struct {
    Cfg           *config.Config
    Client        *client.HyperFleetClient
    K8sClient     *k8sclient.Client
    MaestroClient *maestro.Client
}

// TestDataPath resolves a relative path within the testdata directory
// This ensures testdata paths work correctly whether invoked via go test or the e2e binary
func (h *Helper) TestDataPath(relativePath string) string {
    return filepath.Join(h.Cfg.TestDataDir, relativePath)
}

// GetTestCluster creates a new temporary test cluster
func (h *Helper) GetTestCluster(ctx context.Context, payloadPath string) (string, error) {
    cluster, err := h.Client.CreateClusterFromPayload(ctx, payloadPath)
    if err != nil {
        return "", err
    }
    if cluster == nil {
        return "", fmt.Errorf("CreateClusterFromPayload returned nil")
    }
    if cluster.Id == nil {
        return "", fmt.Errorf("created cluster has no ID")
    }
    return *cluster.Id, nil
}

// CleanupTestCluster deletes the temporary test cluster
// TODO: Replace this workaround with API DELETE once HyperFleet API supports
// DELETE operations for clusters resource type:
//
//    return h.Client.DeleteCluster(ctx, clusterID)
//
// Temporary workaround: delete the Kubernetes namespace using client-go (may temporarily hardcode a timeout duration).
func (h *Helper) CleanupTestCluster(ctx context.Context, clusterID string) error {
    logger.Info("deleting cluster namespace (workaround)", "cluster_id", clusterID, "namespace", clusterID)

    // Guard against nil K8sClient
    if h == nil || h.K8sClient == nil {
        err := fmt.Errorf("K8sClient is nil, cannot delete namespace")
        logger.Error("K8sClient is nil", "cluster_id", clusterID)
        return err
    }

    // Delete namespace and wait for deletion to complete
    err := h.K8sClient.DeleteNamespaceAndWait(ctx, clusterID)
    if err != nil {
        logger.Error("failed to delete cluster namespace", "cluster_id", clusterID, "error", err)
        return err
    }

    logger.Info("successfully deleted cluster namespace", "cluster_id", clusterID)
    return nil
}

// GetTestNodePool creates a nodepool on the specified cluster from a payload file
func (h *Helper) GetTestNodePool(ctx context.Context, clusterID, payloadPath string) (*openapi.NodePool, error) {
    return h.Client.CreateNodePoolFromPayload(ctx, clusterID, payloadPath)
}

// CleanupTestNodePool cleans up test nodepool
func (h *Helper) CleanupTestNodePool(ctx context.Context, clusterID, nodepoolID string) error {
    return h.Client.DeleteNodePool(ctx, clusterID, nodepoolID)
}

// GetMaestroClient returns the Maestro client, initializing it lazily on first access
// This avoids the overhead of K8s service discovery for test suites that don't use Maestro
func (h *Helper) GetMaestroClient() *maestro.Client {
    if h.MaestroClient == nil {
        h.MaestroClient = maestro.NewClient("")
    }
    return h.MaestroClient
}
