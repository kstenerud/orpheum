// SPDX-License-Identifier: AGPL-3.0-or-later

package turn

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	pionturn "github.com/pion/turn/v4"

	"github.com/kstenerud/orpheum/internal/core/secrets"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
)

// memorySecrets keeps secrets in memory, as the sealed store would.
type memorySecrets struct {
	mutex  sync.Mutex
	values map[secrets.Name]secrets.Value
	// unopenable is a secret kept under a key this server no longer has.
	unopenable map[secrets.Name]bool
}

func newMemorySecrets() *memorySecrets {
	return &memorySecrets{values: map[secrets.Name]secrets.Value{}, unopenable: map[secrets.Name]bool{}}
}

func (kept *memorySecrets) Open(_ context.Context, name secrets.Name) (secrets.Value, error) {
	kept.mutex.Lock()
	defer kept.mutex.Unlock()
	if kept.unopenable[name] {
		return secrets.Value{}, secrets.ErrNeedsReentry
	}
	value, found := kept.values[name]
	if !found {
		return secrets.Value{}, secrets.ErrNotSet
	}
	return value, nil
}

func (kept *memorySecrets) Set(_ context.Context, name secrets.Name, value secrets.Value, _ *int64) error {
	kept.mutex.Lock()
	defer kept.mutex.Unlock()
	kept.values[name], kept.unopenable[name] = value, false
	return nil
}

// relayStage is a relay on loopback whose one live membership is session 4's member 9.
type relayStage struct {
	relay   *Relay
	secrets *memorySecrets
	clock   *clocktest.Fake
	ended   *sync.Map // memberships ended, by Membership
}

func newRelayStage(t *testing.T) relayStage {
	t.Helper()
	kept := newMemorySecrets()
	if err := EnsureSecret(t.Context(), kept, rand.Reader); err != nil {
		t.Fatal(err)
	}
	fakeClock := clocktest.NewFake(time.Date(2026, 10, 5, 20, 0, 0, 0, time.UTC))
	ended := &sync.Map{}
	isMember := func(sessionID, memberID int64) bool {
		_, isEnded := ended.Load(Membership{SessionID: sessionID, MemberID: memberID})
		return sessionID == 4 && memberID == 9 && !isEnded
	}
	relay, err := Start(t.Context(), Config{
		Listen: "127.0.0.1:0", PublicAddress: net.IPv4(127, 0, 0, 1), RelayPortMin: 50000, RelayPortMax: 50999,
	}, Parts{Secrets: kept, IsMember: isMember, Clock: fakeClock, Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = relay.Close() }) //nolint:errcheck // the test's end
	return relayStage{relay: relay, secrets: kept, clock: fakeClock, ended: ended}
}

// allocate asks the relay for a relayed address with credentials, as a browser's ICE would, over
// UDP, and returns the error.
func (stage relayStage) allocate(t *testing.T, credentials Credentials) error {
	t.Helper()
	relayed, err := stage.allocateOver(t, "udp", credentials)
	if err != nil {
		return err
	}
	return relayed.Close() //nolint:wrapcheck // the test reads it
}

// allocateOver asks the relay for a relayed address over network, "udp" or "tcp".
func (stage relayStage) allocateOver(t *testing.T, network string, credentials Credentials) (net.PacketConn, error) {
	t.Helper()
	var connection net.PacketConn
	if network == "tcp" {
		stream, err := (&net.Dialer{}).DialContext(t.Context(), "tcp", stage.relay.Address().String())
		if err != nil {
			t.Fatal(err)
		}
		connection = pionturn.NewSTUNConn(stream)
	} else {
		packets, err := (&net.ListenConfig{}).ListenPacket(t.Context(), "udp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		connection = packets
	}
	t.Cleanup(func() { _ = connection.Close() }) //nolint:errcheck // the test's end
	client, err := pionturn.NewClient(&pionturn.ClientConfig{
		STUNServerAddr: stage.relay.Address().String(), TURNServerAddr: stage.relay.Address().String(),
		Conn: connection, Username: credentials.Username, Password: credentials.Password, Realm: realm,
		RTO: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	if err = client.Listen(); err != nil {
		t.Fatal(err)
	}
	return client.Allocate() //nolint:wrapcheck // the test reads it
}

// issue is the live membership's credentials, now.
func (stage relayStage) issue(t *testing.T) Credentials {
	t.Helper()
	secret, err := stage.secrets.Open(t.Context(), secrets.TURNSecret)
	if err != nil {
		t.Fatal(err)
	}
	return Issue(secret, Membership{SessionID: 4, MemberID: 9}, stage.clock.Now(), testLifetime)
}

// The relay admits the credentials it issued to a live membership, and refuses them once the
// membership ends, once they expire, and with a password they were not given (D243 §8, D194).
func TestRelay_AdmitsAMembershipsCredentialsUntilItEndsOrTheyExpire(t *testing.T) {
	t.Parallel()
	stage := newRelayStage(t)
	secret, err := stage.secrets.Open(t.Context(), secrets.TURNSecret)
	if err != nil {
		t.Fatal(err)
	}
	member := Membership{SessionID: 4, MemberID: 9}
	issued := stage.issue(t)
	if err = stage.allocate(t, issued); err != nil {
		t.Fatalf("a live membership's credentials: %v; want a relayed address", err)
	}
	forged := issued
	forged.Password = Issue(secrets.NewValue("another secret"), member, stage.clock.Now(), testLifetime).Password
	if err = stage.allocate(t, forged); err == nil {
		t.Error("credentials made with another secret were admitted")
	}
	stranger := Issue(secret, Membership{SessionID: 4, MemberID: 10}, stage.clock.Now(), testLifetime)
	if err = stage.allocate(t, stranger); err == nil {
		t.Error("credentials of a membership that is not live were admitted")
	}
	stage.clock.Advance(testLifetime)
	if err = stage.allocate(t, issued); err == nil {
		t.Error("expired credentials were admitted")
	}
	stage.ended.Store(member, true)
	if err = stage.allocate(t, Issue(secret, member, stage.clock.Now(), testLifetime)); err == nil {
		t.Error("an ended membership's fresh credentials were admitted")
	}
}

// The TURN secret is made when there is none, kept when there is one, and made again when this
// server cannot open it, as after a restore without secret.key (D194).
func TestEnsureSecret_MakesTheSecretOnceAndAgainWhenItCannotBeOpened(t *testing.T) {
	t.Parallel()
	kept := newMemorySecrets()
	if err := EnsureSecret(t.Context(), kept, rand.Reader); err != nil {
		t.Fatal(err)
	}
	first, err := kept.Open(t.Context(), secrets.TURNSecret)
	if err != nil || first.IsZero() {
		t.Fatalf("after EnsureSecret: %v; want a secret", err)
	}
	if err = EnsureSecret(t.Context(), kept, rand.Reader); err != nil {
		t.Fatal(err)
	}
	again, err := kept.Open(t.Context(), secrets.TURNSecret)
	if err != nil || again.Reveal() != first.Reveal() {
		t.Errorf("after a second EnsureSecret: %v; want the secret it could open kept", err)
	}
	kept.unopenable[secrets.TURNSecret] = true
	if err = EnsureSecret(t.Context(), kept, rand.Reader); err != nil {
		t.Fatal(err)
	}
	if _, err = kept.Open(t.Context(), secrets.TURNSecret); errors.Is(err, secrets.ErrNeedsReentry) {
		t.Error("EnsureSecret left a secret this server cannot open")
	}
}

// A browser that cannot send UDP reaches the relay over TCP on the same port (D243 §7); and a
// relayed address reaches peers on the public internet only, never the server's own machine or
// the network it is on.
func TestRelay_ServesTCPAndRelaysOnlyToThePublicInternet(t *testing.T) {
	t.Parallel()
	stage := newRelayStage(t)
	relayed, err := stage.allocateOver(t, "tcp", stage.issue(t))
	if err != nil {
		t.Fatalf("an allocation over TCP: %v; want a relayed address", err)
	}
	defer func() { _ = relayed.Close() }() //nolint:errcheck // the test's end
	public := &net.UDPAddr{IP: net.IPv4(203, 0, 113, 7), Port: 9}
	if _, err = relayed.WriteTo([]byte("hello"), public); err != nil {
		t.Errorf("relaying to a public address: %v; want it sent", err)
	}
	for _, peer := range []string{"127.0.0.1", "10.0.0.5", "192.168.1.20", "100.64.0.1", "192.0.0.2", "fd00::1"} {
		if _, err = relayed.WriteTo([]byte("hello"), &net.UDPAddr{IP: net.ParseIP(peer), Port: 9}); err == nil {
			t.Errorf("relaying to %s was permitted; want the server's own networks refused", peer)
		}
	}
}

// The credentials a core sends its discovery admit any device holding them until they expire, with
// no membership; forged ones admit no one (the discovery protocol's §2.4).
func TestRelay_AdmitsTheDiscoverysCredentialsUntilTheyExpire(t *testing.T) {
	t.Parallel()
	stage := newRelayStage(t)
	secret, err := stage.secrets.Open(t.Context(), secrets.TURNSecret)
	if err != nil {
		t.Fatal(err)
	}
	found := IssueFound(secret, stage.clock.Now())
	if err = stage.allocate(t, found); err != nil {
		t.Fatalf("the discovery's credentials: %v; want a relayed address", err)
	}
	forged := found
	forged.Password = IssueFound(secrets.NewValue("another secret"), stage.clock.Now()).Password
	if err = stage.allocate(t, forged); err == nil {
		t.Error("the discovery's credentials made with another secret were admitted")
	}
	stage.clock.Advance(testLifetime)
	if err = stage.allocate(t, found); err == nil {
		t.Error("the discovery's expired credentials were admitted")
	}
}

// The credentials a core sends its discovery last a quarter of an hour, each heartbeat bringing new
// ones, since every device finding the core in that time holds them; an allocation made while they
// were good keeps being served after, so a relayed film is not cut, and they make no new one (the
// Plex-chain review's relay credential).
func TestRelay_KeepsServingAnAllocationTheDiscoverysCredentialsMadeAfterTheyExpire(t *testing.T) {
	t.Parallel()
	stage := newRelayStage(t)
	secret, err := stage.secrets.Open(t.Context(), secrets.TURNSecret)
	if err != nil {
		t.Fatal(err)
	}
	found := IssueFound(secret, stage.clock.Now())
	if lasts := found.ExpiresAt.Sub(stage.clock.Now()); lasts > 15*time.Minute {
		t.Errorf("the discovery's credentials last %v; want a quarter of an hour at most", lasts)
	}
	relayed, err := stage.allocateOver(t, "udp", found)
	if err != nil {
		t.Fatalf("the discovery's credentials: %v; want a relayed address", err)
	}
	defer func() { _ = relayed.Close() }() //nolint:errcheck // the test's end
	stage.clock.Advance(time.Hour)
	// A new peer asks the relay for a permission, under the credentials, now expired.
	if _, err = relayed.WriteTo([]byte("hello"), &net.UDPAddr{IP: net.IPv4(203, 0, 113, 8), Port: 9}); err != nil {
		t.Errorf("relaying to a new peer after the credentials expired: %v; want the allocation still served", err)
	}
	if err = stage.allocate(t, found); err == nil {
		t.Error("the discovery's expired credentials made a new allocation")
	}
}
