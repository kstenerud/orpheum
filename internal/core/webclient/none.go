// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !webclient

package webclient

import "io/fs"

// Files reports that this binary was built without the web client.
func Files() (fs.FS, bool) {
	return nil, false
}
