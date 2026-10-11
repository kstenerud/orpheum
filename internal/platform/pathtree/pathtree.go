// SPDX-License-Identifier: AGPL-3.0-or-later

package pathtree

import "strings"

// IsAtOrUnder is whether the absolute path is directory, or under it at a separator.
func IsAtOrUnder(path, directory string) bool {
	directory = strings.TrimSuffix(directory, "/")
	return path == directory || directory == "" || strings.HasPrefix(path, directory+"/")
}
