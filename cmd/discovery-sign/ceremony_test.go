// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/kstenerud/orpheum/discovery/addressdoc"
	"github.com/kstenerud/orpheum/discovery/release"
	"github.com/kstenerud/orpheum/discovery/trust"
	"github.com/kstenerud/orpheum/discovery/trust/trusttest"
)

// fakeKeys stands in for ssh-keygen: it makes keys in software (hardware ones marked as a YubiKey's
// are, signing with their PIN unless noPIN) and signs as ssh-keygen -Y sign writes, so no test runs
// ssh-keygen or touches a key.
type fakeKeys struct {
	t *testing.T
	// noPIN has the hardware keys it makes sign without the user-verified flag.
	noPIN bool

	mutex sync.Mutex
	keys  map[string]trusttest.Key
	makes int
	signs int
	// as makes a key file sign as another key.
	as map[string]trusttest.Key
}

func newFakeKeys(t *testing.T) *fakeKeys {
	t.Helper()
	return &fakeKeys{t: t, keys: map[string]trusttest.Key{}, as: map[string]trusttest.Key{}}
}

func (fake *fakeKeys) Make(spec keySpec) error {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.makes++
	var key trusttest.Key
	switch {
	case spec.Kind == kindSoftware:
		key = trusttest.PlainKey(fake.t, spec.Label)
	case fake.noPIN:
		key = trusttest.HardwareKeyWithoutPIN(fake.t, spec.Label)
	default:
		key = trusttest.HardwareKey(fake.t, spec.Label)
	}
	fake.keys[spec.File] = key
	line := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key.PublicKey()))) + " " + spec.Label + "\n"
	files := map[string]string{spec.File: "private\n", spec.File + ".pub": line}
	if spec.Attestation != "" {
		files[spec.Attestation] = "attestation\n"
	}
	for name, contents := range files {
		if err := os.MkdirAll(filepath.Dir(name), 0o750); err != nil {
			return err
		}
		if err := os.WriteFile(name, []byte(contents), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func (fake *fakeKeys) Sign(spec signSpec) error {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.signs++
	key, isMade := fake.keys[spec.KeyFile]
	if !isMade {
		fake.t.Fatalf("signing with a key never made: %s", spec.KeyFile)
	}
	if other, isOther := fake.as[spec.KeyFile]; isOther {
		key = other
	}
	payload, err := os.ReadFile(spec.Payload)
	if err != nil {
		return err
	}
	signed := trusttest.Sign(fake.t, spec.Namespace, payload, key)
	return os.WriteFile(spec.Payload+".sig", []byte(signed.Signature), 0o600)
}

// guidedRun runs a guided command with the answers, one line each, and returns its status and all it said.
func guidedRun(t *testing.T, keys keygen, answers string, arguments ...string) (int, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	status, isGuided := runGuided(arguments, trusttest.Issued(), strings.NewReader(answers), &stdout, &stderr, keys)
	if !isGuided {
		t.Fatalf("%v is not a guided command", arguments)
	}
	return status, stdout.String() + stderr.String()
}

// softwareCeremony is a ceremony of one everyday key and two recovery keys made as plain files.
func softwareCeremony(t *testing.T, fake *fakeKeys, folder, keysDirectory, answers string) (int, string) {
	t.Helper()
	return guidedRun(t, fake, answers, "ceremony", "--folder", folder, "--software", keysDirectory)
}

// wantPublished fails the test unless what the ceremony published in out is what a core follows from
// the genesis alone: both chains, the address document of issuer, with the signatures expected.
func wantPublished(t *testing.T, out, issuer string) {
	t.Helper()
	discovery, err := trust.Genesis(trust.ChainDiscovery, readPublished(t, out, addressdoc.RootPath(1)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = addressdoc.Open(readPublished(t, out, addressdoc.DocumentPath), discovery, issuer); err != nil {
		t.Errorf("the published address document does not open under the genesis: %v", err)
	}
	if _, err = trust.Genesis(trust.ChainPublisher, readPublished(t, out, release.PublisherRootPath(1))); err != nil {
		t.Errorf("the published publisher genesis: %v", err)
	}
	if _, err = os.Stat(filepath.Join(out, "install")); err != nil {
		t.Errorf("the install script is not published: %v", err)
	}
}

// A software ceremony, scripted from the first question to the last, makes both chains, signs and
// checks each, and publishes what a core follows; the private keys are outside the folder.
func TestCeremony_ASoftwareCeremonyPublishesWhatACoreFollows(t *testing.T) {
	t.Parallel()
	folder, keysDirectory := filepath.Join(t.TempDir(), "records"), t.TempDir()
	fake := newFakeKeys(t)
	status, said := softwareCeremony(t, fake, folder, keysDirectory, "\n2\nhttps://orpheum.dev\n\n")
	if status != exitSuccess {
		t.Fatalf("the ceremony = %d: %s", status, said)
	}
	wantPublished(t, filepath.Join(folder, publishDirectory), "https://orpheum.dev")
	expected, err := trust.ReadExpected(filepath.Join(folder, ledgerDirectory, expectedFile))
	if err != nil || len(expected) != 7 {
		t.Errorf("the ledger expects %d signatures, %v; want 3 + 1 + 3", len(expected), err)
	}
	if fake.makes != 6 {
		t.Errorf("%d keys made; want 3 for each of the two chains", fake.makes)
	}
	for _, name := range []string{"discovery-e1", "discovery-r2", "publisher-r1"} {
		if _, statErr := os.Stat(filepath.Join(keysDirectory, name)); statErr != nil {
			t.Errorf("the key %s is not in the keys directory: %v", name, statErr)
		}
		if _, statErr := os.Stat(filepath.Join(folder, "keys", name)); statErr == nil {
			t.Errorf("the private key %s is in the records folder", name)
		}
		if _, statErr := os.Stat(filepath.Join(folder, "keys", name+".pub")); statErr != nil {
			t.Errorf("the public key %s is not kept in the folder: %v", name, statErr)
		}
	}
	if !strings.Contains(said, "What is left for you to do by hand") {
		t.Errorf("the ceremony did not say what is left: %s", said)
	}
}

// A keys directory inside the records folder is refused: private keys would be committed with it.
func TestCeremony_AKeysDirectoryInsideTheFolderIsRefused(t *testing.T) {
	t.Parallel()
	folder := t.TempDir()
	status, said := softwareCeremony(t, newFakeKeys(t), folder, filepath.Join(folder, "secret"), "\n\n\n\n")
	if status != exitFailure || !strings.Contains(said, "inside the records folder") {
		t.Errorf("a keys directory in the folder: %d %q; want refused", status, said)
	}
}

// A hardware ceremony asks for each YubiKey in turn, makes a key per chain per device with its own
// application name, and says exactly the text to paste into suggested.go.
func TestCeremony_AHardwareCeremonyMakesKeysPerChainAndSaysWhatToPaste(t *testing.T) {
	t.Parallel()
	folder := filepath.Join(t.TempDir(), "records")
	fake := newFakeKeys(t)
	status, said := guidedRun(t, fake, strings.Repeat("\n", 100), "ceremony", "--folder", folder)
	if status != exitSuccess {
		t.Fatalf("the ceremony = %d: %s", status, said)
	}
	if fake.makes != 8 {
		t.Errorf("%d keys made; want 4 devices on 2 chains", fake.makes)
	}
	wantPublished(t, filepath.Join(folder, publishDirectory), "https://orpheum.cc")
	for _, want := range []string{"Genesis: trust.Signed{", "od1-", "op1-", "suggested.go"} {
		if !strings.Contains(said, want) {
			t.Errorf("the closing words lack %q", want)
		}
	}
	if _, err := os.Stat(filepath.Join(folder, "attestations", "discovery-e1.att")); err != nil {
		t.Errorf("the attestation was not kept: %v", err)
	}
	stubs, err := os.ReadDir(filepath.Join(folder, stubsDirectory))
	if err != nil || len(stubs) == 0 {
		t.Errorf("the hardware stubs are not in the stubs directory: %v", err)
	}
}

// An interrupted ceremony, run again, carries on: the keys already made are not made again.
func TestCeremony_AnInterruptedCeremonyResumesWithoutMakingKeysAgain(t *testing.T) {
	t.Parallel()
	folder, keysDirectory := filepath.Join(t.TempDir(), "records"), t.TempDir()
	fake := newFakeKeys(t)
	status, said := softwareCeremony(t, fake, folder, keysDirectory, "\n2\n")
	if status != exitFailure || !strings.Contains(said, "run the same command again") {
		t.Fatalf("the interrupted ceremony = %d: %s", status, said)
	}
	held := ledger(filepath.Join(folder, ledgerDirectory))
	if _, isHeld, err := held.chain(trust.ChainDiscovery); err != nil || isHeld {
		t.Fatalf("a chain was kept before the address was asked: %v", err)
	}
	made := fake.makes
	if made != 6 {
		t.Fatalf("%d keys made before the interruption; want 6", made)
	}
	status, said = softwareCeremony(t, fake, folder, keysDirectory, "https://orpheum.dev\n\n")
	if status != exitSuccess {
		t.Fatalf("the resumed ceremony = %d: %s", status, said)
	}
	if fake.makes != made {
		t.Errorf("%d keys made after resuming; want none made again", fake.makes-made)
	}
	wantPublished(t, filepath.Join(folder, publishDirectory), "https://orpheum.dev")
}

// A signature by another key than the one asked for is refused at its step, before any later key is
// asked or anything is kept.
func TestCeremony_AWrongSignatureIsRefusedBeforeTheNextStep(t *testing.T) {
	t.Parallel()
	folder, keysDirectory := filepath.Join(t.TempDir(), "records"), t.TempDir()
	fake := newFakeKeys(t)
	fake.as[filepath.Join(keysDirectory, "discovery-e1")] = trusttest.PlainKey(t, "someone-else")
	status, said := softwareCeremony(t, fake, folder, keysDirectory, "\n2\nhttps://orpheum.dev\n\nno\n")
	if status != exitFailure || !strings.Contains(said, "by another key") {
		t.Fatalf("a wrong signature: %d %s; want refused", status, said)
	}
	if fake.signs != 1 {
		t.Errorf("%d signatures asked; want the ceremony to stop after the first", fake.signs)
	}
	held := ledger(filepath.Join(folder, ledgerDirectory))
	if _, isHeld, err := held.chain(trust.ChainDiscovery); err != nil || isHeld {
		t.Errorf("the chain was kept after a wrong signature: %v", err)
	}
}

// A hardware key's signature made without its PIN is refused at its step.
func TestCeremony_ASignatureWithoutThePINIsRefused(t *testing.T) {
	t.Parallel()
	fake := newFakeKeys(t)
	fake.noPIN = true
	folder := filepath.Join(t.TempDir(), "records")
	status, said := guidedRun(t, fake, strings.Repeat("\n", 17)+"no\n", "ceremony", "--folder", folder)
	if status != exitFailure || !strings.Contains(said, "without the key's PIN") {
		t.Errorf("a signature without the PIN: %d %s; want refused", status, said)
	}
	if fake.signs != 1 {
		t.Errorf("%d signatures asked; want one", fake.signs)
	}
}

// releaseRecord is the unsigned record of tag at digest, as the release workflow writes it for the
// publisher chain in folder.
func releaseRecord(t *testing.T, folder, tag, digest string) string {
	t.Helper()
	chain, isHeld, err := ledger(filepath.Join(folder, ledgerDirectory)).chain(trust.ChainPublisher)
	if err != nil || !isHeld {
		t.Fatalf("no publisher chain: %v", err)
	}
	first := "sha256:" + strings.Repeat("cd", 32)
	older, err := release.Record{ID: chain.ID(), RootVersion: 1}.Released("v0.9.0", first, "",
		[]string{"registry.example/orpheum"}, trusttest.Issued())
	if err != nil {
		t.Fatal(err)
	}
	record, err := older.Released(tag, digest, "A flaw in sign-in.", nil, trusttest.Issued())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "record")
	if err = release.Write(path, record); err != nil {
		t.Fatal(err)
	}
	return path
}

// sign-release shows the release, signs it only once the owner types the digest's start, and
// publishes a record a core opens; a digest mistyped signs nothing.
func TestSignRelease_NeedsTheOwnersConfirmationOfTheDigest(t *testing.T) {
	t.Parallel()
	folder, keysDirectory := filepath.Join(t.TempDir(), "records"), t.TempDir()
	fake := newFakeKeys(t)
	answers := "\n\nhttps://orpheum.dev\n\n"
	if status, said := softwareCeremony(t, fake, folder, keysDirectory, answers); status != exitSuccess {
		t.Fatalf("the ceremony = %d: %s", status, said)
	}
	digest := "sha256:" + strings.Repeat("ab12", 16)
	record := releaseRecord(t, folder, "v1.0.0", digest)
	signsBefore := fake.signs
	status, said := guidedRun(t, fake, "ab12ab12ab99\n", "sign-release", "--folder", folder, "--record", record)
	if status != exitFailure || !strings.Contains(said, "nothing was signed") || fake.signs != signsBefore {
		t.Fatalf("a mistyped digest: %d %s (%d signs); want refused, nothing signed",
			status, said, fake.signs-signsBefore)
	}
	status, said = guidedRun(t, fake, "\n", "sign-release", "--folder", folder, "--record", record)
	if status != exitFailure || fake.signs != signsBefore {
		t.Fatalf("no confirmation: %d %s; want refused", status, said)
	}
	status, said = guidedRun(t, fake, "ab12ab12ab12\n", "sign-release", "--folder", folder, "--record", record)
	if status != exitSuccess || !strings.Contains(said, "A flaw in sign-in.") || !strings.Contains(said, "v1.0.0") {
		t.Fatalf("sign-release = %d: %s; want the release shown and signed", status, said)
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

// test-key reports plainly whether a key's signatures carry the user-verified flag.
func TestTestKey_ReportsWhetherTheSignatureCarriesTheUserVerifiedFlag(t *testing.T) {
	t.Parallel()
	for name, want := range map[string]struct {
		noPIN bool
		said  string
	}{
		"with":    {false, "DO carry the user-verified flag"},
		"without": {true, "do NOT carry the user-verified flag"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fake := newFakeKeys(t)
			fake.noPIN = want.noPIN
			status, said := guidedRun(t, fake, "\n", "test-key", "--dir", t.TempDir())
			if status != exitSuccess || !strings.Contains(said, want.said) {
				t.Errorf("test-key = %d: %s; want %q", status, said, want.said)
			}
		})
	}
}
