// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

// Sessions, retries and connecting: one renewal for every caller, a session's life on the core's
// own clock, waits that double with jitter, and one enrolment however many Connects at once.

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// unjittered is a random source whose every share is exactly the middle, 0x8000…, so a jittered
// wait is unmoved.
type unjittered struct{}

func (unjittered) Read(into []byte) (int, error) {
	clear(into)
	if len(into) > 0 {
		into[0] = 0x80
	}
	return len(into), nil
}

func TestStep_AFailedCallWaitsLongerEachTimeInARowAndASuccessStartsOver(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	ctx := t.Context()
	if _, err := built.service.Connect(ctx, built.issuer); err != nil {
		t.Fatal(err)
	}
	failing := func(isFailing bool) {
		built.fake.mutex.Lock()
		defer built.fake.mutex.Unlock()
		built.fake.failing["GET /status"] = isFailing
	}
	failing(true)
	var waits []time.Duration
	for range 4 {
		waits = append(waits, built.service.step(ctx))
	}
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second}
	if !slices.Equal(waits, want) {
		t.Errorf("failures in a row waited %v; want %v, doubling from the first", waits, want)
	}
	failing(false)
	built.service.step(ctx)
	failing(true)
	if wait := built.service.step(ctx); wait != time.Second {
		t.Errorf("a failure after a success waited %s; want the first wait again", wait)
	}
}

func TestRetryWait_TwoCoresWaitDifferentlyAndNeverPastTheMost(t *testing.T) {
	t.Parallel()
	waitOf := func(random byte, failures int64) time.Duration {
		waits := byKey{}
		waits[settings.DiscoveryRetryFirstSeconds] = time.Second
		waits[settings.DiscoveryRetryMostSeconds] = time.Minute
		service := &Service{parts: Parts{
			Logger: slog.New(slog.DiscardHandler), Settings: waits,
			Random: bytes.NewReader(bytes.Repeat([]byte{random}, 8)),
		}}
		return service.retryWait(t.Context(), failures)
	}
	low, high := waitOf(0x00, 0), waitOf(0xff, 0)
	if low == high || low < 900*time.Millisecond || high > 1100*time.Millisecond {
		t.Errorf("two cores' first waits were %s and %s; want both within a tenth of a second, and apart",
			low, high)
	}
	if most := waitOf(0xff, 30); most != time.Minute {
		t.Errorf("after thirty failures the wait was %s; want the most, a minute", most)
	}
}

// The retry waits are settings, the first at least a second (the registry's bounds), and a most set
// below the first waits the first: they were the config file's, which refused such a pair, until the
// constants sweep (2026-10-20).
func TestRetryWait_AMostBelowTheFirstWaitsTheFirst(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		first, most, want time.Duration
	}{
		{time.Second, 300 * time.Second, 300 * time.Second},
		{time.Second, time.Second, time.Second},
		{2 * time.Second, time.Second, 2 * time.Second},
	} {
		waits := byKey{}
		waits[settings.DiscoveryRetryFirstSeconds] = tc.first
		waits[settings.DiscoveryRetryMostSeconds] = tc.most
		service := &Service{parts: Parts{Logger: slog.New(slog.DiscardHandler), Settings: waits, Random: unjittered{}}}
		if wait := service.retryWait(t.Context(), 30); wait != tc.want {
			t.Errorf("first %s, most %s: thirty failures in a row waited %s; want %s", tc.first, tc.most, wait, tc.want)
		}
	}
	if definition, _ := settings.Lookup(settings.DiscoveryRetryFirstSeconds); definition.Minimum != 1 {
		t.Errorf("discovery_retry_first_seconds may be %d; want a wait of at least a second", definition.Minimum)
	}
}

func TestSignal_ADroppedConnectionIsDialledAgainWithinAboutASecond(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	built.service.parts.Answerer = &answering{}
	if _, err := built.service.Connect(t.Context(), built.issuer); err != nil {
		t.Fatal(err)
	}
	built.fake.claim()
	built.service.step(t.Context())
	dials := make(chan time.Time, 8)
	built.fake.mutex.Lock()
	built.fake.dropsSignalling = true
	built.fake.onSignal = func() { dials <- built.clock.Now() }
	built.fake.mutex.Unlock()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		built.service.signal(ctx)
	}()
	first := testutil.Receive(t, dials, "the first dial")
	second := testutil.Receive(t, dials, "the dial after the connection dropped")
	third := testutil.Receive(t, dials, "the dial after it dropped again")
	cancel()
	testutil.Receive(t, done, "the signalling loop to end")
	// Each connection opened before it dropped, so each wait starts over from the first.
	if gap := second.Sub(first); gap != time.Second {
		t.Errorf("the connection was dialled again %s after it dropped; want about a second", gap)
	}
	if gap := third.Sub(second); gap != time.Second {
		t.Errorf("after it opened and dropped again, the wait was %s; want the first wait again", gap)
	}
}

// signalOverTime runs the signalling loop of a claimed core while its clock moves a second every few
// milliseconds of real time, and is each dial's time on that clock, until stop.
func signalOverTime(t *testing.T, built *world, answersPings bool) (dials <-chan time.Time, stop func()) {
	t.Helper()
	built.service.parts.Answerer = &answering{}
	if _, err := built.service.Connect(t.Context(), built.issuer); err != nil {
		t.Fatal(err)
	}
	built.fake.claim()
	built.service.step(t.Context())
	dialled := make(chan time.Time, 8)
	built.fake.mutex.Lock()
	built.fake.answersPings = answersPings
	built.fake.onSignal = func() { dialled <- built.clock.Now() }
	built.fake.mutex.Unlock()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		built.service.signal(ctx)
	}()
	go func() {
		// Each step gives the code a real moment to answer a ping and arm its next timer before the
		// fake time moves to that timer; a connection that answers its pings is given until its ping
		// is answered, however busy the machine (make check, 2026-10-16: two missed under load).
		tick := time.NewTicker(100 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				for answersPings && built.service.pingsUnanswered.Load() > 0 && ctx.Err() == nil {
					time.Sleep(time.Millisecond)
				}
				built.clock.AdvanceToNext()
			}
		}
	}()
	return dialled, func() {
		cancel()
		testutil.Receive(t, done, "the signalling loop to end")
	}
}

// A signalling connection that answers no ping is closed after two missed, each given pongWait, the
// second sent as the first is missed, and dialled again: a proxy that dropped it, or a discovery gone,
// is found within about a ping's interval and two pong waits (the protocol's §2.5; C11).
func TestSignal_AConnectionThatAnswersNoPingIsDialledAgain(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	dials, stop := signalOverTime(t, built, false)
	defer stop()
	first := testutil.Receive(t, dials, "the first dial")
	second := testutil.Receive(t, dials, "the dial after two pings went unanswered")
	soonest := pingEvery*91/100 + pingsMissedAt*pongWait
	latest := pingEvery*109/100 + pingsMissedAt*pongWait + 2*time.Second
	if gap := second.Sub(first); gap < soonest || gap > latest {
		t.Errorf("the connection was dialled again %s after it opened; want after two missed pings, %s to %s",
			gap, soonest, latest)
	}
}

// A discovery restarted behind its tunnel leaves the core's link open and answering nothing: the
// core finds it dead by its pings within about a minute, dials again, logs that it is open again and
// how long it was lost, and answers the offers the restarted discovery passes on (the owner,
// 2026-10-29: "It should be able to figure itself out either way").
func TestSignal_ADiscoveryRestartedBehindALinkLeftOpenIsFoundAgainAndAnswered(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	logged := &testutil.Log{}
	built.service.parts.Logger = slog.New(slog.NewTextHandler(logged, nil))
	built.service.parts.OfferKey = offerKeys
	built.fake.mutex.Lock()
	built.fake.silentDials, built.fake.passesAtOnce = 1, true
	built.fake.mutex.Unlock()
	dials, stop := signalOverTime(t, built, false)
	defer stop()
	first := testutil.Receive(t, dials, "the dial to the discovery before its restart")
	second := testutil.Receive(t, dials, "the dial to the restarted discovery")
	if gap := second.Sub(first); gap > pingEvery*109/100+pingsMissedAt*pongWait+2*time.Second {
		t.Errorf("the restarted discovery was dialled %s after the dead link opened; want within about a minute", gap)
	}
	built.fake.signals <- provenOffer("after-restart", "an offer\r\na=fingerprint:sha-256 AB:CD")
	if answered := testutil.Receive(t, built.fake.replies, "the core's answer"); answered.Type !=
		discoveryproto.SignalAnswer || answered.OfferID != "after-restart" {
		t.Errorf("the restarted discovery's offer was answered %+v; want its answer", answered)
	}
	if text := logged.String(); !strings.Contains(text, `msg="discovery: the signalling connection is open again"`) ||
		!strings.Contains(text, "lostMs=") || !strings.Contains(text, "dials=1") {
		t.Errorf("the log says\n%s\nwant the connection open again, how long it was lost and after how many dials",
			text)
	}
}

// A connection that answers its pings is kept: the pings keep it from looking idle to a proxy, which
// on staging closed it every two minutes (2026-10-14).
func TestSignal_AConnectionThatAnswersItsPingsIsKept(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	dials, stop := signalOverTime(t, built, true)
	first := testutil.Receive(t, dials, "the first dial")
	for built.clock.Now().Sub(first) < 5*time.Minute {
		select {
		case again := <-dials:
			stop()
			t.Fatalf("the connection was dialled again %s after it opened, though it answered its pings",
				again.Sub(first))
		case <-time.After(10 * time.Millisecond):
		}
	}
	stop()
}

func TestConnect_TwoAtOnceEnrolOnce(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	var connecting sync.WaitGroup
	errs := make([]error, 2)
	for index := range errs {
		connecting.Go(func() { _, errs[index] = built.service.Connect(t.Context(), built.issuer) })
	}
	connecting.Wait()
	built.fake.mutex.Lock()
	enrolled := built.fake.enrolCount
	built.fake.mutex.Unlock()
	refused := 0
	for _, err := range errs {
		if errors.Is(err, ErrAlreadyEnrolled) {
			refused++
		}
	}
	if enrolled != 1 || refused != 1 {
		t.Errorf("two Connects at once enrolled %d times, %d refused (%v); want one enrolment, the other refused",
			enrolled, refused, errs)
	}
}

func TestSession_LastsAsLongAsTheDiscoverySaysCountedOnTheCoresOwnClock(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	ctx := t.Context()
	built.fake.mutex.Lock()
	built.fake.sessionLasts = int64((sessionMargin + 2*heartbeatEvery - time.Minute) / time.Second)
	built.fake.mutex.Unlock()
	if _, err := built.service.Connect(ctx, built.issuer); err != nil {
		t.Fatal(err)
	}
	built.service.step(ctx)
	sessions := func() int {
		built.fake.mutex.Lock()
		defer built.fake.mutex.Unlock()
		return built.fake.sessionCount
	}
	opened := sessions()
	built.clock.Advance(heartbeatEvery)
	built.service.step(ctx) // a heartbeat, well within the session's life: the same session
	if count := sessions(); count != opened {
		t.Errorf("%d sessions after a heartbeat within the session's life; want still %d", count, opened)
	}
	built.clock.Advance(heartbeatEvery)
	built.service.step(ctx) // within sessionMargin of its end: renewed first
	if count := sessions(); count != opened+1 {
		t.Errorf("%d sessions after a heartbeat near the session's end; want one more than %d", count, opened)
	}
}

// A core newly claimed dials its signalling connection at once, not at the loop's next look: its
// owner's first visit through the discovery waited up to signalCheckEvery for it (the owner, 2026-10-18).
func TestSignal_ACoreNewlyClaimedDialsAtOnce(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	built.service.parts.Answerer = &answering{}
	if _, err := built.service.Connect(t.Context(), built.issuer); err != nil {
		t.Fatal(err)
	}
	dials := make(chan time.Time, 8)
	built.fake.mutex.Lock()
	built.fake.onSignal = func() { dials <- built.clock.Now() }
	built.fake.mutex.Unlock()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		built.service.signal(ctx)
	}()
	claimedAt := built.clock.Now()
	built.fake.claim()
	built.service.step(t.Context())
	dialled := testutil.Receive(t, dials, "the dial once claimed")
	cancel()
	testutil.Receive(t, done, "the signalling loop to end")
	if gap := dialled.Sub(claimedAt); gap != 0 {
		t.Errorf("the core dialled %s after it was claimed; want at once", gap)
	}
}
