// SPDX-License-Identifier: GPL-3.0-or-later

package readonly

import "io"

// Close closes a handle that was only read, and lets its error go: nothing written can be lost.
// Never give it a file that was written; return that Close's error instead.
func Close(handle io.Closer) {
	_ = handle.Close() //nolint:errcheck // only read: a close error loses nothing
}
