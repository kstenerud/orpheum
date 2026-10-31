// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/discovery/release"
	"github.com/kstenerud/orpheum/discovery/trust"
	"github.com/kstenerud/orpheum/discovery/trust/trusttest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// The fake tools the bootstrap calls: curl copies from the fake site, ssh-keygen says whether the
// signature holds as FAKE_VERIFY says, once it is given allowed signers for release records alone,
// and docker logs each call, its install script printing what it was given.
var fakeTools = map[string]string{
	"curl": `#!/bin/sh
url=$2; out=$4
path=${url#*://}; path=/${path#*/}
[ -f "$FAKE_SITE$path" ] || exit 22
cp "$FAKE_SITE$path" "$out"
`,
	"ssh-keygen": `#!/bin/sh
echo "ssh-keygen $*" >> "$FAKE_LOG"
grep -q 'namespaces="orpheum-release-v1" sk-ssh-ed25519@openssh.com' "$4" || exit 1
exit "${FAKE_VERIFY:-0}"
`,
	"docker": `#!/bin/sh
echo "docker $*" >> "$FAKE_LOG"
case $1 in run) echo 'echo installed "$@"' ;; esac
`,
}

// bootstrapWorld is a bootstrap rendered for a publisher set, the fake site serving its record, and
// the fake tools.
type bootstrapWorld struct {
	script, site, log string
	environment       []string
}

func newBootstrapWorld(t *testing.T, record string) bootstrapWorld {
	t.Helper()
	publisher := trusttest.NewChain(t, trust.ChainPublisher, []trusttest.Key{trusttest.HardwareKey(t, "p1")},
		[]trusttest.Key{trusttest.HardwareKey(t, "pr1")})
	script, err := renderBootstrap(publisher.Last, []string{"https://site.example"})
	if err != nil {
		t.Fatal(err)
	}
	world := bootstrapWorld{script: script, site: t.TempDir(), log: filepath.Join(t.TempDir(), "log")}
	tools := t.TempDir()
	for name, body := range fakeTools {
		tool := filepath.Join(tools, name)
		if err = os.WriteFile(tool, []byte(body), 0o700); err != nil { //nolint:gosec // a fake tool
			t.Fatal(err)
		}
	}
	served := filepath.Join(world.site, filepath.FromSlash(release.RecordPath))
	if err = os.MkdirAll(filepath.Dir(served), 0o700); err != nil {
		t.Fatal(err)
	}
	if record != "" {
		for path, contents := range map[string]string{served: record, served + ".sig": "a signature"} {
			if err = os.WriteFile(path, []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	world.environment = []string{
		"PATH=" + tools + ":/usr/bin:/bin", "FAKE_SITE=" + world.site, "FAKE_LOG=" + world.log, "TMPDIR=" + t.TempDir(),
	}
	return world
}

func (world bootstrapWorld) logged(t *testing.T) string {
	t.Helper()
	logged, err := os.ReadFile(world.log)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return string(logged)
}

const bootstrapRecord = "orpheum-signed release 1\nid op1-x\nroot-version 1\nsequence 3\n" +
	"issued 2026-11-01T12:00:00Z\nexpires 2026-12-01T12:00:00Z\nlatest v1.2.0\n" +
	"image v1.2.0 sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n" +
	"image v1.1.0 sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\n" +
	"pull registry.example/orpheum\n"

// The install verifies before it runs anything (the root key audit's §3.9, F10): it checks the
// record's signature by the publisher's keys, pulls the latest's image by its digest, and runs that
// image's install script with its arguments; a signature that does not hold, or no record, runs
// nothing.
func TestBootstrap_VerifiesTheRecordBeforeItRunsAnything(t *testing.T) {
	t.Parallel()
	world := newBootstrapWorld(t, bootstrapRecord)
	output, err := testutil.RunShell(t, world.script, []string{"--media", "/films"}, world.environment)
	image := "registry.example/orpheum@sha256:" + strings.Repeat("a", 64)
	if err != nil || !strings.Contains(string(output), "installed --image "+image+" --media /films") {
		t.Fatalf("a record that holds: %v, %q; want the latest's image installed by its digest", err, output)
	}
	logged := world.logged(t)
	verified := strings.Index(logged, "ssh-keygen -Y verify")
	pulled := strings.Index(logged, "docker pull --quiet "+image)
	if verified < 0 || pulled < verified || !strings.Contains(logged, "-n orpheum-release-v1") {
		t.Errorf("the log %q; want the signature checked in the release namespace before any pull", logged)
	}

	refused := newBootstrapWorld(t, bootstrapRecord)
	refused.environment = append(refused.environment, "FAKE_VERIFY=1")
	output, err = testutil.RunShell(t, refused.script, nil, refused.environment)
	if err == nil || strings.Contains(refused.logged(t), "docker") ||
		!strings.Contains(string(output), "does not hold") {
		t.Errorf("a signature that does not hold: %v, %q; want nothing run", err, output)
	}
	absent := newBootstrapWorld(t, "")
	if output, err = testutil.RunShell(t, absent.script, nil, absent.environment); err == nil ||
		strings.Contains(absent.logged(t), "docker") {
		t.Errorf("no record: %v, %q; want nothing run", err, output)
	}
	if !strings.Contains(world.script, `orpheum namespaces="orpheum-release-v1" sk-ssh-ed25519@openssh.com`) ||
		strings.Count(world.script, "orpheum namespaces=") != 2 {
		t.Error("the script does not hold the publisher's two keys for release records alone")
	}
}
