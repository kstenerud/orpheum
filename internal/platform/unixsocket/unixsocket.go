// SPDX-License-Identifier: GPL-3.0-or-later

package unixsocket

import (
	"context"
	"net"
	"net/http"
)

// Transport is an HTTP transport whose every connection is to the unix socket at socketPath,
// whatever host a request names.
func Transport(socketPath string) *http.Transport {
	return &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", socketPath)
		},
	}
}
