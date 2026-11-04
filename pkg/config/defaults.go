package config

import (
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/identity"
)

// DefaultAuthMode enables gateway and API validation by default.
const DefaultAuthMode = AuthModeEdgeAPI

// DefaultTenantModel matches the hyperfleet-infra deploy default (TENANT_MODEL).
const DefaultTenantModel = identity.TenantModelOnPrem

// DefaultE2ERunnerServiceAccountName is the fixed machine identity for E2E requests.
const DefaultE2ERunnerServiceAccountName = "hyperfleet-e2e-runner"

// Log level constants
const (
	// LogLevelDebug enables detailed test steps and all framework internal logs
	LogLevelDebug = "debug"

	// LogLevelInfo enables detailed test steps and high-level framework logs (default)
	LogLevelInfo = "info"

	// LogLevelWarn shows only warnings and errors (minimal output for CI/CD)
	LogLevelWarn = "warn"

	// LogLevelError shows only errors (absolute minimal output)
	LogLevelError = "error"
)

// Log format constants
const (
	LogFormatJSON = "json"
	LogFormatText = "text"
)

// Log output constants
const (
	LogOutputStdout = "stdout"
	LogOutputStderr = "stderr"
)

// Default timeout values. Per-kind reconciled and deleted timeouts are row
// defaults in the descriptor table (descriptor.go).
const (
	// DefaultAdapterProcessingTimeout is the default timeout for waiting for adapter conditions
	DefaultAdapterProcessingTimeout = 5 * time.Minute

	// DefaultPollInterval is the default interval for polling operations
	DefaultPollInterval = 10 * time.Second

	// DefaultLogLevel is the default log level
	DefaultLogLevel = LogLevelInfo

	// DefaultLogFormat is the default log format
	DefaultLogFormat = LogFormatText

	// DefaultLogOutput is the default log output
	DefaultLogOutput = LogOutputStdout

	// DefaultTokenRequestExpirationSeconds is the default lifetime for TokenRequest tokens
	DefaultTokenRequestExpirationSeconds int64 = 7200

	// DefaultTokenRequestAudience is the default audience for TokenRequest tokens
	DefaultTokenRequestAudience = "hyperfleet-api"

	// DefaultHumanSubject is the subject, and the email claim, of the human
	// behind h.Client.
	DefaultHumanSubject = "hyperfleet-e2e@example.com"

	// DefaultHumanAudience is the audience of the default human's token.
	DefaultHumanAudience = DefaultTokenRequestAudience

	// DefaultHumanSubtenant is the default human's optional tenant dimension.
	// The tenant itself is the run ID.
	DefaultHumanSubtenant = "e2e"

	// DefaultSystemIdentity is the audit identity the API records when it has
	// no caller identity (hyperfleet-api pkg/services/util.go defaultSystemUser).
	DefaultSystemIdentity = "system@hyperfleet.local"

	// DefaultHyperfleetAPIBaseURL is the in-cluster base URL for the HyperFleet API.
	DefaultHyperfleetAPIBaseURL = "http://hyperfleet-gateway:8000"

	// DefaultDesireAdapter is the remote-transport cluster adapter hyperfleet-infra deploys;
	// it is one of the Cluster row's required adapters.
	DefaultDesireAdapter = "cl-desire"

	// DefaultBrokerType is the default broker type
	DefaultBrokerType = BrokerTypeGooglePubSub
)
