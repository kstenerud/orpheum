// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"

	"github.com/kstenerud/orpheum/internal/deployment"
)

// fillValues fills in the values file a deployment began with.
func fillValues(t *testing.T, dir string) {
	t.Helper()
	path := filepath.Join(dir, deployment.ValuesFile)
	text, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	filled := strings.NewReplacer(
		`contact = ""`, `contact = "owner@example.org"`,
		`endpoint = ""`, `endpoint = "[2001:db8::7]:51820"`,
		`public_ipv4 = ""`, `public_ipv4 = "11.22.44.146"`,
		`public_key = ""`, `public_key = "YWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWE="`,
		`roots = []`, `roots = ["/srv/media"]`,
		`address = ""`, `address = "192.168.1.20"`,
	).Replace(string(text))
	if err = os.WriteFile(path, []byte(filled), 0o600); err != nil { //nolint:gosec // G703: a test's own temporary file
		t.Fatal(err)
	}
}

func TestWriteDeployment_BeginsThenMakesTheInstancesOnceAndWritesTheFiles(t *testing.T) {
	t.Parallel()
	dir, runDir := t.TempDir(), filepath.Join(t.TempDir(), "run")
	begun, err := writeDeployment(dir, runDir, deployment.Staging, "nas", runtimeOf("orpheum:v0.1.0"))
	if err != nil {
		t.Fatal(err)
	}
	if !begun.isBegun || !strings.Contains(begun.text, "public key") ||
		!strings.Contains(begun.text, "vps.tunnel_ipv4") {
		t.Errorf("the first run said %q; want it begun, giving the tunnel's public key and naming the "+
			"tunnel addresses its peer on the VPS is given", begun.text)
	}
	if _, err = writeDeployment(dir, runDir, deployment.Staging, "nas", runtimeOf("orpheum:v0.1.0")); err == nil ||
		!strings.Contains(err.Error(), "media.roots") {
		t.Errorf("writing before the values are filled in = %v; want it refused, naming what they need: "+
			"the media, all a household with no VPS gives (D314 §3)", err)
	}
	fillValues(t, dir)
	written, err := writeDeployment(dir, runDir, deployment.Release, "nas", runtimeOf("orpheum:v0.1.0"))
	if err != nil {
		t.Fatal(err)
	}
	if written.isBegun || !strings.Contains(written.text, "docker compose up -d") {
		t.Errorf("the run after filling in said %q; want it written, saying how to start it", written.text)
	}
	core, err := loadInstance(filepath.Join(dir, "core"), kindCore)
	if err != nil {
		t.Fatal(err)
	}
	if core.RunDir != filepath.Join(runDir, "core") {
		t.Errorf("the core's run directory is %s, want %s, where the compose file mounts run/", core.RunDir,
			filepath.Join(runDir, "core"))
	}
	config := map[string]map[string]any{}
	if _, err = toml.DecodeFile(core.fullConfigPath(), &config); err != nil {
		t.Fatal(err)
	}
	if config["database"]["when_unmigratable"] != "start_over" {
		t.Errorf("the core config says %v when unmigratable; want staging's start_over, as the values file began",
			config["database"]["when_unmigratable"])
	}
	if _, err = writeDeployment(dir, runDir, deployment.Release, "nas", runtimeOf("orpheum:v0.2.0")); err != nil {
		t.Errorf("writing again over the instances made = %v; want them kept and the files written", err)
	}
}

func TestWriteDeployment_TheVPSStepIsATunnelsAndACoreWithNoNameIsReachedThroughItsDiscovery(t *testing.T) {
	t.Parallel()
	dir, runDir := t.TempDir(), filepath.Join(t.TempDir(), "run")
	begun, err := writeDeployment(dir, runDir, deployment.Release, "nas", runtimeOf("orpheum:v0.1.0"))
	if err != nil {
		t.Fatal(err)
	}
	fillValues(t, dir)
	written, err := writeDeployment(dir, runDir, deployment.Release, "nas", runtimeOf("orpheum:v0.1.0"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(begun.text, "Only with a VPS tunnel") ||
		!strings.Contains(written.text, "reached through its discovery") {
		t.Errorf("begun %q, written %q; want the VPS's step said to apply only with a tunnel, and a core with "+
			"no name of its own said to be reached through its discovery (D314 §3)", begun.text, written.text)
	}
}

func TestDispatch_WriteDeploymentIsASubcommandThatNeedsItsImage(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	status := dispatch(t.Context(), []string{"write-deployment", "--deployment", "release"}, noEnvironment, nil,
		&stdout, &stderr)
	if status != exitUsageError || !strings.Contains(stderr.String(), "docker run --rm") {
		t.Errorf("write-deployment without --image: status %d, stderr %q; want its usage, saying how the "+
			"image runs it", status, stderr.String())
	}
}

func TestDispatch_WriteDeploymentTakesNVIDIAByCDIOrDriverAndRefusesAnythingElse(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	status := dispatch(t.Context(), []string{"write-deployment", "--image", "orpheum:v0.1.0", "--nvidia", "all"},
		noEnvironment, nil, &stdout, &stderr)
	if status != exitUsageError || !strings.Contains(stderr.String(), "[--nvidia cdi|driver]") ||
		strings.Contains(stderr.String(), "not defined") {
		t.Errorf("write-deployment --nvidia all: status %d, stderr %q; want its usage, naming cdi and driver",
			status, stderr.String())
	}
}

// The install script names the deployment's directory, which gives its network a subnet of its own.
func TestParseWriteDeployment_TheDirectoryNamedReachesTheRuntime(t *testing.T) {
	t.Parallel()
	kind, runtime, isParsed := parseWriteDeployment(
		[]string{"--image", "orpheum:v0.1.0", "--deployment", "staging", "--directory", "/srv/orpheum"}, io.Discard)
	if !isParsed || kind != deployment.Staging || runtime.Image != "orpheum:v0.1.0" ||
		runtime.Directory != "/srv/orpheum" {
		t.Errorf("parsed %v: kind %q, runtime %+v; want staging, the image and the directory /srv/orpheum",
			isParsed, kind, runtime)
	}
}

// A kernel with no MPTCP has no net.mptcp.enabled, and the deployment's compose file then names none,
// which Docker would refuse a container for; one that has it turns it off (CVE-2026-98432).
func TestParseWriteDeployment_SaysWhetherTheKernelLacksMPTCP(t *testing.T) { //nolint:paralleltest // sets the setting's path
	saved := multipathTCPSetting
	t.Cleanup(func() { multipathTCPSetting = saved })
	present := filepath.Join(t.TempDir(), "enabled")
	if err := os.WriteFile(present, []byte("1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for path, wantLacks := range map[string]bool{present: false, filepath.Join(t.TempDir(), "absent"): true} {
		multipathTCPSetting = path
		_, runtime, isParsed := parseWriteDeployment([]string{"--image", "orpheum:v0.1.0"}, io.Discard)
		if !isParsed || runtime.KernelLacksMultipathTCP != wantLacks {
			t.Errorf("with the setting at %s: lacks MPTCP %v; want %v", path, runtime.KernelLacksMultipathTCP, wantLacks)
		}
	}
}

// The configs a deployment is written with set every key, so a key added to the binary is added to
// the deployment's templates too, rather than run on its starting value with a warning at each start.
func TestWriteDeployment_TheConfigsItWritesSetEveryKey(t *testing.T) {
	t.Parallel()
	dir, runDir := t.TempDir(), filepath.Join(t.TempDir(), "run")
	if _, err := writeDeployment(dir, runDir, deployment.Release, "nas", runtimeOf("orpheum:v0.1.0")); err != nil {
		t.Fatal(err)
	}
	fillValues(t, dir)
	if _, err := writeDeployment(dir, runDir, deployment.Release, "nas", runtimeOf("orpheum:v0.1.0")); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []instanceKind{kindCore, kindNode} {
		instance, err := loadInstance(filepath.Join(dir, string(kind)), kind)
		if err != nil {
			t.Fatal(err)
		}
		_, report, err := loadConfig(instance.fullConfigPath(), kind)
		if err != nil {
			t.Fatalf("the written %s config: %v", kind, err)
		}
		for _, key := range report.OnStartingValue {
			t.Errorf("the written %s config leaves %s out: add it to internal/deployment/templates", kind, key.path())
		}
		if len(report.Unknown) > 0 {
			t.Errorf("the written %s config sets keys the binary does not know: %v", kind, report.Unknown)
		}
	}
}

func TestInstallImage_IsThisVersionsPublishedImageOrTheLatestForABuildOfNoTag(t *testing.T) {
	t.Parallel()
	for version, want := range map[string]string{
		"v0.2.0-nas.11": "ghcr.io/kstenerud/orpheum:v0.2.0-nas.11",
		"v0.3.0":        "ghcr.io/kstenerud/orpheum:v0.3.0",
		"v0.2.0-nas.10.0.20261018180503-e6c7c4eab3f8": "ghcr.io/kstenerud/orpheum:latest",
		"(devel)": "ghcr.io/kstenerud/orpheum:latest",
	} {
		if got := installImage(version); got != want {
			t.Errorf("installImage(%q) = %q; want %q", version, got, want)
		}
	}
}

func TestDispatch_InstallScriptPrintsTheScriptInstallingThisImage(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	status := dispatch(t.Context(), []string{"install-script"}, noEnvironment, nil, &stdout, &stderr)
	if status != exitSuccess || !strings.HasPrefix(stdout.String(), "#!/bin/sh") ||
		!strings.Contains(stdout.String(), `image="`+installImage(version)+`"`) {
		t.Errorf("install-script: status %d, stdout begins %.80q; want the install script naming this image",
			status, stdout.String())
	}
}

// runtimeOf is the runtime write-deployment finds on a machine with no GPU, running image.
func runtimeOf(image string) deployment.Runtime {
	return deployment.Runtime{Image: image, UID: os.Getuid(), GID: os.Getgid(), RenderGID: -1}
}
