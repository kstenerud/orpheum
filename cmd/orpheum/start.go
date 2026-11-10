// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/kstenerud/orpheum/internal/deployment"
)

// startMedia is where a deployment started with no --media finds its media. The compose file names
// each folder at its own path, mounted read-only there in the node's container, so a library shows
// the path the household knows (deploy/omv/compose.yaml, D449 §4).
const startMedia = "/media"

// setupCodeFile is the file in the deployment's directory a compose file's start shows the setup code
// in while no admin exists: the data folder a household browses in its NAS's panel (D449 §3).
const setupCodeFile = "SETUP-CODE.txt"

// startUsage says how a compose file runs the core of a deployment it starts on its own (D447, D449).
const startUsage = `usage: orpheum start [--media <folder>]... [--http-port <port>] [--this-machine-port <port>]
                     [--compose-version <n>] [--compose-file <route>] [--image-tag <tag>]

Runs the core of a deployment whose directory is mounted at ` + deploymentMount + ` and its run/ at
` + deploymentRun + `, as one compose file starts it (deploy/omv/compose.yaml). At every start it writes the
values file and the core's and the node's configs from these flags, the network settings the System
page keeps and this image's templates, making the instances the first time and keeping them, the
database and everything else (D449, D462 §2). A flag this release does not take is logged and
skipped, so a newer compose file still starts it (D462 §10.10). While no admin exists, the setup code
is also in ` + setupCodeFile + ` in the directory.

  --media              a media folder as the node sees it (` + startMedia + ` unless named); repeat for more,
                       an empty one, or one under ` + deployment.Unset + `, is skipped
  --http-port          the port the core takes plain HTTP on from the house's network (8440 unless named)
  --this-machine-port  the port it takes on this machine's loopback alone, a browser on which is at home
                       (8441 unless named)
  --compose-version    the version of the compose file starting it, 1 unless named: the Update page says
                       when a newer one brings something (D461 §3)
  --compose-file       the route the compose file is published for: omv, synology, portainer, casaos or
                       docker, the Update page showing its steps; docker unless named, or not known (D462 §9)
  --image-tag          the image tag the file runs, latest unless named: the Update page says to change a
                       pinned one (D462 §10.7)
`

// startHTTPSPort is the port a compose file's deployment takes HTTPS and WebRTC on at home until the
// System page sets another, and the one it falls back to when that will not bind (D462 §10.11):
// 8444, as OpenMediaVault's own web UI may take 8443.
const startHTTPSPort = 8444

// startOptions are what a start writes a deployment from: what values.toml's [media] and [http] say,
// and how it was installed.
type startOptions struct {
	media           []string
	httpPort        int
	thisMachinePort int
	// composeVersion is the version the compose file says it is, 1 for one that says none (D461 §3).
	composeVersion int
	// composeFile is the route the compose file names (D462 §9), taken as docker when unknown.
	composeFile deployment.InstallRoute
	// imageTag is the image tag the compose file runs (D462 §10.7).
	imageTag string
}

// commandStart runs a deployment's core, writing the deployment from its flags at every start (D447,
// D449 §1): one compose file, its variables filled into the command line, installs a server and
// changes it, with no script on the machine. A release reads no environment (D204), so compose
// substitutes them.
func commandStart(ctx context.Context, call invocation) int {
	options, isParsed := parseStart(call.arguments, call.stderr)
	if !isParsed {
		return exitUsageError
	}
	hostName, err := os.Hostname()
	if err != nil {
		return call.fail(fmt.Errorf("this machine's host name: %w", err))
	}
	fellBack, err := startDeployment(deploymentMount, deploymentRun, hostName, options, portBinds)
	if err != nil {
		return call.fail(err)
	}
	var said strings.Builder
	said.WriteString("orpheum: wrote the deployment's configs in " + deploymentMount + " from the flags\n")
	for _, each := range fellBack {
		said.WriteString("orpheum: " + each + "\n")
	}
	if status := write(call.stderr, said.String(), exitSuccess); status != exitSuccess {
		return status
	}
	call.arguments = []string{"--instance", filepath.Join(deploymentMount, string(kindCore))}
	shown := setupCodeShown{file: filepath.Join(deploymentMount, setupCodeFile), httpPort: options.httpPort}
	return runCore(ctx, call, shown)
}

// parseStart reads start's flags, logging and skipping one it does not take (D462 §10.10). It is
// false once it has written to stderr what was wrong.
func parseStart(arguments []string, stderr io.Writer) (startOptions, bool) {
	flags := flag.NewFlagSet("orpheum start", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() { write(stderr, startUsage, exitUsageError) }
	defaults := deployment.StartingValues(deployment.Release)
	options := startOptions{}
	flags.Func("media", "a media folder as the node sees it; repeat for more", func(folder string) error {
		// A compose file's optional folder that is not set comes empty, or under /unset/ (D449 §4).
		if folder = strings.TrimSpace(folder); folder != "" && !strings.HasPrefix(folder, deployment.Unset) {
			options.media = append(options.media, filepath.Clean(folder))
		}
		return nil
	})
	flags.IntVar(&options.httpPort, "http-port", defaults.HTTP.Port, "the port plain HTTP is taken on")
	flags.IntVar(&options.thisMachinePort, "this-machine-port", defaults.HTTP.ThisMachinePort,
		"the port published on this machine's loopback alone")
	flags.IntVar(&options.composeVersion, "compose-version", 1, "the version of the compose file starting it")
	composeFile := flags.String("compose-file", string(deployment.ByDocker), "the route the compose file is for")
	flags.StringVar(&options.imageTag, "image-tag", "latest", "the image tag the compose file runs")
	known, skipped := knownFlags(flags, arguments)
	for _, each := range skipped {
		write(stderr, "orpheum start: "+each+", which this release does not take, is skipped\n", exitSuccess)
	}
	if err := flags.Parse(known); err != nil {
		return startOptions{}, false
	}
	if options.composeVersion < 1 {
		write(stderr, fmt.Sprintf("orpheum start: --compose-version %d: want 1 or more\n", options.composeVersion),
			exitUsageError)
		return startOptions{}, false
	}
	if flags.NArg() != 0 {
		write(stderr, startUsage, exitUsageError)
		return startOptions{}, false
	}
	if options.composeFile = deployment.ComposeRoute(*composeFile); string(options.composeFile) != *composeFile {
		write(stderr, fmt.Sprintf("orpheum start: --compose-file %q is not a route this release knows; "+
			"taken as %s\n", *composeFile, options.composeFile), exitSuccess)
	}
	if options.media == nil {
		options.media = []string{startMedia}
	}
	options.imageTag = strings.TrimSpace(options.imageTag)
	return options, true
}

// knownFlags are the arguments flags takes, and the flags it does not, each as the argument named it,
// left out with the value after it unless that is another flag or the flag carries its value ("=").
func knownFlags(flags *flag.FlagSet, arguments []string) (known, skipped []string) {
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		name, _, hasValue := strings.Cut(strings.TrimLeft(argument, "-"), "=")
		if !strings.HasPrefix(argument, "-") || argument == "--" || flags.Lookup(name) != nil {
			known = append(known, argument)
			continue
		}
		skipped = append(skipped, argument)
		if !hasValue && index+1 < len(arguments) && !strings.HasPrefix(arguments[index+1], "-") {
			index++
		}
	}
	return known, skipped
}

// errNoDeployment is a start's refusal when the deployment's directory is not there: the compose file
// mounts the data folder's orpheum/ there, and start makes nothing outside it.
var errNoDeployment = errors.New("the deployment's directory is not there; the compose file mounts the data " +
	"folder's orpheum folder at it")

// startDeployment writes the deployment in dir, its instances' run directories in runDir, from
// options, at every start (D449 §1): the instances the first time, kept after, and the core's and the
// node's configs and the values file each time, from the options, the network settings the System
// page keeps and this binary's templates, as `write-deployment` writes them on each install (D272
// §7). The values file is written last, so it never says what the configs do not. A kept port that
// binds will not falls back to its default, each said in what it returns (D462 §10.11).
func startDeployment(dir, runDir, hostName string, options startOptions, binds func(Ports) error) ([]string, error) {
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("%w: %s", errNoDeployment, dir)
	}
	values := deployment.StartingValues(deployment.Release)
	values.HTTP.Port = options.httpPort
	values.Media.Roots = options.media
	values.HTTP.ThisMachinePort = options.thisMachinePort
	values.LAN.HTTPSPort, values.LAN.WebRTCPort = startHTTPSPort, startHTTPSPort
	if err := values.Check(); err != nil {
		return nil, fmt.Errorf("the flags, as %w", err)
	}
	if err := makeMissingInstances(dir, runDir); err != nil {
		return nil, err
	}
	network, err := deployment.KeptNetwork(dir, startSeed(dir, values))
	if err != nil {
		return nil, err
	}
	network, fellBack := bindable(network, binds)
	written := network.Applied(values)
	if err = written.Check(); err != nil {
		return nil, fmt.Errorf("the network settings, as %w", err)
	}
	values = written
	written.Node.Name = hostName
	installation := deployment.Installation{
		Route: options.composeFile, ComposeVersion: options.composeVersion, ImageTag: options.imageTag,
	}
	if err := deployment.WriteConfigs(dir, written, installation); err != nil {
		return nil, err
	}
	return fellBack, deployment.WriteStartValues(dir, values)
}

// startSeed is the network settings a deployment starts with before the System page keeps any: the
// ones the values file an earlier start wrote names, else values' (D462 §10.6).
func startSeed(dir string, values deployment.Values) deployment.Network {
	text, err := os.ReadFile(filepath.Join(dir, deployment.ValuesFile)) //nolint:gosec // G304: the deployment's own
	if err != nil {
		return deployment.NetworkOf(values)
	}
	written, err := deployment.ParseValues(string(text), "")
	if err != nil || len(deployment.NetworkOf(written).Problems(values.HTTP.Port)) != 0 {
		return deployment.NetworkOf(values)
	}
	return deployment.NetworkOf(written)
}
