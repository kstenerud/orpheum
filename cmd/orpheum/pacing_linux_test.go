// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/platform/outbound"
)

// acceptedListener is a listener that keeps every connection it accepts.
type acceptedListener struct {
	net.Listener

	mutex    sync.Mutex
	accepted []net.Conn
}

func (listener *acceptedListener) Accept() (net.Conn, error) {
	conn, err := listener.Listener.Accept()
	if err == nil {
		listener.mutex.Lock()
		listener.accepted = append(listener.accepted, conn)
		listener.mutex.Unlock()
	}
	return conn, err
}

// pacingRates are the kernel's pacing rates of the connections the listener accepted.
func (listener *acceptedListener) pacingRates(t *testing.T) []int {
	t.Helper()
	listener.mutex.Lock()
	defer listener.mutex.Unlock()
	rates := make([]int, 0, len(listener.accepted))
	for _, conn := range listener.accepted {
		tcp, isTCP := conn.(*net.TCPConn)
		if !isTCP {
			t.Fatalf("%T is not TCP", conn)
		}
		raw, err := tcp.SyscallConn()
		if err != nil {
			t.Fatal(err)
		}
		var rate int
		var rateErr error
		if err := raw.Control(func(fd uintptr) {
			rate, rateErr = unix.GetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_MAX_PACING_RATE)
		}); err != nil || rateErr != nil {
			t.Fatal(err, rateErr)
		}
		rates = append(rates, rate)
	}
	return rates
}

// The plain-HTTP server keeps each connection in its requests' context, so a film's download a
// player asks to pace is paced (D262 §2): without it, nothing was, and no error said so.
func TestServe_PacesAFilmsDownloadThatAsksIt(t *testing.T) {
	t.Parallel()
	config, _ := testConfig(t)
	inner, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	listener := &acceptedListener{Listener: inner}
	server := startServingOn(t, listener, config, newCapturedLog("ready"), "ready", nil)
	request, err := http.NewRequestWithContext(
		t.Context(), http.MethodGet, server.baseURL+"/bytes/media/none", http.NoBody,
	)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set(api.PaceHeader, "2000")
	client, err := outbound.NewClient(30*time.Second, version)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body) //nolint:errcheck // only the connection matters
	_ = response.Body.Close()                 //nolint:errcheck // test teardown
	const wantBytesPerSecond = 2000 * 1000 / 8
	rates := listener.pacingRates(t)
	if len(rates) != 1 || rates[0] != wantBytesPerSecond {
		t.Errorf("the connections' pacing rates are %v; want one, paced at %d bytes a second",
			rates, wantBytesPerSecond)
	}
}
