// SPDX-License-Identifier: AGPL-3.0-or-later

package release_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/discovery/release"
)

const (
	digestA = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	digestB = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

// aRecord is a record of four releases: v0.3.0 and v0.3.1 lack a security fix v0.3.2 has, and v0.3.3
// is the latest, a plain one.
func aRecord() release.Record {
	fix := release.Note{Security: true, Summary: "a sign-in could be guessed", FixedIn: "v0.3.2"}
	return release.Record{
		Latest: "v0.3.3", Digest: digestA,
		Versions: map[string]release.Note{"v0.3.0": fix, "v0.3.1": fix, "v0.3.2": {}},
	}
}

// A core is told the latest and its digest only when it is newer than its own version, and only its
// own version's fix; a development build's version is told nothing.
func TestAnswer_TellsACoreOfItsOwnVersionAlone(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "release.json")
	if err := release.Write(path, aRecord()); err != nil {
		t.Fatal(err)
	}
	file := release.File{Path: path}
	fix := &discoveryproto.SecurityFix{Summary: "a sign-in could be guessed", FixedIn: "v0.3.2"}
	for version, want := range map[string]discoveryproto.ReleaseResponse{
		"v0.3.1": {Latest: "v0.3.3", Digest: digestA, Security: fix},
		"v0.3.2": {Latest: "v0.3.3", Digest: digestA},
		"v0.3.3": {},
		"v0.4.0": {},
		"dev":    {},
		// One the record never named is told of the latest, with no fix it cannot know of.
		"v0.2.9": {Latest: "v0.3.3", Digest: digestA},
	} {
		if told := file.Answer(t.Context(), version); !reflect.DeepEqual(told, want) {
			t.Errorf("Answer(%s) = %+v; want %+v", version, told, want)
		}
	}
}

// A file absent, or not a record, tells the core nothing, as does a discovery with no file named.
func TestAnswer_AFileAbsentOrBrokenIsNothingKnown(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	broken := filepath.Join(directory, "broken.json")
	if err := os.WriteFile(broken, []byte(`{"latest": `), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"", filepath.Join(directory, "absent.json"), broken} {
		told := release.File{Path: path}.Answer(t.Context(), "v0.3.1")
		if told != (discoveryproto.ReleaseResponse{}) {
			t.Errorf("Answer from %q = %+v; want nothing", path, told)
		}
	}
}

// A security release marks every older release that lacks a fix, the one before it among them, as
// fixed in it; one already pending keeps its first fix. A plain release marks none.
func TestReleased_ASecurityFixMarksEveryOlderReleaseThatLacksOne(t *testing.T) {
	t.Parallel()
	plain, err := aRecord().Released("v0.3.4", digestB, "")
	if err != nil {
		t.Fatal(err)
	}
	if plain.Latest != "v0.3.4" || plain.Digest != digestB || plain.Versions["v0.3.3"] != (release.Note{}) ||
		plain.Versions["v0.3.2"].Security {
		t.Errorf("plain release = %+v; want v0.3.4 latest, v0.3.3 and v0.3.2 named with no fix pending", plain)
	}
	secure, err := plain.Released("v0.3.5", digestA, "a link could be reused")
	if err != nil {
		t.Fatal(err)
	}
	newFix := release.Note{Security: true, Summary: "a link could be reused", FixedIn: "v0.3.5"}
	oldFix := release.Note{Security: true, Summary: "a sign-in could be guessed", FixedIn: "v0.3.2"}
	want := map[string]release.Note{
		"v0.3.0": oldFix, "v0.3.1": oldFix, "v0.3.2": newFix, "v0.3.3": newFix, "v0.3.4": newFix,
	}
	if !reflect.DeepEqual(secure.Versions, want) || secure.Latest != "v0.3.5" {
		t.Errorf("security release = %+v; want %+v", secure.Versions, want)
	}
	first, err := release.Record{}.Released("v0.1.0", digestA, "a fix")
	if err != nil || first.Latest != "v0.1.0" || len(first.Versions) != 0 {
		t.Errorf("first release = %+v, %v; want only the latest", first, err)
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
		if _, err := aRecord().Released(refused.tag, refused.digest, ""); !errors.Is(err, refused.want) {
			t.Errorf("Released(%s, %s) = %v; want %v", refused.tag, refused.digest, err, refused.want)
		}
	}
	again, err := aRecord().Released("v0.3.3", digestB, "")
	if err != nil || again.Digest != digestB || len(again.Versions) != len(aRecord().Versions) {
		t.Errorf("the latest again = %+v, %v; want its new digest and nothing else changed", again, err)
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
