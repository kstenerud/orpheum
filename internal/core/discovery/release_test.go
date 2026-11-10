// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/discovery/release"
	"github.com/kstenerud/orpheum/discovery/trust/trusttest"
)

// aSecurityRelease is a release record by which a core at v0.1.0 lacks a security fix.
func aSecurityRelease() *release.Record {
	return &release.Record{
		RootVersion: 1, Sequence: 1, Issued: trusttest.Issued(), Expires: trusttest.Issued().Add(30 * 24 * time.Hour),
		Latest: "v0.2.1", Pulls: []string{"registry.example/orpheum"},
		Images: map[string]string{
			"v0.1.0": "sha256:" + strings.Repeat("a", 64), "v0.2.0": "sha256:" + strings.Repeat("b", 64),
			"v0.2.1": "sha256:" + strings.Repeat("d", 64),
		},
		Pending: map[string]release.Fix{"v0.1.0": {FixedIn: "v0.2.0", Summary: "a sign-in could be guessed"}},
	}
}

// releaseAsked is how many times the core read the release record.
func (built *world) releaseAsked() []int {
	built.fake.mutex.Lock()
	defer built.fake.mutex.Unlock()
	return make([]int, built.fake.releaseFetches)
}

// setRelease has the fake discovery serve record, signed by the publisher's key; nil serves none.
func (built *world) setRelease(record *release.Record) {
	built.fake.mutex.Lock()
	defer built.fake.mutex.Unlock()
	built.fake.release = record
}

// setReleaseFailing has the fake discovery fail to serve its record, or serve it again.
func (built *world) setReleaseFailing(isFailing bool) {
	built.fake.mutex.Lock()
	defer built.fake.mutex.Unlock()
	built.fake.failing["GET "+release.RecordPath] = isFailing
}

// connected is a world whose core is enrolled with its discovery.
func connected(t *testing.T) *world {
	t.Helper()
	built := newWorld(t)
	if _, err := built.service.Connect(t.Context(), built.issuer, built.fake.chain.ID()); err != nil {
		t.Fatal(err)
	}
	return built
}

// A record no key of the publisher's root signed is never believed, a discovery's key's no more than
// a stranger's (D393 §2): the core knows nothing from it, says so, and reads again an hour on.
func TestRelease_ARecordNoRootKeySignedIsNotBelieved(t *testing.T) {
	t.Parallel()
	built := connected(t)
	for _, signer := range []trusttest.Key{trusttest.HardwareKey(t, "x"), built.fake.e1} {
		built.fake.mutex.Lock()
		built.fake.release, built.fake.recordSigner = aSecurityRelease(), &signer
		built.fake.mutex.Unlock()
		built.clock.Advance(releaseRetryAfter)
		built.service.step(t.Context())
		if known := built.service.Release(); known.Security != nil || known.Latest != "" || known.Problem == "" {
			t.Errorf("from a record %s signed the core knows %+v; want nothing, and the problem said",
				signer.Label, known)
		}
	}
	built.fake.mutex.Lock()
	built.fake.recordSigner = nil
	built.fake.mutex.Unlock()
	built.clock.Advance(releaseRetryAfter)
	built.service.step(t.Context())
	if known := built.service.Release(); known.Security == nil || known.Location != "registry.example/orpheum" {
		t.Errorf("from the publisher's own record an hour on the core knows %+v; want the fix and where to pull", known)
	}
}

// A core reads the record at its start and then hourly (D462 §8), pending as claimed, and keeps what it says for
// its admins; a later record with nothing pending clears it.
func TestRelease_ACoreAsksAtItsStartAndHourlyAboutItsOwnVersion(t *testing.T) {
	t.Parallel()
	built := connected(t)
	ctx := t.Context()
	built.setRelease(aSecurityRelease())
	built.service.step(ctx)
	known := built.service.Release()
	if asked := built.releaseAsked(); len(asked) != 1 {
		t.Fatalf("read the record %d times; want once, at its start", len(asked))
	}
	if known.Version != "v0.1.0" || known.Latest != "v0.2.1" || known.Security == nil ||
		known.Security.FixedIn != "v0.2.0" || !known.CheckedAt.Equal(built.clock.Now()) {
		t.Errorf("knows %+v; want the latest, the fix it lacks and when it was told", known)
	}
	built.service.step(ctx)
	built.clock.Advance(releaseCheckEvery - time.Minute)
	built.service.step(ctx)
	if asked := built.releaseAsked(); len(asked) != 1 {
		t.Errorf("read %d times within the hour; want once", len(asked))
	}
	plain := aSecurityRelease()
	plain.Sequence, plain.Latest = 2, "v0.1.0"
	plain.Images = map[string]string{"v0.1.0": "sha256:" + strings.Repeat("a", 64)}
	plain.Pending = nil
	built.setRelease(plain)
	built.clock.Advance(time.Minute)
	built.service.step(ctx)
	if asked, known := built.releaseAsked(), built.service.Release(); len(asked) != 2 || known.Security != nil ||
		known.Latest != "" {
		t.Errorf("an hour on, read %d times and knows %+v; want read again and nothing pending", len(asked), known)
	}
}

// An admin's Check now reads the record at once, at most once a minute, and the hourly check counts
// on from it (D462 §8).
func TestRelease_AnAdminChecksNowAtMostOnceAMinute(t *testing.T) {
	t.Parallel()
	built := connected(t)
	ctx := t.Context()
	built.service.step(ctx)
	before := len(built.releaseAsked())
	built.setRelease(aSecurityRelease())
	built.clock.Advance(time.Minute)
	known, err := built.service.CheckReleaseNow(ctx)
	if err != nil || len(built.releaseAsked()) != before+1 || known.Latest != "v0.2.1" {
		t.Fatalf("Check now read %d times, knows %+v (%v); want read again and the latest known",
			len(built.releaseAsked()), known, err)
	}
	built.clock.Advance(ReleaseCheckNowEvery - time.Second)
	if _, err = built.service.CheckReleaseNow(ctx); !errors.Is(err, ErrCheckedJustNow) ||
		len(built.releaseAsked()) != before+1 {
		t.Errorf("Check now again within the minute = %v, read %d times; want refused, unread",
			err, len(built.releaseAsked()))
	}
	built.clock.Advance(time.Second)
	if _, err = built.service.CheckReleaseNow(ctx); err != nil || len(built.releaseAsked()) != before+2 {
		t.Errorf("Check now a minute on = %v, read %d times; want read", err, len(built.releaseAsked()))
	}
	built.clock.Advance(releaseCheckEvery - time.Minute)
	built.service.step(ctx)
	if asked := built.releaseAsked(); len(asked) != before+2 {
		t.Errorf("under an hour after Check now, the loop read %d times; want none more", len(asked))
	}
}

// A development build has no version to ask about: it reads nothing, and knows nothing.
func TestRelease_ADevelopmentBuildAsksNothing(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	built.version = "dev"
	built.service = built.restart(t)
	built.setRelease(aSecurityRelease())
	ctx := t.Context()
	if _, err := built.service.Connect(ctx, built.issuer, built.fake.chain.ID()); err != nil {
		t.Fatal(err)
	}
	built.service.step(ctx)
	if asked, known := built.releaseAsked(), built.service.Release(); len(asked) != 0 || known.IsRelease() ||
		known.Security != nil {
		t.Errorf("a development build read %v and knows %+v; want nothing", asked, known)
	}
}

// A discovery that serves no record knows nothing, which is no failure; a failed reading is no
// problem for the core's standing, keeps what was known, and is read again an hour on.
func TestRelease_ADiscoveryWithoutTheCallKnowsNothingAndAFailureAsksAgainInAnHour(t *testing.T) {
	t.Parallel()
	built := connected(t)
	ctx := t.Context()
	built.service.step(ctx)
	if known := built.service.Release(); known.Latest != "" || built.service.Status().Problem != "" {
		t.Fatalf("no record: knows %+v, problem %q; want nothing known and no problem",
			known, built.service.Status().Problem)
	}
	built.setRelease(aSecurityRelease())
	built.clock.Advance(releaseRetryAfter)
	built.service.step(ctx)
	built.setReleaseFailing(true)
	built.clock.Advance(releaseCheckEvery)
	built.service.step(ctx)
	if known := built.service.Release(); known.Security == nil || built.service.Status().Problem != "" {
		t.Errorf("after a failed reading: knows %+v, problem %q; want what was known kept and no problem",
			known, built.service.Status().Problem)
	}
	built.setReleaseFailing(false)
	built.clock.Advance(releaseRetryAfter)
	built.service.step(ctx)
	if asked := built.releaseAsked(); len(asked) != 2 {
		t.Errorf("read %d times; want once a day, and again an hour after the failure", len(asked))
	}
}

// A record naming another digest for a version the core has seen signed is refused and said: a
// released version's digest never changes (the audit's F11).
func TestRelease_ARecordChangingAReleasedDigestIsRefusedAndSaid(t *testing.T) {
	t.Parallel()
	built := connected(t)
	built.setRelease(aSecurityRelease())
	built.service.step(t.Context())
	changed := aSecurityRelease()
	changed.Sequence = 2
	changed.Images["v0.2.1"] = "sha256:" + strings.Repeat("e", 64)
	built.setRelease(changed)
	built.clock.Advance(releaseCheckEvery)
	built.service.step(t.Context())
	if known := built.service.Release(); known.Digest != "sha256:"+strings.Repeat("d", 64) ||
		!strings.Contains(known.Problem, "another image for v0.2.1") {
		t.Errorf("after a changed digest the core knows %+v; want the first kept and the change said", known)
	}
}

// An automatic install waits until the core has known the latest for 24 hours on its own clock, from
// first sight, and the hold starts again for a newer latest; the admin's banner is not held (the
// audit's F4; D393 §8).
func TestRelease_AnAutomaticInstallWaitsADayFromFirstSight(t *testing.T) {
	t.Parallel()
	built := connected(t)
	built.setRelease(aSecurityRelease())
	built.service.step(t.Context())
	known := built.service.Release()
	if known.Security == nil || known.MayInstallAutomatically(built.clock.Now()) {
		t.Fatalf("at first sight: %+v; want it shown and not installed", known)
	}
	built.clock.Advance(AutomaticInstallHold - time.Minute)
	if built.service.Release().MayInstallAutomatically(built.clock.Now()) {
		t.Error("installed before the day was out")
	}
	built.clock.Advance(time.Minute)
	if !built.service.Release().MayInstallAutomatically(built.clock.Now()) {
		t.Error("not installed after a day")
	}
	newer := aSecurityRelease()
	newer.Sequence, newer.Latest = 2, "v0.2.2"
	newer.Images["v0.2.2"] = "sha256:" + strings.Repeat("f", 64)
	built.setRelease(newer)
	built.clock.Advance(releaseCheckEvery)
	built.service.step(t.Context())
	latest := built.service.Release()
	if latest.Latest != "v0.2.2" || latest.MayInstallAutomatically(built.clock.Now()) {
		t.Errorf("a newer latest: %+v; want its own day's hold", latest)
	}
}

// A release build holding no publisher root reads no record, and says so (D393 §2).
func TestRelease_ABuildWithNoPublisherRootChecksNothingAndSaysSo(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	built.noPublisher = true
	built.service = built.restart(t)
	built.setRelease(aSecurityRelease())
	if _, err := built.service.Connect(t.Context(), built.issuer, built.fake.chain.ID()); err != nil {
		t.Fatal(err)
	}
	built.service.step(t.Context())
	if known := built.service.Release(); known.Latest != "" || !strings.Contains(known.Problem, "no publisher root") ||
		len(built.releaseAsked()) != 0 {
		t.Errorf("with no publisher root: %+v; want nothing read, and said", known)
	}
}
