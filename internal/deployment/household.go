// SPDX-License-Identifier: AGPL-3.0-or-later

package deployment

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// The compose-file routes a household installs by, each with its own published file (D462 §9,
// §10.2): the file names itself to start as --compose-file, so the Update page shows its steps alone.
const (
	// ByOMV is OpenMediaVault's compose plugin, deploy/omv/compose.yaml.
	ByOMV InstallRoute = "omv"
	// BySynology is a Synology's Container Manager, deploy/synology/compose.yaml, its values in the file.
	BySynology InstallRoute = "synology"
	// ByPortainer is Portainer's app template, deploy/portainer/compose.yaml.
	ByPortainer InstallRoute = "portainer"
	// ByCasaOS is CasaOS's app store, deploy/catalogue/casaos/docker-compose.yml.
	ByCasaOS InstallRoute = "casaos"
	// ByDocker is plain `docker compose` beside a .env file, deploy/docker/compose.yaml; it is also
	// the route of a file naming no route, or one this release does not know (D462 §9, §10.10).
	ByDocker InstallRoute = "docker"
)

// ComposeRoutes are the compose-file routes, each with its published file, in the order the Update
// page lists them.
func ComposeRoutes() []InstallRoute {
	return []InstallRoute{ByOMV, BySynology, ByPortainer, ByCasaOS, ByDocker}
}

// IsCompose is whether the route is a compose file's, whose every start is `orpheum start`.
func (route InstallRoute) IsCompose() bool { return slices.Contains(ComposeRoutes(), route) }

// ComposeRoute is the route a compose file names with --compose-file: itself when known, else
// ByDocker, so a newer file's route an older release does not know still starts (D462 §10.10).
func ComposeRoute(named string) InstallRoute {
	if route := InstallRoute(strings.TrimSpace(named)); route.IsCompose() {
		return route
	}
	return ByDocker
}

// PublishedPath is where in the repository the route's compose file is published, which `make
// generate` writes and a release attaches (D462 §5, §10.2); empty for a route with none.
func (route InstallRoute) PublishedPath() string {
	switch route {
	case ByCasaOS:
		return "deploy/catalogue/casaos/docker-compose.yml"
	case ByOMV, BySynology, ByPortainer, ByDocker:
		return "deploy/" + string(route) + "/compose.yaml"
	case ByInstallScript:
	}
	return ""
}

// Unset is the path a Synology's media folder not set names, which start skips: Docker refuses a
// bind with no target, and a relative one, so a folder not set is made empty at /unset/<n> (D462
// §10.5).
const Unset = "/unset/"

// Yours are the values a Synology's compose file holds under "Yours", which the Update page fills
// into a newer file (D462 §4, §10.5): every other route's values are in its variables, kept where
// the household set them.
type Yours struct {
	// Media are up to four media folders, the first needed; one not set is Unset with its number.
	Media []string
	// User is the uid:gid the server runs as.
	User string
	// TimeZone is the household's time zone, Europe/Berlin.
	TimeZone string
	// HTTPPort and ThisMachinePort are the ports a browser at home, and on this machine alone, opens
	// the server on.
	HTTPPort, ThisMachinePort int
}

// DefaultYours are the published Synology file's values.
func DefaultYours() Yours {
	defaults := StartingValues(Release)
	return Yours{
		Media: []string{"/volume1/video"}, User: "1026:100", TimeZone: "Europe/Berlin",
		HTTPPort: defaults.HTTP.Port, ThisMachinePort: defaults.HTTP.ThisMachinePort,
	}
}

// mediaFolders is the most media folders a compose file names.
const mediaFolders = 4

// householdFile is what templates/household.yaml is rendered from: the route, and for a Synology
// the household's values.
type householdFile struct {
	Route   InstallRoute
	Version int
	Yours   yoursShown
}

// yoursShown are Yours as the Synology file's lines under "Yours" say them.
type yoursShown struct {
	Media1, Media2, Media3, Media4 string
	User, TimeZone                 string
	HTTPPort, ThisMachinePort      int
}

// More are the numbers of the optional media folders.
func (householdFile) More() []int { return []int{2, 3, 4} }

func (file householdFile) isSynology() bool { return file.Route == BySynology }

func (file householdFile) isCasaOS() bool { return file.Route == ByCasaOS }

// DataFolderNeeded is the data folder where the compose file first names it, refusing to start
// without it; DataFolder is it everywhere after.
func (file householdFile) DataFolderNeeded() string {
	if file.isSynology() || file.isCasaOS() {
		return file.DataFolder()
	}
	return "${PATH_TO_APPDATA:?set PATH_TO_APPDATA to the folder that holds each app's data}/orpheum"
}

// DataFolder is where the deployment's directory is on the machine.
func (file householdFile) DataFolder() string {
	switch {
	case file.isSynology():
		return "./data"
	case file.isCasaOS():
		return "/DATA/AppData/orpheum"
	}
	return "${PATH_TO_APPDATA:?}/orpheum"
}

// User is who the services run as.
func (file householdFile) User() string {
	switch {
	case file.isSynology():
		return "*user"
	case file.isCasaOS():
		return `"${PUID:-1000}:${PGID:-100}"`
	}
	return `"${APPUSER_PUID:-1000}:${APPUSER_PGID:-100}"`
}

// Owner is who orpheum-folders gives the data's folders to, inside its command's string.
func (file householdFile) Owner() string {
	if file.isCasaOS() {
		return "${PUID:-1000}:${PGID:-100}"
	}
	return "${APPUSER_PUID:-1000}:${APPUSER_PGID:-100}"
}

// TimeZone is the services' TZ.
func (file householdFile) TimeZone() string {
	switch {
	case file.isSynology():
		return "*timezone"
	case file.isCasaOS():
		return "${TZ:-Etc/UTC}"
	}
	return "${TIME_ZONE_VALUE:-Etc/UTC}"
}

// HTTPPort and ThisMachinePort are the ports start is told; the health check asks the second.
func (file householdFile) HTTPPort() string {
	if file.isSynology() {
		return "*httpport"
	}
	return "${ORPHEUM_HTTP_PORT:-" + strconv.Itoa(StartingValues(Release).HTTP.Port) + "}"
}

// ThisMachinePort is the port on this machine's loopback alone.
func (file householdFile) ThisMachinePort() string {
	if file.isSynology() {
		return "*thismachineport"
	}
	return "${ORPHEUM_THIS_MACHINE_PORT:-" + strconv.Itoa(StartingValues(Release).HTTP.ThisMachinePort) + "}"
}

// ThisMachinePortQuoted is ThisMachinePort where YAML's flow sequence needs it quoted: a variable's
// braces are YAML's own.
func (file householdFile) ThisMachinePortQuoted() string {
	if file.isSynology() {
		return file.ThisMachinePort()
	}
	return `"` + file.ThisMachinePort() + `"`
}

// Media1Needed is the first media folder where the node first names it, refusing to start without
// it; Media1 is it everywhere else.
func (file householdFile) Media1Needed() string {
	if file.isSynology() || file.isCasaOS() {
		return file.Media1()
	}
	return "${ORPHEUM_MEDIA:?set ORPHEUM_MEDIA to the folder that holds your films and series}"
}

// Media1 is the first media folder.
func (file householdFile) Media1() string {
	switch {
	case file.isSynology():
		return "*media"
	case file.isCasaOS():
		return "${ORPHEUM_MEDIA:-/DATA/Media}"
	}
	return "${ORPHEUM_MEDIA}"
}

// Media is the optional media folder number, as start's --media is given it: empty when not set,
// or a Synology's Unset path.
func (file householdFile) Media(number int) string {
	if file.isSynology() {
		return "*media" + strconv.Itoa(number)
	}
	return "${ORPHEUM_MEDIA_" + strconv.Itoa(number) + ":-}"
}

// HouseholdComposeFile is the compose file the route publishes, as `make generate` writes it at
// its PublishedPath (D462 §10.2).
func HouseholdComposeFile(route InstallRoute) ([]byte, error) {
	return FilledComposeFile(route, DefaultYours())
}

// FilledComposeFile is the route's compose file with a Synology's values under "Yours" (ignored
// for any other route, whose values are its variables): the file the Update page hands a household
// to paste over its own (D462 §4).
func FilledComposeFile(route InstallRoute, yours Yours) ([]byte, error) {
	if !route.IsCompose() {
		return nil, fmt.Errorf("%q has no compose file; want one of %v", route, ComposeRoutes())
	}
	shown := yoursShown{
		User: yours.User, TimeZone: yours.TimeZone, HTTPPort: yours.HTTPPort, ThisMachinePort: yours.ThisMachinePort,
	}
	media := []*string{&shown.Media1, &shown.Media2, &shown.Media3, &shown.Media4}
	for index, field := range media {
		*field = Unset + strconv.Itoa(index+1)
		if index < len(yours.Media) && strings.TrimSpace(yours.Media[index]) != "" {
			*field = yours.Media[index]
		}
	}
	if len(yours.Media) > mediaFolders {
		return nil, fmt.Errorf("%d media folders; a compose file names %d at most", len(yours.Media), mediaFolders)
	}
	text, err := render("household.yaml", householdFile{Route: route, Version: ComposeFileVersion, Yours: shown})
	if err != nil {
		return nil, err
	}
	return []byte(text), nil
}

// WriteHouseholdComposeFiles writes every route's compose file at its PublishedPath under root, the
// repository, leaving a file that is already right untouched: `orpheum compose-files`, which `make
// generate` runs (D462 §10.2).
func WriteHouseholdComposeFiles(root string) error {
	for _, route := range ComposeRoutes() {
		text, err := HouseholdComposeFile(route)
		if err != nil {
			return err
		}
		path := filepath.Join(root, route.PublishedPath())
		//nolint:gosec // G304: the repository's own file
		if old, readErr := os.ReadFile(path); readErr == nil && bytes.Equal(old, text) {
			continue
		}
		if err = os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return fmt.Errorf("make %s's folder: %w", path, err)
		}
		if err := writeFile(path, text, publicFileMode); err != nil {
			return err
		}
	}
	return nil
}
