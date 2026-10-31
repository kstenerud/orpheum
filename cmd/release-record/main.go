// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/kstenerud/orpheum/discovery/release"
)

// Exit statuses: a usage error is told apart from a release refused.
const (
	exitSuccess = 0
	exitFailure = 1
	exitUsage   = 2
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run writes the release its arguments name into the record they name, and returns the exit status.
func run(arguments []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("release-record", flag.ContinueOnError)
	flags.SetOutput(stderr)
	path := flags.String("record", "", "the release record's file, made when absent (required)")
	tag := flags.String("version", "", "the release's tag, v0.3.2 (required)")
	digest := flags.String("digest", "", "the release image's digest, sha256:... (required)")
	security := flags.String("security", "",
		"what the release fixes, in a sentence for an admin: every older release lacking a fix is marked as "+
			"needing this one; omitted, a plain release")
	if err := flags.Parse(arguments); err != nil || *path == "" || *tag == "" || *digest == "" || flags.NArg() != 0 {
		return write(stderr, "usage: release-record --record <file> --version <tag> --digest <sha256:...> "+
			"[--security <summary>]\n", exitUsage)
	}
	record, err := release.Read(*path)
	if err == nil {
		record, err = record.Released(*tag, *digest, *security)
	}
	if err == nil {
		err = release.Write(*path, record)
	}
	if err != nil {
		return write(stderr, fmt.Sprintf("release-record: %v\n", err), exitFailure)
	}
	pending := 0
	for _, note := range record.Versions {
		if note.Security {
			pending++
		}
	}
	return write(stdout, fmt.Sprintf("%s: latest %s; %d older releases named, %d with a security fix pending\n",
		*path, record.Latest, len(record.Versions), pending), exitSuccess)
}

// write writes text and returns status, or a failure when text could not be written.
func write(writer io.Writer, text string, status int) int {
	if _, err := io.WriteString(writer, text); err != nil {
		return exitFailure
	}
	return status
}
