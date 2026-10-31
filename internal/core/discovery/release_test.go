// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
)

// aSecurityRelease is a discovery's answer to a core whose version lacks a security fix.
func aSecurityRelease() *discoveryproto.ReleaseResponse {
	return &discoveryproto.ReleaseResponse{
		Latest: "v0.2.1", Digest: "sha256:" + strings.Repeat("d", 64),
		Security: &discoveryproto.SecurityFix{Summary: "a sign-in could be guessed", FixedIn: "v0.2.0"},
	}
}

// releaseAsked is the versions the core asked its discovery about, in turn.
func (built *world) releaseAsked() []string {
	built.fake.mutex.Lock()
	defer built.fake.mutex.Unlock()
	return slices.Clone(built.fake.releaseAsked)
}

// setRelease has the fake discovery answer the release call with answer; nil has no such call.
func (built *world) setRelease(answer *discoveryproto.ReleaseResponse) {
	built.fake.mutex.Lock()
	defer built.fake.mutex.Unlock()
	built.fake.release = answer
}

// setReleaseFailing has the fake discovery fail the release call, or answer it again.
func (built *world) setReleaseFailing(isFailing bool) {
	built.fake.mutex.Lock()
	defer built.fake.mutex.Unlock()
	built.fake.failing["POST /release"] = isFailing
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
	if asked := built.releaseAsked(); !slices.Equal(asked, []string{"v0.1.0"}) {
		t.Fatalf("asked about %v; want its own version once, at its start", asked)
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
	built.setRelease(&discoveryproto.ReleaseResponse{})
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

// A discovery older than the call knows nothing, which is no failure; a failed asking is no problem
// for the core's standing, keeps what was known, and is asked again an hour on.
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
