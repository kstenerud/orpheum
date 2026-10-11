// SPDX-License-Identifier: AGPL-3.0-or-later

package pacing

import (
	"context"
	"crypto/tls"
	"net"
	"syscall"
)

type connKey struct{}

// ConnContext keeps conn in its requests' context: http.Server's ConnContext.
func ConnContext(ctx context.Context, conn net.Conn) context.Context {
	return context.WithValue(ctx, connKey{}, conn)
}

// socketOf is the socket under the connection ctx's requests arrived on, and whether there is one:
// a TLS connection's own underneath.
func socketOf(ctx context.Context) (syscall.RawConn, bool) {
	conn, isKept := ctx.Value(connKey{}).(net.Conn)
	if !isKept {
		return nil, false
	}
	if secured, isTLS := conn.(*tls.Conn); isTLS {
		conn = secured.NetConn()
	}
	tcp, isTCP := conn.(*net.TCPConn)
	if !isTCP {
		return nil, false
	}
	raw, err := tcp.SyscallConn()
	return raw, err == nil
}

// Pace makes the connection ctx's request arrived on send at most bytesPerSecond, its packets
// marked lower than best effort; a connection not kept is left as it is.
func Pace(ctx context.Context, bytesPerSecond uint32) error {
	raw, isSocket := socketOf(ctx)
	if !isSocket {
		return nil
	}
	return setSocket(raw, bytesPerSecond, true)
}

// Unpace sets the connection ctx's request arrived on back to unpaced and best effort.
func Unpace(ctx context.Context) error {
	raw, isSocket := socketOf(ctx)
	if !isSocket {
		return nil
	}
	return setSocket(raw, unpaced, false)
}
