package identity

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	k8sclient "github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client/kubernetes"
)

// fakeIssuer stands in for the API server proxy to the mock OIDC server: it
// serves the token endpoint at the proxy path and records each request.
type fakeIssuer struct {
	server    *httptest.Server
	requests  []url.Values
	hosts     []string
	expiresIn int
	mu        sync.Mutex
	hits      atomic.Int32
	// failNext rejects the next mint.
	failNext atomic.Bool
}

func newFakeIssuer(t *testing.T, expiresIn int) *fakeIssuer {
	t.Helper()
	f := &fakeIssuer{expiresIn: expiresIn}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := f.hits.Add(1)
		if f.failNext.CompareAndSwap(true, false) {
			http.Error(w, "issuer unavailable", http.StatusServiceUnavailable)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// The API server proxy consumes the Authorization header, so client
		// credentials must arrive in the form body.
		if r.URL.Path != fakeProxyPath+"/default/token" || r.Header.Get("Authorization") != "" ||
			r.PostForm.Get("client_id") != mockClientID || r.PostForm.Get("client_secret") == "" {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		f.requests = append(f.requests, r.PostForm)
		f.hosts = append(f.hosts, r.Host)
		f.mu.Unlock()
		// Slow enough that concurrent callers overlap with one mint.
		time.Sleep(50 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "token-" + string('0'+n),
			"token_type":   "Bearer",
			"expires_in":   f.expiresIn,
		})
	}))
	t.Cleanup(f.server.Close)
	return f
}

const (
	fakeProxyPath  = "/api/v1/namespaces/ns/services/https:hyperfleet-mock-oidc:8443/proxy"
	fakeIssuerHost = "hyperfleet-mock-oidc.ns.svc.cluster.local:8443"
)

// issuer returns a MockIssuer that mints from the fake server.
func (f *fakeIssuer) issuer(t *testing.T, logs *bytes.Buffer) *MockIssuer {
	t.Helper()
	m := newMockIssuer(func() (*http.Client, string, error) {
		return f.server.Client(), f.server.URL + fakeProxyPath + "/default/token", nil
	}, fakeIssuerHost)
	if logs != nil {
		m.logger = slog.New(slog.NewTextHandler(logs, nil))
	}
	return m
}

func onPremHuman() Human {
	return Human{
		Subject: "e2e@example.com", Audience: "hyperfleet-api",
		TenantModel: TenantModelOnPrem, Tenant: "run-1", Subtenant: "e2e",
	}
}

func TestTokenSourceSharesOneMintAcrossCallers(t *testing.T) {
	t.Parallel()
	f := newFakeIssuer(t, 600)
	m := f.issuer(t, nil)

	var wg sync.WaitGroup
	tokens := make([]string, 16)
	for i := range tokens {
		wg.Go(func() {
			tok, err := m.TokenSource(onPremHuman()).Token()
			if err != nil {
				t.Errorf("Token: %v", err)
				return
			}
			tokens[i] = tok.AccessToken
		})
	}
	wg.Wait()

	if got := f.hits.Load(); got != 1 {
		t.Fatalf("issuer served %d mints, want 1", got)
	}
	for _, tok := range tokens {
		if tok != tokens[0] {
			t.Fatalf("callers got different tokens: %q", tokens)
		}
	}
	if f.hosts[0] != fakeIssuerHost {
		t.Fatalf("issuer saw Host %q, want the in-cluster issuer host", f.hosts[0])
	}
}

func TestTokenSourceMintCount(t *testing.T) {
	t.Parallel()
	human := onPremHuman()
	other := onPremHuman()
	other.Tenant = "run-2"
	tests := []struct {
		name      string
		reads     []Human
		expiresIn int
		wantMints int32
	}{
		{name: "one mint per distinct human", expiresIn: 600, reads: []Human{human, other, human}, wantMints: 2},
		// 10 s of lifetime is under the 30 s margin, so every read mints again.
		{name: "every read mints inside the margin", expiresIn: 10, reads: []Human{human, human, human}, wantMints: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFakeIssuer(t, tt.expiresIn)
			m := f.issuer(t, nil)
			for _, h := range tt.reads {
				if _, err := m.TokenSource(h).Token(); err != nil {
					t.Fatalf("Token: %v", err)
				}
			}
			if got := f.hits.Load(); got != tt.wantMints {
				t.Fatalf("issuer served %d mints, want %d", got, tt.wantMints)
			}
		})
	}
}

func TestMintSendsTheModelClaims(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		want  map[string]string
		human Human
	}{
		{
			name:  "onprem full",
			human: onPremHuman(),
			want: map[string]string{"scope": "hyperfleet-onprem-full", "org_id": "run-1", "project_id": "e2e",
				"subject": "e2e@example.com", "audience": "hyperfleet-api"},
		},
		{
			name: "oracle full",
			human: Human{Subject: "o@example.com", Audience: "aud", TenantModel: TenantModelOracle,
				Tenant: "ocid1.tenancy.oc1..a", Subtenant: "ocid1.compartment.oc1..b"},
			want: map[string]string{"scope": "hyperfleet-oracle-full", "tenancy_ocid": "ocid1.tenancy.oc1..a",
				"compartment_id": "ocid1.compartment.oc1..b", "subject": "o@example.com", "audience": "aud"},
		},
		{
			name: "oracle required only",
			human: Human{Subject: "o@example.com", Audience: "aud", TenantModel: TenantModelOracle,
				Tenant: "ocid1.tenancy.oc1..a"},
			want: map[string]string{"scope": "hyperfleet-oracle-required", "tenancy_ocid": "ocid1.tenancy.oc1..a",
				"compartment_id": "", "subject": "o@example.com", "audience": "aud"},
		},
		{
			name: "onprem missing required claim",
			human: Human{Subject: "m@example.com", Audience: "aud", TenantModel: TenantModelOnPrem,
				OmitRequired: true},
			want: map[string]string{"scope": "hyperfleet-onprem-missing", "org_id": "", "project_id": "",
				"subject": "m@example.com", "audience": "aud"},
		},
		{
			// The issuer's missing mapping keeps the optional claim.
			name: "oracle missing required claim",
			human: Human{Subject: "m@example.com", Audience: "aud", TenantModel: TenantModelOracle,
				Subtenant: "ocid1.compartment.oc1..b", OmitRequired: true},
			want: map[string]string{"scope": "hyperfleet-oracle-missing", "tenancy_ocid": "",
				"compartment_id": "ocid1.compartment.oc1..b", "subject": "m@example.com", "audience": "aud"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFakeIssuer(t, 600)
			m := f.issuer(t, nil)
			if _, err := m.TokenSource(tt.human).Token(); err != nil {
				t.Fatalf("Token: %v", err)
			}
			got := f.requests[0]
			for key, want := range tt.want {
				if got.Get(key) != want {
					t.Errorf("%s = %q, want %q", key, got.Get(key), want)
				}
			}
			for _, foreign := range []string{"org_id", "project_id", "tenancy_ocid", "compartment_id"} {
				if _, ok := tt.want[foreign]; !ok && got.Has(foreign) {
					t.Errorf("sent claim %s, which belongs to the other tenant model", foreign)
				}
			}
		})
	}
}

func TestMintLogsWithoutTheToken(t *testing.T) {
	t.Parallel()
	f := newFakeIssuer(t, 600)
	var logs bytes.Buffer
	m := f.issuer(t, &logs)
	tok, err := m.TokenSource(onPremHuman()).Token()
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if !strings.Contains(logs.String(), "minted human token") {
		t.Fatalf("no mint log line: %q", logs.String())
	}
	if strings.Contains(logs.String(), tok.AccessToken) {
		t.Fatalf("log contains the token: %q", logs.String())
	}
}

func TestNewMockIssuerTargetsTheNamespaceService(t *testing.T) {
	t.Parallel()
	// A zero Client has no REST config, so a mint stops at the connect step.
	m := NewMockIssuer(&k8sclient.Client{}, "ns")
	if m.host != fakeIssuerHost {
		t.Errorf("host = %q, want %q", m.host, fakeIssuerHost)
	}
	_, err := m.TokenSource(onPremHuman()).Token()
	if err == nil || !strings.Contains(err.Error(), "reach "+MockReleaseName) {
		t.Fatalf("error = %v, want it to name the unreachable issuer", err)
	}
}

func TestFailedMintIsReportedAndRetried(t *testing.T) {
	t.Parallel()
	f := newFakeIssuer(t, 600)
	f.failNext.Store(true)
	src := f.issuer(t, nil).TokenSource(onPremHuman())

	_, err := src.Token()
	if err == nil || !strings.Contains(err.Error(), "mint human token for e2e@example.com") {
		t.Fatalf("error = %v, want it to name the human", err)
	}
	if _, err := src.Token(); err != nil {
		t.Fatalf("Token after a failed mint: %v", err)
	}
	if got := f.hits.Load(); got != 2 {
		t.Fatalf("issuer served %d requests, want 2", got)
	}
}

func TestMintRejectsAnInvalidHumanWithoutARequest(t *testing.T) {
	t.Parallel()
	f := newFakeIssuer(t, 600)
	h := onPremHuman()
	h.Subject = ""
	_, err := f.issuer(t, nil).TokenSource(h).Token()
	if err == nil || !strings.Contains(err.Error(), "needs a subject") {
		t.Fatalf("error = %v, want the validation error", err)
	}
	if got := f.hits.Load(); got != 0 {
		t.Fatalf("issuer got %d requests, want none", got)
	}
}

func TestHumanValidate(t *testing.T) {
	t.Parallel()
	valid := onPremHuman()
	tests := []struct {
		name   string
		mutate func(*Human)
		want   string
	}{
		{name: "valid"},
		{name: "unknown model", mutate: func(h *Human) { h.TenantModel = "cloud" }, want: "unknown tenant model"},
		{name: "no tenant", mutate: func(h *Human) { h.Tenant = "" }, want: "needs a tenant"},
		{name: "no tenant with omit", mutate: func(h *Human) { h.Tenant, h.OmitRequired = "", true }},
		{name: "no subject", mutate: func(h *Human) { h.Subject = "" }, want: "needs a subject"},
		{name: "no audience", mutate: func(h *Human) { h.Audience = "" }, want: "needs an audience"},
		{name: "unsafe tenant", mutate: func(h *Human) { h.Tenant = "a b" }, want: "unsupported characters"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := valid
			if tt.mutate != nil {
				tt.mutate(&h)
			}
			err := h.Validate()
			if tt.want == "" && err != nil {
				t.Fatalf("Validate: %v", err)
			}
			if tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}
