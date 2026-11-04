package client

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"golang.org/x/oauth2"
)

type failingCloseBody struct {
	io.Reader
	closed bool
}

func (b *failingCloseBody) Close() error {
	b.closed = true
	return errors.New("response close failed")
}

func TestResponseCleanupPreservesResult(t *testing.T) {
	t.Parallel()
	for _, status := range []int{http.StatusOK, http.StatusBadRequest} {
		body := &failingCloseBody{Reader: strings.NewReader(`{"name":"test"}`)}
		resource, err := handleHTTPResponse[Resource](
			&http.Response{StatusCode: status, Body: body}, http.StatusOK, "get resource",
		)
		if !body.closed {
			t.Fatal("response body was not closed")
		}
		if status == http.StatusOK {
			if err != nil || resource == nil || resource.Name != "test" {
				t.Fatalf("cleanup changed successful response: resource=%+v err=%v", resource, err)
			}
		} else {
			var httpErr *HTTPError
			if !errors.As(err, &httpErr) || httpErr.StatusCode != status {
				t.Fatalf("cleanup replaced the HTTP error: %v", err)
			}
		}
	}
	body := &failingCloseBody{Reader: strings.NewReader("")}
	if err := handleHTTPNoBodyResponse(
		&http.Response{StatusCode: http.StatusNoContent, Body: body}, http.StatusNoContent, "delete resource",
	); err != nil || !body.closed {
		t.Fatalf("no-body cleanup changed the result: closed=%v err=%v", body.closed, err)
	}
}

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

func TestWithAuthorizationTokenScheme(t *testing.T) {
	tests := []struct {
		name   string
		scheme string
		want   string
	}{
		{name: "ServiceAccount", scheme: "ServiceAccount", want: "ServiceAccount token-value"},
		{name: "empty scheme defaults to Bearer", want: "Bearer token-value"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var gotAuth string
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotAuth = r.Header.Get("Authorization")
				w.WriteHeader(http.StatusOK)
			}))
			defer ts.Close()

			c, err := NewHyperFleetClient(ts.URL, nil, WithAuthorizationToken("token-value", tt.scheme))
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

			if gotAuth != tt.want {
				t.Errorf("Authorization header = %q, want %q", gotAuth, tt.want)
			}
		})
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

	transport := &authTransport{
		base:   nil,
		src:    oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "fallback-token"}),
		scheme: "Bearer",
	}
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

type countingTokenSource struct {
	err   error
	token string
	calls atomic.Int32
}

func (s *countingTokenSource) Token() (*oauth2.Token, error) {
	n := s.calls.Add(1)
	if s.err != nil {
		return nil, s.err
	}
	return &oauth2.Token{AccessToken: fmt.Sprintf("%s-%d", s.token, n)}, nil
}

func TestWithTokenSourceReadsTokenPerRequest(t *testing.T) {
	t.Parallel()

	var gotAuth []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = append(gotAuth, r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	src := &countingTokenSource{token: "human"}
	c, err := NewHyperFleetClient(ts.URL, nil, WithTokenSource(src, "Bearer"))
	if err != nil {
		t.Fatalf("NewHyperFleetClient: %v", err)
	}
	for range 2 {
		if _, err := c.GetResourceHTTPStatus(t.Context(), "clusters"); err != nil {
			t.Fatalf("GetResourceHTTPStatus: %v", err)
		}
	}

	want := []string{"Bearer human-1", "Bearer human-2"}
	if !slices.Equal(gotAuth, want) {
		t.Fatalf("Authorization headers = %q, want %q", gotAuth, want)
	}
}

func TestWithTokenSourceErrorStopsRequest(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	mintErr := errors.New("mint failed")
	c, err := NewHyperFleetClient(ts.URL, nil, WithTokenSource(&countingTokenSource{err: mintErr}, "Bearer"))
	if err != nil {
		t.Fatalf("NewHyperFleetClient: %v", err)
	}
	_, err = c.GetResourceHTTPStatus(t.Context(), "clusters")
	if !errors.Is(err, mintErr) || !strings.Contains(err.Error(), "obtain API token") {
		t.Fatalf("error = %v, want wrapped mint error", err)
	}
	if hits.Load() != 0 {
		t.Fatalf("server received %d requests, want none", hits.Load())
	}
}

func TestWithTokenSourceSkipsOtherHosts(t *testing.T) {
	t.Parallel()

	var gotAuth string
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer other.Close()

	src := &countingTokenSource{token: "human"}
	c, err := NewHyperFleetClient("https://api.example.com", nil, WithTokenSource(src, "Bearer"))
	if err != nil {
		t.Fatalf("NewHyperFleetClient: %v", err)
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, other.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if gotAuth != "" || src.calls.Load() != 0 {
		t.Fatalf("other host got Authorization %q after %d token reads, want none", gotAuth, src.calls.Load())
	}
}
