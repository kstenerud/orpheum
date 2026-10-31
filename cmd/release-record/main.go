// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/kstenerud/orpheum/discovery/release"
	"github.com/kstenerud/orpheum/discovery/trust"
	"github.com/kstenerud/orpheum/internal/platform/clock"
)

// Exit statuses: a usage error is told apart from a release refused.
const (
	exitSuccess = 0
	exitFailure = 1
	exitUsage   = 2
)

func main() {
	os.Exit(run(os.Args[1:], clock.System{}.Now(), os.Stdout, os.Stderr))
}

// run writes the release its arguments name into the record they name, at now, says how to sign it,
// and returns the exit status.
func run(arguments []string, now time.Time, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("release-record", flag.ContinueOnError)
	flags.SetOutput(stderr)
	path := flags.String("record", "", "the unsigned release record's file, made when absent (required)")
	tag := flags.String("version", "", "the release's tag, v0.3.2 (required)")
	digest := flags.String("digest", "", "the release image's digest, sha256:... (required)")
	security := flags.String("security", "",
		"what the release fixes, in a sentence for an admin: every older release lacking a fix is marked as "+
			"needing this one; omitted, a plain release")
	var locations []string
	flags.Func("location", "an image repository the release may be pulled from, registry.example/orpheum; "+
		"repeated for each mirror; omitted, the record's locations stay", func(location string) error {
		locations = append(locations, location)
		return nil
	})
	if err := flags.Parse(arguments); err != nil || *path == "" || *tag == "" || *digest == "" || flags.NArg() != 0 {
		return write(stderr, "usage: release-record --record <file> --version <tag> --digest <sha256:...> "+
			"[--security <summary>] [--location <repository>]...\n", exitUsage)
	}
	record, err := release.Read(*path)
	if err == nil {
		record, err = record.Released(*tag, *digest, *security, locations, now)
	}
	if err == nil {
		err = release.Write(*path, record)
	}
	if err != nil {
		return write(stderr, fmt.Sprintf("release-record: %v\n", err), exitFailure)
	}
	pending := 0
	for _, note := range record.Releases {
		if note.Security {
			pending++
		}
	}
	return write(stdout, fmt.Sprintf("%s: latest %s; %d releases named, %d with a security fix pending\n"+
		"Sign it with any one root key, a YubiKey's resident key or a key file:\n"+
		"  go run ./cmd/discovery-sign record --record %s --signer <key> --out <signed record>\n"+
		"or by hand, then put the record and its signature together:\n"+
		"  ssh-keygen -Y sign -n %s -f <key> %s\n"+
		"  go run ./cmd/discovery-sign seal --kind %s --payload %s --signature %s.sig --out <signed record>\n",
		*path, record.Latest, len(record.Releases), pending, *path, trust.KindReleaseRecord, *path,
		trust.KindReleaseRecord, *path, *path), exitSuccess)
}

// write writes text and returns status, or a failure when text could not be written.
func write(writer io.Writer, text string, status int) int {
	if _, err := io.WriteString(writer, text); err != nil {
		return exitFailure
	}
	return status
}
