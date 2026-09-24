// SPDX-License-Identifier: GPL-3.0-or-later

package outbound

import (
	"fmt"
	"net/http"
	"time"
)

// NewClient returns a client for requests to other hosts, each bounded by timeout. It starts
// from Go's default transport for its connection limits and HTTP/2 support, and drops the
// proxy that transport would read from the environment (D204).
func NewClient(timeout time.Duration) (*http.Client, error) {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, fmt.Errorf("http.DefaultTransport is a %T, not an *http.Transport", http.DefaultTransport)
	}
	transport := base.Clone()
	transport.Proxy = nil
	return &http.Client{Transport: transport, Timeout: timeout}, nil
}
