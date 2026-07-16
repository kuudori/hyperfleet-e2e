package client

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWithBearerToken(t *testing.T) {
	t.Parallel()

	var gotAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	c, err := NewHyperFleetClient(ts.URL, nil, WithBearerToken("test-token-123"))
	if err != nil {
		t.Fatalf("NewHyperFleetClient: %v", err)
	}

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/test", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if gotAuth != "Bearer test-token-123" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer test-token-123")
	}
}

func TestAuthTransportNilBase(t *testing.T) {
	t.Parallel()

	var gotAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	transport := &authTransport{base: nil, token: "fallback-token"}
	httpClient := &http.Client{Transport: transport}

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/test", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if gotAuth != "Bearer fallback-token" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer fallback-token")
	}
}
