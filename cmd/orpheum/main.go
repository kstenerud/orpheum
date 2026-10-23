// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/providers/cloudflare"
	"github.com/kstenerud/orpheum/internal/core/providers/opensubtitles"
	"github.com/kstenerud/orpheum/internal/core/providers/tmdb"
	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/platform/build"
)

// version is the build this binary is, in a word: its release tag, or "dev" and its commit, as
// Go recorded them when it was built (D225).
var version = build.This().String()

// thisBuild is what this binary was built from, with its own hash, so a node and the server can
// be told apart when they were built from one modified tree (D225).
func thisBuild() build.Provenance {
	built := build.This()
	self, err := os.Executable()
	if err != nil {
		return built
	}
	hash, err := fileSha256(self)
	if err != nil {
		return built
	}
	return built.WithBinarySha256(hash)
}

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

// subcommand is one planned role or operation of the binary (cut-a §3). A nil implementation is
// planned and not built yet.
type subcommand struct {
	name           string
	summary        string
	implementation command
}

// plannedSubcommands lists every subcommand cut-a §3 names, in the order the usage shows them.
func plannedSubcommands() []subcommand {
	planned := []subcommand{
		{
			name: "core", summary: "run the core: the server, its API and the node channel",
			implementation: commandCore,
		},
		{
			name: "node", summary: "run a media node, a service of its own beside the core (D233 §2)",
			implementation: commandNode,
		},
		{
			name: "default-config", summary: "make an instance directory, a core's or with --node a node's (D242)",
			implementation: commandDefaultConfig,
		},
		{
			name: "write-deployment", summary: "write a deployment's directory from its values file: " +
				"the compose file, the configs, the tunnel's keys (D272 §3)",
			implementation: commandWriteDeployment,
		},
		{
			name: "install-script", summary: "print the script a household installs a core with, from this image " +
				"(D314)",
			implementation: commandInstallScript,
		},
		{name: "migrate", summary: "migrate the database without starting the server", implementation: commandMigrate},
		{name: "backup", summary: "write a backup of the database", implementation: commandBackup},
		{name: "restore", summary: "restore a backup, with the server stopped", implementation: commandRestore},
		{
			name: "start-over", summary: "set the database aside, with the server stopped, so it starts with an " +
				"empty one; the instance's identity and claim stay", implementation: commandStartOver,
		},
		{name: "doctor", summary: "report what is wrong and how to fix it", implementation: commandDoctor},
		{
			name: "setup-code", summary: "print the one-time code that creates the first admin",
			implementation: commandSetupCode,
		},
		{
			name: "reset-admin", summary: "make one person the only admin, with a new password, " +
				"read from standard input (D294)",
			implementation: commandResetAdmin,
		},
		{
			name: "pair", summary: "pair the running core with its discovery by the account's key, the admin's " +
				"password and the key read from standard input (D334 §3)",
			implementation: commandPair,
		},
		{name: "openapi", summary: "print the API's OpenAPI document", implementation: commandOpenAPI},
	}
	if isDevBuild {
		planned = append(planned, subcommand{
			name:           "dev",
			summary:        "development only: seed the dev database from deploy/dev/seed.toml (make dev)",
			implementation: devCommand(),
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
	status := dispatch(ctx, os.Args[1:], environmentLookup(), os.Stdin, os.Stdout, os.Stderr)
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

// dispatch executes one invocation and returns its exit status. It takes the environment and its
// streams as parameters so the test can observe them; a nil stdin reads nothing.
func dispatch(
	ctx context.Context, arguments []string, lookupEnv func(string) (string, bool), stdin io.Reader,
	stdout, stderr io.Writer,
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
		if planned.implementation == nil {
			return write(stderr, fmt.Sprintf("orpheum %s: not implemented yet\n", name), exitUsageError)
		}
		if stdin == nil {
			stdin = strings.NewReader("")
		}
		call := invocation{
			arguments: arguments[1:], lookupEnv: lookupEnv, stdin: stdin, stdout: stdout, stderr: stderr,
		}
		return planned.implementation(ctx, call)
	}
	return write(stderr, fmt.Sprintf("orpheum: unknown subcommand %q\n\n", name)+usage(), exitUsageError)
}

// commandCore runs the core. Its nodes are services of their own (`orpheum node`), which dial its
// socket: the core starts none (D233 §2, D235 §4).
func commandCore(ctx context.Context, call invocation) int {
	config, configReport, err := call.loadConfigFromFlags("core", kindCore, nil)
	if err != nil {
		return call.fail(err)
	}
	logger, closeLog, err := call.logger(config)
	if err != nil {
		return call.fail(err)
	}
	defer closeLog() //nolint:errcheck // nothing is logged after it
	for _, note := range configReport.Notes() {
		logger.WarnContext(ctx, "the config file needs attention; doctor lists it too", "note", note)
	}
	listener, err := listenForClients(ctx, config.Server, logger)
	if err != nil {
		return call.fail(err)
	}
	outside := releaseWiring()
	outside.machineZone = machineZone(systemZoneSources(call.lookupEnv))
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

// releaseWiring is what a running server reaches outside itself: the providers' real addresses,
// which a test replaces with its fakes.
func releaseWiring() wiring {
	return wiring{
		tmdbBaseURL: tmdb.BaseURL, tmdbImagesBaseURL: tmdb.ImageBaseURL, openSubtitlesBaseURL: opensubtitles.BaseURL,
		cloudflareBaseURL: cloudflare.BaseURL,
		waitsForDNS:       true,
	}
}
