package helper

import (
	"context"
	"fmt"
	"net/http"

	"golang.org/x/oauth2"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/config"
)

// CheckDefaultHuman lists clusters with the default human's token and errors
// unless the deployment answers 200.
func CheckDefaultHuman(ctx context.Context, cfg *config.Config, src oauth2.TokenSource) error {
	cl, err := client.NewHyperFleetClient(cfg.API.URL, nil,
		client.WithTokenSource(src, config.AuthorizationSchemeBearer))
	if err != nil {
		return fmt.Errorf("build the default human's API client: %w", err)
	}
	status, err := cl.GetResourceHTTPStatus(ctx, client.ClustersPath)
	if err != nil {
		return fmt.Errorf("list clusters as the default human: %w", err)
	}

	var hint string
	switch status {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized:
		hint = fmt.Sprintf("the deployment rejected the human token; check that AUTH_MODE (%s) "+
			"matches the deployment and that it trusts the mock OIDC issuer in the namespace", cfg.AuthMode)
	case http.StatusForbidden:
		hint = fmt.Sprintf("the gateway denied the human; check that TENANT_MODEL (%s) "+
			"matches the deployed tenant model", cfg.TenantModel)
	default:
		hint = "the API did not answer 200"
	}
	return fmt.Errorf("list clusters as the default human: HTTP %d: %s", status, hint)
}
