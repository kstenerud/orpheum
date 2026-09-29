// SPDX-License-Identifier: GPL-3.0-or-later

package webclient

import (
	"embed"
	"io/fs"
)

// dist holds the built web client once `make build` has copied it in; in a clone it holds only
// the placeholder that keeps the directory, so every build compiles.
//
//go:embed all:dist
var dist embed.FS

// Files is the built web client, or false when this binary was built without it.
func Files() (fs.FS, bool) {
	return builtClient(dist)
}

// builtClient is the client inside embedded, or false when there is no index.html to serve.
func builtClient(embedded fs.FS) (fs.FS, bool) {
	files, err := fs.Sub(embedded, "dist")
	if err != nil {
		return nil, false
	}
	if _, err := fs.Stat(files, "index.html"); err != nil {
		return nil, false
	}
	return files, true
}
