// SPDX-License-Identifier: AGPL-3.0-or-later

package webclient

import (
	"embed"
	"io/fs"
	"regexp"
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

// scriptNamePattern finds the built client's own script in its index.html: Vite names it by a hash
// of what it holds, so a client built from other code has another name.
var scriptNamePattern = regexp.MustCompile(`<script[^>]*\ssrc="/assets/(index-[A-Za-z0-9_-]+\.js)"`)

// ScriptName is the name of the built client's own script, "index-DCLi77_b.js", which a page
// running it can compare with the server's to know it is out of date; "" when files has none.
func ScriptName(files fs.FS) string {
	index, err := fs.ReadFile(files, "index.html")
	if err != nil {
		return ""
	}
	found := scriptNamePattern.FindSubmatch(index)
	if found == nil {
		return ""
	}
	return string(found[1])
}
