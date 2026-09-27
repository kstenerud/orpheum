// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/identify/tmdb"
	"github.com/kstenerud/orpheum/internal/node/confine"
)

// version is the release this binary was built from. The Makefile sets it with
// -ldflags "-X main.version=<version>"; an unset build says "dev".
var version = "dev"

// Exit statuses. A usage error and a planned-but-unbuilt subcommand both exit 2, so a script
// cannot mistake either for success.
const (
	exitSuccess      = 0
	exitFailure      = 1
	exitWriteFailure = 1
	exitUsageError   = 2
)

// command runs one subcommand and returns its exit status.
type command func(ctx context.Context, call invocation) int

// subcommand is one planned role or operation of the binary (cut-a §3). A nil run is planned
// and not built yet.
type subcommand struct {
	name    string
	summary string
	run     command
}

// plannedSubcommands lists every subcommand cut-a §3 names, in the order the usage shows them.
func plannedSubcommands() []subcommand {
	planned := []subcommand{
		{name: "run", summary: "start the core, which supervises the node as a child process", run: commandRun},
		{name: "core", summary: "run the core alone"},
		{name: "node", summary: "run a media node: the child `run` starts, or a node alone", run: commandNode},
		{name: "migrate", summary: "migrate the database without starting the server", run: commandMigrate},
		{name: "backup", summary: "write a backup of the database", run: commandBackup},
		{name: "restore", summary: "restore a backup, with the server stopped", run: commandRestore},
		{name: "doctor", summary: "report what is wrong and how to fix it", run: commandDoctor},
		{name: "setup-code", summary: "print the one-time code that creates the first admin", run: commandSetupCode},
		{name: "openapi", summary: "print the API's OpenAPI document", run: commandOpenAPI},
	}
	if isDevBuild {
		planned = append(planned, subcommand{
			name:    "dev",
			summary: "development only: seed the dev database from dev/seed.toml (make dev)",
			run:     devCommand(),
		})
	}
	return planned
}

func main() {
	if status, isShim := shimIfAsked(os.Args, os.Stderr); isShim {
		os.Exit(status)
	}
	useUTC()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	api.UseErrorShape()
	status := run(ctx, os.Args[1:], environment(), os.Stdout, os.Stderr)
	stop()
	os.Exit(status)
}

// shimIfAsked makes this process the shim when its arguments ask for it, before anything else
// runs (cut-a §7): it confines itself and becomes an FFmpeg tool, so it returns only when it could
// not, with why on stderr and the status to exit with. It reports false for any other arguments.
func shimIfAsked(arguments []string, stderr io.Writer) (int, bool) {
	if len(arguments) != 3 || arguments[1] != confine.ShimArgument {
		return exitSuccess, false
	}
	return write(stderr, fmt.Sprintf("orpheum: %v\n", confine.Shim(arguments[2])), exitFailure), true
}

// useUTC makes every time the server formats UTC, whatever TZ says: the server's times are
// configuration-free, and a client formats them for its viewer (D204).
func useUTC() {
	time.Local = time.UTC
}

// run executes one invocation and returns its exit status. It takes the environment and its
// streams as parameters so the test can observe them.
func run(
	ctx context.Context, arguments []string, lookupEnv func(string) (string, bool), stdout, stderr io.Writer,
) int {
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
		if planned.name != name {
			continue
		}
		if planned.run == nil {
			return write(stderr, fmt.Sprintf("orpheum %s: not implemented yet\n", name), exitUsageError)
		}
		call := invocation{arguments: arguments[1:], lookupEnv: lookupEnv, stdout: stdout, stderr: stderr}
		return planned.run(ctx, call)
	}
	return write(stderr, fmt.Sprintf("orpheum: unknown subcommand %q\n\n", name)+usage(), exitUsageError)
}

// commandRun starts the server with the config file.
func commandRun(ctx context.Context, call invocation) int {
	config, configReport, err := call.configFlags("run", nil)
	if err != nil {
		return call.fail(err)
	}
	logger, err := call.logger(config)
	if err != nil {
		return call.fail(err)
	}
	for _, note := range configReport.Notes() {
		logger.WarnContext(ctx, "the config file needs attention; doctor lists it too", "note", note)
	}
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", config.Server.Listen)
	if err != nil {
		return call.fail(fmt.Errorf("listen on %s: %w", config.Server.Listen, err))
	}
	outside := wiring{
		startNode: selfStarter(config, call.stderr), tmdbBaseURL: tmdb.BaseURL, tmdbImagesBaseURL: tmdb.ImageBaseURL,
	}
	if err = serve(ctx, config, configReport, listener, outside, logger); err != nil {
		return exitFailure
	}
	return exitSuccess
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
