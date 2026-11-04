package helper

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/oauth2"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/identity"
)

func TestCheckDefaultHuman(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		wantErr  []string
		status   int
		wantPass bool
	}{
		{name: "accepted", status: http.StatusOK, wantPass: true},
		{name: "token rejected", status: http.StatusUnauthorized, wantErr: []string{"HTTP 401", "AUTH_MODE (EDGE+API)"}},
		{name: "tenant model mismatch", status: http.StatusForbidden, wantErr: []string{"HTTP 403", "TENANT_MODEL (oracle)"}},
		{name: "other status", status: http.StatusInternalServerError, wantErr: []string{"HTTP 500"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var gotAuth, gotPath string
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotAuth, gotPath = r.Header.Get("Authorization"), r.URL.Path
				w.WriteHeader(tt.status)
			}))
			t.Cleanup(ts.Close)
			cfg := &config.Config{
				API:         config.APIConfig{URL: ts.URL},
				AuthMode:    config.AuthModeEdgeAPI,
				TenantModel: identity.TenantModelOracle,
			}

			err := CheckDefaultHuman(t.Context(), cfg, oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "human-token"}))

			if gotAuth != "Bearer human-token" {
				t.Errorf("Authorization = %q, want the default human's Bearer token", gotAuth)
			}
			if gotPath != "/api/hyperfleet/v1/clusters" {
				t.Errorf("path = %q, want the clusters list", gotPath)
			}
			if tt.wantPass {
				if err != nil {
					t.Fatalf("CheckDefaultHuman: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("CheckDefaultHuman succeeded on HTTP %d", tt.status)
			}
			for _, want := range tt.wantErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error = %q, want it to contain %q", err, want)
				}
			}
		})
	}
}

type failingTokenSource struct{}

func (failingTokenSource) Token() (*oauth2.Token, error) { return nil, errors.New("mint failed") }

func TestCheckDefaultHumanReportsATokenError(t *testing.T) {
	t.Parallel()
	called := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(ts.Close)
	cfg := &config.Config{API: config.APIConfig{URL: ts.URL}}

	err := CheckDefaultHuman(t.Context(), cfg, failingTokenSource{})
	if err == nil || !strings.Contains(err.Error(), "mint failed") {
		t.Fatalf("error = %v, want the token source error", err)
	}
	if called {
		t.Error("the API got a request without a token")
	}
}
