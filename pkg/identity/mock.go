package identity

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	k8sclient "github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client/kubernetes"
)

// The mock issuer contract, from the hyperfleet-infra mock-oidc chart.
const (
	// MockReleaseName is the Helm release and Service of the mock issuer.
	MockReleaseName = "hyperfleet-mock-oidc"
	mockPort        = 8443
	mockIssuerID    = "default"
	mockClientID    = "hyperfleet-human-helper"
	// mockClientSecret is sent because the issuer requires client
	// authentication to be present. It checks no secret value.
	mockClientSecret = "unused"
)

// MockIssuer mints human tokens from the namespace's mock OIDC issuer and
// caches them per Human. Each test process keeps its own MockIssuer.
type MockIssuer struct {
	logger *slog.Logger
	// connect returns an HTTP client and the token endpoint URL it reaches.
	connect func() (*http.Client, string, error)
	sources map[Human]oauth2.TokenSource
	// host is the Host header of a token request. The issuer builds the iss
	// claim from it, so it must be the in-cluster issuer host.
	host string
	mu   sync.Mutex
}

// NewMockIssuer returns an issuer for the mock OIDC server in namespace,
// reached through the API server's service proxy.
func NewMockIssuer(k8s *k8sclient.Client, namespace string) *MockIssuer {
	host := net.JoinHostPort(
		fmt.Sprintf("%s.%s.svc.cluster.local", MockReleaseName, namespace), strconv.Itoa(mockPort))
	return newMockIssuer(func() (*http.Client, string, error) {
		httpClient, proxyURL, err := k8s.ServiceProxy(namespace, MockReleaseName, "https", mockPort)
		if err != nil {
			return nil, "", err
		}
		return httpClient, proxyURL + "/" + mockIssuerID + "/token", nil
	}, host)
}

func newMockIssuer(connect func() (*http.Client, string, error), host string) *MockIssuer {
	return &MockIssuer{
		logger:  slog.Default(),
		connect: connect,
		sources: map[Human]oauth2.TokenSource{},
		host:    host,
	}
}

// TokenSource returns the cached token source for h. A token is minted again
// once less than client.DefaultHTTPTimeout of its lifetime is left.
func (m *MockIssuer) TokenSource(h Human) oauth2.TokenSource {
	m.mu.Lock()
	defer m.mu.Unlock()
	if src, ok := m.sources[h]; ok {
		return src
	}
	src := oauth2.ReuseTokenSourceWithExpiry(nil, mintSource{issuer: m, human: h}, client.DefaultHTTPTimeout)
	m.sources[h] = src
	return src
}

type mintSource struct {
	issuer *MockIssuer
	human  Human
}

func (s mintSource) Token() (*oauth2.Token, error) {
	return s.issuer.mint(s.human)
}

func (m *MockIssuer) mint(h Human) (*oauth2.Token, error) {
	if err := h.Validate(); err != nil {
		return nil, err
	}
	// oauth2.TokenSource takes no context, so each mint gets its own deadline.
	ctx, cancel := context.WithTimeout(context.Background(), client.DefaultHTTPTimeout)
	defer cancel()
	httpClient, tokenURL, err := m.connect()
	if err != nil {
		return nil, fmt.Errorf("mint human token: reach %s: %w", MockReleaseName, err)
	}
	ctx = context.WithValue(ctx, oauth2.HTTPClient, &http.Client{
		Transport: hostHeader{base: httpClient.Transport, host: m.host},
	})

	required, optional, err := h.TenantModel.Claims()
	if err != nil {
		return nil, err
	}
	params := url.Values{"audience": {h.Audience}, "subject": {h.Subject}}
	params.Set(required, h.Tenant)
	params.Set(optional, h.Subtenant)
	cfg := clientcredentials.Config{
		ClientID:     mockClientID,
		ClientSecret: mockClientSecret,
		TokenURL:     tokenURL,
		Scopes:       []string{h.scope()},
		// The API server proxy consumes the Authorization header, so the
		// client credentials go in the form body.
		AuthStyle:      oauth2.AuthStyleInParams,
		EndpointParams: params,
	}
	tok, err := cfg.Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("mint human token for %s: %w", h.Subject, err)
	}
	m.logger.Info("minted human token",
		"subject", h.Subject,
		"audience", h.Audience,
		"tenant-model", h.TenantModel,
		"tenant", h.Tenant,
		"subtenant", h.Subtenant,
		"omit-required", h.OmitRequired,
		"expires-in", time.Until(tok.Expiry).Round(time.Second))
	return tok, nil
}

// hostHeader sets the Host header of every request it sends.
type hostHeader struct {
	base http.RoundTripper
	host string
}

func (t hostHeader) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Host = t.host
	return t.base.RoundTrip(req)
}
