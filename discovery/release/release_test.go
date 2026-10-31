// SPDX-License-Identifier: AGPL-3.0-or-later

package release_test

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/discovery/release"
	"github.com/kstenerud/orpheum/discovery/trust"
	"github.com/kstenerud/orpheum/discovery/trust/trusttest"
)

const (
	digestA = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	digestB = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	digestC = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	digestD = "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
)

// written is when the tests' records were written.
var written = time.UnixMilli(1_800_000_000_000)

// aRecord is a record of four releases: v0.3.0 and v0.3.1 lack a security fix v0.3.2 has, and v0.3.3
// is the latest, a plain one.
func aRecord() release.Record {
	fix := func(digest string) release.Note {
		return release.Note{Digest: digest, Security: true, Summary: "a sign-in could be guessed", FixedIn: "v0.3.2"}
	}
	return release.Record{
		IssuedAt: written.UnixMilli(), Latest: "v0.3.3", Locations: []string{"registry.example/orpheum"},
		Releases: map[string]release.Note{
			"v0.3.0": fix(digestC), "v0.3.1": fix(digestD), "v0.3.2": {Digest: digestB}, "v0.3.3": {Digest: digestA},
		},
	}
}

// A core is told the latest, its digest and where to pull it only when it is newer than its own
// version, and only its own version's fix; a development build's version is told nothing.
func TestFor_TellsACoreOfItsOwnVersionAlone(t *testing.T) {
	t.Parallel()
	fix := &release.Fix{Summary: "a sign-in could be guessed", FixedIn: "v0.3.2"}
	latest := release.Told{Latest: "v0.3.3", Digest: digestA, Location: "registry.example/orpheum"}
	for version, want := range map[string]release.Told{
		"v0.3.1": {Latest: latest.Latest, Digest: latest.Digest, Location: latest.Location, Security: fix},
		"v0.3.2": latest,
		"v0.3.3": {},
		"v0.4.0": {},
		"dev":    {},
		// One the record never named is told of the latest, with no fix it cannot know of.
		"v0.2.9": latest,
	} {
		if told := aRecord().For(version); !reflect.DeepEqual(told, want) {
			t.Errorf("For(%s) = %+v; want %+v", version, told, want)
		}
	}
}

// A record is taken only signed by a root key of the set, never an online key's, and only when it
// is a record; the release an image's digest is, is found by its digest alone (D391 §6).
func TestOpen_ARecordIsTakenOnlySignedByARootKey(t *testing.T) {
	t.Parallel()
	root := trusttest.NewRoot(t)
	signed := root.Signs(t, trust.KindReleaseRecord, aRecord())
	opened, err := release.Open(signed, root.Pinned)
	if err != nil || !reflect.DeepEqual(opened, aRecord()) {
		t.Fatalf("the signed record = %+v, %v; want it as written", opened, err)
	}
	if version, isNamed := opened.VersionOf(digestB); !isNamed || version != "v0.3.2" {
		t.Errorf("VersionOf(digestB) = %q, %v; want v0.3.2", version, isNamed)
	}
	if _, isNamed := opened.VersionOf("sha256:" + digestA[len("sha256:")+1:] + "0"); isNamed {
		t.Error("a digest no release has was named")
	}
	stranger := trusttest.NewRoot(t)
	if _, err = release.Open(stranger.Signs(t, trust.KindReleaseRecord, aRecord()), root.Pinned); !errors.Is(
		err, trust.ErrUntrustedKey) {
		t.Errorf("a record a stranger signed: %v; want ErrUntrustedKey", err)
	}
	// The same bytes signed as an address document are no record.
	if _, err = release.Open(root.Signs(t, trust.KindAddressDocument, aRecord()), root.Pinned); !errors.Is(
		err, trust.ErrWrongNamespace) {
		t.Errorf("a record signed as a document: %v; want ErrWrongNamespace", err)
	}
	if _, err = release.Open(root.Signs(t, trust.KindReleaseRecord, map[string]int{"x": 1}), root.Pinned); !errors.Is(
		err, release.ErrNotARecord) {
		t.Errorf("a signed thing that is no record: %v; want ErrNotARecord", err)
	}
}

// A security release marks every older release that lacks a fix, the one before it among them, as
// fixed in it; one already pending keeps its first fix. A plain release marks none. Each keeps its
// digest, and the record is dated anew.
func TestReleased_ASecurityFixMarksEveryOlderReleaseThatLacksOne(t *testing.T) {
	t.Parallel()
	later := written.Add(time.Hour)
	plain, err := aRecord().Released("v0.3.4", digestB, "", nil, later)
	if err != nil {
		t.Fatal(err)
	}
	if plain.Latest != "v0.3.4" || plain.Releases["v0.3.4"] != (release.Note{Digest: digestB}) ||
		plain.Releases["v0.3.2"].Security || plain.IssuedAt != later.UnixMilli() ||
		!reflect.DeepEqual(plain.Locations, aRecord().Locations) {
		t.Errorf("plain release = %+v; want v0.3.4 latest with its digest, no new fix, dated now", plain)
	}
	mirror := []string{"mirror.example/orpheum"}
	secure, err := plain.Released("v0.3.5", digestA, "a link could be reused", mirror, later)
	if err != nil {
		t.Fatal(err)
	}
	newFix := func(digest string) release.Note {
		return release.Note{Digest: digest, Security: true, Summary: "a link could be reused", FixedIn: "v0.3.5"}
	}
	oldFix := func(digest string) release.Note {
		return release.Note{Digest: digest, Security: true, Summary: "a sign-in could be guessed", FixedIn: "v0.3.2"}
	}
	want := map[string]release.Note{
		"v0.3.0": oldFix(digestC), "v0.3.1": oldFix(digestD), "v0.3.2": newFix(digestB),
		"v0.3.3": newFix(digestA), "v0.3.4": newFix(digestB), "v0.3.5": {Digest: digestA},
	}
	if !reflect.DeepEqual(secure.Releases, want) || secure.Latest != "v0.3.5" ||
		!reflect.DeepEqual(secure.Locations, []string{"mirror.example/orpheum"}) {
		t.Errorf("security release = %+v; want %+v from the new location", secure, want)
	}
}

// A release older than the latest, a tag that is none, and a digest that is no image's are refused;
// the latest released again takes its new digest.
func TestReleased_RefusesAnOlderReleaseOrADigestThatIsNone(t *testing.T) {
	t.Parallel()
	for _, refused := range []struct {
		tag, digest string
		want        error
	}{
		{"v0.3.2", digestB, release.ErrOlder},
		{"latest", digestB, release.ErrNotARelease},
		{"v0.4.0", "sha256:abc", release.ErrNotADigest},
	} {
		if _, err := aRecord().Released(refused.tag, refused.digest, "", nil, written); !errors.Is(err, refused.want) {
			t.Errorf("Released(%s, %s) = %v; want %v", refused.tag, refused.digest, err, refused.want)
		}
	}
	again, err := aRecord().Released("v0.3.3", digestB, "", nil, written)
	if err != nil || again.Releases["v0.3.3"].Digest != digestB || len(again.Releases) != len(aRecord().Releases) {
		t.Errorf("the latest again = %+v, %v; want its new digest and nothing else changed", again, err)
	}
}

func TestWrite_ReadsBackWhatItWrote(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "release.json")
	if err := release.Write(path, aRecord()); err != nil {
		t.Fatal(err)
	}
	read, err := release.Read(path)
	if err != nil || !reflect.DeepEqual(read, aRecord()) {
		t.Errorf("read back %+v, %v; want %+v", read, err, aRecord())
	}
}

func TestCompare_OrdersAsSemanticVersions(t *testing.T) {
	t.Parallel()
	ordered := []string{
		"v0.9.9", "v1.0.0-alpha", "v1.0.0-alpha.1", "v1.0.0-alpha.beta", "v1.0.0-beta", "v1.0.0-beta.2",
		"v1.0.0-beta.11", "v1.0.0-rc.1", "v1.0.0", "v1.0.1", "v1.2.0", "v1.10.0", "v2.0.0",
	}
	for index := range len(ordered) - 1 {
		older, newer := ordered[index], ordered[index+1]
		if order, isRelease := release.Compare(older, newer); !isRelease || order >= 0 {
			t.Errorf("Compare(%s, %s) = %d, %v; want older", older, newer, order, isRelease)
		}
		if order, _ := release.Compare(newer, older); order <= 0 {
			t.Errorf("Compare(%s, %s) = %d; want newer", newer, older, order)
		}
	}
	if order, isRelease := release.Compare("v1.0.0+build.5", "v1.0.0"); !isRelease || order != 0 {
		t.Errorf("build metadata counted: %d, %v", order, isRelease)
	}
	for _, none := range []string{"dev", "1.0.0", "v1.0", "v1.0.0-", "v01.0.0", ""} {
		if _, isRelease := release.Compare(none, "v1.0.0"); isRelease {
			t.Errorf("Compare(%q) took it for a release", none)
		}
	}
}
