// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"errors"
	"net"
	"sync"
)

// joinedListener accepts the connections of several listeners as one, so one HTTP server serves
// them all: the house's plain HTTP and the port the server's own machine alone reaches (D334 §4). A
// handler tells them apart by the connection's local address (http.LocalAddrContextKey).
type joinedListener struct {
	listeners []net.Listener
	accepted  chan accepted
	closing   sync.Once
	closed    chan struct{}
}

// accepted is a connection one of the listeners took, or the error it stopped with.
type accepted struct {
	connection net.Conn
	err        error
}

// joinListeners is listeners accepted as one, the first's address its own.
func joinListeners(listeners ...net.Listener) net.Listener {
	joined := &joinedListener{listeners: listeners, accepted: make(chan accepted), closed: make(chan struct{})}
	for _, listener := range listeners {
		go func() {
			for {
				connection, err := listener.Accept()
				select {
				case joined.accepted <- accepted{connection, err}:
				case <-joined.closed:
					if connection != nil {
						_ = connection.Close() //nolint:errcheck // taken after the close, never served
					}
					return
				}
				if err != nil {
					return
				}
			}
		}()
	}
	return joined
}

// Accept returns the next connection any listener took; once one has stopped, its error, and the
// listener is closed.
func (joined *joinedListener) Accept() (net.Conn, error) {
	select {
	case taken := <-joined.accepted:
		if taken.err != nil {
			return nil, errors.Join(taken.err, joined.Close())
		}
		return taken.connection, nil
	case <-joined.closed:
		return nil, net.ErrClosed
	}
}

// Close closes every listener.
func (joined *joinedListener) Close() error {
	var err error
	joined.closing.Do(func() {
		close(joined.closed)
		for _, listener := range joined.listeners {
			err = errors.Join(err, listener.Close())
		}
	})
	return err
}

// Addr is the first listener's address.
func (joined *joinedListener) Addr() net.Addr {
	return joined.listeners[0].Addr()
}
