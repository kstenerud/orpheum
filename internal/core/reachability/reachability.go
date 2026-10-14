// SPDX-License-Identifier: AGPL-3.0-or-later

package reachability

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/pion/stun/v3"

	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/randbytes"
)

// EchoPath is the path under the API's prefix that answers a check's own request, by its nonce:
// the request that came back from outside.
const EchoPath = "/reachability/"

// nonceBytes is a check's nonce's length before encoding: enough that no other server answers it
// by chance.
const nonceBytes = 16

// nonceLifetime is how long a nonce is recognised: longer than the check's own request may take.
const nonceLifetime = time.Minute

// probeNonceLifetime is how long a nonce sent to the discovery for its outside probe is recognised:
// longer than a heartbeat's wait, heartbeats coming at least every 5 minutes, so the newest one the
// discovery holds is always live (the protocol's §2.6).
const probeNonceLifetime = 10 * time.Minute

// stunWait is how long a STUN binding is given to be answered over the relay's port, and
// stunTries how many times it is sent over UDP, which may lose one.
const (
	stunWait  = 3 * time.Second
	stunTries = 3
)

// Outcome is what one hop's check found: reached, or where it failed (remote-access.md, "Am I
// reachable?": a failure names its hop).
type Outcome string

// The outcomes.
const (
	// Reached: the request came back to this server from outside.
	Reached Outcome = "reached"
	// NotChecked: the server has no public name, or no relay, to check.
	NotChecked Outcome = "notChecked"
	// NameUnresolved: the name has no address; its DNS records are missing or wrong.
	NameUnresolved Outcome = "nameUnresolved"
	// NotAnswering: nothing answered at the address: the VPS, its forwards or the tunnel.
	NotAnswering Outcome = "notAnswering"
	// CertificateRefused: something answered, but not with a certificate for the name.
	CertificateRefused Outcome = "certificateRefused"
	// AnotherServer: something answered for the name, but not this server.
	AnotherServer Outcome = "anotherServer"
)

// Result is one hop's outcome, with what failed in the failing layer's own words.
type Result struct {
	Outcome Outcome
	Detail  string
}

// Report is a check of the whole path from outside in: the name and where it points, HTTPS
// through it to this server, and the voice relay's port over UDP and TCP.
type Report struct {
	CheckedAt time.Time
	Name      string
	Addresses []string
	HTTPS     Result
	RelayUDP  Result
	RelayTCP  Result
}

// Resolver finds a name's addresses: net.DefaultResolver, or a test's.
type Resolver interface {
	LookupHost(ctx context.Context, host string) ([]string, error)
}

// Parts are what a Checker needs.
type Parts struct {
	// Name is the server's public name; "" checks no HTTPS.
	Name string
	// RelayAddress is the relay's public host:port; "" checks no relay.
	RelayAddress string
	// HTTP makes the check's request, as a browser outside would: the outbound client.
	HTTP     *http.Client
	Resolver Resolver
	// Dialer reaches the relay's port.
	Dialer *net.Dialer
	Clock  clock.Clock
	Random io.Reader
}

// Checker checks the server's public path, and recognises its own request coming back.
type Checker struct {
	parts  Parts
	mutex  sync.Mutex
	nonces map[string]time.Time // guarded by mutex: each live nonce, by when it stops being recognised
}

// New is a checker.
func New(parts Parts) *Checker {
	return &Checker{parts: parts, nonces: map[string]time.Time{}}
}

// Check checks each hop now.
func (checker *Checker) Check(ctx context.Context) Report {
	report := Report{
		CheckedAt: checker.parts.Clock.Now(), Name: checker.parts.Name,
		HTTPS:    Result{Outcome: NotChecked},
		RelayUDP: Result{Outcome: NotChecked}, RelayTCP: Result{Outcome: NotChecked},
	}
	if checker.parts.Name != "" {
		report.Addresses, report.HTTPS = checker.checkHTTPS(ctx)
	}
	if checker.parts.RelayAddress != "" {
		report.RelayUDP = checker.checkRelay(ctx, "udp")
		report.RelayTCP = checker.checkRelay(ctx, "tcp")
	}
	return report
}

// Recognises is whether nonce is one this server made, a check's or a probe's, still within its
// lifetime.
func (checker *Checker) Recognises(nonce string) bool {
	checker.mutex.Lock()
	defer checker.mutex.Unlock()
	endsAt, found := checker.nonces[nonce]
	return found && checker.parts.Clock.Now().Before(endsAt)
}

// ProbeNonce is a new nonce for the discovery's outside probe to ask the echo for, recognised for
// probeNonceLifetime: the core issues it, so an answer to it is this server (the protocol's §2.6).
func (checker *Checker) ProbeNonce() (string, error) {
	return checker.newNonce(probeNonceLifetime)
}

// checkHTTPS resolves the name, then asks it for the echo of a new nonce: an answer from this
// server is the whole path working, through DNS, the VPS and the tunnel.
func (checker *Checker) checkHTTPS(ctx context.Context) ([]string, Result) {
	addresses, err := checker.parts.Resolver.LookupHost(ctx, checker.parts.Name)
	if err != nil || len(addresses) == 0 {
		return nil, Result{Outcome: NameUnresolved, Detail: errorText(err)}
	}
	nonce, err := checker.newNonce(nonceLifetime)
	if err != nil {
		return addresses, Result{Outcome: NotAnswering, Detail: err.Error()}
	}
	url := "https://" + checker.parts.Name + "/api/v1" + EchoPath + nonce
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return addresses, Result{Outcome: NotAnswering, Detail: err.Error()}
	}
	answer, err := checker.parts.HTTP.Do(request)
	if err != nil {
		if isCertificateError(err) {
			return addresses, Result{Outcome: CertificateRefused, Detail: err.Error()}
		}
		return addresses, Result{Outcome: NotAnswering, Detail: err.Error()}
	}
	_ = answer.Body.Close() //nolint:errcheck // the status is all it says
	switch answer.StatusCode {
	case http.StatusNoContent:
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		// A proxy on the way (the VPS's) answered for a server it could not reach, or this one is
		// not ready: nothing behind the address answered (M4's audit).
		return addresses, Result{Outcome: NotAnswering, Detail: answer.Status}
	default:
		return addresses, Result{Outcome: AnotherServer, Detail: answer.Status}
	}
	return addresses, Result{Outcome: Reached}
}

// newNonce makes a nonce and keeps it for lifetime, forgetting those past theirs.
func (checker *Checker) newNonce(lifetime time.Duration) (string, error) {
	nonce, err := randbytes.Base64URLSecret(checker.parts.Random, nonceBytes)
	if err != nil {
		return "", fmt.Errorf("make a reachability nonce: %w", err)
	}
	now := checker.parts.Clock.Now()
	checker.mutex.Lock()
	defer checker.mutex.Unlock()
	for each, endsAt := range checker.nonces {
		if !now.Before(endsAt) {
			delete(checker.nonces, each)
		}
	}
	checker.nonces[nonce] = now.Add(lifetime)
	return nonce, nil
}

// checkRelay sends a STUN binding to the relay's public address over network, "udp" or "tcp":
// an answer is the VPS forwarding that port through the tunnel to this server's relay, and back.
func (checker *Checker) checkRelay(ctx context.Context, network string) Result {
	connection, err := checker.parts.Dialer.DialContext(ctx, network, checker.parts.RelayAddress)
	if err != nil {
		return Result{Outcome: NotAnswering, Detail: err.Error()}
	}
	defer func() { _ = connection.Close() }() //nolint:errcheck // a check's connection
	tries := 1
	if network == "udp" {
		tries = stunTries
	}
	for range tries {
		if err = bind(ctx, connection); err == nil {
			return Result{Outcome: Reached}
		}
	}
	return Result{Outcome: NotAnswering, Detail: err.Error()}
}

// bind sends one STUN binding request on connection and waits stunWait for its answer; a wait that
// ends unanswered ends the connection with it.
func bind(ctx context.Context, connection net.Conn) error {
	request, err := stun.Build(stun.NewTransactionIDSetter(stun.NewTransactionID()), stun.BindingRequest)
	if err != nil {
		return fmt.Errorf("build a STUN binding: %w", err)
	}
	if err = connection.SetDeadline(time.Time{}); err != nil { // a try before may have set one
		return fmt.Errorf("clear the STUN binding's deadline: %w", err)
	}
	waiting, cancel := context.WithTimeout(ctx, stunWait)
	defer cancel()
	stopWaiting := context.AfterFunc(waiting, func() {
		_ = connection.SetDeadline(time.Unix(1, 0)) //nolint:errcheck // a deadline in the past wakes the read
	})
	defer stopWaiting()
	if _, err = request.WriteTo(connection); err != nil {
		return fmt.Errorf("send a STUN binding: %w", err)
	}
	answer := &stun.Message{Raw: make([]byte, 0, 1500)}
	if _, err = answer.ReadFrom(connection); err != nil {
		return fmt.Errorf("no answer to a STUN binding within %v: %w", stunWait, err)
	}
	if answer.Type != stun.BindingSuccess || answer.TransactionID != request.TransactionID {
		return fmt.Errorf("the relay's port answered %v, not the binding's success", answer.Type)
	}
	return nil
}

// isCertificateError is whether err is TLS refusing the certificate it was shown.
func isCertificateError(err error) bool {
	var unknownAuthority x509.UnknownAuthorityError
	var invalid x509.CertificateInvalidError
	var hostname x509.HostnameError
	var verification *tls.CertificateVerificationError
	return errors.As(err, &unknownAuthority) || errors.As(err, &invalid) || errors.As(err, &hostname) ||
		errors.As(err, &verification)
}

// errorText is err's text, or that there were no addresses.
func errorText(err error) string {
	if err == nil {
		return "the name has no addresses"
	}
	return err.Error()
}
