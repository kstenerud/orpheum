// SPDX-License-Identifier: GPL-3.0-or-later

package outbound

import (
	"fmt"
	"net/http"
	"time"
)

// NewClient returns a client for requests to other hosts, each bounded by timeout and each
// saying it comes from Orpheum at version (D213 §6). It starts from Go's default transport for
// its connection limits and HTTP/2 support, and drops the proxy that transport would read from
// the environment (D204).
func NewClient(timeout time.Duration, version string) (*http.Client, error) {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, fmt.Errorf("http.DefaultTransport is a %T, not an *http.Transport", http.DefaultTransport)
	}
	transport := base.Clone()
	transport.Proxy = nil
	identified := userAgentTransport{Transport: transport, userAgent: "Orpheum/" + version}
	return &http.Client{Transport: identified, Timeout: timeout}, nil
}

// userAgentTransport sends every request with our User-Agent: a provider's terms ask an application
// not to conceal who it is (TMDB's §1.C).
type userAgentTransport struct {
	*http.Transport
	userAgent string
}

// RoundTrip sends a copy of the request that carries the User-Agent.
func (transport userAgentTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	named := request.Clone(request.Context())
	named.Header.Set("User-Agent", transport.userAgent)
	return transport.Transport.RoundTrip(named)
}
