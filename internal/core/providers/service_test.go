// SPDX-License-Identifier: GPL-3.0-or-later

package providers_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/providers"
	"github.com/kstenerud/orpheum/internal/core/providers/providerstest"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue/jobqueuetest"
	"github.com/kstenerud/orpheum/internal/platform/outbound"
)

// host is the tests' outside host.
const host = jobs.DestinationTMDBAPI

// errNoKey and errKeyRefused are the tests' host's credentials failing.
var (
	errNoKey      = errors.New("no key")
	errKeyRefused = errors.New("key refused")
)

func testConfig(limits providers.Limits) providers.Config {
	return providers.Config{
		Name: "TMDB", Destination: host, Limits: limits, CredentialsName: "key",
		CredentialsFailure: func(err error) providers.CredentialsState {
			switch {
			case errors.Is(err, errNoKey):
				return providers.CredentialsMissing
			case errors.Is(err, errKeyRefused):
				return providers.CredentialsRefused
			}
			return ""
		},
	}
}

type fixture struct {
	t       *testing.T
	clock   *clocktest.Fake
	health  *providerstest.Health
	service *providers.Service
}

func newFixture(t *testing.T, limits providers.Limits) fixture {
	t.Helper()
	fakeClock := clocktest.NewFake(time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC))
	health := providerstest.NewHealth(t, fakeClock)
	service := providers.New(testConfig(limits), health, fakeClock)
	return fixture{t: t, clock: fakeClock, health: health, service: service}
}

// ask is the service's answer to a request of importance and cost.
func (fixed fixture) ask(importance providers.Importance, cost providers.Cost) providers.Answer {
	return fixed.service.Ask(providers.WithImportance(fixed.t.Context(), importance), cost)
}

// call makes a person's call to the host, which answers answer.
func (fixed fixture) call(ctx context.Context, answer error) error {
	//nolint:wrapcheck // the call's answer, as the test gave it
	return fixed.service.Call(ctx, providers.CostRequest, func(context.Context) error { return answer })
}

func TestAsk_BackgroundKeepsBelowItsShareOfTheRateAndAPersonFindsRoom(t *testing.T) {
	t.Parallel()
	fixed := newFixture(t, providers.Limits{RatePerSecond: 10, BackgroundShare: 0.8})
	for index := range 8 {
		if answer := fixed.ask(providers.Background, providers.CostRequest); answer.Verdict != providers.VerdictNow {
			t.Fatalf("background request %d: %+v, want now: 8 a second are its share of 10", index+1, answer)
		}
	}
	answer := fixed.ask(providers.Background, providers.CostRequest)
	if answer.Verdict != providers.VerdictAt || answer.At.Sub(fixed.clock.Now()) != 125*time.Millisecond {
		t.Errorf("the ninth background request: %+v; want it at its share's next turn, 125 ms on", answer)
	}
	for _, importance := range []providers.Importance{providers.PersonsRequest, providers.SomeoneWaiting} {
		if answer := fixed.ask(importance, providers.CostRequest); answer.Verdict != providers.VerdictNow {
			t.Errorf("%s once background used its share: %+v, want now (D231 §5)", importance, answer)
		}
	}
	if answer := fixed.ask(providers.SomeoneWaiting, providers.CostRequest); answer.Verdict != providers.VerdictAt {
		t.Errorf("an eleventh request in the second: %+v, want it held to the whole rate", answer)
	}
	fixed.clock.Advance(time.Second)
	if answer := fixed.ask(providers.Background, providers.CostRequest); answer.Verdict != providers.VerdictNow {
		t.Errorf("a second later: %+v, want the rate to have filled again", answer)
	}
}

func TestAsk_BackgroundNeverSpendsTheLastDownloadsKeptForPeople(t *testing.T) {
	t.Parallel()
	fixed := newFixture(t, providers.Limits{RatePerSecond: 100, ReservedForPeople: 5})
	if answer := fixed.ask(providers.Background, providers.CostCounted); answer.Verdict != providers.VerdictNow {
		t.Errorf("before the host said its quota: %+v, want now", answer)
	}
	resetAt := fixed.clock.Now().Add(6 * time.Hour)
	fixed.service.RecordQuota(6, resetAt)
	if answer := fixed.ask(providers.Background, providers.CostCounted); answer.Verdict != providers.VerdictNow {
		t.Errorf("background with 6 left: %+v, want now", answer)
	}
	answer := fixed.ask(providers.Background, providers.CostCounted)
	if answer.Verdict != providers.VerdictQuotaSpent || !answer.At.Equal(resetAt) {
		t.Errorf("background with 5 left: %+v; want refused until the reset, the last 5 kept for people", answer)
	}
	if request := fixed.ask(providers.Background, providers.CostRequest); request.Verdict != providers.VerdictNow {
		t.Errorf("a background request that spends no download: %+v, want now", request)
	}
	for left := 5; left > 0; left-- {
		download := fixed.ask(providers.SomeoneWaiting, providers.CostCounted)
		if download.Verdict != providers.VerdictNow {
			t.Fatalf("someone waiting with %d left: %+v, want now", left, download)
		}
	}
	answer = fixed.ask(providers.SomeoneWaiting, providers.CostCounted)
	if answer.Verdict != providers.VerdictQuotaSpent {
		t.Errorf("someone waiting with none left: %+v, want the quota spent", answer)
	}
	fixed.clock.Set(resetAt)
	if answer := fixed.ask(providers.Background, providers.CostCounted); answer.Verdict != providers.VerdictNow {
		t.Errorf("after the reset: %+v, want now: the count started again", answer)
	}
	if state := fixed.service.State(); state.Quota != nil {
		t.Errorf("after the reset the state still shows a quota %+v; the next reply says where it stands", state.Quota)
	}
}

func TestCall_APersonsCallToAParkedHostAlwaysTriesAndIsItsProbe(t *testing.T) {
	t.Parallel()
	fixed := newFixture(t, providers.Limits{RatePerSecond: 100})
	ctx := t.Context()
	unreachable := fmt.Errorf("%w: connection refused", outbound.ErrUnavailable)
	if err := fixed.call(ctx, unreachable); !errors.Is(err, outbound.ErrUnavailable) {
		t.Fatalf("a person's call to a host that is down: %v, want its failure", err)
	}
	state, isParked := fixed.health.DestinationState(host)
	if !isParked || state.Reason != "unreachable" ||
		state.NextProbeAt.Sub(fixed.clock.Now()) != jobqueue.FirstProbeDelay {
		t.Fatalf("after a person's call found the host down: %+v (%v); want it parked, probed in 30 s", state, isParked)
	}
	answer := fixed.ask(providers.Background, providers.CostRequest)
	if answer.Verdict != providers.VerdictParked || answer.Why != "unreachable" || !answer.Since.Equal(state.Since) {
		t.Errorf("background asking of the parked host: %+v, want parked, with why and since when", answer)
	}
	called := false
	if err := fixed.service.Call(ctx, providers.CostRequest, func(context.Context) error {
		called = true
		return unreachable
	}); err == nil || !called {
		t.Fatalf("a person's call to the parked host: called %v, %v; want it tried (D231 §6)", called, err)
	}
	if state, _ = fixed.health.DestinationState(host); state.NextProbeAt.Sub(fixed.clock.Now()) != time.Minute {
		t.Errorf("after the person's call failed, the next probe is in %v; want it counted as a failed probe, a minute",
			state.NextProbeAt.Sub(fixed.clock.Now()))
	}
	if err := fixed.call(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if _, isParked := fixed.health.DestinationState(host); isParked {
		t.Error("the host is still parked after a person's call reached it")
	}
}

func TestCall_AHostThatAsksUsToWaitIsWaitedForByEveryone(t *testing.T) {
	t.Parallel()
	fixed := newFixture(t, providers.Limits{RatePerSecond: 100})
	ctx := t.Context()
	limited := &outbound.RateLimitedError{Provider: "TMDB", RetryAfter: time.Minute}
	if err := fixed.call(ctx, limited); !errors.Is(err, limited) {
		t.Fatalf("a 429: %v", err)
	}
	called := false
	err := fixed.service.Call(ctx, providers.CostRequest, func(context.Context) error {
		called = true
		return nil
	})
	rateLimited, isLimited := errors.AsType[*outbound.RateLimitedError](err)
	if called || !isLimited || rateLimited.RetryAfter != time.Minute {
		t.Errorf("someone waiting, within the host's wait: called %v, %v; want no call, a rate limit of a minute",
			called, err)
	}
	fixed.clock.Advance(time.Minute)
	if err := fixed.call(ctx, nil); err != nil {
		t.Errorf("after the host's wait: %v, want the call made", err)
	}
}

func TestCall_AWaitUnderOurOwnRateIsWaitedInPlace(t *testing.T) {
	t.Parallel()
	fixed := newFixture(t, providers.Limits{RatePerSecond: 1})
	ctx := t.Context()
	start := fixed.clock.Now()
	for range 2 {
		if err := fixed.call(ctx, nil); err != nil {
			t.Fatal(err)
		}
	}
	if waited := fixed.clock.Now().Sub(start); waited != time.Second {
		t.Errorf("two calls at one a second took %v, want the second to wait its turn, a second", waited)
	}
}

func TestCall_InAJobAFailureThatSaysTheHostIsDownParksTheJobAndTheQueueRecordsIt(t *testing.T) {
	t.Parallel()
	fakeClock := clocktest.NewFake(time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC))
	storage := &jobqueuetest.Memory[int64]{}
	queue := jobqueue.New(storage, fakeClock, jobqueue.Config{
		Random: jobqueuetest.Unjittered(), Logger: slog.New(slog.DiscardHandler), Policy: noRetries{},
	})
	service := providers.New(testConfig(providers.Limits{RatePerSecond: 100}), queue, fakeClock)
	for _, check := range []struct {
		failure error
		want    jobs.Parking
	}{
		{
			fmt.Errorf("%w: status 503", outbound.ErrUnavailable),
			jobs.Parking{Destination: host, Reason: "unreachable"},
		},
		{errNoKey, jobs.Parking{Destination: host, Reason: "its key is not set", AwaitsSignal: true}},
		{errKeyRefused, jobs.Parking{Destination: host, Reason: "it refused its key", AwaitsSignal: true}},
		{
			&outbound.RateLimitedError{Provider: "TMDB", RetryAfter: time.Minute},
			jobs.Parking{Destination: host, Reason: "asked us to wait", ProbeAt: time.Time{}.Add(time.Minute)},
		},
	} {
		var answer error
		runner := queue.NewRunner("fetch", func(ctx context.Context, _ jobqueue.Job[int64]) error {
			answer = service.Call(ctx, providers.CostRequest, func(context.Context) error { return check.failure })
			return fmt.Errorf("fetch: %w", answer)
		})
		if _, err := queue.Enqueue(t.Context(), jobqueue.Request[int64]{Kind: "fetch", Target: 1}); err != nil {
			t.Fatal(err)
		}
		if err := runner.RunDue(t.Context()); err != nil {
			t.Fatal(err)
		}
		want := check.want
		if !want.ProbeAt.IsZero() { // a wait from the time of the call
			want.ProbeAt = fakeClock.Now().Add(want.ProbeAt.Sub(time.Time{}))
		}
		if parking, isParked := jobqueue.ParkingOf(answer); !isParked || parking != want {
			t.Errorf("%v in a job: parks %+v (%v), want %+v", check.failure, parking, isParked, want)
		}
		if _, isParked := queue.DestinationState(host); !isParked {
			t.Errorf("%v in a job: the queue did not park the host", check.failure)
		}
		queue.Unpark(t.Context(), host)
		fakeClock.Advance(time.Hour)
	}
	if rows := storage.Rows(); rows[0].AttemptCount != 0 || rows[0].Destination != host {
		t.Errorf("the job %+v; want it kept with the host, no attempt spent", rows[0])
	}
}

// noRetries is a retry policy the parking tests never reach.
type noRetries struct{}

func (noRetries) AttemptLimit(context.Context) (int64, error)            { return 1, nil }
func (noRetries) FirstRetryDelay(context.Context) (time.Duration, error) { return time.Hour, nil }

func TestCredentialsChanged_BringsBackAHostParkedForItsKeyAndNoOtherPark(t *testing.T) {
	t.Parallel()
	fixed := newFixture(t, providers.Limits{RatePerSecond: 100})
	ctx := t.Context()
	if err := fixed.call(ctx, errKeyRefused); !errors.Is(err, errKeyRefused) {
		t.Fatalf("a refused key: %v", err)
	}
	state, isParked := fixed.health.DestinationState(host)
	if !isParked || !state.AwaitsSignal || fixed.service.State().Credentials != providers.CredentialsRefused {
		t.Fatalf("a refused key: %+v (%v), credentials %s; want the host parked until the key changes", state,
			isParked, fixed.service.State().Credentials)
	}
	fixed.service.CredentialsChanged(ctx)
	if _, isParked := fixed.health.DestinationState(host); isParked ||
		fixed.service.State().Credentials != providers.CredentialsUnknown {
		t.Errorf("after the key changed: still parked %v, credentials %s; want it back, the key's state unknown",
			isParked, fixed.service.State().Credentials)
	}
	if err := fixed.call(ctx, fmt.Errorf("%w: timeout", outbound.ErrUnavailable)); err == nil {
		t.Fatal("a timeout was not a failure")
	}
	fixed.service.CredentialsChanged(ctx)
	if _, isParked := fixed.health.DestinationState(host); !isParked {
		t.Error("a change of key brought back a host parked as unreachable; only its probe does")
	}
}

func TestJobImportance_IsSomeoneWaitingToWatchAPersonsRequestOrTheBackground(t *testing.T) {
	t.Parallel()
	for priority, want := range map[int64]providers.Importance{
		jobs.PriorityPlayNow: providers.SomeoneWaiting, jobs.PriorityPersonsRequest: providers.PersonsRequest,
		0: providers.Background, -10: providers.Background,
	} {
		if got := providers.JobImportance(priority); got != want {
			t.Errorf("a job of priority %d is %s, want %s", priority, got, want)
		}
	}
	if got := providers.ImportanceOf(t.Context()); got != providers.SomeoneWaiting {
		t.Errorf("a call outside a job is %s, want someone waiting: a person's call is answered while they wait", got)
	}
}
