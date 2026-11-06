package client

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// HyperFleetClient is the E2E test client for the HyperFleet API.
type HyperFleetClient struct {
	httpClient *http.Client
	baseURL    string
	apiHost    string
}

// ClientOption configures the HyperFleet client.
type ClientOption func(*HyperFleetClient)

// DefaultHTTPTimeout bounds every API request. Token sources use it as their
// refresh margin, so a token cannot expire mid-request.
const DefaultHTTPTimeout = 30 * time.Second

// WithTokenSource returns a ClientOption that asks src for a token on every
// request to the API host, so a cached source can refresh during a spec.
func WithTokenSource(src oauth2.TokenSource, scheme string) ClientOption {
	if scheme == "" {
		scheme = "Bearer"
	}
	return func(c *HyperFleetClient) {
		c.httpClient.Transport = &authTransport{
			base:   c.httpClient.Transport,
			src:    src,
			scheme: scheme,
			host:   c.apiHost,
		}
	}
}

// WithAuthorizationToken returns a ClientOption that injects a fixed token with
// the requested Authorization scheme into requests for the API host.
func WithAuthorizationToken(token, scheme string) ClientOption {
	return WithTokenSource(oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token}), scheme)
}

// WithBearerToken returns a ClientOption that injects a JWT bearer token.
func WithBearerToken(token string) ClientOption {
	return WithAuthorizationToken(token, "Bearer")
}

// authTransport injects an Authorization header into requests targeting the API host.
// It does not attach the token to cross-origin redirects (CWE-200/CWE-522).
type authTransport struct {
	base   http.RoundTripper
	src    oauth2.TokenSource
	scheme string
	host   string
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	if t.host == "" || r.URL.Host == t.host {
		token, err := t.src.Token()
		if err != nil {
			return nil, fmt.Errorf("obtain API token: %w", err)
		}
		r.Header.Set("Authorization", t.scheme+" "+token.AccessToken)
	}
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(r)
}

// NewHyperFleetClient creates a new HyperFleet API client.
func NewHyperFleetClient(baseURL string, httpClient *http.Client, opts ...ClientOption) (*HyperFleetClient, error) {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: DefaultHTTPTimeout}
	}

	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL %q: %w", baseURL, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("invalid base URL %q: scheme and host are required", baseURL)
	}

	c := &HyperFleetClient{
		httpClient: httpClient,
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiHost:    u.Host,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c, nil
}

// HTTPError represents an unexpected HTTP status code from the API.
type HTTPError struct {
	Action     string
	Body       string
	StatusCode int
}

func (e *HTTPError) Error() string {
	if e.Body != "" {
		return fmt.Sprintf("unexpected status code %d for %s: %s", e.StatusCode, e.Action, e.Body)
	}
	return fmt.Sprintf("unexpected status code %d for %s", e.StatusCode, e.Action)
}

func handleHTTPResponse[T any](resp *http.Response, expectedStatus int, action string) (*T, error) {
	defer closeResponseBody(resp.Body)

	if resp.StatusCode != expectedStatus {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, &HTTPError{
				StatusCode: resp.StatusCode,
				Action:     action,
				Body:       fmt.Sprintf("failed to read error response body: %v", err),
			}
		}
		return nil, &HTTPError{
			StatusCode: resp.StatusCode,
			Action:     action,
			Body:       string(body),
		}
	}

	var result T
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode %s response: %w", action, err)
	}

	return &result, nil
}

func handleHTTPNoBodyResponse(resp *http.Response, expectedStatus int, action string) error {
	defer closeResponseBody(resp.Body)

	if resp.StatusCode != expectedStatus {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return &HTTPError{
				StatusCode: resp.StatusCode,
				Action:     action,
				Body:       fmt.Sprintf("failed to read error response body: %v", err),
			}
		}
		return &HTTPError{
			StatusCode: resp.StatusCode,
			Action:     action,
			Body:       string(body),
		}
	}

	return nil
}

func closeResponseBody(body io.Closer) {
	if err := body.Close(); err != nil {
		// Closing the response is cleanup; preserve the request/decode result for the caller.
		slog.Warn("failed to close HTTP response body", "error", err)
	}
}
