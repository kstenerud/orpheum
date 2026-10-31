// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/kstenerud/orpheum/discovery/addressdoc"
	"github.com/kstenerud/orpheum/discovery/release"
	"github.com/kstenerud/orpheum/discovery/trust"
	"github.com/kstenerud/orpheum/discovery/trust/trusttest"
)

// owner is the owner at the Mac: a ledger, and keys whose .pub files are in a directory.
type owner struct {
	t      *testing.T
	ledger string
	keys   string
	now    time.Time
}

func newOwner(t *testing.T) *owner {
	t.Helper()
	return &owner{t: t, ledger: t.TempDir(), keys: t.TempDir(), now: trusttest.Issued()}
}

// pub writes key's .pub file, labelled, and returns its path.
func (person *owner) pub(key trusttest.Key) string {
	person.t.Helper()
	path := filepath.Join(person.keys, key.Label+".pub")
	line := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key.PublicKey()))) + " " + key.Label + "\n"
	if err := os.WriteFile(path, []byte(line), 0o600); err != nil {
		person.t.Fatal(err)
	}
	return path
}

// runs has the tool run, its ledger given, and returns its status and what it said.
func (person *owner) runs(arguments ...string) (int, string) {
	person.t.Helper()
	var stdout, stderr bytes.Buffer
	arguments = append(arguments[:1:1], append([]string{"--ledger", person.ledger}, arguments[1:]...)...)
	status := run(arguments, person.now, &stdout, &stderr)
	return status, stdout.String() + stderr.String()
}

// does runs the tool and fails the test unless it succeeds.
func (person *owner) does(arguments ...string) string {
	person.t.Helper()
	status, said := person.runs(arguments...)
	if status != exitSuccess {
		person.t.Fatalf("discovery-sign %v = %d: %s", arguments, status, said)
	}
	return said
}

// signs has keys sign the pending payload name in namespace with keys, as ssh-keygen -Y sign writes it, and
// returns the payload's path.
func (person *owner) signs(name, namespace string, keys ...trusttest.Key) string {
	person.t.Helper()
	path := filepath.Join(person.ledger, "pending", name)
	payload, err := os.ReadFile(path)
	if err != nil {
		person.t.Fatal(err)
	}
	signed := trusttest.Sign(person.t, namespace, payload, keys...)
	if err = os.WriteFile(path+".sig", []byte(signed.Signature), 0o600); err != nil {
		person.t.Fatal(err)
	}
	return path
}

// The owner's day one and a rotation, through the tool (the root key audit's §3.6, §3.7, §4): the
// discovery's genesis signed by e1, checked and kept; the address document; e1 lost, replaced by e3
// with e2 and e3 signing; the publisher's genesis and a record; and what is published is what a core
// follows to the new set, from the genesis.
func TestSign_TheOwnersDayOneAndARotationArePublishedAsACoreFollowsThem(t *testing.T) {
	t.Parallel()
	person := newOwner(t)
	carried, home := trusttest.HardwareKey(t, "e1"), trusttest.HardwareKey(t, "e2")
	bank := trusttest.HardwareKey(t, "r1")
	said := person.does("genesis", "--chain", "discovery", "--everyday", person.pub(carried), "--everyday",
		person.pub(home), "--recovery", person.pub(bank), "--location", "https://discovery.example")
	if !strings.Contains(said, "ssh-keygen -Y sign -n orpheum-discovery-root-v1") ||
		!strings.Contains(said, "key everyday uv sk-ssh-ed25519@openssh.com") {
		t.Errorf("the genesis said %q; want the payload and the command that signs it", said)
	}
	person.does("check", "--kind", "root", "--payload",
		person.signs("discovery-root-1", trust.NamespaceDiscoveryRoot, carried, home, bank))
	person.does("address", "--issuer", "discovery.example", "--name", "Discovery",
		"--core-api", "https://api.discovery.example/core/v1",
		"--client-api", "https://api.discovery.example/client/v1",
		"--loader-origin", "https://discovery.example", "--client-origins", "https://{core}.client.example",
		"--location", "https://discovery.example")
	person.does("check", "--kind", "address", "--payload",
		person.signs("address-1-1", trust.NamespaceDiscoveryAddress, carried))

	replacement := trusttest.HardwareKey(t, "e3")
	said = person.does("root", "--chain", "discovery", "--remove", person.pub(carried),
		"--add-everyday", person.pub(replacement))
	if !strings.Contains(said, "- key everyday uv") || !strings.Contains(said, "+ removed SHA256:") ||
		!strings.Contains(said, "an everyday or a recovery key") {
		t.Errorf("the rotation said %q; want what changed and who signs", said)
	}
	// carried signing its own removal is refused: it leaves, and no key of the new set signed.
	if status, refused := person.runs("check", "--kind", "root", "--payload",
		person.signs("discovery-root-2", trust.NamespaceDiscoveryRoot, carried)); status != exitFailure {
		t.Errorf("e1's own removal, signed by it alone: %d %q; want refused", status, refused)
	}
	person.does("check", "--kind", "root", "--payload",
		person.signs("discovery-root-2", trust.NamespaceDiscoveryRoot, home, replacement))

	publishing, pr1 := trusttest.HardwareKey(t, "p1"), trusttest.HardwareKey(t, "pr1")
	person.does("genesis", "--chain", "publisher", "--everyday", person.pub(publishing), "--recovery", person.pub(pr1))
	person.does("check", "--kind", "root", "--chain", "publisher", "--payload",
		person.signs("publisher-root-1", trust.NamespacePublisherRoot, publishing))
	publisherChain, _, err := ledger(person.ledger).chain(trust.ChainPublisher)
	if err != nil {
		t.Fatal(err)
	}
	record, err := release.Record{ID: publisherChain.ID(), RootVersion: 1}.Released("v1.0.0",
		"sha256:"+strings.Repeat("a", 64), "", []string{"registry.example/orpheum"}, person.now)
	if err != nil {
		t.Fatal(err)
	}
	recordPath := filepath.Join(person.ledger, "pending", "release")
	if err = release.Write(recordPath, record); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	person.does("check", "--kind", "release", "--payload", person.signs("release", trust.NamespaceRelease, publishing),
		"--publish", out)

	wantFollowedFrom(t, out)
	expected, err := trust.ReadExpected(filepath.Join(person.ledger, expectedFile))
	if err != nil || len(expected) != 8 {
		t.Errorf("the ledger expects %d signatures, %v; want each of the eight the owner made", len(expected), err)
	}
}

// wantFollowedFrom fails the test unless what is published in out is what a core holding the genesis
// alone follows to the new set, the document carried signed no longer holding, and the publisher's
// record opening.
func wantFollowedFrom(t *testing.T, out string) {
	t.Helper()
	genesis := readPublished(t, out, addressdoc.RootPath(1))
	core, err := trust.Genesis(trust.ChainDiscovery, genesis)
	if err != nil {
		t.Fatal(err)
	}
	core, _ = core.Follow([]trust.Signed{readPublished(t, out, addressdoc.RootLatestPath)})
	if core.Version() != 2 {
		t.Fatalf("the core follows the published chain to %d; want 2", core.Version())
	}
	document := readPublished(t, out, addressdoc.DocumentPath)
	if _, err = addressdoc.Open(document, core, "https://discovery.example"); err == nil {
		t.Error("the document carried signed holds after carried was removed; the owner must sign it again")
	}
	publisher, err := trust.Genesis(trust.ChainPublisher, readPublished(t, out, release.PublisherRootPath(1)))
	if err != nil {
		t.Fatal(err)
	}
	if opened, openErr := release.Open(readPublished(t, out, release.RecordPath), publisher); openErr != nil ||
		opened.Latest != "v1.0.0" {
		t.Errorf("the published record: %+v, %v", opened, openErr)
	}
}

// A record that changes a released version's digest is refused before it is kept (the audit's F11),
// and a drill names the keys that signed it.
func TestCheck_ARecordChangingADigestIsRefusedAndADrillNamesItsKeys(t *testing.T) {
	t.Parallel()
	person := newOwner(t)
	publishing, pr1 := trusttest.HardwareKey(t, "p1"), trusttest.HardwareKey(t, "pr1")
	person.does("genesis", "--chain", "publisher", "--everyday", person.pub(publishing), "--recovery", person.pub(pr1))
	person.does("check", "--kind", "root", "--chain", "publisher", "--payload",
		person.signs("publisher-root-1", trust.NamespacePublisherRoot, publishing))
	chain, _, err := ledger(person.ledger).chain(trust.ChainPublisher)
	if err != nil {
		t.Fatal(err)
	}
	first, err := release.Record{ID: chain.ID(), RootVersion: 1}.Released("v1.0.0", "sha256:"+strings.Repeat("a", 64),
		"", nil, person.now)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(person.ledger, "pending", "release")
	if err = release.Write(path, first); err != nil {
		t.Fatal(err)
	}
	person.does("check", "--kind", "release", "--payload", person.signs("release", trust.NamespaceRelease, publishing))
	changed := first
	changed.Sequence, changed.Images = 2, map[string]string{"v1.0.0": "sha256:" + strings.Repeat("b", 64)}
	if err = release.Write(path, changed); err != nil {
		t.Fatal(err)
	}
	if status, said := person.runs("check", "--kind", "release", "--payload",
		person.signs("release", trust.NamespaceRelease, publishing)); status != exitFailure ||
		!strings.Contains(said, "never changes") {
		t.Errorf("a changed digest: %d %q; want refused", status, said)
	}
	person.does("drill", "--chain", "publisher")
	entries, err := os.ReadDir(filepath.Join(person.ledger, "pending"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "drill-") && !strings.HasSuffix(entry.Name(), ".sig") {
			said := person.does("check", "--kind", "drill", "--chain", "publisher", "--payload",
				person.signs(entry.Name(), trust.NamespaceDrill, pr1))
			if !strings.Contains(said, trust.Fingerprint(pr1.PublicKey())) {
				t.Errorf("the drill said %q; want the recovery key named", said)
			}
		}
	}
}

func readPublished(t *testing.T, directory, path string) trust.Signed {
	t.Helper()
	name := filepath.Join(directory, filepath.FromSlash(strings.TrimPrefix(path, trust.WellKnownPrefix)))
	payload, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	signature, err := os.ReadFile(name + ".sig")
	if err != nil {
		t.Fatal(err)
	}
	return trust.Signed{Payload: string(payload), Signature: string(signature)}
}
