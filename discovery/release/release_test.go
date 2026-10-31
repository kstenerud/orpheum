// SPDX-License-Identifier: AGPL-3.0-or-later

package release_test

import (
	"errors"
	"path/filepath"
	"reflect"
	"strings"
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

// aRecord is a record of four releases: v0.3.0 and v0.3.1 lack a security fix v0.3.2 has, and v0.3.3
// is the latest, a plain one.
func aRecord(id string) release.Record {
	fix := release.Fix{FixedIn: "v0.3.2", Summary: "a sign-in could be guessed"}
	return release.Record{
		ID: id, RootVersion: 1, Sequence: 4, Issued: trusttest.Issued(),
		Expires: trusttest.Issued().Add(720 * time.Hour),
		Latest:  "v0.3.3", Pulls: []string{"registry.example/orpheum"},
		Images:  map[string]string{"v0.3.0": digestC, "v0.3.1": digestD, "v0.3.2": digestB, "v0.3.3": digestA},
		Pending: map[string]release.Fix{"v0.3.0": fix, "v0.3.1": fix},
	}
}

func publisher(t *testing.T) (*trusttest.Chain, trusttest.Key) {
	t.Helper()
	everyday := trusttest.HardwareKey(t, "e1")
	return trusttest.NewChain(t, trust.ChainPublisher, []trusttest.Key{everyday},
		[]trusttest.Key{trusttest.HardwareKey(t, "r1")}), everyday
}

func signed(t *testing.T, record release.Record, key trusttest.Key) trust.Signed {
	t.Helper()
	payload, err := record.Payload()
	if err != nil {
		t.Fatal(err)
	}
	return trusttest.Sign(t, trust.NamespaceRelease, payload, key)
}

// A core is told the latest, its digest and where to pull it only when it is newer than its own
// version, and only its own version's fix; a development build's version is told nothing.
func TestFor_TellsACoreOfItsOwnVersionAlone(t *testing.T) {
	t.Parallel()
	fix := &release.Fix{FixedIn: "v0.3.2", Summary: "a sign-in could be guessed"}
	latest := release.Told{Latest: "v0.3.3", Digest: digestA, Location: "registry.example/orpheum"}
	for version, want := range map[string]release.Told{
		"v0.3.1": {Latest: latest.Latest, Digest: latest.Digest, Location: latest.Location, Security: fix},
		"v0.3.2": latest,
		"v0.3.3": {},
		"v0.4.0": {},
		"dev":    {},
		"v0.2.9": latest,
	} {
		if told := aRecord("op1-x").For(version); !reflect.DeepEqual(told, want) {
			t.Errorf("For(%s) = %+v; want %+v", version, told, want)
		}
	}
}

// A record is taken only signed by a key of the publisher's chain in the release namespace, naming its
// id and a root version it has reached; a discovery's chain never signs one (D393 §2). Its lines read
// back as written, and an image is a release only by the digest the record names.
func TestOpen_ARecordIsTakenOnlySignedByThePublishersKey(t *testing.T) {
	t.Parallel()
	chain, key := publisher(t)
	held := chain.Follower(t)
	made := aRecord(chain.ID())
	opened, err := release.Open(signed(t, made, key), held)
	if err != nil || !reflect.DeepEqual(opened, made) {
		t.Fatalf("the signed record = %+v, %v; want it as written", opened, err)
	}
	if version, isNamed := opened.VersionOf(digestB); !isNamed || version != "v0.3.2" {
		t.Errorf("VersionOf(digestB) = %q, %v; want v0.3.2", version, isNamed)
	}
	discovery, discoveryKey, _ := trusttest.NewDiscovery(t)
	if _, err = release.Open(signed(t, aRecord(discovery.ID()), discoveryKey), discovery.Follower(t)); !errors.Is(
		err, release.ErrNotThePublisher) {
		t.Errorf("a discovery's chain: %v; want ErrNotThePublisher", err)
	}
	stranger := signed(t, made, trusttest.HardwareKey(t, "x"))
	if _, err = release.Open(stranger, held); !errors.Is(err, trust.ErrUntrustedKey) {
		t.Errorf("a stranger's record: %v; want ErrUntrustedKey", err)
	}
	ahead := made
	ahead.RootVersion = 3
	if _, err = release.Open(signed(t, ahead, key), held); !errors.Is(err, release.ErrAheadOfChain) {
		t.Errorf("a root version not reached: %v; want ErrAheadOfChain", err)
	}
	if !ahead.IsNewerThan(made) || made.IsNewerThan(ahead) {
		t.Error("records are ordered by root version, then sequence")
	}
}

// A released version's digest never changes: the record refuses another for it, and a core that has
// seen one names the version another record changes (the audit's F11).
func TestReleased_AVersionsDigestNeverChanges(t *testing.T) {
	t.Parallel()
	if _, err := aRecord("op1-x").Released("v0.3.3", digestB, "", nil, trusttest.Issued()); !errors.Is(
		err, release.ErrDigestChanged) {
		t.Errorf("the latest with another digest: %v; want ErrDigestChanged", err)
	}
	changed := aRecord("op1-x")
	changed.Images["v0.3.2"] = digestC
	if tag, isChanged := changed.ChangedDigest(aRecord("op1-x").Images); !isChanged || tag != "v0.3.2" {
		t.Errorf("ChangedDigest = %q, %v; want v0.3.2", tag, isChanged)
	}
}

// A security release marks every older release that lacks a fix as fixed in it; one already pending
// keeps its first fix; a plain release marks none. Each release dates the record anew, a month current,
// its sequence one more.
func TestReleased_ASecurityFixMarksEveryOlderReleaseThatLacksOne(t *testing.T) {
	t.Parallel()
	later := trusttest.Issued().Add(time.Hour)
	plain, err := aRecord("op1-x").Released("v0.3.4", digestB, "", nil, later)
	if err != nil {
		t.Fatal(err)
	}
	if plain.Latest != "v0.3.4" || plain.Images["v0.3.4"] != digestB || len(plain.Pending) != 2 ||
		plain.Sequence != 5 || !plain.Issued.Equal(later) || !plain.Expires.Equal(later.Add(720*time.Hour)) {
		t.Errorf("plain release = %+v", plain)
	}
	mirror := []string{"mirror.example/orpheum"}
	secure, err := plain.Released("v0.3.5", digestA, "a link could be reused", mirror, later)
	if err != nil {
		t.Fatal(err)
	}
	newFix := release.Fix{FixedIn: "v0.3.5", Summary: "a link could be reused"}
	oldFix := release.Fix{FixedIn: "v0.3.2", Summary: "a sign-in could be guessed"}
	want := map[string]release.Fix{
		"v0.3.0": oldFix, "v0.3.1": oldFix, "v0.3.2": newFix, "v0.3.3": newFix, "v0.3.4": newFix,
	}
	if !reflect.DeepEqual(secure.Pending, want) || !reflect.DeepEqual(secure.Pulls, mirror) {
		t.Errorf("security release = %+v; want %+v from the new place", secure, want)
	}
	if _, err = aRecord("op1-x").Released("v0.3.2", digestB, "", nil, later); !errors.Is(err, release.ErrOlder) {
		t.Errorf("an older release: %v; want ErrOlder", err)
	}
	if _, err = aRecord("op1-x").Released("latest", digestB, "", nil, later); !errors.Is(err, release.ErrNotARelease) {
		t.Errorf("no tag: %v; want ErrNotARelease", err)
	}
	if _, err = aRecord("op1-x").Released("v0.4.0", "sha256:abc", "", nil, later); !errors.Is(
		err, release.ErrNotADigest) {
		t.Errorf("no digest: %v; want ErrNotADigest", err)
	}
}

func TestWrite_ReadsBackWhatItWrote(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "release")
	if err := release.Write(path, aRecord("op1-x")); err != nil {
		t.Fatal(err)
	}
	read, err := release.Read(path)
	if err != nil || !reflect.DeepEqual(read, aRecord("op1-x")) {
		t.Errorf("read back %+v, %v", read, err)
	}
	payload, err := aRecord("op1-x").Payload()
	if err != nil || !strings.HasPrefix(string(payload), "orpheum-signed release 1\nid op1-x\n") ||
		!strings.Contains(string(payload), "image v0.3.3 "+digestA+"\nimage v0.3.2") {
		t.Errorf("the payload %q, %v; want the format's lines, newest first", payload, err)
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
