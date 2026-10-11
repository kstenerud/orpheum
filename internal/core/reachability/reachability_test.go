// SPDX-License-Identifier: AGPL-3.0-or-later

package reachability

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pion/stun/v3"

	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
)

// name is the public name the checks are of: the one httptest's certificate is for.
const name = "example.com"

// fixedResolver answers every name with addresses, or err.
type fixedResolver struct {
	addresses []string
	err       error
}

func (resolver fixedResolver) LookupHost(context.Context, string) ([]string, error) {
	return resolver.addresses, resolver.err
}

// stage is a checker whose name reaches server, whatever the name resolves to, as the VPS would.
type stage struct {
	checker *Checker
	server  *httptest.Server
	clock   *clocktest.Fake
}

// newStage is a checker of name through a stand-in for the VPS and the tunnel, whose handler
// answers as answer says; isTrusted says whether its certificate is.
func newStage(t *testing.T, answer func(checker *Checker) http.HandlerFunc, isTrusted bool) stage {
	t.Helper()
	var checker *Checker
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		answer(checker)(writer, request)
	}))
	t.Cleanup(server.Close)
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
		},
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
	}
	if isTrusted {
		trusting, isHTTP := server.Client().Transport.(*http.Transport)
		if !isHTTP {
			t.Fatal("httptest's client has no *http.Transport")
		}
		transport.TLSClientConfig.RootCAs = trusting.TLSClientConfig.RootCAs
	}
	fakeClock := clocktest.NewFake(time.Date(2026, 10, 5, 20, 0, 0, 0, time.UTC))
	checker = New(Parts{
		Name: name, HTTP: &http.Client{Transport: transport, Timeout: 5 * time.Second},
		Resolver: fixedResolver{addresses: []string{"203.0.113.7"}}, Dialer: &net.Dialer{},
		Clock: fakeClock, Random: rand.Reader,
	})
	return stage{checker: checker, server: server, clock: fakeClock}
}

// echo is this server's echo: 204 for a nonce it made.
func echo(checker *Checker) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if checker.Recognises(strings.TrimPrefix(request.URL.Path, "/api/v1"+EchoPath)) {
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		writer.WriteHeader(http.StatusNotFound)
	}
}

// The check reaches this server through its public name when the request comes back to it, and
// otherwise names the hop that failed: the name, the address, the certificate, or another server
// answering for it (remote-access.md, "Am I reachable?").
func TestCheck_ReachesThisServerByItsNameOrNamesTheHopThatFailed(t *testing.T) {
	t.Parallel()
	reached := newStage(t, echo, true).checker.Check(t.Context())
	if reached.HTTPS.Outcome != Reached || len(reached.Addresses) != 1 || reached.Addresses[0] != "203.0.113.7" {
		t.Errorf("through the name to this server = %+v; want reached, at the name's address", reached)
	}
	if reached.RelayUDP.Outcome != NotChecked || reached.RelayTCP.Outcome != NotChecked {
		t.Errorf("with no relay = %+v; want the relay not checked", reached)
	}

	unresolved := newStage(t, echo, true)
	unresolved.checker.parts.Resolver = fixedResolver{err: errors.New("no such host")}
	if outcome := unresolved.checker.Check(t.Context()).HTTPS.Outcome; outcome != NameUnresolved {
		t.Errorf("a name with no address = %s; want %s", outcome, NameUnresolved)
	}

	silent := newStage(t, echo, true)
	silent.server.Close()
	if outcome := silent.checker.Check(t.Context()).HTTPS.Outcome; outcome != NotAnswering {
		t.Errorf("nothing at the address = %s; want %s", outcome, NotAnswering)
	}

	if outcome := newStage(t, echo, false).checker.Check(t.Context()).HTTPS.Outcome; outcome != CertificateRefused {
		t.Errorf("a certificate not trusted = %s; want %s", outcome, CertificateRefused)
	}

	another := func(*Checker) http.HandlerFunc {
		return func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(http.StatusOK) }
	}
	if outcome := newStage(t, another, true).checker.Check(t.Context()).HTTPS.Outcome; outcome != AnotherServer {
		t.Errorf("another server answering = %s; want %s", outcome, AnotherServer)
	}
	// A proxy that cannot reach the server behind it answers 502, 503 or 504: nothing answered.
	for _, status := range []int{http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout} {
		proxy := func(*Checker) http.HandlerFunc {
			return func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(status) }
		}
		if outcome := newStage(t, proxy, true).checker.Check(t.Context()).HTTPS.Outcome; outcome != NotAnswering {
			t.Errorf("a proxy answering %d = %s; want %s", status, outcome, NotAnswering)
		}
	}
}

// A nonce is recognised for a minute after its check, and one never made never is.
func TestRecognises_OnlyANonceItMadeWithinAMinute(t *testing.T) {
	t.Parallel()
	staged := newStage(t, echo, true)
	checker := staged.checker
	nonce, err := checker.newNonce()
	if err != nil {
		t.Fatal(err)
	}
	if !checker.Recognises(nonce) || checker.Recognises("made-up") {
		t.Fatal("a nonce it made is not recognised, or one it never made is")
	}
	staged.clock.Advance(nonceLifetime)
	if checker.Recognises(nonce) {
		t.Error("a nonce is recognised a minute after its check")
	}
}

// answerBindings answers STUN bindings on UDP and TCP at one port, as the relay does, until the
// test ends; it returns the port's address.
func answerBindings(t *testing.T) string {
	t.Helper()
	packets, err := (&net.ListenConfig{}).ListenPacket(t.Context(), "udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	streams, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", packets.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = packets.Close(), streams.Close() }) //nolint:errcheck // the test's end
	answer := func(raw []byte) []byte {
		request := &stun.Message{Raw: raw}
		if request.Decode() != nil {
			return nil
		}
		success, err := stun.Build(stun.NewTransactionIDSetter(request.TransactionID), stun.BindingSuccess)
		if err != nil {
			return nil
		}
		return success.Raw
	}
	go func() {
		buffer := make([]byte, 1500)
		for {
			size, from, readErr := packets.ReadFrom(buffer)
			if readErr != nil {
				return
			}
			_, _ = packets.WriteTo(answer(buffer[:size]), from) //nolint:errcheck // the check's to notice
		}
	}()
	go func() {
		for {
			connection, acceptErr := streams.Accept()
			if acceptErr != nil {
				return
			}
			buffer := make([]byte, 1500)
			if size, readErr := connection.Read(buffer); readErr == nil {
				_, _ = connection.Write(answer(buffer[:size])) //nolint:errcheck // the check's to notice
			}
			_ = connection.Close() //nolint:errcheck // answered
		}
	}()
	return packets.LocalAddr().String()
}

// The relay's port answers a STUN binding over UDP and TCP when the VPS forwards it; one where
// nothing answers is named as not answering.
func TestCheck_TheRelaysPortAnswersOverUDPAndTCP(t *testing.T) {
	t.Parallel()
	checker := newStage(t, echo, true).checker
	checker.parts.RelayAddress = answerBindings(t)
	report := checker.Check(t.Context())
	if report.RelayUDP.Outcome != Reached || report.RelayTCP.Outcome != Reached {
		t.Errorf("a relay answering on both = UDP %+v, TCP %+v; want both reached", report.RelayUDP, report.RelayTCP)
	}
	nobody, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	checker.parts.RelayAddress = nobody.Addr().String()
	_ = nobody.Close() //nolint:errcheck // a port nothing listens on, now
	report = checker.Check(t.Context())
	if report.RelayUDP.Outcome != NotAnswering || report.RelayTCP.Outcome != NotAnswering {
		t.Errorf("nothing at the relay's port = UDP %+v, TCP %+v; want neither answering", report.RelayUDP,
			report.RelayTCP)
	}
}
