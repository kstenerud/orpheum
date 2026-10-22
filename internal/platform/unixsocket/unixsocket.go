// SPDX-License-Identifier: AGPL-3.0-or-later

package unixsocket

import (
	"context"
	"net"
	"net/http"
)

// Client is an HTTP client whose every connection is to the Unix socket at socketPath, whatever
// host a request names, and whose every request presents the key kept beside the socket (KeyPath,
// D331 §1). It keeps up to idleConnections connections open between requests; 0 is Go's default.
func Client(socketPath string, idleConnections int) *http.Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", socketPath)
		},
		MaxIdleConnsPerHost: idleConnections,
	}
	return &http.Client{Transport: keyed{transport: transport, socketPath: socketPath}}
}
