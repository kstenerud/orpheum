// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/kstenerud/orpheum/internal/deployment"
)

// startMedia is where a deployment started from one compose file finds its media: the household's
// folders, mounted read-only there in the node's container (deploy/omv/compose.yaml).
const startMedia = "/media"

// startUsage says how a compose file runs the core of a deployment it starts on its own (D447).
const startUsage = `usage: orpheum start --lan-address <address> [--media <folder>]... [--lan-https-port <port>]
                     [--lan-webrtc-port <port>] [--router <address>]

Runs the core of a deployment whose directory is mounted at ` + deploymentMount + ` and its run/ at
` + deploymentRun + `, as one compose file starts it (deploy/omv/compose.yaml). The first time, when the
directory holds no deployment, it writes one from these flags: the values file, and the core's and the
node's instances and configs, which the node's service then runs. After that the files rule and the
flags are not read; remove values.toml from the directory and start again to write the configs again
from the flags, keeping the database and everything else.

  --lan-address      this machine's address on the house's network, 192.168.1.20 (needed the first time)
  --media            a media folder as the node sees it (` + startMedia + ` unless named); repeat for more
  --lan-https-port   the port HTTPS is published on at that address (8443 unless named)
  --lan-webrtc-port  the UDP port WebRTC is published on there, the same inside (8443 unless named)
  --router           the house's router, which port mapping asks to forward the HTTPS port; none maps nothing
`

// startOptions are what a first start writes a deployment from: what values.toml's [media] and [lan]
// would say.
type startOptions struct {
	lanAddress string
	media      []string
	httpsPort  int
	webrtcPort int
	router     string
}

// commandStart runs a deployment's core, writing the deployment from its flags the first time
// (D447): one compose file, its variables filled into the command line, installs a server, with no
// script on the machine. A release reads no environment (D204), so compose substitutes them.
func commandStart(ctx context.Context, call invocation) int {
	options, isParsed := parseStart(call.arguments, call.stderr)
	if !isParsed {
		return exitUsageError
	}
	hostName, err := os.Hostname()
	if err != nil {
		return call.fail(fmt.Errorf("this machine's host name: %w", err))
	}
	isWritten, err := firstStart(deploymentMount, deploymentRun, hostName, options)
	if err != nil {
		return call.fail(err)
	}
	said := "orpheum: the deployment in " + deploymentMount + " is there already; its files rule, not the " +
		"flags (remove values.toml there and start again to write its configs from them)\n"
	if isWritten {
		said = "orpheum: wrote the deployment in " + deploymentMount + " from the flags; from now on its files rule\n"
	}
	if status := write(call.stderr, said, exitSuccess); status != exitSuccess {
		return status
	}
	call.arguments = []string{"--instance", filepath.Join(deploymentMount, string(kindCore))}
	return commandCore(ctx, call)
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
		options.media = append(options.media, folder)
		return nil
	})
	flags.IntVar(&options.httpsPort, "lan-https-port", defaults.LAN.HTTPSPort, "the port HTTPS is published on")
	flags.IntVar(&options.webrtcPort, "lan-webrtc-port", defaults.LAN.WebRTCPort, "the UDP port WebRTC is published on")
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

// errNoLANAddress is a first start's refusal when it is not told this machine's address at home,
// which its container cannot see, and which the house's network, the one an admin administers from,
// is found by.
var errNoLANAddress = errors.New("--lan-address is needed the first time: this machine's address on the " +
	"house's network, 192.168.1.20, which the server's container cannot see (in the compose file, " +
	"ORPHEUM_LAN_ADDRESS); keep it fixed with a reservation in the router")

// firstStart writes the deployment in dir, its instances' run directories in runDir, from options
// when dir holds none, and says whether it wrote one; a deployment there already it leaves as it is.
// The values file is written last, so a start cut short is written again the next time; instances
// there already are kept.
func firstStart(dir, runDir, hostName string, options startOptions) (bool, error) {
	if _, err := os.Stat(filepath.Join(dir, deployment.ValuesFile)); err == nil {
		return false, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return false, fmt.Errorf("look for the deployment in %s: %w", dir, err)
	}
	if options.lanAddress == "" {
		return false, errNoLANAddress
	}
	values := deployment.StartingValues(deployment.Release)
	values.Media.Roots = options.media
	values.LAN.Address = options.lanAddress
	values.LAN.HTTPSPort = options.httpsPort
	values.LAN.WebRTCPort = options.webrtcPort
	values.LAN.Router = options.router
	if err := values.Check(); err != nil {
		return false, fmt.Errorf("the flags, as %w", err)
	}
	if err := makeMissingInstances(dir, runDir); err != nil {
		return false, err
	}
	written := values
	written.Node.Name = hostName
	if err := deployment.WriteConfigs(dir, written); err != nil {
		return false, err
	}
	if _, err := deployment.BeginWith(dir, values); err != nil {
		return false, err
	}
	return true, nil
}
