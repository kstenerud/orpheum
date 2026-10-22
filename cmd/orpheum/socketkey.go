// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/kstenerud/orpheum/internal/platform/randbytes"
	"github.com/kstenerud/orpheum/internal/platform/unixsocket"
)

// socketKeyBytes is a socket's key's length: 256 random bits, as every bearer secret's (D154).
const socketKeyBytes = 32

// keepSocketKey makes a fresh key for the socket at socketPath, keeps it beside the socket for
// whoever dials it, and returns handler serving only requests that present it: a process of the
// same user without the key, a GPU run that may open a Unix socket among them, is refused and
// logged (D331 §1).
func keepSocketKey(
	ctx context.Context, socketPath string, handler http.Handler, logger *slog.Logger,
) (http.Handler, error) {
	key, err := randbytes.Base64URLSecret(rand.Reader, socketKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("make the key of %s: %w", socketPath, err)
	}
	if err := unixsocket.WriteKey(socketPath, key); err != nil {
		return nil, err
	}
	return unixsocket.RequireKey(key, handler, func(request *http.Request) {
		logger.WarnContext(ctx, "refused a request without the socket's key", "socket", socketPath,
			"method", request.Method, "path", request.URL.Path)
	}), nil
}
