// SPDX-License-Identifier: AGPL-3.0-or-later

package single_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/discovery/addressdoc"
	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/discovery/link"
	"github.com/kstenerud/orpheum/discovery/proof"
	"github.com/kstenerud/orpheum/discovery/single"
	"github.com/kstenerud/orpheum/discovery/trust"
	"github.com/kstenerud/orpheum/discovery/trust/trusttest"
	"github.com/kstenerud/orpheum/internal/core/discovery"
	"github.com/kstenerud/orpheum/internal/core/identity"
	"github.com/kstenerud/orpheum/internal/core/instancedir"
	"github.com/kstenerud/orpheum/internal/core/rtcapi"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// writeDocument writes document, signed by a root key made for the test, where a discovery serves it
// from: the operator's signed file.
func writeDocument(t *testing.T, path string, document addressdoc.Document) {
	t.Helper()
	root := trusttest.NewRoot(t)
	encoded, err := json.Marshal(addressdoc.Served{
		Document: root.Signs(t, trust.KindAddressDocument, document), RootSets: []trust.Signed{root.Set},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
}

// memoryFiles is the core's persistent directory, in memory.
type memoryFiles struct {
	mutex sync.Mutex
	files map[string][]byte
}

func (files *memoryFiles) ReadPersistentFile(name string) ([]byte, error) {
	files.mutex.Lock()
	defer files.mutex.Unlock()
	contents, found := files.files[name]
	if !found {
		return nil, instancedir.ErrNotFound
	}
	return contents, nil
}

func (files *memoryFiles) WritePersistentFile(name string, contents []byte) error {
	files.mutex.Lock()
	defer files.mutex.Unlock()
	files.files[name] = contents
	return nil
}

func (files *memoryFiles) RemovePersistentFile(name string) error {
	files.mutex.Lock()
	defer files.mutex.Unlock()
	delete(files.files, name)
	return nil
}

// answering answers every offer as the core's WebRTC server would.
type answering struct{}

func (answering) AnswerRelayed(
	_ context.Context, offer rtcapi.Description, _ func(rtcapi.ProvenAtHome),
) (rtcapi.Description, error) {
	return rtcapi.Description{Type: "answer", SDP: "answer to " + offer.SDP}, nil
}

// The core's own discovery client, unchanged, enrols with a single-target discovery by its issuer,
// trusting the address document the discovery serves there, is claimed by the code it shows, sends
// its addresses, and answers the offer the discovery passes it (W4b.2; D391 §3).
func TestCore_ACoreEnrolsIsClaimedAndSendsItsAddresses(t *testing.T) {
	t.Parallel()
	coreClock := clocktest.NewFake(time.Date(2026, 10, 11, 20, 0, 0, 0, time.UTC))
	logs := &testutil.Log{}
	// The discovery keeps a clock of its own, which stands still: the core's sleeps move the core's
	// fake clock on by hours, which would expire every nonce the discovery issued meanwhile.
	discoveryNow := coreClock.Now()
	// The server is up before the discovery is made, which is told the address the core calls.
	var handler http.Handler
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		handler.ServeHTTP(writer, request)
	}))
	t.Cleanup(server.Close)
	documentPath := filepath.Join(t.TempDir(), "address-document.json")
	writeDocument(t, documentPath, addressdoc.Document{
		Issuer: server.URL, CoreAPI: server.URL, ClientAPI: server.URL + "/client", LoaderOrigin: server.URL,
		ClientOrigins: "http://" + addressdoc.CorePlace + ".localhost:8443", IssuedAt: discoveryNow.UnixMilli(),
	})
	held, err := single.New(single.Parts{
		Issuer: server.URL, AddressDocument: documentPath, Zone: "home.example",
		Store: single.FileStore{Path: filepath.Join(t.TempDir(), "discovery.json")},
		Now:   func() time.Time { return discoveryNow }, Random: rand.Reader,
		Logger: slog.New(slog.NewTextHandler(logs, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle(addressdoc.WellKnownPath, held.WellKnownHandler())
	mux.Handle("/", held.Handler())
	handler = mux
	key, err := identity.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	core, err := discovery.New(discovery.Parts{
		Files: &memoryFiles{files: map[string][]byte{}}, Key: key, HTTP: &http.Client{}, Clock: coreClock,
		Logger: slog.New(slog.NewTextHandler(logs, nil)), Version: "v0.2.0", APIMajor: 1,
		Candidates: func() []discoveryproto.Candidate {
			return []discoveryproto.Candidate{{Address: "2a0f:1000::1", Port: 443, Source: "interface"}}
		},
		Answerer: answering{}, OnClaimed: func(context.Context) {}, Random: rand.Reader,
		Settings: quickRetries{},
		OfferKey: func(_ context.Context, hash string) ([]byte, error) {
			if hash != link.LocatorHash("a device's locator") {
				return nil, errors.New("no such locator")
			}
			return proof.SessionOfferKey("a session's token"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	status, err := core.Connect(t.Context(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	// The owner types the code the core's System page shows: `orpheum discovery claim`.
	if err = held.Claim(status.ClaimCode); err != nil {
		t.Fatalf("claiming with the core's own code %q: %v", status.ClaimCode, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		core.Run(ctx)
	}()
	// The core's loop waits on its clock: it is moved on until the core has done its part.
	ticker := time.NewTicker(20 * time.Millisecond)
	go func() {
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return
			case <-ticker.C:
				coreClock.Advance(time.Second)
			}
		}
	}()
	t.Cleanup(func() {
		cancel()
		testutil.Receive(t, done, "the core's discovery loop to end")
	})
	logs.AwaitLine(t, "the core sent its addresses", "candidates=1", "orpheumVersion=v0.2.0")
	logs.AwaitLine(t, "the core's signalling connected")
	offer := "a browser's offer\r\na=fingerprint:sha-256 AB:CD"
	// The client proves its offer under its session's key; the core checks it before answering.
	answer, err := held.Offer(t.Context(), discoveryproto.ClientOffer{
		SDP: offer, Proof: proof.Offer(proof.SessionOfferKey("a session's token"), "sha-256 AB:CD"),
	}, link.LocatorHash("a device's locator"), netip.Prefix{})
	if err != nil || answer.SDP != "answer to "+offer {
		t.Errorf("the offer passed through the discovery was answered %q, %v", answer.SDP, err)
	}
	// The core's answer comes back signed, which the client checks with the core's key (§2.5 item 4).
	signed := proof.Answer{
		CoreID: core.Status().CoreID, OfferID: "1", OfferFingerprint: "sha-256 AB:CD", SDP: answer.SDP,
	}
	if !proof.VerifyAnswer(key.Public(), signed, answer.Signature) {
		t.Error("the answer through the discovery is not the core's, signed over its SDP")
	}
}

// quickRetries is the core's settings with its retries on the discovery quick, a test's: the first
// 10 ms and the most 100 ms; every other wait its starting value.
type quickRetries struct{}

func (quickRetries) Duration(_ context.Context, key settings.Key) (time.Duration, error) {
	if key == settings.DiscoveryRetryFirstSeconds {
		return 10 * time.Millisecond, nil
	}
	if key == settings.DiscoveryRetryMostSeconds {
		return 100 * time.Millisecond, nil
	}
	return settings.StartingDuration(key), nil
}
