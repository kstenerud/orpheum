// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/kstenerud/orpheum/discovery/addressdoc"
	"github.com/kstenerud/orpheum/discovery/release"
	"github.com/kstenerud/orpheum/discovery/trust"
	"github.com/kstenerud/orpheum/pkg/strictjson"
)

// recordAsset is where the release workflow attaches each release's unsigned record, by its tag
// (.github/workflows/release-image.yml).
const recordAsset = "https://github.com/kstenerud/orpheum/releases/download/%s/release-record"

// The folder's own notes no repository should hold: where the published files are copied to, a
// server's address, kept out of git by the folder's own .gitignore.
const (
	localDirectory = "local"
	publishToFile  = "publish-to"
)

// releaseRun is one run of the release command: the folder, the record and what it is checked by.
type releaseRun struct {
	place   *folder
	version string
	record  release.Record
}

// signRelease is the release command: it fetches the release's unsigned record (or reads one given),
// shows its digest beside the one the image registry serves for the tag, signs on the owner's yes,
// copies the published files to the server, reads the record back from where cores read it, and
// commits and pushes the folder; a push that fails is said, not a failure.
func (guide *guided) signRelease(arguments []string) error {
	flags := flag.NewFlagSet("discovery-sign release", flag.ContinueOnError)
	flags.SetOutput(guide.err)
	where := flags.String("folder", "", "the records folder (required)")
	recordFile := flags.String("record", "", "the release's unsigned record, when not fetched from its GitHub release")
	if flags.Parse(arguments) != nil {
		return flag.ErrHelp
	}
	// The version may come before the flags or after them.
	version := flags.Arg(0)
	if flags.NArg() > 0 && flags.Parse(flags.Args()[1:]) != nil {
		return flag.ErrHelp
	}
	if *where == "" || version == "" || flags.NArg() != 0 {
		return flag.ErrHelp
	}
	place, err := openRecords(*where)
	if err != nil {
		return err
	}
	record, err := guide.unsignedRecord(version, *recordFile)
	if err != nil {
		return err
	}
	run := releaseRun{place: place, version: version, record: record}
	if isSigned, confirmErr := guide.confirmRelease(run); confirmErr != nil || !isSigned {
		return confirmErr
	}
	for _, step := range []func(releaseRun) error{guide.signRun, guide.copyPublished, guide.readBack} {
		if err := step(run); err != nil {
			return err
		}
	}
	guide.keepInGit(run)
	return nil
}

// signRun signs the run's record.
func (guide *guided) signRun(run releaseRun) error {
	return guide.signRecord(run.place, run.record)
}

// unsignedRecord is the release's unsigned record: the file given, or the one its GitHub release
// carries.
func (guide *guided) unsignedRecord(version, file string) (release.Record, error) {
	var payload []byte
	var err error
	if file != "" {
		payload, err = os.ReadFile(file) //nolint:gosec // the owner names it
	} else {
		address := fmt.Sprintf(recordAsset, version)
		guide.say.sayf("Fetching the release record from %s", address)
		payload, err = guide.world.Fetch(address)
	}
	if err != nil {
		return release.Record{}, fmt.Errorf("the release record: %w", err)
	}
	record, err := release.Parse(payload)
	if err != nil {
		return release.Record{}, fmt.Errorf("the release record: %w", err)
	}
	if record.Latest != version {
		return release.Record{}, fmt.Errorf("the record's latest release is %q, not %s, so nothing was signed",
			record.Latest, version)
	}
	return record, nil
}

// confirmRelease shows the release, its digest from the record and from the registry, and asks the
// owner whether to sign it; it refuses digests that differ, or a registry that cannot be asked.
func (guide *guided) confirmRelease(run releaseRun) (bool, error) {
	record := run.record
	digest := record.Images[record.Latest]
	if digest == "" || len(record.Pulls) == 0 {
		return false, errors.New("the record names no image for its latest release, so nothing was signed")
	}
	say := guide.say
	say.sayf("This release record says:\n  version:  %s\n  expires:  %s\n%s", record.Latest,
		trust.TimeText(record.Expires), securityNote(record))
	say.sayf("The digest is the fingerprint of the exact software cores will install:")
	say.sayf("  the record:   %s", digest)
	for _, pull := range record.Pulls {
		served, err := guide.world.Digest(pull, record.Latest)
		if err != nil {
			return false, fmt.Errorf("could not ask the registry for %s:%s, so nothing was signed: %w",
				pull, record.Latest, err)
		}
		say.sayf("  the registry: %s  (%s:%s)", served, pull, record.Latest)
		if served != digest {
			return false, errors.New("the registry serves another image under this tag than the record names, " +
				"so nothing was signed: do not sign it, and find out why")
		}
	}
	say.sayf("They are the same.")
	isYes, err := say.yes(fmt.Sprintf("Sign %s?", record.Latest), false)
	if err != nil || !isYes {
		if err == nil {
			say.sayf("Nothing was signed.")
		}
		return false, err
	}
	return true, nil
}

// copyPublished copies the published files to the server, at the destination the folder keeps, asked
// for the first time.
func (guide *guided) copyPublished(run releaseRun) error {
	destination, err := guide.publishTo(run.place)
	if err != nil {
		return err
	}
	out := run.place.join(publishDirectory)
	entries, err := os.ReadDir(out)
	if err != nil {
		return err
	}
	sources := make([]string, 0, len(entries))
	for _, entry := range entries {
		sources = append(sources, filepath.Join(out, entry.Name()))
	}
	guide.say.sayf("Copying %s to %s", out, destination)
	if err = guide.world.Copy(sources, destination); err != nil {
		return fmt.Errorf("copying to the server: %w; it is signed and kept, so run this again once it "+
			"can copy", err)
	}
	return nil
}

// publishTo is where the published files are copied: the folder's note, or the owner's answer, kept.
func (guide *guided) publishTo(place *folder) (string, error) {
	note := place.join(localDirectory, publishToFile)
	kept, err := os.ReadFile(note) //nolint:gosec // the owner's own folder
	if err == nil && strings.TrimSpace(string(kept)) != "" {
		return strings.TrimSpace(string(kept)), nil
	}
	guide.say.sayf("Where are the published files copied to? An scp target: the server, and the folder it "+
		"serves as /.well-known/ (user@host:/path/). It is kept in %s, which git leaves out.", note)
	answer, err := guide.say.ask("Copy to", "")
	if err != nil {
		return "", err
	}
	if !strings.Contains(answer, ":") {
		return "", fmt.Errorf("%q is not an scp target (user@host:/path/)", answer)
	}
	if !strings.HasSuffix(answer, "/") {
		answer += "/"
	}
	local := place.join(localDirectory)
	if mkdirErr := os.MkdirAll(local, 0o700); mkdirErr != nil {
		return "", mkdirErr
	}
	// Git leaves the folder out by its own .gitignore, whatever the records folder's says.
	if ignoreErr := os.WriteFile(filepath.Join(local, ".gitignore"), []byte("*\n"), 0o600); ignoreErr != nil {
		return "", ignoreErr
	}
	if noteErr := os.WriteFile(note, []byte(answer+"\n"), 0o600); noteErr != nil {
		return "", noteErr
	}
	return answer, nil
}

// readBack reads the record and its signature from each place cores read it, and refuses one that is
// not what was published.
func (guide *guided) readBack(run releaseRun) error {
	locations := slices.Clone(run.record.Locations)
	if len(locations) == 0 && run.place.state.Address != nil {
		locations = []string{run.place.state.Address.Issuer}
	}
	if len(locations) == 0 {
		guide.say.sayf("The record names no place it is served at, so it was not read back: check it yourself.")
		return nil
	}
	out := run.place.join(publishDirectory)
	for _, location := range locations {
		for _, name := range []string{release.RecordPath, release.RecordPath + addressdoc.SignatureEnd} {
			published := filepath.Join(out, filepath.FromSlash(strings.TrimPrefix(name, trust.WellKnownPrefix)))
			want, err := os.ReadFile(published) //nolint:gosec // the owner's own folder
			if err != nil {
				return err
			}
			address := strings.TrimSuffix(location, "/") + name
			served, err := guide.world.Fetch(address)
			if err != nil {
				return fmt.Errorf("reading it back: %w", err)
			}
			if !bytes.Equal(served, want) {
				return fmt.Errorf("%s is not the file just signed: %s; was it copied to the folder served "+
					"as /.well-known/? (%s)", address, servedSequence(served),
					run.place.join(localDirectory, publishToFile))
			}
		}
		guide.say.sayf("Read back from %s: it serves %s.", location, run.version)
	}
	return nil
}

// servedSequence says which record a server serves, for a refusal.
func servedSequence(served []byte) string {
	record, err := release.Parse(served)
	if err != nil {
		return "it serves something else"
	}
	return fmt.Sprintf("it serves sequence %d, latest %s", record.Sequence, record.Latest)
}

// keepInGit commits the folder and pushes it. A failure is said, never the run's: the release is
// signed and published by then.
func (guide *guided) keepInGit(run releaseRun) {
	folder := run.place.path
	say := guide.say
	if err := guide.world.Git(folder, "add", "-A"); err != nil {
		say.sayf("Not committed (%v): commit %s yourself.", err, folder)
		return
	}
	if err := guide.world.Git(folder, "commit", "-m", "Release "+run.version); err != nil {
		say.sayf("Not committed (%v): commit %s yourself.", err, folder)
		return
	}
	if err := guide.world.Git(folder, "push"); err != nil {
		say.sayf("Committed, but the push failed (%v). The release is published; push %s once that is "+
			"mended.", err, folder)
		return
	}
	say.sayf("Committed and pushed. %s is published.", run.version)
}

// tokenOf is the token in a registry's token answer.
func tokenOf(body []byte) (string, error) {
	var answer struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"` //nolint:tagliatelle // the registry's name
	}
	// A registry's answer is another system's (D458), with fields beside the token.
	if err := strictjson.LastWins.Unmarshal(body, &answer); err != nil {
		return "", fmt.Errorf("the registry's token: %w", err)
	}
	if answer.Token != "" {
		return answer.Token, nil
	}
	if answer.AccessToken != "" {
		return answer.AccessToken, nil
	}
	return "", errors.New("the registry's token answer holds no token")
}
