package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// HyperFleetClient is the E2E test client for the HyperFleet API.
type HyperFleetClient struct {
	httpClient *http.Client
	baseURL    string
}

// ClientOption configures the HyperFleet client.
type ClientOption func(*HyperFleetClient)

// WithBearerToken returns a ClientOption that injects a JWT bearer token
// into all outgoing requests via a custom round-tripper.
func WithBearerToken(token string) ClientOption {
	return func(c *HyperFleetClient) {
		c.httpClient.Transport = &authTransport{
			base:  c.httpClient.Transport,
			token: token,
		}
	}
}

// authTransport injects an Authorization header into every request.
type authTransport struct {
	base  http.RoundTripper
	token string
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	r.Header.Set("Authorization", "Bearer "+t.token)
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(r)
}

// NewHyperFleetClient creates a new HyperFleet API client.
func NewHyperFleetClient(baseURL string, httpClient *http.Client, opts ...ClientOption) (*HyperFleetClient, error) {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	c := &HyperFleetClient{
		httpClient: httpClient,
		baseURL:    strings.TrimRight(baseURL, "/"),
	}

	for _, opt := range opts {
		opt(c)
	}

	return c, nil
}

// HTTPError represents an unexpected HTTP status code from the API.
type HTTPError struct {
	StatusCode int
	Action     string
	Body       string
}

func (e *HTTPError) Error() string {
	if e.Body != "" {
		return fmt.Sprintf("unexpected status code %d for %s: %s", e.StatusCode, e.Action, e.Body)
	}
	return fmt.Sprintf("unexpected status code %d for %s", e.StatusCode, e.Action)
}

func handleHTTPResponse[T any](resp *http.Response, expectedStatus int, action string) (*T, error) {
	defer func() { _ = resp.Body.Close() }()

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
	defer func() { _ = resp.Body.Close() }()

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
