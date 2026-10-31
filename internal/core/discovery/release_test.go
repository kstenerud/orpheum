// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import (
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/discovery/release"
	"github.com/kstenerud/orpheum/discovery/trust/trusttest"
)

// aSecurityRelease is a release record by which a core at v0.1.0 lacks a security fix.
func aSecurityRelease() *release.Record {
	return &release.Record{
		IssuedAt: 1, Latest: "v0.2.1", Locations: []string{"registry.example/orpheum"},
		Releases: map[string]release.Note{
			"v0.1.0": {
				Digest: "sha256:" + strings.Repeat("a", 64), Security: true, Summary: "a sign-in could be guessed",
				FixedIn: "v0.2.0",
			},
			"v0.2.0": {Digest: "sha256:" + strings.Repeat("b", 64)},
			"v0.2.1": {Digest: "sha256:" + strings.Repeat("d", 64)},
		},
	}
}

// releaseAsked is how many times the core read its discovery's release record.
func (built *world) releaseAsked() []int {
	built.fake.mutex.Lock()
	defer built.fake.mutex.Unlock()
	return make([]int, built.fake.releaseFetches)
}

// setRelease has the fake discovery serve record, signed by its root; nil serves none.
func (built *world) setRelease(record *release.Record) {
	built.fake.mutex.Lock()
	defer built.fake.mutex.Unlock()
	built.fake.release = record
}

// setReleaseFailing has the fake discovery fail to serve its record, or serve it again.
func (built *world) setReleaseFailing(isFailing bool) {
	built.fake.mutex.Lock()
	defer built.fake.mutex.Unlock()
	built.fake.failing["GET "+release.WellKnownPath] = isFailing
}

// A record a key outside the discovery's root key set signed is never believed: the core knows
// nothing from it, and reads again an hour on (D391 §6).
func TestRelease_ARecordNoRootKeySignedIsNotBelieved(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	ctx := t.Context()
	if _, err := built.service.Connect(ctx, built.issuer); err != nil {
		t.Fatal(err)
	}
	stranger := trusttest.NewRoot(t)
	built.fake.mutex.Lock()
	built.fake.release, built.fake.recordSigner = aSecurityRelease(), stranger.Signer
	built.fake.mutex.Unlock()
	built.service.step(ctx)
	if known := built.service.Release(); known.Security != nil || known.Latest != "" || !known.CheckedAt.IsZero() {
		t.Errorf("from a forged record the core knows %+v; want nothing", known)
	}
	built.fake.mutex.Lock()
	built.fake.recordSigner = nil
	built.fake.mutex.Unlock()
	built.clock.Advance(releaseRetryAfter)
	built.service.step(ctx)
	if known := built.service.Release(); known.Security == nil || known.Location != "registry.example/orpheum" {
		t.Errorf("from the root's own record an hour on the core knows %+v; want the fix and where to pull", known)
	}
}

// A core asks its discovery about its own version at its start and then daily, pending as claimed,
// and keeps what it was told for its admins; a later answer of nothing pending clears it.
func TestRelease_ACoreAsksAtItsStartAndDailyAboutItsOwnVersion(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	ctx := t.Context()
	if _, err := built.service.Connect(ctx, built.issuer); err != nil {
		t.Fatal(err)
	}
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
		t.Errorf("asked %d times within the day; want once", len(asked))
	}
	plain := aSecurityRelease()
	plain.IssuedAt, plain.Latest = 2, "v0.1.0"
	plain.Releases = map[string]release.Note{"v0.1.0": {Digest: "sha256:" + strings.Repeat("a", 64)}}
	built.setRelease(plain)
	built.clock.Advance(time.Minute)
	built.service.step(ctx)
	if asked, known := built.releaseAsked(), built.service.Release(); len(asked) != 2 || known.Security != nil ||
		known.Latest != "" {
		t.Errorf("a day on, asked %d times and knows %+v; want asked again and nothing pending", len(asked), known)
	}
}

// A development build has no version to ask about: it asks nothing, and knows nothing.
func TestRelease_ADevelopmentBuildAsksNothing(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	built.version = "dev"
	built.service = built.restart(t)
	built.setRelease(aSecurityRelease())
	ctx := t.Context()
	if _, err := built.service.Connect(ctx, built.issuer); err != nil {
		t.Fatal(err)
	}
	built.service.step(ctx)
	if asked, known := built.releaseAsked(), built.service.Release(); len(asked) != 0 || known.IsRelease() ||
		known.Security != nil {
		t.Errorf("a development build asked %v and knows %+v; want nothing", asked, known)
	}
}

// A discovery that serves no record knows nothing, which is no failure; a failed reading is no
// problem for the core's standing, keeps what was known, and is read again an hour on.
func TestRelease_ADiscoveryWithoutTheCallKnowsNothingAndAFailureAsksAgainInAnHour(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	ctx := t.Context()
	if _, err := built.service.Connect(ctx, built.issuer); err != nil {
		t.Fatal(err)
	}
	built.service.step(ctx)
	if known := built.service.Release(); known.CheckedAt.IsZero() || known.Latest != "" ||
		built.service.Status().Problem != "" {
		t.Fatalf("a discovery without the call: knows %+v, problem %q; want nothing known and no problem",
			known, built.service.Status().Problem)
	}
	built.setRelease(aSecurityRelease())
	built.clock.Advance(releaseCheckEvery)
	built.service.step(ctx)
	built.setReleaseFailing(true)
	built.clock.Advance(releaseCheckEvery)
	built.service.step(ctx)
	if known := built.service.Release(); known.Security == nil || built.service.Status().Problem != "" {
		t.Errorf("after a failed asking: knows %+v, problem %q; want what was known kept and no problem",
			known, built.service.Status().Problem)
	}
	built.setReleaseFailing(false)
	built.clock.Advance(releaseRetryAfter)
	built.service.step(ctx)
	if asked := built.releaseAsked(); len(asked) != 2 {
		t.Errorf("asked %d times; want once a day, and again an hour after the failure", len(asked))
	}
}
