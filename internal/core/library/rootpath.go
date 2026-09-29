// SPDX-License-Identifier: GPL-3.0-or-later

package library

import (
	"strings"

	"golang.org/x/text/unicode/norm"

	"github.com/kstenerud/orpheum/internal/core/textindex"
)

// PathText is a path's bytes as the library shows and relates the path: valid UTF-8, in NFC for
// D49's equality, and cleaned (principles §22). Cut at a separator, the text of each part is the
// text of the whole cut there.
func PathText(raw []byte) string {
	return textindex.Path(norm.NFC.String(strings.ToValidUTF8(string(raw), "\uFFFD")))
}
