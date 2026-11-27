// SPDX-License-Identifier: AGPL-3.0-or-later

package rtcapi_test

import (
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/kstenerud/orpheum/internal/core/rtcapi"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
)

// relayPatience is how long a test waits in real time for what takes milliseconds on the loopback.
const relayPatience = 10 * time.Second

// patient is a context that ends at relayPatience, or with the test.
func patient(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), relayPatience)
	t.Cleanup(cancel)
	return ctx
}

// relayedOffer is an offer from a client on the loopback, with a relayed candidate put before its
// own: an offer the core answers through its relay (D559 §2).
func relayedOffer(t *testing.T) rtcapi.Description {
	t.Helper()
	settings := webrtc.SettingEngine{}
	settings.SetIncludeLoopbackCandidate(true)
	settings.SetIPFilter(func(ip net.IP) bool { return ip.IsLoopback() })
	connection, err := webrtc.NewAPI(webrtc.WithSettingEngine(settings)).NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() }) //nolint:errcheck // the test is over
	if _, err = connection.CreateDataChannel(rtcapi.NegotiationLabel, nil); err != nil {
		t.Fatal(err)
	}
	offer, err := connection.CreateOffer(nil)
	if err != nil {
		t.Fatal(err)
	}
	gathered := webrtc.GatheringCompletePromise(connection)
	if err = connection.SetLocalDescription(offer); err != nil {
		t.Fatal(err)
	}
	select {
	case <-gathered:
	case <-time.After(relayPatience):
		t.Fatal("the client gathered nothing")
	}
	sdp := connection.LocalDescription().SDP
	relayed := strings.Replace(sdp, "a=candidate:", relayedCandidate+"\r\na=candidate:", 1)
	if !strings.Contains(relayed, relayedCandidate) {
		t.Fatalf("the offer has no candidate line to put the relayed one before:\n%s", sdp)
	}
	return rtcapi.Description{Type: "offer", SDP: relayed}
}

// relayStage is a core on the loopback and a fake clock, its one relay answering nothing, its
// failures reported and its discovery's word of the relay's announcements set by the test.
type relayStage struct {
	server  *rtcapi.Server
	clock   *clocktest.Fake
	offer   rtcapi.Description
	reports chan rtcapi.RelayFailure

	mutex       sync.Mutex
	announcedAt time.Time
	isAnnounced bool
}

func newRelayStage(t *testing.T, onLoopback bool) *relayStage {
	t.Helper()
	stage := &relayStage{
		server: newServer(t), clock: clocktest.NewFake(time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)),
		offer: relayedOffer(t), reports: make(chan rtcapi.RelayFailure, 16),
	}
	if onLoopback {
		rtcapi.AnswerOnLoopback(stage.server)
	} else {
		rtcapi.AnswerOnNoAddress(stage.server)
	}
	rtcapi.SetClock(stage.server, stage.clock)
	relayURL, _ := askedRelay(t)
	stage.server.UseICEServers(func() []webrtc.ICEServer {
		return []webrtc.ICEServer{{URLs: []string{relayURL}, Username: "core", Credential: "word"}}
	})
	stage.server.ReportRelayFailuresWith(func(_ context.Context, failure rtcapi.RelayFailure) {
		stage.reports <- failure
	})
	stage.server.UseRelayAnnouncements(func(name string) (time.Time, bool) {
		stage.mutex.Lock()
		defer stage.mutex.Unlock()
		if name != "127.0.0.1" {
			t.Errorf("the core asked of the announcements of %q; want the relay's host", name)
		}
		return stage.announcedAt, stage.isAnnounced
	})
	return stage
}

// announced sets what the discovery says of the relay's last announcement.
func (stage *relayStage) announced(at time.Time, isAnnounced bool) {
	stage.mutex.Lock()
	defer stage.mutex.Unlock()
	stage.announcedAt, stage.isAnnounced = at, isAnnounced
}

// answered is an answer's outcome.
type answered struct {
	answer rtcapi.Description
	err    error
}

// answer answers the stage's offer in the background.
func (stage *relayStage) answer(t *testing.T) chan answered {
	t.Helper()
	done := make(chan answered, 1)
	go func() {
		answer, err := stage.server.Answer(t.Context(), stage.offer)
		done <- answered{answer: answer, err: err}
	}()
	return done
}

// await is an answer's outcome, failing the test when none comes within relayPatience.
func await(t *testing.T, done chan answered) answered {
	t.Helper()
	select {
	case got := <-done:
		return got
	case <-time.After(relayPatience):
		t.Fatal("the answer did not come")
		return answered{}
	}
}

// failRelay answers once through the relay, which gives nothing, and returns the failure reported.
func (stage *relayStage) failRelay(t *testing.T) (answered, rtcapi.RelayFailure) {
	t.Helper()
	done := stage.answer(t)
	if !stage.clock.AwaitArmed(patient(t), 1) {
		t.Fatal("the answer did not wait for the relay")
	}
	stage.clock.Advance(rtcapi.RelayWait)
	got := await(t, done)
	select {
	case failure := <-stage.reports:
		return got, failure
	case <-time.After(relayPatience):
		t.Fatal("the relay's failure was not reported")
		return got, rtcapi.RelayFailure{}
	}
}

// asksTheRelay is whether an answer asks the relay: one that does waits for it on the fake clock,
// which the test then moves on; one that does not is answered on the loopback at once.
func (stage *relayStage) asksTheRelay(t *testing.T) bool {
	t.Helper()
	done := stage.answer(t)
	armed := make(chan struct{})
	go func() {
		if stage.clock.AwaitArmed(t.Context(), 1) {
			close(armed)
		}
	}()
	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("an answer leaving the relay out: %v", got.err)
		}
		return false
	case <-armed:
		stage.clock.Advance(rtcapi.RelayWait)
		await(t, done)
		return true
	case <-time.After(relayPatience):
		t.Fatal("the answer neither came nor waited for the relay")
		return false
	}
}

// A relay that gives the core no relayed candidate holds the answer relayWait (two seconds, a STUN
// server's wait) and no longer: the answer then goes with the core's other candidates, the relay is
// reported failed once, and the next answer leaves it out (D567 §3).
func TestAnswer_AnUnansweringRelayHoldsTheAnswerTwoSecondsThenIsLeftOut(t *testing.T) {
	t.Parallel()
	stage := newRelayStage(t, true)
	done := stage.answer(t)
	if !stage.clock.AwaitArmed(patient(t), 1) {
		t.Fatal("the answer did not wait for the relay")
	}
	stage.clock.Advance(rtcapi.RelayWait - time.Millisecond)
	select {
	case got := <-done:
		t.Fatalf("answered %v before the relay's two seconds were over", got.err)
	case <-time.After(200 * time.Millisecond):
	}
	failedAt := stage.clock.Now().Add(time.Millisecond)
	stage.clock.Advance(time.Millisecond)
	got := await(t, done)
	isOwn := strings.Contains(got.answer.SDP, "typ host") && !strings.Contains(got.answer.SDP, "typ relay")
	if got.err != nil || !isOwn {
		t.Fatalf("the answer %v:\n%s\nwant the core's own candidates, no relayed one", got.err, got.answer.SDP)
	}
	select {
	case failure := <-stage.reports:
		if failure != (rtcapi.RelayFailure{Name: "127.0.0.1", At: failedAt}) {
			t.Errorf("reported %+v; want the relay's host failed at %s", failure, failedAt)
		}
	case <-time.After(relayPatience):
		t.Fatal("the relay's failure was not reported")
	}
	if stage.asksTheRelay(t) {
		t.Error("the next answer asked the relay that failed")
	}
	select {
	case again := <-stage.reports:
		t.Errorf("a second report %+v; a failure is reported once", again)
	default:
	}
}

// An answer through a relay that gives nothing, when the core has no candidate of its own either,
// fails once the relay's wait is over.
func TestAnswer_ThroughAnUnansweringRelayWithNoCandidateAtAllFails(t *testing.T) {
	t.Parallel()
	stage := newRelayStage(t, false)
	got, _ := stage.failRelay(t)
	if got.err == nil {
		t.Fatalf("answered with no candidate at all:\n%s", got.answer.SDP)
	}
}

// A failed relay is taken back when the discovery says it announced itself after the failure, and
// not for an announcement before it (D567 §3).
func TestAnswer_AFailedRelayIsTakenBackByANewerAnnouncementOnly(t *testing.T) {
	t.Parallel()
	stage := newRelayStage(t, true)
	stage.announced(stage.clock.Now(), true)
	_, failure := stage.failRelay(t)
	stage.announced(failure.At.Add(-time.Second), true)
	stage.clock.Advance(rtcapi.RelayLeftOutFor) // a discovery's word holds, however long
	if stage.asksTheRelay(t) {
		t.Error("an announcement older than the failure took the relay back")
	}
	stage.announced(failure.At.Add(time.Millisecond), true)
	if !stage.asksTheRelay(t) {
		t.Error("an announcement newer than the failure did not take the relay back")
	}
}

// A discovery that says nothing of announcements, as one before protocol 1.1, has a failed relay
// left out relayLeftOutFor and then taken back.
func TestAnswer_AFailedRelayIsTakenBackAfterFiveMinutesWhenTheDiscoverySaysNothing(t *testing.T) {
	t.Parallel()
	stage := newRelayStage(t, true)
	_, failure := stage.failRelay(t)
	stage.clock.Set(failure.At.Add(rtcapi.RelayLeftOutFor - time.Second))
	if stage.asksTheRelay(t) {
		t.Error("the relay was taken back before five minutes")
	}
	stage.clock.Set(failure.At.Add(rtcapi.RelayLeftOutFor))
	if !stage.asksTheRelay(t) {
		t.Error("the relay was not taken back after five minutes")
	}
}

// A relay's name is its TURN URL's host.
func TestRelayName_IsATURNURLsHost(t *testing.T) {
	t.Parallel()
	for url, want := range map[string]string{
		"turn:relay.example:3478?transport=udp": "relay.example",
		"turns:relay.example:443?transport=tcp": "relay.example",
		"turn:relay.example":                    "relay.example",
		"turn:[2001:db8::1]:3478?transport=udp": "2001:db8::1",
		"stun:relay.example:3478":               "",
	} {
		if got := rtcapi.RelayName(url); got != want {
			t.Errorf("RelayName(%q) = %q; want %q", url, got, want)
		}
	}
}
