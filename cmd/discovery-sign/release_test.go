// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/discovery/release"
	"github.com/kstenerud/orpheum/discovery/trust"
)

// The test's server, which serves what was copied to it as /.well-known/, at the ceremony's issuer.
const (
	testIssuer      = "https://orpheum.dev"
	testDestination = "root@server.example:/srv/connect/published/"
)

// fakeWorld stands in for the network, scp and git: a registry that serves digests, GitHub's release
// assets, and a server whose published folder is a directory, so no test touches the network.
type fakeWorld struct {
	t *testing.T
	// assets are what the addresses beyond the server answer with.
	assets map[string][]byte
	// digests are the registry's, by "pull:tag".
	digests map[string]string
	// served is the server's folder served as /.well-known/; landing, when set, is where a copy lands
	// instead, as files copied to the wrong folder do.
	served  string
	landing string
	// copiedTo are the destinations the files were copied to; gits the git commands run.
	copiedTo []string
	gits     [][]string
	// failing is the git command that fails.
	failing string
}

func newFakeWorld(t *testing.T) *fakeWorld {
	t.Helper()
	return &fakeWorld{t: t, assets: map[string][]byte{}, digests: map[string]string{}, served: t.TempDir()}
}

func (world *fakeWorld) Fetch(address string) ([]byte, error) {
	if body, isKnown := world.assets[address]; isKnown {
		return body, nil
	}
	if path, isServed := strings.CutPrefix(address, testIssuer+trust.WellKnownPrefix); isServed {
		body, err := os.ReadFile(filepath.Join(world.served, filepath.FromSlash(path)))
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%s answered 404", address)
		}
		return body, err
	}
	return nil, fmt.Errorf("%s: no such host", address)
}

func (world *fakeWorld) Digest(pull, tag string) (string, error) {
	digest, isKnown := world.digests[pull+":"+tag]
	if !isKnown {
		return "", fmt.Errorf("%s:%s: the registry answered 404", pull, tag)
	}
	return digest, nil
}

// Copy copies each source into the server's folder, as scp -r does into the destination folder.
func (world *fakeWorld) Copy(sources []string, destination string) error {
	world.copiedTo = append(world.copiedTo, destination)
	into := world.served
	if world.landing != "" {
		into = world.landing
	}
	for _, source := range sources {
		base := filepath.Dir(source)
		err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			relative, err := filepath.Rel(base, path)
			if err != nil {
				return err
			}
			target := filepath.Join(into, relative)
			if entry.IsDir() {
				return os.MkdirAll(target, 0o750)
			}
			contents, err := os.ReadFile(path) //nolint:gosec // the test's own folder
			if err != nil {
				return err
			}
			return os.WriteFile(target, contents, 0o600) //nolint:gosec // the test's own folders
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (world *fakeWorld) Git(_ string, arguments ...string) error {
	world.gits = append(world.gits, arguments)
	if arguments[0] == world.failing {
		return fmt.Errorf("git %s: exit status 1: fatal: the remote end hung up", arguments[0])
	}
	return nil
}

// releaseFolder is a ceremony's folder at testIssuer, the unsigned record of v1.0.0 at digest attached
// to its GitHub release, and a registry that serves served for it.
func releaseFolder(t *testing.T, digest, served string) (string, *fakeKeys, *fakeWorld) {
	t.Helper()
	folder, keysDirectory := filepath.Join(t.TempDir(), "records"), t.TempDir()
	fake := newFakeKeys(t)
	status, said := softwareCeremony(t, fake, folder, keysDirectory, "\n\n"+testIssuer+"\n\n")
	if status != exitSuccess {
		t.Fatalf("the ceremony = %d: %s", status, said)
	}
	record, err := os.ReadFile(releaseRecord(t, folder, "v1.0.0", digest))
	if err != nil {
		t.Fatal(err)
	}
	world := newFakeWorld(t)
	world.assets[fmt.Sprintf(recordAsset, "v1.0.0")] = record
	world.digests["registry.example/orpheum:v1.0.0"] = served
	return folder, fake, world
}

// release shows the record's digest beside the registry's and signs only on the owner's yes, and
// publishes a record a core opens; a no signs nothing.
func TestSignRelease_NeedsTheOwnersConfirmationOfTheDigest(t *testing.T) {
	t.Parallel()
	digest := "sha256:" + strings.Repeat("ab12", 16)
	folder, fake, world := releaseFolder(t, digest, digest)
	signsBefore := fake.signs
	status, said := guidedRunIn(t, fake, world, "no\n", "release", "v1.0.0", "--folder", folder)
	if status != exitSuccess || !strings.Contains(said, "Nothing was signed") || fake.signs != signsBefore {
		t.Fatalf("a no: %d %s (%d signs); want nothing signed", status, said, fake.signs-signsBefore)
	}
	status, said = guidedRunIn(t, fake, world, "\n", "release", "v1.0.0", "--folder", folder)
	if fake.signs != signsBefore || len(world.copiedTo) != 0 {
		t.Fatalf("no answer: %d %s; want nothing signed", status, said)
	}
	status, said = guidedRunIn(t, fake, world, "yes\n"+testDestination+"\n", "release", "v1.0.0", "--folder", folder)
	if status != exitSuccess || !strings.Contains(said, "A flaw in sign-in.") ||
		strings.Count(said, digest) != 2 {
		t.Fatalf("release = %d: %s; want the release shown with both digests, and signed", status, said)
	}
	out := filepath.Join(folder, publishDirectory)
	publisher, err := trust.Genesis(trust.ChainPublisher, readPublished(t, out, release.PublisherRootPath(1)))
	if err != nil {
		t.Fatal(err)
	}
	if opened, openErr := release.Open(readPublished(t, out, release.RecordPath), publisher); openErr != nil ||
		opened.Latest != "v1.0.0" {
		t.Errorf("the published record: %+v, %v", opened, openErr)
	}
}

// A registry that serves another image under the tag, or cannot be asked, signs nothing and asks
// nothing.
func TestRelease_RefusesADigestTheRegistryDoesNotServe(t *testing.T) {
	t.Parallel()
	digest := "sha256:" + strings.Repeat("ab12", 16)
	folder, fake, world := releaseFolder(t, digest, "sha256:"+strings.Repeat("ef34", 16))
	signsBefore := fake.signs
	status, said := guidedRunIn(t, fake, world, "yes\n"+testDestination+"\n", "release", "v1.0.0", "--folder", folder)
	if status != exitFailure || !strings.Contains(said, "another image") || fake.signs != signsBefore ||
		strings.Contains(said, "Sign v1.0.0?") {
		t.Fatalf("another image: %d %s (%d signs); want refused before the question", status, said,
			fake.signs-signsBefore)
	}
	delete(world.digests, "registry.example/orpheum:v1.0.0")
	status, said = guidedRunIn(t, fake, world, "yes\n"+testDestination+"\n", "release", "v1.0.0", "--folder", folder)
	if status != exitFailure || !strings.Contains(said, "could not ask the registry") || fake.signs != signsBefore {
		t.Fatalf("no registry: %d %s; want refused, nothing signed", status, said)
	}
}

// A signed release is copied to the destination asked once and kept, read back from the issuer, and
// committed; a push that fails is said, and the run still succeeds.
func TestRelease_CopiesReadsBackAndCommits(t *testing.T) {
	t.Parallel()
	digest := "sha256:" + strings.Repeat("ab12", 16)
	folder, fake, world := releaseFolder(t, digest, digest)
	world.failing = "push"
	status, said := guidedRunIn(t, fake, world, "yes\n"+testDestination+"\n", "release", "v1.0.0", "--folder", folder)
	if status != exitSuccess || !strings.Contains(said, "Read back from "+testIssuer) ||
		!strings.Contains(said, "the push failed") {
		t.Fatalf("release = %d: %s; want read back, and the failed push said", status, said)
	}
	if !slices.Equal(world.copiedTo, []string{testDestination}) {
		t.Errorf("copied to %v; want %s", world.copiedTo, testDestination)
	}
	want := [][]string{{"add", "-A"}, {"commit", "-m", "Release v1.0.0"}, {"push"}}
	if !slices.EqualFunc(world.gits, want, slices.Equal) {
		t.Errorf("git ran %v; want %v", world.gits, want)
	}
	ignored, err := os.ReadFile(filepath.Join(folder, localDirectory, ".gitignore"))
	if err != nil || string(ignored) != "*\n" {
		t.Errorf("the destination's note is not kept out of git: %q, %v", ignored, err)
	}

	// The next release asks only its yes: the destination is kept.
	world.assets[fmt.Sprintf(recordAsset, "v1.0.1")] = nextRecord(t, world, "v1.0.1", digest)
	world.digests["registry.example/orpheum:v1.0.1"] = digest
	status, said = guidedRunIn(t, fake, world, "yes\n", "release", "v1.0.1", "--folder", folder)
	if status != exitSuccess || len(world.copiedTo) != 2 || world.copiedTo[1] != testDestination {
		t.Fatalf("the next release = %d: %s, copied to %v; want the kept destination", status, said, world.copiedTo)
	}
}

// A record whose image is pulled from another registry than the last release signed names, as one
// changed on its way from GitHub could, signs nothing and asks no registry, unless the owner says the
// move is meant.
func TestRelease_RefusesARegistryTheLastReleaseWasNotPulledFrom(t *testing.T) {
	t.Parallel()
	digest := "sha256:" + strings.Repeat("ab12", 16)
	folder, fake, world := releaseFolder(t, digest, digest)
	status, said := guidedRunIn(t, fake, world, "yes\n"+testDestination+"\n", "release", "v1.0.0", "--folder", folder)
	if status != exitSuccess {
		t.Fatalf("the first release = %d: %s", status, said)
	}
	world.assets[fmt.Sprintf(recordAsset, "v1.0.1")] = nextRecordFrom(t, world, "v1.0.1", digest,
		[]string{"elsewhere.example/orpheum"})
	world.digests["elsewhere.example/orpheum:v1.0.1"] = digest
	signsBefore := fake.signs
	status, said = guidedRunIn(t, fake, world, "yes\n", "release", "v1.0.1", "--folder", folder)
	if status != exitFailure || !strings.Contains(said, "the last release signed was pulled from") ||
		fake.signs != signsBefore || strings.Contains(said, "the registry:") {
		t.Fatalf("another registry: %d %s (%d signs); want refused before the registry is asked", status, said,
			fake.signs-signsBefore)
	}
	status, said = guidedRunIn(t, fake, world, "yes\n", "release", "v1.0.1", "--folder", folder, "--new-registry")
	if status != exitSuccess || !strings.Contains(said, "as you said") || fake.signs == signsBefore {
		t.Fatalf("a move said: %d %s; want signed", status, said)
	}
}

// nextRecord is the record after the one served, releasing tag at digest.
func nextRecord(t *testing.T, world *fakeWorld, tag, digest string) []byte {
	t.Helper()
	return nextRecordFrom(t, world, tag, digest, nil)
}

// nextRecordFrom is the record after the one served, releasing tag at digest pulled from pulls (the
// served record's when nil).
func nextRecordFrom(t *testing.T, world *fakeWorld, tag, digest string, pulls []string) []byte {
	t.Helper()
	served, err := release.Parse(world.assets[fmt.Sprintf(recordAsset, "v1.0.0")])
	if err != nil {
		t.Fatal(err)
	}
	next, err := served.Released(tag, digest, "", pulls, served.Issued)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "record")
	if err = release.Write(path, next); err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

// A server that serves another record than the one signed (the files copied to a folder it does not
// serve) fails the run and says what it serves; nothing is committed.
func TestRelease_ReadBackRefusesWhatIsNotTheSignedRecord(t *testing.T) {
	t.Parallel()
	digest := "sha256:" + strings.Repeat("ab12", 16)
	folder, fake, world := releaseFolder(t, digest, digest)
	older := filepath.Join(world.served, "orpheum")
	if err := os.MkdirAll(older, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(older, "release"), []byte("an older record\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	world.landing = t.TempDir()
	status, said := guidedRunIn(t, fake, world, "yes\n"+testDestination+"\n", "release", "v1.0.0", "--folder", folder)
	if status != exitFailure || !strings.Contains(said, "is not the file just signed") || len(world.gits) != 0 {
		t.Fatalf("another record served: %d %s, git %v; want refused, nothing committed", status, said, world.gits)
	}
}

// Every command but the ceremony refuses a folder the ceremony did not make before it asks anything,
// and makes no folder.
func TestGuided_RefusesAFolderTheCeremonyDidNotMake(t *testing.T) {
	t.Parallel()
	for _, arguments := range [][]string{{"release", "v1.0.0"}, {"suspend"}, {"add"}} {
		folder := filepath.Join(t.TempDir(), "typo")
		status, said := guidedRun(t, newFakeKeys(t), "", append(arguments, "--folder", folder)...)
		if status != exitFailure || !strings.Contains(said, "holds no "+stateFile) || strings.Contains(said, "?") {
			t.Errorf("%v in a folder the ceremony did not make: %d %s; want refused before a question",
				arguments, status, said)
		}
		if _, err := os.Stat(folder); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%v made the folder %s: %v", arguments, folder, err)
		}
	}
}

// The registry's digest is asked as docker pull asks: anonymously, with the token its challenge
// offers, for the multi-architecture index.
func TestNetwork_DigestTakesTheRegistrysAnonymousToken(t *testing.T) {
	t.Parallel()
	const digest = "sha256:" + "ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12"
	var registry *httptest.Server
	registry = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/token":
			if request.URL.Query().Get("scope") != "repository:owner/orpheum:pull" {
				http.Error(writer, "scope", http.StatusBadRequest)
				return
			}
			if _, err := writer.Write([]byte(`{"token":"anonymous","expires_in":300}`)); err != nil {
				t.Error(err)
			}
		case "/v2/owner/orpheum/manifests/v1.0.0":
			if request.Header.Get("Authorization") != "Bearer anonymous" {
				writer.Header().Set("Www-Authenticate", `Bearer realm="`+registry.URL+
					`/token",service="registry.example",scope="repository:owner/orpheum:pull"`)
				writer.WriteHeader(http.StatusUnauthorized)
				return
			}
			if !strings.Contains(request.Header.Get("Accept"), "application/vnd.oci.image.index.v1+json") {
				http.Error(writer, "accept", http.StatusNotAcceptable)
				return
			}
			writer.Header().Set("Docker-Content-Digest", digest)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer registry.Close()
	world := network{client: registry.Client(), scheme: "http"}
	got, err := world.Digest(strings.TrimPrefix(registry.URL, "http://")+"/owner/orpheum", "v1.0.0")
	if err != nil || got != digest {
		t.Fatalf("Digest = %q, %v; want %s", got, err, digest)
	}
	if _, err = world.Digest(strings.TrimPrefix(registry.URL, "http://")+"/owner/orpheum", "v9.9.9"); err == nil {
		t.Error("a tag the registry does not hold: want an error")
	}
}
