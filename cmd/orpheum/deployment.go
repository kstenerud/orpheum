// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"github.com/kstenerud/orpheum/internal/deployment"
)

// The deployment's directory and its run directory as the runtime image sees them, where
// write-deployment is run with them mounted, as the compose file mounts each part of them for its
// services: the instances it makes name these paths (D272 §3).
const (
	deploymentMount = "/var/lib/orpheum"
	deploymentRun   = "/run/orpheum"
)

// renderNode is the GPU's render node, whose group the node joins to encode on it.
const renderNode = "/dev/dri/renderD128"

// writeDeploymentUsage says how the image's setup is run: the build command and ./staging run it so.
const writeDeploymentUsage = `usage: orpheum write-deployment --image <image> [--deployment release|staging]

Run in the runtime image, as the owner of the deployment's directory, with it mounted at ` +
	deploymentMount + ` and its run/ at ` + deploymentRun + `, the machine's host name, and /dev/dri
when the machine has a GPU, and --nvidia when Docker hands its NVIDIA GPUs to a container (cdi or
driver, as the install script finds):

  docker run --rm --user "$(id -u):$(id -g)" --hostname "$(hostname)" \
    -v DIR:` + deploymentMount + ` -v DIR/run:` + deploymentRun + ` [-v /dev/dri:/dev/dri:ro] \
    IMAGE write-deployment --image IMAGE [--nvidia cdi|driver]

The first time, it writes the values file and the tunnel's keys, and stops. After that it writes the
compose file and the configs from the values file, and ` + "`docker compose up -d`" + ` in DIR starts them.
`

// commandWriteDeployment writes the deployment's directory (D272 §3).
func commandWriteDeployment(_ context.Context, call invocation) int {
	flags := flag.NewFlagSet("orpheum write-deployment", flag.ContinueOnError)
	flags.SetOutput(call.stderr)
	image := flags.String("image", "", "the runtime image the services run, as the .env file names it")
	kind := flags.String("deployment", string(deployment.Release),
		"the first time only: the kind the values file starts as, release or staging")
	nvidiaName := flags.String("nvidia", "",
		"how Docker hands this machine's NVIDIA GPUs to a container, as the install script found: cdi or driver")
	if err := flags.Parse(call.arguments); err != nil {
		return exitUsageError
	}
	nvidia, err := deployment.ParseNVIDIAAccess(*nvidiaName)
	if flags.NArg() != 0 || *image == "" || err != nil {
		return write(call.stderr, writeDeploymentUsage, exitUsageError)
	}
	hostName, err := os.Hostname()
	if err != nil {
		return call.fail(fmt.Errorf("this machine's host name: %w", err))
	}
	message, err := writeDeployment(deploymentMount, deploymentRun, deployment.Kind(*kind), hostName,
		deployment.Runtime{
			Image: *image, UID: os.Getuid(), GID: os.Getgid(), RenderGID: renderGroup(renderNode), NVIDIA: nvidia,
		})
	if err != nil {
		return call.fail(err)
	}
	if message.isBegun {
		return write(call.stderr, message.text, exitFailure)
	}
	return write(call.stdout, message.text, exitSuccess)
}

// deploymentMessage is what write-deployment says it did: begun, with what to fill in, or written.
type deploymentMessage struct {
	text    string
	isBegun bool
}

// writeDeployment begins the deployment in dir when it has no values file yet, and otherwise makes
// its instances when they are not there and writes its files. The instances' run directories go
// in runDir.
func writeDeployment(dir, runDir string, kind deployment.Kind, hostName string, runtime deployment.Runtime,
) (deploymentMessage, error) {
	text, err := os.ReadFile(filepath.Join(dir, deployment.ValuesFile)) //nolint:gosec // G304: the deployment's own
	if errors.Is(err, fs.ErrNotExist) {
		return beginDeployment(dir, kind)
	}
	if err != nil {
		return deploymentMessage{}, fmt.Errorf("read the values file: %w", err)
	}
	values, err := deployment.ParseValues(string(text), hostName)
	if err != nil {
		return deploymentMessage{}, err
	}
	if problems := values.Check(); problems != nil {
		return deploymentMessage{}, problems
	}
	if makeErr := makeMissingInstances(dir, runDir); makeErr != nil {
		return deploymentMessage{}, makeErr
	}
	if writeErr := deployment.Write(dir, values, runtime); writeErr != nil {
		return deploymentMessage{}, writeErr
	}
	reached := "reached through its discovery"
	if values.Domain != "" {
		reached = "for " + values.Domain
	}
	return deploymentMessage{text: fmt.Sprintf("wrote the %s deployment %s, running %s; "+
		"`docker compose up -d` in its directory starts it\n", values.Deployment, reached, runtime.Image)}, nil
}

// beginDeployment writes a new deployment's values file, as kind, and the tunnel's keys, and says
// what to do before it can start.
func beginDeployment(dir string, kind deployment.Kind) (deploymentMessage, error) {
	if kind != deployment.Release && kind != deployment.Staging {
		return deploymentMessage{}, fmt.Errorf("--deployment %q: want release or staging", kind)
	}
	publicKey, err := deployment.Begin(dir, kind)
	if err != nil {
		return deploymentMessage{}, err
	}
	return deploymentMessage{isBegun: true, text: fmt.Sprintf(`began a %s deployment. Before it can start:
  1. Fill in %s: every value marked "fill in".
  2. Only with a VPS tunnel ([vps] filled in): on the VPS, give this end a [Peer] in
     /etc/wireguard/wg0.conf, unless it already has one: its
     PublicKey this end's public key, below, and its AllowedIPs this deployment's tunnel addresses,
     %s's vps.tunnel_ipv4 and vps.tunnel_ipv6 (10.77.0.2 and fd77::2 unless it names others);
     then restart the tunnel there (systemctl restart wg-quick@wg0).
       %s
  3. Run the command that began it again: deploy/release/build.sh, or ./staging.
`, kind, deployment.ValuesFile, deployment.ValuesFile, publicKey)}, nil
}

// makeMissingInstances makes the core's and the node's instances in dir, each the first time.
func makeMissingInstances(dir, runDir string) error {
	for _, kind := range []instanceKind{kindCore, kindNode} {
		instanceDir := filepath.Join(dir, string(kind))
		if _, err := os.Stat(filepath.Join(instanceDir, instanceFileName)); err == nil {
			continue
		}
		if _, err := makeInstance(instanceDir, kind, filepath.Join(runDir, string(kind)), version); err != nil {
			return err
		}
	}
	return nil
}

// renderGroup is the group of the render node at path, or -1 when there is none.
func renderGroup(path string) int {
	info, err := os.Stat(path)
	if err != nil {
		return -1
	}
	if stat, isUnix := info.Sys().(*syscall.Stat_t); isUnix {
		return int(stat.Gid)
	}
	return -1
}

// publishedImage is where a version's runtime image is published (D314 §1; .github/workflows/release-image.yml).
const publishedImage = "ghcr.io/kstenerud/orpheum"

// untaggedVersion matches a build of no tag: its version is the last tag, a commit time and a hash.
var untaggedVersion = regexp.MustCompile(`\.0\.\d{14}-[0-9a-f]{12}$`)

// installImage is the published image of this binary's version, or the latest for a build of no tag.
func installImage(version string) string {
	if untaggedVersion.MatchString(version) || !strings.HasPrefix(version, "v") {
		return publishedImage + ":latest"
	}
	return publishedImage + ":" + version
}

// commandInstallScript prints the script a household installs a core with, installing this image
// (D314): `docker run --rm IMAGE install-script | sh -s -- --media /path/to/films`.
func commandInstallScript(_ context.Context, call invocation) int {
	if len(call.arguments) != 0 {
		return write(call.stderr, "usage: orpheum install-script\n", exitUsageError)
	}
	script, err := deployment.InstallScript(installImage(version))
	if err != nil {
		return call.fail(err)
	}
	return write(call.stdout, script, exitSuccess)
}
