// SPDX-License-Identifier: GPL-3.0-or-later

//go:build webclient

package webclient

import (
	"embed"
	"io/fs"
)

// dist is the built web client, copied here by `make build` before it builds with -tags webclient.
//
//go:embed all:dist
var dist embed.FS

// Files is the built web client.
func Files() (fs.FS, bool) {
	files, err := fs.Sub(dist, "dist")
	return files, err == nil
}
