// SPDX-License-Identifier: AGPL-3.0-or-later

package rtcapi_test

import (
	"context"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pion/stun/v3"
	"github.com/pion/webrtc/v4"

	"github.com/kstenerud/orpheum/internal/core/rtcapi"
	"github.com/kstenerud/orpheum/internal/core/rtcapi/rtcapitest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// A client on the core's own network reaches an address the core has: the connection is counted,
// and logged, as direct (D274 §3).
func TestAnswer_CountsThePathEachConnectionTook(t *testing.T) {
	t.Parallel()
	log := &testutil.Log{}
	server := rtcapi.New(t.Context(), echo, slog.New(slog.NewTextHandler(log, nil)))
	t.Cleanup(func() { _ = server.Close() }) //nolint:errcheck // the test is over
	rtcapitest.Connect(t, server.Answer, rtcapitest.OverIPv4()).Do(rtcapi.RequestHead{Method: "GET", Path: "/"}, nil)
	log.AwaitLine(t, "webrtc: a client's connection", "path=direct_ipv4", "connectMs=")
	if counts := server.PathCounts(); counts[rtcapi.PathDirectIPv4] != 1 || len(counts) != 1 {
		t.Errorf("the paths counted are %v; want one direct over IPv4", counts)
	}
}

// publicAddress is what the fake STUN server says the core's NAT gave it.
const (
	publicAddress = "203.0.113.9"
	publicPort    = 40001
)

// startSTUN answers each binding request with publicAddress, as a STUN server outside the core's
// NAT would; it returns the server's URL.
func startSTUN(t *testing.T) string {
	t.Helper()
	connection, err := (&net.ListenConfig{}).ListenPacket(t.Context(), "udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() }) //nolint:errcheck // the test is over
	go func() {
		buffer := make([]byte, 1500)
		for {
			count, from, readErr := connection.ReadFrom(buffer)
			if readErr != nil {
				return
			}
			request := &stun.Message{Raw: append([]byte(nil), buffer[:count]...)}
			if request.Decode() != nil || request.Type != stun.BindingRequest {
				continue
			}
			answer, buildErr := stun.Build(stun.NewTransactionIDSetter(request.TransactionID), stun.BindingSuccess,
				&stun.XORMappedAddress{IP: net.ParseIP(publicAddress), Port: publicPort}, stun.Fingerprint)
			if buildErr == nil {
				_, _ = connection.WriteTo(answer.Raw, from) //nolint:errcheck // the client asks again
			}
		}
	}()
	return "stun:" + connection.LocalAddr().String()
}

// With its discovery's STUN server, the core's answer carries the public address its NAT gives it,
// so a client outside can punch through to it (D274 §2).
func TestUseICEServers_TheAnswerCarriesThePublicAddressSTUNFound(t *testing.T) {
	t.Parallel()
	server := newServer(t)
	stunURL := startSTUN(t)
	server.UseICEServers(func() []webrtc.ICEServer { return []webrtc.ICEServer{{URLs: []string{stunURL}}} })
	var answered string
	rtcapitest.Connect(t, func(ctx context.Context, offer rtcapi.Description) (rtcapi.Description, error) {
		answer, err := server.Answer(ctx, offer)
		answered = answer.SDP
		return answer, err
	})
	want := publicAddress + " " + strconv.Itoa(publicPort) + " typ srflx"
	if !strings.Contains(answered, want) {
		t.Errorf("the answer's candidates are\n%s\nwant one %q", candidateLines(answered), want)
	}
}

// silentSTUN is a STUN server that never answers, as one is by a family the core's network cannot
// send out by; it returns the server's URL.
func silentSTUN(t *testing.T) string {
	t.Helper()
	connection, err := (&net.ListenConfig{}).ListenPacket(t.Context(), "udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() }) //nolint:errcheck // the test is over
	return "stun:" + connection.LocalAddr().String()
}

// A STUN server that never answers costs the core's answer a short wait, not its whole time: the
// answer still comes well within its bound, with the candidates the core has (staging, 2026-10-14).
func TestUseICEServers_AStunServerThatNeverAnswersDoesNotSinkTheAnswer(t *testing.T) {
	t.Parallel()
	server := newServer(t)
	stunURL := silentSTUN(t)
	server.UseICEServers(func() []webrtc.ICEServer { return []webrtc.ICEServer{{URLs: []string{stunURL}}} })
	rtcapitest.Connect(t, func(ctx context.Context, offer rtcapi.Description) (rtcapi.Description, error) {
		// Shorter than the 5 s pion waits by default, longer than the core's own wait.
		ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
		defer cancel()
		return server.Answer(ctx, offer)
	})
}

// A STUN server the core cannot reach by one family holds its gathering for stunGatherTimeout; once
// another has given it its public address, the answer goes without waiting that out, and carries it
// (staging, 2026-10-23: every answer took 2.1 s).
func TestAnswer_GoesSoonAfterTheCoresPublicAddressNotWhenGatheringEnds(t *testing.T) {
	t.Parallel()
	server := newServer(t)
	answering, silent := startSTUN(t), silentSTUN(t)
	server.UseICEServers(func() []webrtc.ICEServer {
		return []webrtc.ICEServer{{URLs: []string{answering}}, {URLs: []string{silent}}}
	})
	var answered string
	rtcapitest.Connect(t, func(ctx context.Context, offer rtcapi.Description) (rtcapi.Description, error) {
		// Shorter than the 2 s the silent server holds gathering for.
		ctx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
		defer cancel()
		answer, err := server.Answer(ctx, offer)
		answered = answer.SDP
		return answer, err
	})
	if want := publicAddress + " " + strconv.Itoa(publicPort) + " typ srflx"; !strings.Contains(answered, want) {
		t.Errorf("the answer's candidates are\n%s\nwant one %q", candidateLines(answered), want)
	}
}

// candidateLines are an SDP's candidate lines.
func candidateLines(sdp string) string {
	var lines []string
	for line := range strings.SplitSeq(sdp, "\r\n") {
		if strings.HasPrefix(line, "a=candidate") {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}
