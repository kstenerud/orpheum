// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/kstenerud/orpheum/discovery/release"
	"github.com/kstenerud/orpheum/discovery/trust"
)

// releaseCheckEvery is how often the core reads the release record about its version: at its start,
// and hourly, so the Update page's mark comes within the hour of a release (D462 §8).
const releaseCheckEvery = time.Hour

// ReleaseCheckNowEvery is how often an admin's Check now may have the core read the release record
// (D462 §8): at most once a minute, whoever asks.
const ReleaseCheckNowEvery = time.Minute

// ErrCheckedJustNow is CheckReleaseNow's when an admin had the record read under a minute ago.
var ErrCheckedJustNow = errors.New("the release record was read at an admin's asking under a minute ago")

// releaseRetryAfter is how long after a reading that found no record that holds the core reads again.
const releaseRetryAfter = time.Hour

// AutomaticInstallHold is how long a core must have known a release as the latest, on its own
// monotonic clock from first sight, before it installs it automatically: a stolen everyday key's
// record is caught by the owner's watcher and undone by a recovery statement within it (the root key
// audit's F4; D393 §8). The admin's banner and an install by hand are not held.
const AutomaticInstallHold = 24 * time.Hour

// Release is what the core knows of releases newer than its own, for its admins (security-updates.md
// Part B).
type Release struct {
	// Version is this build's: a release's tag, or "dev" for a build of none, which reads nothing.
	Version string
	// Latest is the newest release, only when it is newer than Version, Digest its image's and
	// Location where it may be pulled from; all empty when the core runs the latest, or knows of none.
	Latest, Digest, Location string
	// Security is the security fix this version lacks, nil when none is pending.
	Security *release.Fix
	// CheckedAt is when a record that holds was last read; zero before the first.
	CheckedAt time.Time
	// LatestSince is when this run first knew Latest as the latest, on the monotonic clock.
	LatestSince time.Time
	// StaleSince is when the record held expired, zero while it is current: the server cannot confirm
	// it knows of every fix since then. Problem is what the last reading found wrong: a record whose
	// digest for a version changed, two of one sequence, no publisher root in this build.
	StaleSince time.Time
	Problem    string
	// isCaughtUp is whether, this run, a place answered a look at the publisher's chain.
	isCaughtUp bool
}

// IsRelease is whether the build is a release's, which reads the release record: a development build
// has no version to ask about.
func (known Release) IsRelease() bool {
	_, isRelease := release.Compare(known.Version, known.Version)
	return isRelease
}

// MayInstallAutomatically is whether an automatic install (security-updates.md Part C, not built)
// may install Latest at now: the core caught up with the publisher's chain this run, and has known
// Latest as the latest for AutomaticInstallHold (the audit's §3.5 step 6). It never installs a version
// at or below its own, which Latest never is.
func (known Release) MayInstallAutomatically(now time.Time) bool {
	return known.Latest != "" && known.isCaughtUp && !known.LatestSince.IsZero() &&
		now.Sub(known.LatestSince) >= AutomaticInstallHold
}

// publisherHeld is the publisher root as the core holds it: its chain, the record as received and as
// read, the floor a newer one must be above, every version's digest seen, and the places it is served.
type publisherHeld struct {
	chain   trust.Chain
	signed  trust.Signed
	record  release.Record
	floor   floor
	digests map[string]string
}

// publisherTrust is trust-publisher, kept as trust-discovery is (§3.3); a Leave keeps it.
type publisherTrust struct {
	Identity   string            `json:"identity"`
	Statements []trust.Signed    `json:"statements"`
	Record     trust.Signed      `json:"record"`
	Floor      floor             `json:"floor"`
	Digests    map[string]string `json:"digests"`
}

// loadPublisher holds the publisher root: trust-publisher checked again from the genesis this build
// holds, or that genesis alone; none when this build holds none, which a release build says.
func (service *Service) loadPublisher() error {
	genesis := service.parts.Publisher.Genesis
	if genesis.Payload == "" {
		return nil
	}
	chain, err := trust.Genesis(trust.ChainPublisher, genesis)
	if err != nil {
		return fmt.Errorf("this build's publisher root: %w", err)
	}
	held := &publisherHeld{chain: chain, digests: map[string]string{}}
	var state publisherTrust
	isFound, err := service.readJSON(PublisherTrustFile, &state)
	if err != nil {
		return err
	}
	if isFound {
		loaded, loadErr := trust.Load(trust.ChainPublisher, chain.ID(), state.Statements)
		if loadErr != nil {
			service.parts.Logger.Error("discovery: trust-publisher does not hold; starting from this build's root",
				"err", loadErr)
		} else {
			held.chain, held.floor = loaded, state.Floor
			held.digests = maps.Clone(state.Digests)
			if record, openErr := release.Open(state.Record, loaded); openErr == nil {
				held.signed, held.record = state.Record, record
			}
		}
	}
	if held.digests == nil {
		held.digests = map[string]string{}
	}
	service.publisher = held
	return nil
}

// CheckReleaseNow reads the release record at once, at an admin's asking, at most once a minute
// (D462 §8), and is what the core knows after; the hourly check counts on from it.
func (service *Service) CheckReleaseNow(ctx context.Context) (Release, error) {
	now := service.parts.Clock.Now()
	service.mutex.Lock()
	if !service.releaseNowAt.IsZero() && now.Sub(service.releaseNowAt) < ReleaseCheckNowEvery {
		service.mutex.Unlock()
		return service.Release(), ErrCheckedJustNow
	}
	service.releaseNowAt, service.releaseAskedAt = now, time.Time{}
	service.mutex.Unlock()
	service.checkRelease(ctx, Status{})
	return service.Release(), nil
}

// checkRelease reads the signed release record at the core's start and hourly, an hour after a reading
// that found none that holds (§3.5): it follows the publisher's chain, then takes the newest record
// that holds under it, above the floor, from the record's places, this build's and the discovery's
// (every discovery serves it); refuses one naming another digest for a version it has seen, and two
// of one sequence; and keeps what the record says of its own version. A development build reads
// nothing, and a failure is never the step's problem.
func (service *Service) checkRelease(ctx context.Context, _ Status) {
	service.releaseChecking.Lock()
	defer service.releaseChecking.Unlock()
	now := service.parts.Clock.Now()
	service.mutex.Lock()
	isDue := service.releaseAskedAt.IsZero() || now.Sub(service.releaseAskedAt) >= service.releaseWait
	publisher := service.publisher
	discoveryPlaces := service.held.livePlaces(now)
	service.mutex.Unlock()
	if !isDue || !(Release{Version: service.parts.Version}).IsRelease() {
		return
	}
	if publisher == nil {
		service.setReleaseChecked(now, releaseCheckEvery)
		service.setReleaseProblem("this build holds no publisher root, so no release record is checked")
		return
	}
	held := *publisher
	held.digests = maps.Clone(publisher.digests)
	places := placesOf(service.parts.Publisher.Locations, held.record.Locations, discoveryPlaces)
	_, isReached := service.followChain(ctx, &held.chain, places, release.PublisherRootPath,
		release.PublisherRootLatestPath)
	if held.chain.Version() > publisher.chain.Version() {
		held.floor = higherFloor(held.floor, floor{RootVersion: held.chain.Version()})
	}
	newest, problems := service.newestRecord(ctx, held, places)
	wait := releaseCheckEvery
	if newest != nil {
		held.signed, held.record = newest.signed, newest.record
		held.floor = floor{RootVersion: newest.record.RootVersion, Sequence: newest.record.Sequence}
		maps.Copy(held.digests, newest.record.Images)
	} else if held.record.Latest == "" || len(problems) > 0 {
		wait = releaseRetryAfter
	}
	service.adoptRecord(ctx, held, now, wait, isReached, problems)
}

// placesOf are the places among lists, each once, in order, with no closing slash.
func placesOf(lists ...[]string) []string {
	var places []string
	for _, list := range lists {
		for _, each := range list {
			if each = strings.TrimSuffix(each, "/"); !slices.Contains(places, each) {
				places = append(places, each)
			}
		}
	}
	return places
}

// openedRecord is a record as received and as read.
type openedRecord struct {
	signed trust.Signed
	record release.Record
}

// newestRecord is the newest record above held's floor that holds, from any of places, or nil, and
// what was wrong with those that did not.
func (service *Service) newestRecord(
	ctx context.Context, held publisherHeld, places []string,
) (*openedRecord, []string) {
	var newest *openedRecord
	var problems []string
	for _, place := range places {
		signed, isFound, err := service.fetchSigned(ctx, place+release.RecordPath)
		if err != nil {
			problems = append(problems, err.Error())
		}
		if err != nil || !isFound {
			continue
		}
		record, err := release.Open(signed, held.chain)
		if err != nil {
			problems = append(problems, place+": "+err.Error())
			continue
		}
		atFloor := record.RootVersion == held.floor.RootVersion && record.Sequence == held.floor.Sequence
		if changed, isChanged := record.ChangedDigest(held.digests); isChanged {
			problems = append(problems, fmt.Sprintf("%s names another image for %s than was signed before: refused",
				place, changed))
			service.parts.Logger.ErrorContext(ctx, "discovery: a release record changes a released version's digest",
				"place", place, "version", changed)
			continue
		}
		switch {
		case atFloor && signed.Payload != held.signed.Payload:
			problems = append(problems, fmt.Sprintf("%s serves another release record of sequence %d: refused",
				place, record.Sequence))
		case held.floor.isBelowOrAt(record.RootVersion, record.Sequence):
		case newest == nil || record.IsNewerThan(newest.record):
			newest = &openedRecord{signed: signed, record: record}
		}
	}
	return newest, problems
}

// adoptRecord holds the publisher's state the reading reached, keeps it, and tells what changed.
func (service *Service) adoptRecord(ctx context.Context, held publisherHeld, now time.Time, wait time.Duration,
	isReached bool, problems []string,
) {
	told := held.record.For(service.parts.Version)
	service.mutex.Lock()
	was := service.release
	service.publisher = &held
	service.releaseAskedAt, service.releaseWait = now, wait
	known := Release{
		Latest: told.Latest, Digest: told.Digest, Location: told.Location, Security: told.Security,
		CheckedAt: was.CheckedAt, LatestSince: was.LatestSince, Problem: strings.Join(problems, "; "),
		isCaughtUp: was.isCaughtUp || isReached,
	}
	if held.record.Latest != "" {
		known.CheckedAt = now
	}
	if told.Latest != was.Latest {
		known.LatestSince = time.Time{}
		if told.Latest != "" {
			known.LatestSince = now
		}
	}
	if !held.record.Expires.IsZero() && held.record.Expires.Before(now) {
		known.StaleSince = held.record.Expires
	}
	service.release = known
	service.mutex.Unlock()
	if err := service.writeJSON(PublisherTrustFile, publisherTrust{
		Identity: held.chain.ID(), Statements: held.chain.Statements(), Record: held.signed, Floor: held.floor,
		Digests: held.digests,
	}); err != nil {
		service.parts.Logger.WarnContext(ctx, "discovery: could not keep the publisher's trust state", "err", err)
	}
	service.toldRelease(ctx, was, told)
}

// setReleaseChecked records a reading at now, the next after wait.
func (service *Service) setReleaseChecked(now time.Time, wait time.Duration) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	service.releaseAskedAt, service.releaseWait = now, wait
}

// setReleaseProblem says what is wrong with reading the release record.
func (service *Service) setReleaseProblem(problem string) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	service.release.Problem = problem
}

// Release is what the core knows now of releases newer than its own.
func (service *Service) Release() Release {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	known := service.release
	known.Version = service.parts.Version
	if known.Security != nil {
		fix := *known.Security
		known.Security = &fix
	}
	return known
}

// toldRelease logs what the record changed: a security fix newly pending, or a newer release newly
// known.
func (service *Service) toldRelease(ctx context.Context, was Release, told release.Told) {
	switch {
	case told.Security != nil && (was.Security == nil || *was.Security != *told.Security):
		service.parts.Logger.WarnContext(ctx, "discovery: a security update is available; the admins are shown it",
			"version", service.parts.Version, "fixedIn", told.Security.FixedIn, "latest", told.Latest,
			"summary", told.Security.Summary)
	case told.Latest != was.Latest && told.Latest != "":
		service.parts.Logger.InfoContext(ctx, "discovery: a newer release is available",
			"version", service.parts.Version, "latest", told.Latest)
	}
}
