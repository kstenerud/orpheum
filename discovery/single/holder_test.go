// SPDX-License-Identifier: AGPL-3.0-or-later

package single

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket/wsjson"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
)

// sessionFrom opens a session for the core coreID with signer, as the instance instanceID of the
// installation installationID, returning the status, the token on success and the refusal otherwise.
func (built *stage) sessionFrom(
	t *testing.T, signer key, coreID, instanceID, installationID string,
) (int, string, discoveryproto.Refusal) {
	t.Helper()
	nonce := built.nonce(t, signer)
	var sent bytes.Buffer
	if err := json.NewEncoder(&sent).Encode(discoveryproto.SessionRequest{
		CoreID: coreID, InstanceID: instanceID, InstallationID: installationID, Nonce: nonce,
		Signature: signer.sign(discoveryproto.PurposeSession, coreID, nonce),
	}); err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost,
		built.server.URL+discoveryproto.PathSession, &sent)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set(discoveryproto.ProtocolHeader, discoveryproto.ProtocolVersion)
	response, err := built.server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }() //nolint:errcheck // read in full below
	if response.StatusCode == http.StatusOK {
		var opened discoveryproto.SessionResponse
		if err = json.NewDecoder(response.Body).Decode(&opened); err != nil {
			t.Fatal(err)
		}
		return response.StatusCode, opened.Token, discoveryproto.Refusal{}
	}
	var refusal discoveryproto.Refusal
	_ = json.NewDecoder(response.Body).Decode(&refusal) //nolint:errcheck // a refusal's body, or none
	return response.StatusCode, "", refusal
}

// answering opens the signalling connection with token and answers pings on it, handing on each
// message the discovery sends.
func (built *stage) answering(t *testing.T, token string) <-chan discoveryproto.SignalMessage {
	t.Helper()
	connection := built.dial(t, token)
	received := make(chan discoveryproto.SignalMessage, 4)
	go func() {
		defer close(received)
		for {
			var message discoveryproto.SignalMessage
			if err := wsjson.Read(t.Context(), connection, &message); err != nil {
				return
			}
			received <- message
		}
	}()
	built.logs.AwaitLine(t, "the core's signalling connected")
	return received
}

// The installation that last held the core is admitted at once, its live instance replaced without
// a ping, as a deploy's restart; another installation is told how long to wait while the holder is
// away, for the window from when it was last connected, and is heard after it; a copy refused while
// the live one answers is told to the live one, its address and when (D437 §4, §5).
func TestSession_TheLastHolderIsAdmittedAtOnceAndAnotherWaitsItsWindow(t *testing.T) {
	t.Parallel()
	built := newStage(t)
	built.discovery.pingWait = 200 * time.Millisecond
	signer, coreID := built.claimed(t)
	status, token, _ := built.sessionFrom(t, signer, coreID, "a1", "installation-a")
	if status != http.StatusOK {
		t.Fatalf("the first installation's session answered %d", status)
	}
	held := built.answering(t, token)
	// The holder's next run is admitted at once though the first still answers: a deploy's restart.
	status, token, _ = built.sessionFrom(t, signer, coreID, "a2", "installation-a")
	if status != http.StatusOK {
		t.Fatalf("the last holder's new instance answered %d; want it admitted at once", status)
	}
	for range held { //nolint:revive // drained until the replaced connection ends
	}
	served := built.answering(t, token)
	// Another installation, while the holder answers, is refused, and the holder is told.
	status, _, refusal := built.sessionFrom(t, signer, coreID, "b1", "installation-b")
	if status != http.StatusConflict || refusal.Code != discoveryproto.RefusalAnotherInstance {
		t.Fatalf("another installation while the holder answers got %d %+v; want another_instance", status, refusal)
	}
	built.awaitCopyRefused(t, served)
	// The holder goes away: another installation waits the window from then, the protocol's
	// seconds rounded up, and one naming no installation is another installation too.
	built.discovery.signal.DropSilent("a2")
	built.advance(time.Minute)
	for _, installation := range []string{"installation-b", ""} {
		status, _, refusal = built.sessionFrom(t, signer, coreID, "b1", installation)
		if !isHolderAway(status, refusal, 300) {
			t.Errorf("installation %q while the holder is away got %d %+v; want holder_away, 300 s", installation,
				status, refusal)
		}
	}
	built.advance(5*time.Minute - time.Millisecond)
	status, _, refusal = built.sessionFrom(t, signer, coreID, "b1", "installation-b")
	if !isHolderAway(status, refusal, 1) {
		t.Errorf("a millisecond before the window ends got %d %+v; want 1 s to wait", status, refusal)
	}
	built.advance(time.Millisecond)
	if status, _, refusal = built.sessionFrom(t, signer, coreID, "b1", "installation-b"); status != http.StatusOK {
		t.Fatalf("another installation once the window passed got %d %+v", status, refusal)
	}
	// It now holds the realm: the one before, coming back, waits for it in turn.
	status, _, refusal = built.sessionFrom(t, signer, coreID, "a3", "installation-a")
	if !isHolderAway(status, refusal, 360) {
		t.Errorf("the holder before, after another took over, got %d %+v; want holder_away, 360 s", status, refusal)
	}
}

// isHolderAway is whether a session was refused as holder_away, saying to wait seconds.
func isHolderAway(status int, refusal discoveryproto.Refusal, seconds int64) bool {
	return status == http.StatusConflict && refusal.Code == discoveryproto.RefusalHolderAway &&
		refusal.RetryAfterSeconds == seconds
}

// awaitCopyRefused waits for the serving core to be told of a copy refused from the test's own
// address now.
func (built *stage) awaitCopyRefused(t *testing.T, served <-chan discoveryproto.SignalMessage) {
	t.Helper()
	select {
	case told := <-served:
		if told.Type != discoveryproto.SignalCopyRefused || told.Address != "127.0.0.1" ||
			told.At != built.clock().UnixMilli() {
			t.Errorf("the serving core was told %+v; want copy_refused from 127.0.0.1 now", told)
		}
	case <-t.Context().Done():
		t.Fatal("the serving core was not told of the refused copy")
	}
}

// The last holder is kept across the discovery's restart, and its window counts from the restart, so
// a holder coming back with it still gets its chance; a core naming no installation holds it as none,
// and then waits no one (D437 §5).
func TestSession_TheLastHolderOutlivesARestartAndItsWindowCountsFromIt(t *testing.T) {
	t.Parallel()
	built := newStage(t)
	signer, coreID := built.claimed(t)
	if status, _, _ := built.sessionFrom(t, signer, coreID, "a1", "installation-a"); status != http.StatusOK {
		t.Fatalf("the first installation's session answered %d", status)
	}
	built.advance(time.Hour)
	restarted, err := New(built.discovery.parts)
	if err != nil {
		t.Fatal(err)
	}
	built.discovery = restarted
	built.server = httptest.NewServer(restarted.Handler())
	t.Cleanup(built.server.Close)
	built.advance(time.Minute)
	status, _, refusal := built.sessionFrom(t, signer, coreID, "b1", "installation-b")
	if !isHolderAway(status, refusal, 300) {
		t.Fatalf("another installation a minute after the restart got %d %+v; want holder_away, 300 s", status, refusal)
	}
	if status, _, _ = built.sessionFrom(t, signer, coreID, "a2", "installation-a"); status != http.StatusOK {
		t.Fatalf("the last holder after the restart got %d; want it admitted at once", status)
	}
	// A core before D437 is another installation, heard once the window passed; it then holds the
	// realm as no installation, and another is heard by the ping alone.
	built.advance(HolderWindowDefault)
	if status, _, _ = built.sessionFrom(t, signer, coreID, "old", ""); status != http.StatusOK {
		t.Fatalf("a core naming no installation, the holder silent, got %d", status)
	}
	if status, _, refusal = built.sessionFrom(t, signer, coreID, "b2", "installation-b"); status != http.StatusOK {
		t.Errorf("an installation after a holder of none got %d %+v; want it admitted", status, refusal)
	}
}
