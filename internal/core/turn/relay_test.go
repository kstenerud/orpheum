// SPDX-License-Identifier: GPL-3.0-or-later

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

// allocate asks the relay for a relayed address with credentials, as a browser's ICE would.
func (stage relayStage) allocate(t *testing.T, credentials Credentials) error {
	t.Helper()
	connection, err := (&net.ListenConfig{}).ListenPacket(t.Context(), "udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = connection.Close() }() //nolint:errcheck // the test's end
	client, err := pionturn.NewClient(&pionturn.ClientConfig{
		STUNServerAddr: stage.relay.Address().String(), TURNServerAddr: stage.relay.Address().String(),
		Conn: connection, Username: credentials.Username, Password: credentials.Password, Realm: realm,
		RTO: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err = client.Listen(); err != nil {
		t.Fatal(err)
	}
	relayed, err := client.Allocate()
	if err != nil {
		return err //nolint:wrapcheck // the test reads it
	}
	return relayed.Close() //nolint:wrapcheck // the test reads it
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
	issued := Issue(secret, member, stage.clock.Now())
	if err = stage.allocate(t, issued); err != nil {
		t.Fatalf("a live membership's credentials: %v; want a relayed address", err)
	}
	forged := issued
	forged.Password = Issue(secrets.NewValue("another secret"), member, stage.clock.Now()).Password
	if err = stage.allocate(t, forged); err == nil {
		t.Error("credentials made with another secret were admitted")
	}
	stranger := Issue(secret, Membership{SessionID: 4, MemberID: 10}, stage.clock.Now())
	if err = stage.allocate(t, stranger); err == nil {
		t.Error("credentials of a membership that is not live were admitted")
	}
	stage.clock.Advance(credentialLifetime)
	if err = stage.allocate(t, issued); err == nil {
		t.Error("expired credentials were admitted")
	}
	stage.ended.Store(member, true)
	if err = stage.allocate(t, Issue(secret, member, stage.clock.Now())); err == nil {
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
