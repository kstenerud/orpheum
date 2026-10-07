// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/kstenerud/orpheum/internal/platform/pacing"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// pacingOf is the server side of conn's pacing rate and IPv4 traffic class, as the kernel has them.
func pacingOf(t *testing.T, conn net.Conn) (rate uint32, class int) {
	t.Helper()
	tcp, isTCP := conn.(*net.TCPConn)
	if !isTCP {
		t.Fatalf("%T is not TCP", conn)
	}
	raw, err := tcp.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	var rateValue int
	var rateErr, classErr error
	if err := raw.Control(func(fd uintptr) {
		socket := int(fd)
		rateValue, rateErr = unix.GetsockoptInt(socket, unix.SOL_SOCKET, unix.SO_MAX_PACING_RATE)
		class, classErr = unix.GetsockoptInt(socket, unix.IPPROTO_IP, unix.IP_TOS)
	}); err != nil || rateErr != nil || classErr != nil {
		t.Fatal(err, rateErr, classErr)
	}
	return uint32(rateValue), class //nolint:gosec // the kernel's unsigned rate
}

// A film's download asked to be paced, as a player whose person's voice is live asks (D262 §2),
// is: its connection sends at the rate asked and marks its packets lower than best effort; the
// next download not asking sets it back; other requests leave it as it is.
func TestPacedMedia_PacesAFilmsDownloadAsItsPlayerAsksAndSetsItBackWhenItDoesNot(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	conns := make(chan net.Conn, 1)
	served := httptest.NewUnstartedServer(harness.server.Handler())
	served.Config.ConnContext = func(ctx context.Context, conn net.Conn) context.Context {
		conns <- conn
		return pacing.ConnContext(ctx, conn)
	}
	served.Start()
	t.Cleanup(served.Close)
	client := served.Client()
	get := func(path, pace string) {
		t.Helper()
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, served.URL+path, http.NoBody)
		if err != nil {
			t.Fatal(err)
		}
		if pace != "" {
			request.Header.Set(PaceHeader, pace)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		// Read whole, so the connection is kept for the next request.
		_, _ = io.Copy(io.Discard, response.Body) //nolint:errcheck // what the test reads is the socket
		_ = response.Body.Close()                 //nolint:errcheck // test teardown
	}
	media := Prefix + "/bytes/media/chunks/no-such-token/master.m3u8"
	get(media, "1500")
	conn := testutil.Receive(t, conns, "the test server's connection")
	if rate, class := pacingOf(t, conn); rate != 1500*1000/8 || class != 1<<2 {
		t.Errorf("a download paced at 1500 kb/s: rate %d B/s, class %#x; want 187500 and DSCP LE", rate, class)
	}
	get(Prefix+"/health", "")
	if rate, _ := pacingOf(t, conn); rate != 1500*1000/8 {
		t.Errorf("another request on the connection set its rate to %d; want it left paced", rate)
	}
	get(media, "")
	if rate, class := pacingOf(t, conn); rate != ^uint32(0) || class != 0 {
		t.Errorf("a download not asking: rate %d, class %#x; want unpaced and best effort", rate, class)
	}
	get(media, "999999999")
	if rate, _ := pacingOf(t, conn); rate != ^uint32(0) {
		t.Errorf("a pace out of bounds set the rate to %d; want unpaced", rate)
	}
}
