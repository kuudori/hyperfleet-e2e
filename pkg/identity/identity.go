// Package identity mints the human identities that E2E specs use for normal
// API operations. Machine identities stay with the adapters and the
// machine-auth specs, which mint their own ServiceAccount tokens.
package identity

import (
	"errors"
	"fmt"
	"regexp"
)

// TenantModel names a deployment's tenant model. It selects the claim names a
// human token carries.
type TenantModel string

const (
	// TenantModelOnPrem uses org_id (required) and project_id (optional).
	TenantModelOnPrem TenantModel = "onprem"
	// TenantModelOracle uses tenancy_ocid (required) and compartment_id (optional).
	TenantModelOracle TenantModel = "oracle"
)

// Claims returns the token claim names of the model's required and optional
// tenant dimensions.
func (m TenantModel) Claims() (required, optional string, err error) {
	switch m {
	case TenantModelOnPrem:
		return "org_id", "project_id", nil
	case TenantModelOracle:
		return "tenancy_ocid", "compartment_id", nil
	default:
		return "", "", fmt.Errorf("unknown tenant model %q (expected onprem or oracle)", m)
	}
}

// Human describes one human caller. It is comparable and is the cache key for
// that caller's tokens, so two equal values share one token.
type Human struct {
	Subject     string
	Audience    string
	TenantModel TenantModel
	// Tenant is the required tenant dimension: org_id or tenancy_ocid.
	Tenant string
	// Subtenant is the optional tenant dimension: project_id or compartment_id.
	// An empty value omits the claim.
	Subtenant string
	// OmitRequired mints a token without the required tenant claim.
	OmitRequired bool
}

// safeValue is the character set the infra token helper accepts
// (hyperfleet-infra scripts/mint-human-token.sh validate_value).
var safeValue = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:@+-]*$`)

// Validate reports whether the human can be minted.
func (h Human) Validate() error {
	if _, _, err := h.TenantModel.Claims(); err != nil {
		return err
	}
	if !h.OmitRequired && h.Tenant == "" {
		return errors.New("human identity needs a tenant unless OmitRequired is set")
	}
	if h.Subject == "" {
		return errors.New("human identity needs a subject")
	}
	if h.Audience == "" {
		return errors.New("human identity needs an audience")
	}
	for _, f := range []struct{ name, value string }{
		{"subject", h.Subject}, {"audience", h.Audience}, {"tenant", h.Tenant}, {"subtenant", h.Subtenant},
	} {
		if f.value != "" && !safeValue.MatchString(f.value) {
			return fmt.Errorf("human identity %s %q contains unsupported characters", f.name, f.value)
		}
	}
	return nil
}

// scope selects the mock issuer request mapping
// (hyperfleet-infra helm/mock-oidc/values.yaml requestMappings).
func (h Human) scope() string {
	switch {
	case h.OmitRequired:
		return fmt.Sprintf("hyperfleet-%s-missing", h.TenantModel)
	case h.Subtenant != "":
		return fmt.Sprintf("hyperfleet-%s-full", h.TenantModel)
	default:
		return fmt.Sprintf("hyperfleet-%s-required", h.TenantModel)
	}
}
