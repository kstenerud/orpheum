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
	identity := flags.String("id", "", "the publisher's identity, op1-...: needed for a record made new")
	rootVersion := flags.Int64("root-version", 0, "the publisher chain's version the record is signed under; "+
		"omitted, the record's own")
	var pulls, locations []string
	flags.Func("pull", "an image repository the release may be pulled from, registry.example/orpheum; "+
		"repeated; omitted, the record's own", appendTo(&pulls))
	flags.Func("location", "a place the record is served; repeated; omitted, the record's own", appendTo(&locations))
	if err := flags.Parse(arguments); err != nil || *path == "" || *tag == "" || *digest == "" || flags.NArg() != 0 {
		return write(stderr, "usage: release-record --record <file> --version <tag> --digest <sha256:...> "+
			"[--security <summary>] [--id <op1-...>] [--root-version <n>] [--pull <repository>]... "+
			"[--location <url>]...\n", exitUsage)
	}
	record, err := release.Read(*path)
	if err == nil {
		if *identity != "" {
			record.ID = *identity
		}
		if *rootVersion > 0 {
			record.RootVersion = *rootVersion
		}
		if len(locations) > 0 {
			record.Locations = locations
		}
		record, err = record.Released(*tag, *digest, *security, pulls, now)
	}
	if err == nil {
		err = release.Write(*path, record)
	}
	if err != nil {
		return write(stderr, fmt.Sprintf("release-record: %v\n", err), exitFailure)
	}
	return write(stdout, fmt.Sprintf("%s: latest %s, sequence %d; %d releases named, %d with a security fix pending\n"+
		"Check the digest, then sign it with an everyday key of the publisher root:\n"+
		"  ssh-keygen -Y sign -n %s -f <the key's stub> %s\n"+
		"and check, keep and publish it:\n"+
		"  go run ./cmd/discovery-sign check --ledger <ledger> --kind release --payload %s --publish <dir>\n",
		*path, record.Latest, record.Sequence, len(record.Images), len(record.Pending), trust.NamespaceRelease, *path,
		*path), exitSuccess)
}

// appendTo is a flag that appends each value it is given to values.
func appendTo(values *[]string) func(string) error {
	return func(value string) error {
		*values = append(*values, value)
		return nil
	}
}

// write writes text and returns status, or a failure when text could not be written.
func write(writer io.Writer, text string, status int) int {
	if _, err := io.WriteString(writer, text); err != nil {
		return exitFailure
	}
	return status
}
