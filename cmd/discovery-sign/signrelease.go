// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/kstenerud/orpheum/discovery/release"
	"github.com/kstenerud/orpheum/discovery/trust"
)

// signRelease shows an unsigned release record in plain words, asks the owner to confirm its digest,
// signs it with an everyday key of the publisher chain, checks it, keeps it and writes the files to
// publish.
func (guide *guided) signRelease(arguments []string) error {
	flags := flag.NewFlagSet("discovery-sign sign-release", flag.ContinueOnError)
	flags.SetOutput(guide.err)
	where := flags.String("folder", "", "the records folder (required)")
	recordFile := flags.String("record", "", "the release's unsigned record (required)")
	if flags.Parse(arguments) != nil || flags.NArg() != 0 || *where == "" || *recordFile == "" {
		return flag.ErrHelp
	}
	place, err := openFolder(*where)
	if err != nil {
		return err
	}
	record, err := release.Read(*recordFile)
	if err != nil {
		return err
	}
	digest := record.Images[record.Latest]
	if record.Latest == "" || digest == "" {
		return errors.New("the record names no release")
	}
	say := guide.say
	say.sayf("This release record says:\n  version:  %s\n  digest:   %s\n  expires:  %s", record.Latest, digest,
		trust.TimeText(record.Expires))
	say.sayf("%s", securityNote(record))
	say.sayf("The digest is the fingerprint of the exact software cores will install. Compare it with the one the\n" +
		"release build printed (the workflow's summary page); it must be the same, character for character.")
	typed, err := say.ask("To confirm, type the first 12 characters of the digest after \"sha256:\"", "")
	if err != nil {
		return err
	}
	if typed == "" || !strings.HasPrefix(strings.TrimPrefix(digest, "sha256:"), typed) || len(typed) < 12 {
		return errors.New("that is not the start of the record's digest, so nothing was signed")
	}
	return guide.signRecord(place, record)
}

// securityNote says whether the record's newest release fixes a security problem.
func securityNote(record release.Record) string {
	for _, fix := range record.Pending {
		if fix.FixedIn == record.Latest && fix.Summary != "" {
			return "  security: this release fixes: " + fix.Summary +
				"\n            every older version is told to update."
		}
	}
	return "  security: none (an ordinary release)"
}

// signRecord signs, checks and keeps record, and publishes.
func (guide *guided) signRecord(place *folder, record release.Record) error {
	records := place.ledger()
	chain, isHeld, err := records.chain(trust.ChainPublisher)
	if err != nil || !isHeld {
		return errors.Join(err, errors.New("the folder's ledger holds no publisher chain: run the ceremony first"))
	}
	everyday := place.state.chainKeys(trust.ChainPublisher, trust.RoleEveryday)
	if len(everyday) == 0 {
		return errors.New("the folder holds no publisher everyday key")
	}
	names := make([]string, len(everyday))
	for index, key := range everyday {
		names[index] = key.Name
	}
	chosen := everyday[0].Name
	if len(everyday) > 1 {
		if chosen, err = guide.say.choose("Which everyday key signs it?", names, chosen); err != nil {
			return err
		}
	}
	key := everyday[slices.Index(names, chosen)]
	payload, err := record.Payload()
	if err != nil {
		return err
	}
	path := records.path("pending", fmt.Sprintf("release-%d-%d", record.RootVersion, record.Sequence))
	if mkdirErr := os.MkdirAll(filepath.Dir(path), 0o700); mkdirErr != nil {
		return mkdirErr
	}
	if writeErr := os.WriteFile(path, payload, 0o600); writeErr != nil {
		return writeErr
	}
	signed, err := guide.signWith(place, trust.NamespaceRelease, path, []keyEntry{key}, func(one trust.Signed) error {
		_, openErr := release.Open(one, chain)
		return openErr
	})
	if err != nil {
		return err
	}
	run := signing{now: guide.now, ledger: records, stdout: io.Discard}
	if err := guide.accept(run, string(trust.ChainPublisher), kindRelease, signed); err != nil {
		return err
	}
	if err := records.publish(place.join(publishDirectory)); err != nil {
		return err
	}
	guide.say.sayf("Signed, checked and kept. Copy the contents of %s to the server, then check the watcher.",
		place.join(publishDirectory))
	return nil
}
