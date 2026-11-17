// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
)

// The Update page is told how the server was installed as its config says, the install script's
// folder among it, which the page's command names.
func TestTellTheUpdatePage_HandsItTheRouteTheTagAndTheFolder(t *testing.T) {
	t.Parallel()
	var dependencies api.Dependencies
	tellTheUpdatePage(&dependencies,
		ServerConfig{InstallRoute: "script", ImageTag: "v0.8.3", InstallDirectory: "/srv/orpheum"})
	if dependencies.PublishedImage != publishedImage || dependencies.InstallRoute != "script" ||
		dependencies.ImageTag != "v0.8.3" || dependencies.InstallDirectory != "/srv/orpheum" {
		t.Errorf("the Update page was told %q, %q, %q and %q; want the image, script, v0.8.3 and /srv/orpheum",
			dependencies.PublishedImage, dependencies.InstallRoute, dependencies.ImageTag,
			dependencies.InstallDirectory)
	}
}

// A file without the install folder, as every install but the script's, runs on none and says so:
// the deployment writes the key, and the server knows it (D202).
func TestLoadConfig_AFileWithoutTheInstallFolderRunsOnNone(t *testing.T) {
	t.Parallel()
	text := withoutKey(validConfigTexts[kindCore], configKey{table: tableServer, name: "install_directory"})
	loaded, report, err := loadConfig(writeConfig(t, text), kindCore)
	if err != nil || len(report.OnStartingValue) != 1 ||
		report.OnStartingValue[0].path() != "server.install_directory" || loaded.Server.InstallDirectory != "" {
		t.Errorf("without install_directory: %+v, %q, %v; want it loaded on none and reported",
			report, loaded.Server.InstallDirectory, err)
	}
}
