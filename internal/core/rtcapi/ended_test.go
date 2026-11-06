// SPDX-License-Identifier: AGPL-3.0-or-later

package rtcapi_test

import (
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/rtcapi"
	"github.com/kstenerud/orpheum/internal/core/rtcapi/rtcapitest"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// A connection that carried a 5 MiB response logs, once, as its client closes it: that the browser
// closed it, its path, how long
// it lived since it connected, what SCTP sent and its window, round trip and packet size at the end
// (the throughput review's fix 5, D397).
func TestEnded_AConnectionLogsItsSCTPFiguresAsItEnds(t *testing.T) {
	t.Parallel()
	log := &testutil.Log{}
	server := rtcapi.New(t.Context(), echo, slog.New(slog.NewTextHandler(log, nil)))
	t.Cleanup(func() { _ = server.Close() }) //nolint:errcheck // the test is over
	fake := clocktest.NewFake(time.Unix(1_800_000_000, 0))
	rtcapi.SetClock(server, fake)
	link := rtcapitest.Connect(t, server.Answer, rtcapitest.OverIPv4())
	link.Do(rtcapi.RequestHead{Method: http.MethodGet, Path: "/big"}, nil)
	fake.Advance(90 * time.Second)
	link.Close()
	line := log.AwaitEvent(t, "connectionEnded", "connection=", "path=direct_ipv4",
		`cause="the browser closed it"`, "durationMs=90000",
		"bytesReceived=", "rwndBytes=", "mtu=1191")
	for name, least := range map[string]int{"bytesSent": 5 << 20, "cwndBytes": 1191, "srttMs": 0} {
		match := regexp.MustCompile(` ` + name + `=([0-9.]+)`).FindStringSubmatch(line)
		if match == nil {
			t.Errorf("the end line has no %s: %s", name, line)
			continue
		}
		if value, err := strconv.ParseFloat(match[1], 64); err != nil || value < float64(least) {
			t.Errorf("the end line's %s is %s; want at least %d", name, match[1], least)
		}
	}
}

// A connection the core closes as it stops says so as it ends, told from one its browser closed.
func TestEnded_AConnectionTheCoreClosesAsItStopsSaysSo(t *testing.T) {
	t.Parallel()
	log := &testutil.Log{}
	server := rtcapi.New(t.Context(), echo, slog.New(slog.NewTextHandler(log, nil)))
	link := rtcapitest.Connect(t, server.Answer, rtcapitest.OverIPv4())
	t.Cleanup(link.Close)
	link.Do(rtcapi.RequestHead{Method: http.MethodGet, Path: "/big"}, nil)
	if err := server.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	log.AwaitEvent(t, "connectionEnded", `cause="the core stopped"`)
}
