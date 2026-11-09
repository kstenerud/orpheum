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
const startUsage = `usage: orpheum start [--lan-address <address>] [--media <folder>]... [--http-port <port>]
                     [--lan-https-port <port>] [--lan-webrtc-port <port>] [--this-machine-port <port>]
                     [--router <address>]

Runs the core of a deployment whose directory is mounted at ` + deploymentMount + ` and its run/ at
` + deploymentRun + `, as one compose file starts it (deploy/omv/compose.yaml). At every start it writes the
values file and the core's and the node's configs from these flags and this image's templates, making
the instances the first time and keeping them, the database and everything else (D449): change a
flag in the compose file and start again. While no admin exists, the setup code is also in
` + setupCodeFile + ` in the directory.

  --lan-address        this machine's address on the house's network, 192.168.1.20; learned from the first
                       browser at home when empty (D452 §5)
  --media              a media folder as the node sees it (` + startMedia + ` unless named); repeat for more,
                       an empty one is skipped
  --http-port          the port the core takes plain HTTP on from the house's network (8440 unless named)
  --lan-https-port     the port it serves HTTPS on there (8443 unless named)
  --lan-webrtc-port    the UDP port it takes WebRTC on there (8443 unless named)
  --this-machine-port  the port it takes on this machine's loopback alone, a browser on which is at home
                       (8441 unless named)
  --router             the house's router, which port mapping asks to forward the HTTPS port; none maps nothing
`

// startOptions are what a start writes a deployment from: what values.toml's [media], [http] and
// [lan] say.
type startOptions struct {
	lanAddress      string
	media           []string
	httpPort        int
	httpsPort       int
	webrtcPort      int
	thisMachinePort int
	router          string
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
	if err = startDeployment(deploymentMount, deploymentRun, hostName, options); err != nil {
		return call.fail(err)
	}
	said := "orpheum: wrote the deployment's configs in " + deploymentMount + " from the flags\n"
	if status := write(call.stderr, said, exitSuccess); status != exitSuccess {
		return status
	}
	call.arguments = []string{"--instance", filepath.Join(deploymentMount, string(kindCore))}
	return runCore(ctx, call, filepath.Join(deploymentMount, setupCodeFile))
}

// parseStart reads start's flags. It is false once it has written to stderr what was wrong.
func parseStart(arguments []string, stderr io.Writer) (startOptions, bool) {
	flags := flag.NewFlagSet("orpheum start", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() { write(stderr, startUsage, exitUsageError) }
	defaults := deployment.StartingValues(deployment.Release)
	options := startOptions{}
	flags.StringVar(&options.lanAddress, "lan-address", "", "this machine's address on the house's network")
	flags.Func("media", "a media folder as the node sees it; repeat for more", func(folder string) error {
		// A compose file's optional folder that is not set comes as an empty one (D449 §4).
		if folder = strings.TrimSpace(folder); folder != "" {
			options.media = append(options.media, filepath.Clean(folder))
		}
		return nil
	})
	flags.IntVar(&options.httpPort, "http-port", defaults.HTTP.Port, "the port plain HTTP is taken on")
	flags.IntVar(&options.httpsPort, "lan-https-port", defaults.LAN.HTTPSPort, "the port HTTPS is published on")
	flags.IntVar(&options.webrtcPort, "lan-webrtc-port", defaults.LAN.WebRTCPort, "the UDP port WebRTC is published on")
	flags.IntVar(&options.thisMachinePort, "this-machine-port", defaults.HTTP.ThisMachinePort,
		"the port published on this machine's loopback alone")
	flags.StringVar(&options.router, "router", "", "the house's router")
	if err := flags.Parse(arguments); err != nil {
		return startOptions{}, false
	}
	if flags.NArg() != 0 {
		write(stderr, startUsage, exitUsageError)
		return startOptions{}, false
	}
	if options.media == nil {
		options.media = []string{startMedia}
	}
	options.lanAddress = strings.TrimSpace(options.lanAddress)
	options.router = strings.TrimSpace(options.router)
	return options, true
}

// errNoDeployment is a start's refusal when the deployment's directory is not there: the compose file
// mounts the data folder's orpheum/ there, and start makes nothing outside it.
var errNoDeployment = errors.New("the deployment's directory is not there; the compose file mounts the data " +
	"folder's orpheum folder at it")

// startDeployment writes the deployment in dir, its instances' run directories in runDir, from
// options, at every start (D449 §1): the instances the first time, kept after, and the core's and the
// node's configs and the values file each time, from the options and this binary's templates, as
// `write-deployment` writes them on each install (D272 §7). The values file is written last, so it
// never says what the configs do not.
func startDeployment(dir, runDir, hostName string, options startOptions) error {
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return fmt.Errorf("%w: %s", errNoDeployment, dir)
	}
	values := deployment.StartingValues(deployment.Release)
	values.HTTP.Port = options.httpPort
	values.Media.Roots = options.media
	values.LAN.Address = options.lanAddress
	values.LAN.HTTPSPort = options.httpsPort
	values.LAN.WebRTCPort = options.webrtcPort
	values.LAN.Router = options.router
	values.HTTP.ThisMachinePort = options.thisMachinePort
	if err := values.Check(); err != nil {
		return fmt.Errorf("the flags, as %w", err)
	}
	if err := makeMissingInstances(dir, runDir); err != nil {
		return err
	}
	written := values
	written.Node.Name = hostName
	if err := deployment.WriteConfigs(dir, written, deployment.ByComposeFile); err != nil {
		return err
	}
	return deployment.WriteStartValues(dir, values)
}
