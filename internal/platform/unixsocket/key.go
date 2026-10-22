// SPDX-License-Identifier: AGPL-3.0-or-later

package unixsocket

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strings"
)

// KeyHeader is the header in which a request presents the key of the socket it is sent to.
const KeyHeader = "Orpheum-Socket-Key"

// keyMode keeps a socket's key to the user both sides run as. Landlock keeps it from every FFmpeg
// run, which runs as that user too (D331 §1).
const keyMode = 0o600

// KeyPath is where the key of the socket at socketPath is kept: beside it, so whoever can find the
// socket can find its key, and the core and a node share no other directory (D242 §7).
func KeyPath(socketPath string) string {
	return socketPath + ".key"
}

// WriteKey keeps key beside the socket at socketPath, mode 0600, in place of any key an earlier
// start left. It writes in place: a dialer that reads it mid-write presents a wrong key, is refused,
// and asks again, as a node does after any refused hello.
func WriteKey(socketPath, key string) error {
	if key == "" {
		return errors.New("a socket's key is not empty")
	}
	path := KeyPath(socketPath)
	if err := os.WriteFile(path, []byte(key), keyMode); err != nil {
		return fmt.Errorf("write the key of %s: %w", socketPath, err)
	}
	if err := os.Chmod(path, keyMode); err != nil {
		return fmt.Errorf("make the key of %s mode 0600: %w", socketPath, err)
	}
	return nil
}

// RequireKey serves only the requests that present key; it answers every other one 401 and tells
// onRefused, so the listener can say a process without the key knocked (D331 §1).
func RequireKey(key string, next http.Handler, onRefused func(*http.Request)) http.Handler {
	want := []byte(key)
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		got := []byte(request.Header.Get(KeyHeader))
		if len(want) == 0 || subtle.ConstantTimeCompare(got, want) != 1 {
			onRefused(request)
			http.Error(writer, "this socket serves only the holder of its key", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(writer, request)
	})
}

// keyed presents, with every request, the key kept beside its socket, read as the request is sent,
// so a listener that started again, with a new key, is reached at once. A socket with no key beside
// it is sent no key, and one that requires a key refuses the request.
type keyed struct {
	transport  http.RoundTripper
	socketPath string
}

// RoundTrip sends request with the key beside the socket.
func (sender keyed) RoundTrip(request *http.Request) (*http.Response, error) {
	key, err := os.ReadFile(KeyPath(sender.socketPath))
	switch {
	case err == nil:
		request = request.Clone(request.Context())
		request.Header.Set(KeyHeader, strings.TrimSpace(string(key)))
	case !errors.Is(err, fs.ErrNotExist):
		return nil, fmt.Errorf("read the key of %s: %w", sender.socketPath, err)
	}
	response, err := sender.transport.RoundTrip(request)
	if err != nil {
		return nil, fmt.Errorf("send to %s: %w", sender.socketPath, err)
	}
	return response, nil
}
