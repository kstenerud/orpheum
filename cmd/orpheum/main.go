// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// version is the release this binary was built from. The Makefile sets it with
// -ldflags "-X main.version=<version>"; an unset build says "dev".
var version = "dev"

// Exit statuses. A usage error and a planned-but-unbuilt subcommand both exit 2, so a script
// cannot mistake either for success.
const (
	exitSuccess      = 0
	exitWriteFailure = 1
	exitUsageError   = 2
)

// subcommand is one planned role or operation of the binary (cut-a §3).
type subcommand struct {
	name    string
	summary string
}

// plannedSubcommands lists every subcommand cut-a §3 names, in the order the usage shows them.
func plannedSubcommands() []subcommand {
	planned := []subcommand{
		{name: "run", summary: "start the core, which supervises the node as a child process"},
		{name: "core", summary: "run the core alone"},
		{name: "node", summary: "run a media node alone"},
		{name: "migrate", summary: "migrate the database without starting the server"},
		{name: "backup", summary: "write a backup of the database"},
		{name: "restore", summary: "restore a backup, with the server stopped"},
		{name: "doctor", summary: "report what is wrong and how to fix it"},
		{name: "setup-code", summary: "print the one-time code that creates the first admin"},
	}
	if isDevBuild {
		planned = append(planned, subcommand{
			name:    "dev",
			summary: "development only: seed the dev database from dev/seed.toml (make dev)",
		})
	}
	return planned
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run executes one invocation and returns its exit status. It takes its streams as
// parameters so the test can observe them.
func run(arguments []string, stdout, stderr io.Writer) int {
	if len(arguments) == 0 {
		return write(stderr, usage(), exitUsageError)
	}
	name := arguments[0]
	switch name {
	case "version", "--version":
		return write(stdout, version+"\n", exitSuccess)
	case "help", "-h", "--help":
		return write(stdout, usage(), exitSuccess)
	}
	for _, planned := range plannedSubcommands() {
		if planned.name == name {
			return write(stderr, fmt.Sprintf("orpheum %s: not implemented yet\n", name), exitUsageError)
		}
	}
	return write(stderr, fmt.Sprintf("orpheum: unknown subcommand %q\n\n", name)+usage(), exitUsageError)
}

// write sends text to output and returns status, or exitWriteFailure if the write failed: a
// command whose output was lost must not report success.
func write(output io.Writer, text string, status int) int {
	if _, err := io.WriteString(output, text); err != nil {
		return exitWriteFailure
	}
	return status
}

// usage is the help text listing every subcommand.
func usage() string {
	var text strings.Builder
	text.WriteString("Usage: orpheum <subcommand> [flags]\n\nSubcommands:\n")
	for _, planned := range plannedSubcommands() {
		fmt.Fprintf(&text, "  %-11s %s\n", planned.name, planned.summary)
	}
	fmt.Fprintf(&text, "  %-11s %s\n", "version", "print the version and exit")
	fmt.Fprintf(&text, "  %-11s %s\n", "help", "print this message and exit")
	return text.String()
}
