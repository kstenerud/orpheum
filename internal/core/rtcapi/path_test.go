// SPDX-License-Identifier: AGPL-3.0-or-later

package rtcapi_test

import (
	"context"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"testing"

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
	rtcapitest.Connect(t, server.Answer).Do(rtcapi.RequestHead{Method: "GET", Path: "/"}, nil)
	log.AwaitLine(t, "webrtc: a client's connection", "path=direct_ipv4")
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
