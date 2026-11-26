// SPDX-License-Identifier: AGPL-3.0-or-later

package relay

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pionturn "github.com/pion/turn/v4"

	"github.com/kstenerud/orpheum/discovery/relaycred"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
)

// public is the relay's public address in every test: loopback.
var public = net.IPv4(127, 0, 0, 1)

// logLines is a log written one JSON record a write, each record handed on as it is written.
type logLines chan map[string]any

func (lines logLines) Write(record []byte) (int, error) {
	var fields map[string]any
	if err := json.Unmarshal(record, &fields); err != nil {
		return 0, fmt.Errorf("a log record: %w", err)
	}
	lines <- fields
	return len(record), nil
}

// stage is a relay on loopback, on a fake clock, its log and its drops watched.
type stage struct {
	relay    *Relay
	clock    *clocktest.Fake
	secrets  []relaycred.Secret
	lines    logLines
	drops    chan string
	certPool *x509.CertPool
	files    [2]string
}

func newStage(t *testing.T) *stage {
	t.Helper()
	fakeClock := clocktest.NewFake(time.Date(2026, 11, 26, 20, 0, 0, 0, time.UTC))
	var secrets []relaycred.Secret
	for _, key := range []string{strings.Repeat("a", 32), strings.Repeat("b", 40)} {
		secret, err := relaycred.NewSecret([]byte(key))
		if err != nil {
			t.Fatal(err)
		}
		secrets = append(secrets, secret)
	}
	directory := t.TempDir()
	built := &stage{
		clock: fakeClock, secrets: secrets, lines: make(logLines, 1024), drops: make(chan string, 1024),
		certPool: x509.NewCertPool(),
		files:    [2]string{filepath.Join(directory, "cert.pem"), filepath.Join(directory, "key.pem")},
	}
	built.writeCertificate(t, 1, time.Date(2026, 11, 26, 0, 0, 0, 0, time.UTC))
	logger := slog.New(slog.NewJSONHandler(built.lines, nil))
	relay, err := Start(t.Context(), Config{
		Name: "relay.test", PublicAddress: public, Listen: "127.0.0.1:0", ListenTLS: "127.0.0.1:0",
		RelayPortMin: 40000, RelayPortMax: 49999,
		CertificateFile: built.files[0], KeyFile: built.files[1], Secrets: secrets,
	}, Parts{Clock: fakeClock, Logger: logger, afterDrop: func(reason string) {
		select {
		case built.drops <- reason:
		default:
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = relay.Close() }) //nolint:errcheck // the test's end
	built.relay = relay
	return built
}

// writeCertificate writes a self-signed certificate for relay.test with serial to the stage's
// files, stamped modified, and trusts it.
func (stage *stage) writeCertificate(t *testing.T, serial int64, modified time.Time) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: "relay.test"},
		DNSNames:  []string{"relay.test"},
		NotBefore: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), NotAfter: time.Date(2036, 1, 1, 0, 0, 0, 0, time.UTC),
		IsCA: true, BasicConstraintsValid: true,
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	stage.certPool.AddCert(parsed)
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	for file, block := range map[string]*pem.Block{
		stage.files[0]: {Type: "CERTIFICATE", Bytes: der}, stage.files[1]: {Type: "PRIVATE KEY", Bytes: keyDER},
	} {
		if err := os.WriteFile(file, pem.EncodeToMemory(block), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(file, modified, modified); err != nil {
			t.Fatal(err)
		}
	}
}

func (stage *stage) credential(
	t *testing.T, role relaycred.Role, household string, kbps, secret int,
) (username, password string) {
	t.Helper()
	var err error
	username, password, err = relaycred.Make(stage.secrets[secret], relaycred.Credential{
		ExpiresAt: stage.clock.Now().Add(15 * time.Minute), Role: role, Household: household, Kbps: kbps,
	})
	if err != nil {
		t.Fatal(err)
	}
	return username, password
}

// end is one TURN client's allocation on the relay.
type end struct {
	client  *pionturn.Client
	relayed net.PacketConn
}

// allocate asks the relay for an allocation over transport, "udp", "tcp" or "tls".
func (stage *stage) allocate(t *testing.T, transport, username, password string) (end, error) {
	t.Helper()
	server := stage.relay.Address().String()
	var connection net.PacketConn
	switch transport {
	case "tcp", "tls":
		var stream net.Conn
		var err error
		if transport == "tls" {
			server = stage.relay.TLSAddress().String()
			stream, err = (&tls.Dialer{Config: &tls.Config{
				RootCAs: stage.certPool, ServerName: "relay.test", MinVersion: tls.VersionTLS12,
			}}).DialContext(t.Context(), "tcp", server)
		} else {
			stream, err = (&net.Dialer{}).DialContext(t.Context(), "tcp", server)
		}
		if err != nil {
			t.Fatal(err)
		}
		connection = pionturn.NewSTUNConn(stream)
	default:
		packets, err := (&net.ListenConfig{}).ListenPacket(t.Context(), "udp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		connection = packets
	}
	t.Cleanup(func() { _ = connection.Close() }) //nolint:errcheck // the test's end
	client, err := pionturn.NewClient(&pionturn.ClientConfig{
		STUNServerAddr: server, TURNServerAddr: server, Conn: connection,
		Username: username, Password: password, Realm: relaycred.Realm, RTO: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	if err = client.Listen(); err != nil {
		t.Fatal(err)
	}
	relayed, err := client.Allocate()
	if err != nil {
		return end{}, err
	}
	t.Cleanup(func() { _ = relayed.Close() }) //nolint:errcheck // the test's end
	return end{client: client, relayed: relayed}, nil
}

// mustAllocate is an allocation the relay must make, its permission for the relay's own address
// granted.
func (stage *stage) mustAllocate(
	t *testing.T, transport string, role relaycred.Role, household string, kbps int,
) end {
	t.Helper()
	username, password := stage.credential(t, role, household, kbps, 0)
	made, err := stage.allocate(t, transport, username, password)
	if err != nil {
		t.Fatalf("an allocation for a %s of %s over %s: %v", role, household, transport, err)
	}
	if err := made.client.CreatePermission(&net.UDPAddr{IP: public, Port: 9}); err != nil {
		t.Fatalf("a permission for the relay's own address: %v", err)
	}
	return made
}

// send sends payload from one end to another's relayed address.
func send(t *testing.T, from, to end, payload []byte) {
	t.Helper()
	if _, err := from.relayed.WriteTo(payload, to.relayed.LocalAddr()); err != nil {
		t.Fatal(err)
	}
}

// patience is how long a test waits for what the relay must do on the loopback, which takes
// milliseconds: a test of a rule that no longer holds fails after it, not at the run's own end.
const patience = 10 * time.Second

// patiently is a context that ends at patience, or with the test.
func patiently(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), patience)
	t.Cleanup(cancel)
	return ctx
}

// receive is the next datagram an end is relayed, and whom from.
func receive(t *testing.T, reader end) (string, net.Addr) {
	t.Helper()
	// A deadline long past ends the read once patience is over.
	stop := context.AfterFunc(patiently(t), func() {
		_ = reader.relayed.SetReadDeadline(time.Unix(1, 0)) //nolint:errcheck // the read fails either way
	})
	defer stop()
	buffer := make([]byte, 2048)
	count, from, err := reader.relayed.ReadFrom(buffer)
	if err != nil {
		t.Fatalf("nothing was relayed to %v: %v", reader.relayed.LocalAddr(), err)
	}
	return string(buffer[:count]), from
}

// awaitDrop waits for the relay to drop a datagram, and is why.
func (stage *stage) awaitDrop(t *testing.T) string {
	t.Helper()
	select {
	case reason := <-stage.drops:
		return reason
	case <-patiently(t).Done():
		t.Fatal("no datagram was dropped")
		return ""
	}
}

// awaitLine is the next log line with event.
func (stage *stage) awaitLine(t *testing.T, event string) map[string]any {
	t.Helper()
	waited := patiently(t)
	for {
		select {
		case line := <-stage.lines:
			if line[slogx.EventKey] == event {
				return line
			}
		case <-waited.Done():
			t.Fatalf("no %s line", event)
			return nil
		}
	}
}

// endedIdle is whether an ended line says the relay ended its allocation as idle.
func endedIdle(line map[string]any) bool {
	idle, isBool := line["idle"].(bool)
	return isBool && idle
}

// exchange sends a datagram each way between two ends and checks each arrives from the other.
func exchange(t *testing.T, one, other end) {
	t.Helper()
	send(t, one, other, []byte("to the other"))
	if got, from := receive(t, other); got != "to the other" || from.String() != one.relayed.LocalAddr().String() {
		t.Errorf("the other end read %q from %v, want it from %v", got, from, one.relayed.LocalAddr())
	}
	send(t, other, one, []byte("and back"))
	if got, from := receive(t, one); got != "and back" || from.String() != other.relayed.LocalAddr().String() {
		t.Errorf("the one end read %q from %v, want it from %v", got, from, other.relayed.LocalAddr())
	}
}

func TestRelay_CarriesAHouseholdsClientAndCoreBothWays(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	client := stage.mustAllocate(t, "udp", relaycred.RoleClient, "household-1", 100_000)
	core := stage.mustAllocate(t, "udp", relaycred.RoleCore, "household-1", 100_000)
	exchange(t, client, core)
	if dropped := stage.relay.Dropped(); dropped != 0 {
		t.Errorf("%d datagrams dropped between a household's two ends", dropped)
	}
}

func TestRelay_CarriesAnAllocationMadeOverTCPAndTLS(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	core := stage.mustAllocate(t, "udp", relaycred.RoleCore, "household-1", 100_000)
	for _, transport := range []string{"tcp", "tls"} {
		client := stage.mustAllocate(t, transport, relaycred.RoleClient, "household-1", 100_000)
		exchange(t, client, core)
		if err := client.relayed.Close(); err != nil {
			t.Fatal(err)
		}
		if line := stage.awaitLine(t, "relayAllocationEnded"); line["transport"] != transport {
			t.Errorf("an allocation over %s ended as one over %v", transport, line["transport"])
		}
	}
}

func TestRelay_ReachesNoOtherHouseholdNoOtherClientAndNothingNotAnAllocation(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	client := stage.mustAllocate(t, "udp", relaycred.RoleClient, "household-1", 100_000)
	otherClient := stage.mustAllocate(t, "udp", relaycred.RoleClient, "household-1", 100_000)
	otherCore := stage.mustAllocate(t, "udp", relaycred.RoleCore, "household-2", 100_000)
	send(t, client, otherCore, []byte("to another household's core"))
	if reason := stage.awaitDrop(t); reason != dropNotTheOtherEnd {
		t.Errorf("to another household's core: dropped as %s", reason)
	}
	send(t, client, otherClient, []byte("to a client"))
	if reason := stage.awaitDrop(t); reason != dropNotTheOtherEnd {
		t.Errorf("to a client: dropped as %s", reason)
	}

	outside, err := (&net.ListenConfig{}).ListenPacket(t.Context(), "udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = outside.Close() }) //nolint:errcheck // the test's end
	if _, err := client.relayed.WriteTo([]byte("to no allocation"), outside.LocalAddr()); err != nil {
		t.Fatal(err)
	}
	if reason := stage.awaitDrop(t); reason != dropNotTheOtherEnd {
		t.Errorf("to a socket that is no allocation: dropped as %s", reason)
	}
	// A socket on the relay's own host sending straight to a relayed port is no allocation either.
	relayedAddress, isUDP := client.relayed.LocalAddr().(*net.UDPAddr)
	if !isUDP {
		t.Fatalf("a relayed address %v", client.relayed.LocalAddr())
	}
	straight := &net.UDPAddr{IP: public, Port: relayedAddress.Port}
	if _, err := outside.WriteTo([]byte("from no allocation"), straight); err != nil {
		t.Fatal(err)
	}
	if reason := stage.awaitDrop(t); reason != dropNotTheOtherEnd {
		t.Errorf("from a socket that is no allocation: dropped as %s", reason)
	}
	if dropped := stage.relay.Dropped(); dropped != 4 {
		t.Errorf("%d dropped, want 4", dropped)
	}
}

// Firefox asks a permission for each of the core's candidates and drops its relayed candidates
// when one is refused (D560), so each is granted; the relayed socket still carries nothing there.
func TestRelay_GrantsAPermissionForAnyAddressAndCarriesNothingThere(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	client := stage.mustAllocate(t, "udp", relaycred.RoleClient, "household-1", 100_000)
	core := stage.mustAllocate(t, "udp", relaycred.RoleCore, "household-1", 100_000)
	outside, err := (&net.ListenConfig{}).ListenPacket(t.Context(), "udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = outside.Close() }) //nolint:errcheck // the test's end
	others := []net.Addr{
		&net.UDPAddr{IP: net.IPv4(192, 0, 2, 1), Port: 9}, &net.UDPAddr{IP: net.IPv4(10, 0, 0, 1), Port: 9},
		outside.LocalAddr(),
	}
	for _, other := range others {
		if err := client.client.CreatePermission(other); err != nil {
			t.Errorf("a permission for %v was refused: %v", other, err)
		}
	}
	if _, err := client.relayed.WriteTo([]byte("to a permitted address"), outside.LocalAddr()); err != nil {
		t.Fatal(err)
	}
	if reason := stage.awaitDrop(t); reason != dropNotTheOtherEnd {
		t.Errorf("to an address the client holds a permission for: dropped as %s", reason)
	}
	relayedAddress, isUDP := client.relayed.LocalAddr().(*net.UDPAddr)
	if !isUDP {
		t.Fatalf("a relayed address %v", client.relayed.LocalAddr())
	}
	straight := &net.UDPAddr{IP: public, Port: relayedAddress.Port}
	if _, err := outside.WriteTo([]byte("from a permitted address"), straight); err != nil {
		t.Fatal(err)
	}
	if reason := stage.awaitDrop(t); reason != dropNotTheOtherEnd {
		t.Errorf("from an address the client holds a permission for: dropped as %s", reason)
	}
	exchange(t, client, core)
}

// A browser does not release its allocations as its page goes, and holds spares no connection
// uses; each counted against its household for pion's ten minutes (production, 2026-11-26, D561).
func TestRelay_EndsAnAllocationItsOwnerSendsNothingThroughForAMinute(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	used := stage.mustAllocate(t, "udp", relaycred.RoleClient, "household-1", 100_000)
	stage.mustAllocate(t, "udp", relaycred.RoleClient, "household-1", 100_000)
	core := stage.mustAllocate(t, "udp", relaycred.RoleCore, "household-1", 100_000)
	for range 5 {
		stage.clock.Advance(idleSweepEvery)
		exchange(t, used, core)
	}
	if count := stage.relay.server.AllocationCount(); count != 3 {
		t.Fatalf("%d allocations live before any was idle a minute, want 3", count)
	}
	// The keeper arms its next look in its own time, so the clock steps until one has found it.
	var ended map[string]any
	for waited := patiently(t); ended == nil; {
		stage.clock.Advance(idleSweepEvery)
		exchange(t, used, core)
		select {
		case line := <-stage.lines:
			if line[slogx.EventKey] == "relayAllocationEnded" {
				ended = line
			}
		case <-waited.Done():
			t.Fatal("the unused allocation never ended")
		default:
		}
	}
	if !endedIdle(ended) || ended["role"] != string(relaycred.RoleClient) || ended["bytesFromClient"] != 0.0 {
		t.Errorf("the allocation ended: %v, want the client's unused one, as idle", ended)
	}
	exchange(t, used, core)
	if count := stage.relay.server.AllocationCount(); count != 2 {
		t.Errorf("%d allocations live after the unused one's minute, want the two in use", count)
	}
}

func TestRelay_AdmitsACredentialUnderAnyOfItsSecretsAndRefusesAnyOther(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	for secret := range stage.secrets {
		username, password := stage.credential(t, relaycred.RoleClient, "household-1", 3000, secret)
		if _, err := stage.allocate(t, "udp", username, password); err != nil {
			t.Errorf("a credential under secret %d refused: %v", secret, err)
		}
	}
	username, password := stage.credential(t, relaycred.RoleCore, "household-1", 3000, 0)
	unknown, err := relaycred.NewSecret([]byte(strings.Repeat("c", 32)))
	if err != nil {
		t.Fatal(err)
	}
	unknownUsername, unknownPassword, err := relaycred.Make(unknown, relaycred.Credential{
		ExpiresAt: stage.clock.Now().Add(time.Hour), Role: relaycred.RoleCore, Household: "household-1", Kbps: 3000,
	})
	if err != nil {
		t.Fatal(err)
	}
	expiringUsername, expiringPassword, err := relaycred.Make(stage.secrets[0], relaycred.Credential{
		ExpiresAt: stage.clock.Now().Add(time.Minute), Role: relaycred.RoleCore, Household: "household-1", Kbps: 3000,
	})
	if err != nil {
		t.Fatal(err)
	}
	tampered := []byte(password)
	tampered[3] ^= 1
	for name, refused := range map[string][2]string{
		"a tampered password":    {username, string(tampered)},
		"an unknown secret's":    {unknownUsername, unknownPassword},
		"a raised rate":          {strings.Replace(username, ":3000", ":30000", 1), password},
		"another household":      {strings.Replace(username, "household-1", "household-2", 1), password},
		"another role":           {strings.Replace(username, ":core:", ":client:", 1), password},
		"a username of no shape": {"someone", password},
	} {
		if _, err := stage.allocate(t, "udp", refused[0], refused[1]); err == nil {
			t.Errorf("%s was admitted", name)
		}
	}
	stage.clock.Advance(2 * time.Minute)
	if _, err := stage.allocate(t, "udp", expiringUsername, expiringPassword); err == nil {
		t.Error("an expired credential was admitted")
	}
}

func TestRelay_DropsWhatIsBeyondTheHouseholdsRate(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	// 8 kbit/s is 1000 bytes a second, and the bucket holds a second's.
	client := stage.mustAllocate(t, "udp", relaycred.RoleClient, "household-1", 8)
	core := stage.mustAllocate(t, "udp", relaycred.RoleCore, "household-1", 8)
	datagram := func(mark byte) []byte { return bytes.Repeat([]byte{mark}, 400) }
	send(t, core, client, datagram('1'))
	send(t, core, client, datagram('2'))
	send(t, core, client, datagram('3'))
	if reason := stage.awaitDrop(t); reason != dropBeyondTheRate {
		t.Errorf("the third 400 bytes in a second at 1000 a second: dropped as %s", reason)
	}
	stage.clock.Advance(time.Second)
	send(t, core, client, datagram('4'))
	for _, want := range []byte("124") {
		if got, _ := receive(t, client); got != string(datagram(want)) {
			t.Errorf("the client read %.1q…, want the datagram %c", got, want)
		}
	}
	// Toward the core no rate applies.
	for range 5 {
		send(t, client, core, datagram('c'))
		receive(t, core)
	}
}

// awaitRefusal waits for the line of an allocation refused past limit, asked for by role.
func (stage *stage) awaitRefusal(t *testing.T, limit, role string) {
	t.Helper()
	refused := stage.awaitLine(t, "relayAllocationRefused")
	if refused["limit"] != limit || refused["role"] != role || refused["household"] == "" {
		t.Errorf("the refusal's line is %v; want the limit %s, the role %s and its household", refused, limit, role)
	}
}

func TestRelay_RefusesPastEachLimit(t *testing.T) {
	t.Parallel()
	t.Run("sixteen a client credential", func(t *testing.T) {
		t.Parallel()
		stage := newStage(t)
		username, password := stage.credential(t, relaycred.RoleClient, "household-1", 3000, 0)
		// A browser with two interfaces, four connections and the relay's two URLs asks for sixteen.
		for count := range 16 {
			if _, err := stage.allocate(t, "udp", username, password); err != nil {
				t.Fatalf("allocation %d of a client credential: %v", count+1, err)
			}
		}
		if _, err := stage.allocate(t, "udp", username, password); err == nil {
			t.Error("a client credential's seventeenth allocation was made")
		}
		stage.awaitRefusal(t, "clientCredential", "client")
	})
	t.Run("sixty-four a household", func(t *testing.T) {
		t.Parallel()
		stage := newStage(t)
		for range allocationsPerHousehold {
			stage.mustAllocate(t, "udp", relaycred.RoleCore, "household-1", 3000)
		}
		username, password := stage.credential(t, relaycred.RoleClient, "household-1", 3000, 0)
		if _, err := stage.allocate(t, "udp", username, password); err == nil {
			t.Error("a household's sixty-fifth allocation was made")
		}
		stage.awaitRefusal(t, "household", "client")
	})
	t.Run("a hundred and twenty-eight a source address", func(t *testing.T) {
		t.Parallel()
		stage := newStage(t)
		for count := range allocationsPerSource {
			stage.mustAllocate(t, "udp", relaycred.RoleCore, fmt.Sprintf("household-%03d", count), 3000)
		}
		username, password := stage.credential(t, relaycred.RoleCore, "household-xx", 3000, 0)
		if _, err := stage.allocate(t, "udp", username, password); err == nil {
			t.Error("a source address's hundred and twenty-ninth allocation was made")
		}
		stage.awaitRefusal(t, "sourceAddress", "core")
	})
}

func TestRelay_RecordsAnEndedAllocationWithItsSourceShortenedAndItsTenSecondPeak(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	client := stage.mustAllocate(t, "udp", relaycred.RoleClient, "household-1", 100_000)
	core := stage.mustAllocate(t, "udp", relaycred.RoleCore, "household-1", 100_000)
	burst := func(count int) {
		for range count {
			send(t, core, client, bytes.Repeat([]byte{'x'}, 1000))
			receive(t, client)
		}
	}
	// 5,000 bytes, five seconds on 5,000 more, ten seconds on 1,000: the most any ten seconds held
	// is 10,000 bytes, 8 kbit/s.
	burst(5)
	stage.clock.Advance(5 * time.Second)
	burst(5)
	stage.clock.Advance(10 * time.Second)
	burst(1)
	send(t, client, core, []byte("12345"))
	receive(t, core)
	stage.clock.Advance(40 * time.Second)
	if err := client.relayed.Close(); err != nil {
		t.Fatal(err)
	}
	line := stage.awaitLine(t, "relayAllocationEnded")
	want := map[string]any{
		"household": "household-1", "role": "client", "transport": "udp", "source": "127.0.0.0/24",
		"seconds": 55.0, "bytesToClient": 11000.0, "bytesFromClient": 5.0, "peakKbpsToClient": 8.0, "dropped": 0.0,
	}
	for key, value := range want {
		if line[key] != value {
			t.Errorf("the ended line's %s is %v, want %v", key, line[key], value)
		}
	}
	if encoded, err := json.Marshal(line); err != nil || strings.Contains(string(encoded), "127.0.0.1") {
		t.Errorf("the ended line holds a full address: %s", encoded)
	}

	// At midnight the household's day is logged, with the day's peak toward its clients.
	if !stage.clock.AwaitArmed(t.Context(), 2) {
		t.Fatal("the day's timer and the idle keeper's are not armed")
	}
	// The hours until then end the core's allocation as idle; another is live as the day turns.
	stage.clock.Set(time.Date(2026, 11, 26, 23, 59, 50, 0, time.UTC))
	ended := stage.awaitLine(t, "relayAllocationEnded")
	if !endedIdle(ended) || ended["role"] != "core" {
		t.Errorf("after hours of nothing sent, the allocation ended: %v, want the core's, as idle", ended)
	}
	stage.mustAllocate(t, "udp", relaycred.RoleCore, "household-1", 100_000)
	if !stage.clock.AwaitArmed(t.Context(), 2) {
		t.Fatal("the day's timer is not armed")
	}
	stage.clock.Set(time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC))
	day := stage.awaitLine(t, "relayHouseholdDay")
	for key, value := range map[string]any{
		"household": "household-1", "day": "2026-11-26", "allocationsEnded": 2.0, "allocationsLive": 1.0,
		"bytesToClients": 11000.0, "bytesFromClients": 5.0, "peakKbpsToClients": 8.0,
	} {
		if day[key] != value {
			t.Errorf("the day's %s is %v, want %v", key, day[key], value)
		}
	}
}

func TestShortened_KeepsAnIPv4sSlash24AndAnIPv6sSlash48(t *testing.T) {
	t.Parallel()
	for source, want := range map[net.Addr]string{
		&net.UDPAddr{IP: net.IPv4(203, 0, 113, 77), Port: 5000}:       "203.0.113.0/24",
		&net.TCPAddr{IP: net.ParseIP("2001:db8:1:2::5"), Port: 443}:   "2001:db8:1::/48",
		&net.UDPAddr{IP: net.ParseIP("::ffff:198.51.100.9"), Port: 1}: "198.51.100.0/24",
	} {
		if got := shortened(source); got != want {
			t.Errorf("%v shortened to %s, want %s", source, got, want)
		}
	}
}

func TestRelay_ReadsTheCertificateAgainWhenItsFilesChange(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	served := func() int64 {
		conn, err := (&tls.Dialer{Config: &tls.Config{
			RootCAs: stage.certPool, ServerName: "relay.test", MinVersion: tls.VersionTLS12,
		}}).DialContext(t.Context(), "tcp", stage.relay.TLSAddress().String())
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close() //nolint:errcheck // the handshake is what is read
		secured, isTLS := conn.(*tls.Conn)
		if !isTLS {
			t.Fatalf("a TLS dial gave a %T", conn)
		}
		return secured.ConnectionState().PeerCertificates[0].SerialNumber.Int64()
	}
	if serial := served(); serial != 1 {
		t.Fatalf("serves certificate %d, want 1", serial)
	}
	stage.writeCertificate(t, 2, time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC))
	if serial := served(); serial != 2 {
		t.Errorf("after the files changed, serves certificate %d, want 2", serial)
	}
}
