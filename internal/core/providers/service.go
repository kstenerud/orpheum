// SPDX-License-Identifier: AGPL-3.0-or-later

package providers

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
	"github.com/kstenerud/orpheum/internal/platform/outbound"
)

// Health is where a host's health is kept: the job queue's destinations (jobs.Queue), so a host
// found down by a person's call parks the queue's work for it, and one found down by a job parks
// the person's calls' view of it (D231 §5, §6).
type Health interface {
	DestinationState(destination jobs.Destination) (jobs.DestinationState, bool)
	Park(ctx context.Context, parking jobs.Parking)
	Unpark(ctx context.Context, destination jobs.Destination)
}

// Limits are what a provider keeps its callers to: an admin's settings, each host's own.
type Limits struct {
	// RatePerSecond is the request rate we keep to, whoever asks; at least one.
	RatePerSecond float64
	// BackgroundShare is the part of that rate background work may use, 0 to 1 (D231 §5), so a
	// person's request always finds room; 0 is all of it.
	BackgroundShare float64
	// ReservedForPeoplePercent is the percent of the day's counted requests (OpenSubtitles'
	// downloads) held back for people: background work never spends the last of them, that
	// percent of the host's allowance rounded up to whole requests; 0 for a host that counts none.
	ReservedForPeoplePercent int
}

// LimitsOf reads a host's limits when a request asks: from the settings, so a change takes effect
// at the next request, or fixed (FixedLimits).
type LimitsOf func(ctx context.Context) (Limits, error)

// Settings are the settings a host's limits are read from: settings.Service.
type Settings interface {
	Int(ctx context.Context, key settings.Key) (int64, error)
}

// SettingsLimits reads a host's limits from the settings: its rate, the percent of it background
// work may use, and the percent of its counted requests held back for people. A key left "" is not
// read, and the limit it would give is none: all of the rate, nothing held back.
func SettingsLimits(
	settingsReader Settings, rate, backgroundSharePercent, reservedForPeoplePercent settings.Key,
) LimitsOf {
	return func(ctx context.Context) (Limits, error) {
		read := func(key settings.Key) (int64, error) {
			if key == "" {
				return 0, nil
			}
			value, err := settingsReader.Int(ctx, key)
			if err != nil {
				return 0, fmt.Errorf("read %s: %w", key, err)
			}
			return value, nil
		}
		perSecond, rateErr := read(rate)
		percent, shareErr := read(backgroundSharePercent)
		reserved, reservedErr := read(reservedForPeoplePercent)
		if err := errors.Join(rateErr, shareErr, reservedErr); err != nil {
			return Limits{}, err
		}
		return Limits{
			RatePerSecond: float64(perSecond), BackgroundShare: float64(percent) / 100,
			ReservedForPeoplePercent: int(reserved),
		}, nil
	}
}

// FixedLimits are limits that do not change.
func FixedLimits(limits Limits) LimitsOf {
	return func(context.Context) (Limits, error) { return limits, nil }
}

// Config is one outside host's provider service.
type Config struct {
	// Name is the host as a person knows it: "TMDB", "TMDB's image server", "OpenSubtitles".
	Name        string
	Destination jobs.Destination
	Limits      LimitsOf
	// CredentialsName is what the host's credentials are called, as a person reads it: "key",
	// "login".
	CredentialsName string
	// CredentialsFailure says whether an error is the host's credentials missing or refused
	// (CredentialsMissing, CredentialsRefused), or neither (""); nil for a host that needs none.
	CredentialsFailure func(err error) CredentialsState
}

// Cost is what a request spends of a host's allowance.
type Cost int

// The costs.
const (
	// CostRequest is one request against the rate.
	CostRequest Cost = iota
	// CostCounted is one request against the rate and one of the day's counted ones:
	// OpenSubtitles' download.
	CostCounted
)

// Verdict is a provider's answer to a caller who asks to call its host.
type Verdict string

// The verdicts.
const (
	// VerdictNow is: call it now.
	VerdictNow Verdict = "now"
	// VerdictAt is: not before a time, to keep to our rate or because the host asked us to wait.
	VerdictAt Verdict = "at"
	// VerdictParked is: the host is down, and its probe is not yours.
	VerdictParked Verdict = "parked"
	// VerdictQuotaSpent is: the day's counted requests are spent, or those left are kept for
	// people, until a time.
	VerdictQuotaSpent Verdict = "quota_spent"
)

// Answer is a provider's answer to a caller.
type Answer struct {
	Verdict Verdict
	// At is when to call: VerdictAt's time, or when VerdictQuotaSpent's count starts again (zero
	// when the host did not say).
	At time.Time
	// Why says why not now, in a person's words.
	Why string
	// Since is when a parked host went down.
	Since time.Time
}

// CredentialsState is what a provider knows of its host's credentials.
type CredentialsState string

// The credentials' states.
const (
	CredentialsUnknown  CredentialsState = "unknown"
	CredentialsAccepted CredentialsState = "accepted"
	CredentialsMissing  CredentialsState = "missing"
	CredentialsRefused  CredentialsState = "refused"
)

// quotaUnknownResetWait is how long a job waits for a spent quota whose host did not say when its
// count starts again. It is fixed, not a setting: a fallback only, since OpenSubtitles names its
// reset in every reply that spends a download.
const quotaUnknownResetWait = time.Hour

// maxPaceWait is the longest a call waits in place for its turn under our own rate. It is fixed,
// not a setting: an internal bound that tells our own pacing from a host's wait, since a turn is a
// fraction of a second away at any rate the settings allow (at least one a second), and a longer
// wait is only ever the host's Retry-After.
const maxPaceWait = 2 * time.Second

// Service is one outside host's provider (D231 §5, the owner 2026-09-30): it owns the host's
// health (its destination in the job queue), the request rate we keep to, the quota remaining and
// when it resets, and its credentials' state. A caller says how much its request matters and asks;
// the service answers now, at a time, parked, or quota spent.
type Service struct {
	config Config
	health Health
	clock  clock.Clock

	mutex sync.Mutex
	// limits are the host's limits as last read; everyone and background are the rate kept for
	// every caller and for background work's share of it. All guarded by mutex.
	limits      Limits
	everyone    *bucket
	background  *bucket
	waitUntil   time.Time
	quota       quota
	allowed     allowance
	credentials CredentialsState
}

// quota is what a host said of its counted requests: how many are left, and when the count
// starts again. It is known only from the host's replies, and forgotten once its reset passes.
type quota struct {
	isKnown   bool
	remaining int
	resetAt   time.Time
}

// allowance is how many counted requests the host allows a day, as it last said; 0 before it
// said. It outlives a reset: an account's level changes rarely.
type allowance int

// reservedFor is how many of the allowance are held back for people at percent, rounded up to
// whole requests.
func (allowed allowance) reservedFor(percent int) int {
	return (int(allowed)*percent + 99) / 100
}

// New returns a host's provider service, its health kept by health. Its limits are read at each
// request; until one is read, it keeps to a request a second.
func New(config Config, health Health, clockSource clock.Clock) *Service {
	return &Service{
		config: config, health: health, clock: clockSource, limits: Limits{RatePerSecond: 1},
		credentials: CredentialsUnknown,
	}
}

// readLimits reads the host's limits and has the rate follow them; limits that cannot be read are
// the last that could. The caller holds the mutex.
func (service *Service) readLimits(ctx context.Context, now time.Time) {
	if limits, err := service.config.Limits(ctx); err == nil {
		limits.RatePerSecond = max(limits.RatePerSecond, 1)
		service.limits = limits
	}
	share := service.limits.BackgroundShare
	if share <= 0 || share > 1 {
		share = 1
	}
	rate := service.limits.RatePerSecond
	if service.everyone == nil { // the first read: the buckets start full
		service.everyone, service.background = newBucket(rate, now), newBucket(rate*share, now)
		return
	}
	service.everyone.setRate(rate, now)
	service.background.setRate(rate*share, now)
}

// Destination is the host's destination in the job queue.
func (service *Service) Destination() jobs.Destination { return service.config.Destination }

// Ask is the service's answer to a caller who would call the host now, with the importance ctx
// carries (ImportanceOf). Someone waiting is always let try a parked host, and the try is its probe
// (D231 §6); so is a job the queue made the host's probe. A host that asked us to wait is waited
// for by everyone. A "now" takes the call's turn from the rate: the caller calls.
func (service *Service) Ask(ctx context.Context, cost Cost) Answer {
	importance := ImportanceOf(ctx)
	now := service.clock.Now()
	service.mutex.Lock()
	defer service.mutex.Unlock()
	service.readLimits(ctx, now)
	if now.Before(service.waitUntil) {
		return Answer{Verdict: VerdictAt, At: service.waitUntil, Why: service.config.Name + " asked us to wait"}
	}
	if state, isParked := service.health.DestinationState(service.config.Destination); isParked &&
		importance < SomeoneWaiting && !service.isProbe(ctx) {
		return Answer{Verdict: VerdictParked, Why: state.Reason, Since: state.Since, At: state.NextProbeAt}
	}
	if cost == CostCounted {
		if refusal, isRefused := service.quotaRefusal(importance, now); isRefused {
			return refusal
		}
	}
	readyAt := service.everyone.readyAt(now)
	if importance == Background {
		readyAt = latest(readyAt, service.background.readyAt(now))
	}
	if readyAt.After(now) {
		return Answer{Verdict: VerdictAt, At: readyAt, Why: "keeping to " + service.config.Name + "'s rate"}
	}
	service.everyone.take()
	if importance == Background {
		service.background.take()
	}
	if cost == CostCounted && service.quota.isKnown {
		service.quota.remaining--
	}
	return Answer{Verdict: VerdictNow}
}

// isProbe is whether ctx is the job the queue made the host's probe.
func (service *Service) isProbe(ctx context.Context) bool {
	probed, isProbe := jobqueue.ProbeOf(ctx)
	return isProbe && probed == service.config.Destination
}

// quotaRefusal is a counted request refused: the day's are spent, or those left are kept for
// people and the request is background work. The caller holds the mutex.
func (service *Service) quotaRefusal(importance Importance, now time.Time) (Answer, bool) {
	held := &service.quota
	if held.isKnown && !held.resetAt.IsZero() && !now.Before(held.resetAt) {
		*held = quota{} // the count started again; the next reply says where it stands
	}
	switch {
	case !held.isKnown:
		return Answer{}, false
	case held.remaining <= 0:
		return Answer{Verdict: VerdictQuotaSpent, At: held.resetAt, Why: "the day's downloads are spent"}, true
	case importance == Background &&
		held.remaining <= service.allowed.reservedFor(service.limits.ReservedForPeoplePercent):
		return Answer{
			Verdict: VerdictQuotaSpent, At: held.resetAt, Why: "the day's last downloads are held back for people",
		}, true
	}
	return Answer{}, false
}

func latest(one, other time.Time) time.Time {
	if other.After(one) {
		return other
	}
	return one
}

// Call asks, waits in place for a turn the rate makes it wait a moment for, calls, and records
// what the host's answer says of it. A call refused is a *NotNowError. Within a job, a refusal or
// a failure that says the host is down, asked us to wait, or refused its credentials is answered
// as the job's parking on the host (D231 §5), which the queue records; outside one, the call is a
// person's, and the service records it itself: a success unparks the host, and a failure of a
// parked one is its failed probe.
func (service *Service) Call(ctx context.Context, cost Cost, call func(ctx context.Context) error) error {
	for {
		answer := service.Ask(ctx, cost)
		if answer.Verdict == VerdictNow {
			break
		}
		if wait := answer.At.Sub(service.clock.Now()); answer.Verdict == VerdictAt && wait <= maxPaceWait {
			if err := service.clock.Sleep(ctx, wait); err != nil {
				return fmt.Errorf("wait for %s: %w", service.config.Name, err)
			}
			continue
		}
		return service.inJob(ctx, &NotNowError{
			Provider: service.config.Name, Answer: answer, wait: max(answer.At.Sub(service.clock.Now()), 0),
		})
	}
	err := call(ctx)
	service.record(ctx, err)
	return service.inJob(ctx, err)
}

// record notes what a call's outcome says of the host: its rate limit, its quota, its
// credentials, and, outside a job, its health.
func (service *Service) record(ctx context.Context, err error) {
	if ctx.Err() != nil {
		return // the caller went away: the call says nothing of the host
	}
	now := service.clock.Now()
	parking, isDown := service.parkingFor(err, now)
	service.mutex.Lock()
	limited, isLimited := errors.AsType[*outbound.RateLimitedError](err)
	if isLimited {
		service.waitUntil = latest(service.waitUntil, now.Add(limited.RetryAfter))
	}
	if spent, isSpent := errors.AsType[*QuotaSpentError](err); isSpent {
		service.quota = quota{isKnown: true, remaining: 0, resetAt: spent.ResetAt}
	}
	switch credentials := service.credentialsFailure(err); {
	case credentials != "":
		service.credentials = credentials
	case !isDown:
		service.credentials = CredentialsAccepted // the host answered what it was asked
	}
	service.mutex.Unlock()
	if jobqueue.IsInJob(ctx) {
		return // the queue records the job's answer
	}
	if isDown {
		_, parking.IsProbeFailure = service.health.DestinationState(service.config.Destination)
		service.health.Park(ctx, parking)
		return
	}
	service.health.Unpark(ctx, service.config.Destination)
}

// credentialsFailure is whether err is the host's credentials missing or refused, or "".
func (service *Service) credentialsFailure(err error) CredentialsState {
	if service.config.CredentialsFailure == nil || err == nil {
		return ""
	}
	return service.config.CredentialsFailure(err)
}

// parkingFor is how a call's failure says the host is down (D231 §5), if it does: unreachable or
// failing (a connection, DNS, a timeout, 502, 503, 504), asking us to wait, or refusing its
// credentials, which only a change of them brings back. Any other failure is the request's own.
func (service *Service) parkingFor(err error, now time.Time) (jobs.Parking, bool) {
	parking := jobs.Parking{Destination: service.config.Destination}
	limited, isLimited := errors.AsType[*outbound.RateLimitedError](err)
	switch credentials := service.credentialsFailure(err); {
	case err == nil:
		return jobs.Parking{}, false
	case credentials == CredentialsMissing:
		parking.Reason, parking.AwaitsSignal = "its "+service.config.CredentialsName+" is not set", true
	case credentials == CredentialsRefused:
		parking.Reason, parking.AwaitsSignal = "it refused its "+service.config.CredentialsName, true
	case isLimited:
		parking.Reason, parking.ProbeAt = "asked us to wait", now.Add(limited.RetryAfter)
	case errors.Is(err, outbound.ErrUnavailable):
		parking.Reason = "unreachable"
	default:
		return jobs.Parking{}, false
	}
	return parking, true
}

// inJob is err as a job answers it: a refusal or a failure that says the host is down parks the
// job on the host (D231 §5), and a quota spent waits for its reset; outside a job, err as it is.
func (service *Service) inJob(ctx context.Context, err error) error {
	if err == nil || !jobqueue.IsInJob(ctx) {
		return err
	}
	now := service.clock.Now()
	if refused, isRefused := errors.AsType[*NotNowError](err); isRefused {
		switch refused.Answer.Verdict {
		case VerdictQuotaSpent:
			until := refused.Answer.At
			if until.IsZero() {
				until = now.Add(quotaUnknownResetWait) // the host did not say when its count starts again
			}
			return fmt.Errorf("%s: %w", service.config.Name, jobqueue.Snooze(err, until))
		case VerdictAt:
			return fmt.Errorf("%s: %w", service.config.Name, jobs.Park(jobs.Parking{
				Destination: service.config.Destination, Reason: refused.Answer.Why, ProbeAt: refused.Answer.At,
			}, err))
		case VerdictParked, VerdictNow:
			return fmt.Errorf("%s: %w", service.config.Name, jobs.Park(jobs.Parking{
				Destination: service.config.Destination, Reason: refused.Answer.Why,
			}, err))
		}
	}
	if parking, isDown := service.parkingFor(err, now); isDown {
		return fmt.Errorf("%s: %w", service.config.Name, jobs.Park(parking, err))
	}
	return err
}

// CredentialsChanged is the host's credentials changed by a person: what was known of them is
// forgotten, and a host parked for them, the one thing that parks an outside host until a signal,
// is brought back, so the work waiting on it tries them (D231 §5).
func (service *Service) CredentialsChanged(ctx context.Context) {
	service.mutex.Lock()
	service.credentials = CredentialsUnknown
	service.mutex.Unlock()
	if state, isParked := service.health.DestinationState(service.config.Destination); isParked && state.AwaitsSignal {
		service.health.Unpark(ctx, service.config.Destination)
	}
}

// RecordAllowance notes how many counted requests the host said it allows a day: OpenSubtitles'
// allowed_downloads, which its login and its account's details say.
func (service *Service) RecordAllowance(allowed int) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	service.allowed = allowance(max(allowed, 0))
}

// RecordQuota notes what the host said of its counted requests: how many are left, and when the
// count starts again.
func (service *Service) RecordQuota(remaining int, resetAt time.Time) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	if resetAt.IsZero() {
		resetAt = service.quota.resetAt // a reply that names no reset keeps the one last named
	}
	service.quota = quota{isKnown: true, remaining: remaining, resetAt: resetAt}
}

// NotNowError is a call its provider refused before it was made: the host is parked, asked us to
// wait, or its day's counted requests are spent, as Answer says.
type NotNowError struct {
	Provider string
	Answer   Answer
	// wait is how long from the refusal until Answer.At.
	wait time.Duration
}

func (refused *NotNowError) Error() string {
	text := refused.Provider + ": not now: " + refused.Answer.Why
	if !refused.Answer.At.IsZero() {
		text += " until " + refused.Answer.At.UTC().Format(time.RFC3339)
	}
	return text
}

// Unwrap is the refusal in the terms every provider's caller already reads: a host that asked us
// to wait is its rate limit, a spent quota its QuotaSpentError, and a parked host unavailable.
func (refused *NotNowError) Unwrap() error {
	switch refused.Answer.Verdict {
	case VerdictAt:
		return &outbound.RateLimitedError{Provider: refused.Provider, RetryAfter: refused.wait}
	case VerdictQuotaSpent:
		return &QuotaSpentError{ResetAt: refused.Answer.At}
	case VerdictParked, VerdictNow:
	}
	return outbound.ErrUnavailable
}

// QuotaSpentError is a host's day of counted requests spent: OpenSubtitles' downloads.
type QuotaSpentError struct {
	// ResetAt is when the host says the count starts again; zero when it did not say.
	ResetAt time.Time
}

func (spent *QuotaSpentError) Error() string {
	return "the day's downloads are spent until " + spent.ResetAt.UTC().Format(time.RFC3339)
}

// State is what a provider knows of its host, for the System page (D231 §12).
type State struct {
	Name        string
	Destination jobs.Destination
	// RatePerSecond is the rate we keep to, and BackgroundRatePerSecond background work's share.
	RatePerSecond           float64
	BackgroundRatePerSecond float64
	// WaitUntil is when the host said it may be called again after a 429; zero when it did not.
	WaitUntil   time.Time
	Credentials CredentialsState
	// Quota is what the host last said of its counted requests; nil when it has said nothing yet
	// or counts none.
	Quota *QuotaState
	// Parked is the host's state while it is down; nil while it is up.
	Parked *jobs.DestinationState
}

// QuotaState is a host's counted requests: how many are left, when the count starts again, how
// many it allows a day (0 before it said), and how many of them background work never spends, the
// percent held back for people rounded up.
type QuotaState struct {
	Remaining                int
	ResetAt                  time.Time
	Allowed                  int
	ReservedForPeoplePercent int
	ReservedForPeople        int
}

// State is what the service knows of its host now, its limits as the settings say them.
func (service *Service) State(ctx context.Context) State {
	now := service.clock.Now()
	service.mutex.Lock()
	service.readLimits(ctx, now)
	state := State{
		Name: service.config.Name, Destination: service.config.Destination,
		RatePerSecond: service.everyone.ratePerSecond, BackgroundRatePerSecond: service.background.ratePerSecond,
		Credentials: service.credentials,
	}
	if now.Before(service.waitUntil) {
		state.WaitUntil = service.waitUntil
	}
	if held := service.quota; held.isKnown && (held.resetAt.IsZero() || now.Before(held.resetAt)) {
		state.Quota = &QuotaState{
			Remaining: held.remaining, ResetAt: held.resetAt, Allowed: int(service.allowed),
			ReservedForPeoplePercent: service.limits.ReservedForPeoplePercent,
			ReservedForPeople:        service.allowed.reservedFor(service.limits.ReservedForPeoplePercent),
		}
	}
	service.mutex.Unlock()
	if parked, isParked := service.health.DestinationState(service.config.Destination); isParked {
		state.Parked = &parked
	}
	return state
}
