package kubernetes

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func TestServiceProxyReachesTheServiceThroughTheAPIServer(t *testing.T) {
	t.Parallel()
	var gotPath, gotHost, gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotHost, gotAuth = r.URL.Path, r.Host, r.Header.Get("Authorization")
		_, _ = io.WriteString(w, "ok")
	}))
	t.Cleanup(server.Close)
	cfg := &rest.Config{Host: server.URL, BearerToken: "kube-token"}
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	c := &Client{Interface: clientset, restConfig: cfg}

	httpClient, base, err := c.ServiceProxy("ns", "mock", "https", 8443)
	if err != nil {
		t.Fatalf("ServiceProxy: %v", err)
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, base+"/default/token", strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "mock.ns.svc.cluster.local:8443"
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	_ = resp.Body.Close()

	if want := "/api/v1/namespaces/ns/services/https:mock:8443/proxy/default/token"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
	if gotHost != req.Host {
		t.Errorf("Host = %q, want %q", gotHost, req.Host)
	}
	if gotAuth != "Bearer kube-token" {
		t.Errorf("Authorization = %q, want the kubeconfig credentials", gotAuth)
	}
}

func TestServiceProxyNeedsARESTConfig(t *testing.T) {
	t.Parallel()
	if _, _, err := (&Client{}).ServiceProxy("ns", "mock", "https", 8443); err == nil {
		t.Fatal("ServiceProxy succeeded without a REST config")
	}
}
