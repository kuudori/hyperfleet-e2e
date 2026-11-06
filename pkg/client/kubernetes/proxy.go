package kubernetes

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"k8s.io/client-go/rest"
)

// ServiceProxy returns an API-server-authenticated HTTP client and the URL of
// the API server's proxy to a Service port. The API server forwards the Host
// header and consumes Authorization itself, so a backend must take credentials
// from the body or another header.
func (c *Client) ServiceProxy(namespace, service, scheme string, port int32) (*http.Client, string, error) {
	if c.restConfig == nil {
		return nil, "", errors.New("the service proxy needs a client built by NewClient")
	}
	httpClient, err := rest.HTTPClientFor(c.restConfig)
	if err != nil {
		return nil, "", fmt.Errorf("build API server HTTP client: %w", err)
	}
	proxyURL := c.CoreV1().RESTClient().Get().
		Namespace(namespace).
		Resource("services").
		Name(scheme + ":" + service + ":" + strconv.Itoa(int(port))).
		SubResource("proxy").
		URL()
	return httpClient, proxyURL.String(), nil
}
