// SPDX-License-Identifier: AGPL-3.0-or-later

package relay

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/discovery/relaycred"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
)

// posted is one announcement a discovery received.
type posted struct {
	body      []byte
	signature string
	method    string
}

// announcedStage is a relay announcing to one discovery under secret, and what that discovery
// receives.
func announcedStage(t *testing.T, secret func(*stage) relaycred.Secret) (*stage, chan posted) {
	t.Helper()
	received := make(chan posted, 16)
	discovery := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Error(err)
		}
		signature := request.Header.Get(relaycred.AnnouncementHeader)
		received <- posted{body: body, signature: signature, method: request.Method}
		writer.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(discovery.Close)
	stage := newStageWith(t, func(stage *stage, config *Config, parts *Parts) {
		config.Announce = []Announce{{URL: discovery.URL + "/relay/announce", Secret: secret(stage)}}
		parts.HTTP = discovery.Client()
	})
	return stage, received
}

// awaitPost is the next announcement the discovery receives.
func awaitPost(t *testing.T, received chan posted) posted {
	t.Helper()
	select {
	case each := <-received:
		return each
	case <-patiently(t).Done():
		t.Fatal("the relay announced nothing")
		return posted{}
	}
}

// drained is the events of the stage's log lines written so far.
func (stage *stage) drained() []string {
	var events []string
	for {
		select {
		case line := <-stage.lines:
			event, _ := line[slogx.EventKey].(string) //nolint:errcheck // a line with no event is none
			events = append(events, event)
		default:
			return events
		}
	}
}

// armedTimers is the fake clock's timers while the relay announces: the idle sweep's, the day's and
// the announcer's own.
const armedTimers = 3

// The relay makes an allocation on its own public listener and, when it works, POSTs the
// discovery an announcement signed under their secret, at once and each minute after; the
// self-test's allocation is no household's and is recorded as none.
func TestAnnounce_TestsItselfAndPostsASignedAnnouncementEachMinute(t *testing.T) {
	t.Parallel()
	stage, received := announcedStage(t, func(stage *stage) relaycred.Secret { return stage.secrets[1] })
	for round := range 2 {
		got := awaitPost(t, received)
		announcement, err := relaycred.ReadAnnouncement(stage.secrets[1], got.body, got.signature, stage.clock.Now())
		if err != nil || got.method != http.MethodPost {
			t.Fatalf("round %d: a %s of %s: %v; want a POST signed under the discovery's secret", round,
				got.method, got.body, err)
		}
		if announcement.Name != "relay.test" || announcement.At != stage.clock.Now().UnixMilli() {
			t.Errorf("round %d: %+v; want relay.test at %d", round, announcement, stage.clock.Now().UnixMilli())
		}
		if !stage.clock.AwaitArmed(patiently(t), armedTimers) {
			t.Fatal("the relay did not wait for its next announcement")
		}
		select {
		case early := <-received:
			t.Fatalf("round %d: a second announcement %s before the minute", round, early.body)
		default:
		}
		stage.clock.Advance(announceEvery)
	}
	events := stage.drained()
	if strings.Count(strings.Join(events, " "), stateAnnounced) != 1 {
		t.Errorf("the log's events %v; want one %s line for the state, not one a round", events, stateAnnounced)
	}
	for _, event := range events {
		if event == "relayAllocationEnded" || event == "relayAllocationRefused" {
			t.Errorf("the self-test's allocation was recorded as a household's: %s", event)
		}
	}
}

// A relay whose own allocation fails announces nothing, and says so once.
func TestAnnounce_AFailingSelfTestAnnouncesNothing(t *testing.T) {
	t.Parallel()
	// A secret the relay does not hold: its own credential is refused, as when its listener fails.
	stranger, err := relaycred.NewSecret([]byte(strings.Repeat("s", 32)))
	if err != nil {
		t.Fatal(err)
	}
	stage, received := announcedStage(t, func(*stage) relaycred.Secret { return stranger })
	stage.awaitLine(t, stateSelfTestFailed)
	for range 2 {
		if !stage.clock.AwaitArmed(patiently(t), armedTimers) {
			t.Fatal("the relay did not wait for its next self-test")
		}
		stage.clock.Advance(announceEvery)
	}
	if !stage.clock.AwaitArmed(patiently(t), armedTimers) {
		t.Fatal("the relay did not wait for its next self-test")
	}
	select {
	case got := <-received:
		t.Fatalf("the relay announced %s though its own allocation failed", got.body)
	default:
	}
	for _, event := range stage.drained() {
		if event == stateSelfTestFailed {
			t.Errorf("a second %s line: a state is logged once, as it changes", event)
		}
	}
}
