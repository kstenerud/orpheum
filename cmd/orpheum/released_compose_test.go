// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// releasedComposeDir holds each version of the compose files households are given, as it was frozen,
// under compose-v<n> (D462 §6): a household keeps the file it pasted, and every later release must
// start from it (D455). A version is added by make freeze-compose and never changed
// (scripts/check_released_compose.py).
const releasedComposeDir = "testdata/released"

// composeEnvironments are what a household's .env may say, each the variables a compose file reads
// filled in: the least it can start with, and every one set. The Synology file reads none it needs.
var composeEnvironments = []map[string]string{
	{"PATH_TO_APPDATA": "/srv/appdata", "ORPHEUM_MEDIA": "/srv/films"},
	{
		"PATH_TO_APPDATA": "/srv/appdata", "ORPHEUM_MEDIA": "/srv/films", "ORPHEUM_MEDIA_2": "/srv/series",
		"ORPHEUM_MEDIA_3": "/srv/kids", "ORPHEUM_MEDIA_4": "/srv/concerts", "ORPHEUM_TAG": "v0.4.0",
		"APPUSER_PUID": "1001", "APPUSER_PGID": "100", "TIME_ZONE_VALUE": "Europe/Berlin",
		"ORPHEUM_HTTP_PORT": "9440", "ORPHEUM_THIS_MACHINE_PORT": "9441", "ORPHEUM_NAME": "nas",
	},
}

// releasedVersions are the compose-file versions frozen in releasedComposeDir, oldest first.
func releasedVersions(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(releasedComposeDir)
	if err != nil {
		t.Fatal(err)
	}
	var versions []string
	for _, entry := range entries {
		if entry.IsDir() {
			versions = append(versions, entry.Name())
		}
	}
	if len(versions) == 0 {
		t.Fatalf("%s holds no release", releasedComposeDir)
	}
	slices.SortFunc(versions, compareVersions)
	return versions
}

// compareVersions orders two frozen folders, compose-v2 before compose-v10.
func compareVersions(first, second string) int {
	number := func(folder string) int {
		version, err := strconv.Atoi(strings.TrimPrefix(folder, "compose-v"))
		if err != nil {
			return -1 // not a version's folder: before every version
		}
		return version
	}
	return number(first) - number(second)
}

// releasedComposeFiles are the compose files frozen for version, by their paths.
func releasedComposeFiles(t *testing.T, version string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(releasedComposeDir, version, "*.yaml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("%s/%s holds no compose file (%v)", releasedComposeDir, version, err)
	}
	return files
}

// composeAnchor is a value under the Synology file's "Yours" named for its services to use: an anchor.
var composeAnchor = regexp.MustCompile(`(?m)^\s+[a-z0-9-]+: &([a-z0-9]+) (.*)$`)

// composeSubstitution is one of compose's ${NAME}, ${NAME:-default} or ${NAME:?message} with no other
// inside it: an inner one is filled in first.
var composeSubstitution = regexp.MustCompile(`\$\{([A-Z_][A-Z0-9_]*)(?:(:-|:\?)([^${}]*))?\}`)

// composeResolved is a compose file's text as compose reads it with environment as its .env: its
// variables filled in and its anchors' values where they are used. It fails a variable that is needed
// and not set, as compose refuses to start one.
func composeResolved(t *testing.T, name string, environment map[string]string) string {
	t.Helper()
	raw, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	var kept []string
	for line := range strings.SplitSeq(string(raw), "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "#") {
			kept = append(kept, line)
		}
	}
	text := strings.Join(kept, "\n")
	for _, match := range composeAnchor.FindAllStringSubmatch(text, -1) {
		value := strings.Trim(match[2], `"`)
		text = regexp.MustCompile(`\*`+match[1]+`\b`).ReplaceAllLiteralString(text, value)
	}
	for composeSubstitution.MatchString(text) {
		text = composeSubstitution.ReplaceAllStringFunc(text, func(found string) string {
			parts := composeSubstitution.FindStringSubmatch(found)
			if value := environment[parts[1]]; value != "" {
				return value
			}
			if parts[2] == ":?" {
				t.Fatalf("%s needs %s, which the test's .env does not set", name, parts[1])
			}
			return parts[3]
		})
	}
	return strings.ReplaceAll(text, "$$", "$")
}

// releasedService is what a test reads of one service of a compose file: its command, the mounts it
// gives, each by its source and target, and its health check's test.
type releasedService struct {
	command     []string
	mounts      []releasedMount
	healthCheck string
}

// releasedMount is one of a service's volumes.
type releasedMount struct {
	source, target string
	isReadOnly     bool
}

// releasedServices reads a resolved compose file's services, which keep compose's two-space
// indentation: a service at two spaces, its keys at four, a list's items at six and a long mount's
// keys at eight.
func releasedServices(text string) map[string]*releasedService {
	services := map[string]*releasedService{}
	var current *releasedService
	inServices, key := false, ""
	for line := range strings.SplitSeq(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		switch indent := len(line) - len(strings.TrimLeft(line, " ")); {
		case indent == 0:
			inServices, current = trimmed == "services:", nil
		case !inServices:
		case indent == 2:
			current = &releasedService{}
			services[strings.TrimSuffix(trimmed, ":")] = current
		case current == nil:
		case indent == 4:
			key, _, _ = strings.Cut(trimmed, ":")
			current.readKey(key, strings.TrimSpace(strings.TrimPrefix(trimmed, key+":")))
		default:
			current.readItem(key, trimmed)
		}
	}
	return services
}

// readKey reads a service's key and what follows it on its line.
func (service *releasedService) readKey(key, value string) {
	if key == "command" && strings.HasPrefix(value, "[") {
		for item := range strings.SplitSeq(strings.Trim(value, "[]"), ",") {
			service.command = append(service.command, strings.Trim(strings.TrimSpace(item), `"`))
		}
	}
}

// readItem reads a line under one of a service's keys.
func (service *releasedService) readItem(key, trimmed string) {
	item, isItem := strings.CutPrefix(trimmed, "- ")
	if trimmed == "-" {
		// A variable not set fills an item in empty: an empty argument.
		item, isItem = "", true
	}
	item = strings.Trim(item, `"`)
	switch {
	case key == "command" && isItem:
		service.command = append(service.command, item)
	case key == "volumes":
		service.readVolume(trimmed, item, isItem)
	case key == "healthcheck" && strings.HasPrefix(trimmed, "test:"):
		service.healthCheck = trimmed
	}
}

// readVolume reads a line under a service's volumes: a short mount, source:target[:ro], the first
// line of a long one, or one of a long one's keys.
func (service *releasedService) readVolume(trimmed, item string, isItem bool) {
	switch {
	case isItem && strings.HasPrefix(item, "type:"):
		service.mounts = append(service.mounts, releasedMount{})
	case isItem:
		parts := strings.Split(item, ":")
		service.mounts = append(service.mounts, releasedMount{
			source: parts[0], target: parts[1], isReadOnly: len(parts) > 2 && parts[2] == "ro",
		})
	case len(service.mounts) > 0:
		long := &service.mounts[len(service.mounts)-1]
		name, value, _ := strings.Cut(trimmed, ":")
		value = strings.Trim(strings.TrimSpace(value), `"`)
		switch name {
		case "source":
			long.source = value
		case "target":
			long.target = value
		case "read_only":
			long.isReadOnly = value == "true"
		}
	}
}

// mountAt is the service's mount at target, and whether there is one.
func (service *releasedService) mountAt(target string) (releasedMount, bool) {
	index := slices.IndexFunc(service.mounts, func(mount releasedMount) bool { return mount.target == target })
	if index < 0 {
		return releasedMount{}, false
	}
	return service.mounts[index], true
}

// Every compose file a release sent households to paste still starts this server, its variables as
// any .env sets them: start takes its core's command line and writes the deployment from it, and the
// core and the node find every folder and socket they use where the file mounts one, so a release
// that would stop a household's server fails here, not on its machine (D455, D461 §1).
func TestReleasedCompose_EveryReleasedFileStillStartsThisServer(t *testing.T) {
	t.Parallel()
	for _, version := range releasedVersions(t) {
		for _, name := range releasedComposeFiles(t, version) {
			for index, environment := range composeEnvironments {
				t.Run(version+"/"+filepath.Base(name)+"/env"+strconv.Itoa(index), func(t *testing.T) {
					t.Parallel()
					checkReleasedCompose(t, releasedServices(composeResolved(t, name, environment)))
				})
			}
		}
	}
}

// checkReleasedCompose checks that this server starts from a resolved compose file's services.
func checkReleasedCompose(t *testing.T, services map[string]*releasedService) {
	t.Helper()
	folders, core, node := services["orpheum-folders"], services["orpheum-core"], services["orpheum-node"]
	if folders == nil || core == nil || node == nil {
		t.Fatalf("services %v; want orpheum-folders, orpheum-core and orpheum-node", services)
	}
	if len(core.command) == 0 || core.command[0] != "start" {
		t.Fatalf("the core's command %q; want start's", core.command)
	}
	options := checkReleasedStart(t, core)
	checkReleasedMounts(t, folders, core, node, options)
	nodeSocket := path.Join(deploymentRun, string(kindNode), nodeSocketName)
	if !strings.Contains(node.healthCheck, nodeSocket) {
		t.Errorf("the node's health check %q; want it to look for its socket, %s", node.healthCheck, nodeSocket)
	}
	thisMachine := "/dev/tcp/127.0.0.1/" + strconv.Itoa(options.thisMachinePort)
	healthCheck := core.healthCheck
	if strings.Contains(healthCheck, "/$0 ") {
		// The port is passed as the script's $0, its last argument, so no variable is inside its text.
		items := strings.Split(strings.TrimSuffix(strings.TrimSpace(healthCheck), "]"), ", ")
		healthCheck = strings.Replace(healthCheck, "/$0 ", "/"+strings.Trim(items[len(items)-1], `"`)+" ", 1)
	}
	if !strings.Contains(healthCheck, thisMachine) || !strings.Contains(healthCheck, "/api/v1/health") {
		t.Errorf("the core's health check %q; want /api/v1/health asked at %s, the port start takes",
			core.healthCheck, thisMachine)
	}
}

// checkReleasedStart checks that start takes the core's command line and writes a deployment from it,
// and is the options it gives.
func checkReleasedStart(t *testing.T, core *releasedService) startOptions {
	t.Helper()
	var stderr bytes.Buffer
	options, isParsed := parseStart(core.command[1:], &stderr)
	if !isParsed {
		t.Fatalf("start refuses the core's command line %q: %s", core.command[1:], stderr.String())
	}
	dir := t.TempDir()
	if err := startIn(dir, filepath.Join(dir, "run"), "nas", options); err != nil {
		t.Fatalf("start cannot write the deployment from %q: %v", core.command[1:], err)
	}
	return options
}

// checkReleasedMounts checks that the core and the node find each folder and socket they use where
// the compose file mounts one, and that orpheum-folders makes the node's and the run directory.
func checkReleasedMounts(t *testing.T, folders, core, node *releasedService, options startOptions) {
	t.Helper()
	data, isData := core.mountAt(deploymentMount)
	run, isRun := core.mountAt(deploymentRun)
	if !isData || !isRun {
		t.Fatalf("the core's mounts %+v; want the deployment at %s and its run directory at %s",
			core.mounts, deploymentMount, deploymentRun)
	}
	instance := path.Join(deploymentMount, string(kindNode))
	if want := []string{"node", "--instance", instance}; !slices.Equal(node.command, want) {
		t.Errorf("the node's command %q; want %q, the instance start makes", node.command, want)
	}
	if mount, isThere := node.mountAt(instance); !isThere || mount.source != data.source+"/"+string(kindNode) {
		t.Errorf("the node's mount at %s %+v; want the core's %s/%s", instance, mount, data.source, kindNode)
	}
	if mount, isThere := node.mountAt(deploymentRun); !isThere || mount.source != run.source {
		t.Errorf("the node's mount at %s %+v; want the core's, %s, where each finds the other's socket",
			deploymentRun, mount, run.source)
	}
	for _, root := range options.media {
		if mount, isThere := node.mountAt(root); !isThere || !mount.isReadOnly {
			t.Errorf("the media folder %s, which start is given, is mounted in the node as %+v; want it there, "+
				"read-only", root, mount)
		}
	}
	if mount, isThere := folders.mountAt("/appdata"); !isThere || mount.source != data.source ||
		!strings.Contains(strings.Join(folders.command, " "), "mkdir -p /appdata/node /appdata/run") {
		t.Errorf("orpheum-folders mounts %+v and runs %q; want the core's data folder, its node/ and run/ made",
			folders.mounts, folders.command)
	}
}

// A frozen file passes start only flags this release takes, so start skips none of them: an older
// release skips a newer file's flag it does not know (D462 §10.10), but this one must know them all.
func TestReleasedCompose_StartTakesEveryFlagAFrozenFilePasses(t *testing.T) {
	t.Parallel()
	for _, version := range releasedVersions(t) {
		for _, name := range releasedComposeFiles(t, version) {
			core := releasedServices(composeResolved(t, name, composeEnvironments[1]))["orpheum-core"]
			if core == nil || len(core.command) == 0 {
				t.Fatalf("%s has no orpheum-core with a command", name)
			}
			var stderr bytes.Buffer
			if _, isParsed := parseStart(core.command[1:], &stderr); !isParsed || stderr.Len() != 0 {
				t.Errorf("start says, of %s's command line %q: %s; want every flag taken", name, core.command[1:],
					stderr.String())
			}
		}
	}
}
