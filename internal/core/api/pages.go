// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"encoding/binary"
	"net/http"

	"github.com/kstenerud/orpheum/internal/core/library"
)

// PageInput asks for one page of a list that can grow, as every such list asks (D416 §6): the
// previous page's nextCursor, absent for the first, and at most limit entries, as GET /items takes.
// Its cursor is the browse lists' opaque one (library.EncodeCursor).
type PageInput struct {
	Cursor string `doc:"From the previous page's nextCursor; absent for the first page" query:"cursor"`
	Limit  int64  `maximum:"500" minimum:"1" query:"limit" required:"true"`
}

// errBadCursor is the refusal of a cursor this server did not give, on every list.
func errBadCursor() error {
	return invalidRequestAt(http.StatusUnprocessableEntity, "query.cursor", "the cursor is not one this server gave")
}

// decodeCursor is a page's cursor's key and id, empty for the first page's, or the refusal of one
// this server did not give.
func decodeCursor(cursor string) ([]byte, int64, error) {
	key, id, err := library.DecodeCursor(cursor)
	if err != nil {
		return nil, 0, errBadCursor()
	}
	return key, id, nil
}

// int64Key is a number as a cursor's key: eight bytes, big-endian.
func int64Key(value int64) []byte {
	return binary.BigEndian.AppendUint64(nil, uint64(value)) //nolint:gosec // read back by keyInt64
}

// keyInt64 reads int64Key's number back; false for a key that is not one.
func keyInt64(key []byte) (int64, bool) {
	if len(key) != 8 {
		return 0, false
	}
	return int64(binary.BigEndian.Uint64(key)), true //nolint:gosec // written by int64Key
}

// nextCursorOf is a page's nextCursor: the cursor made from its last entry when the page is full,
// for there may be more, and "" (absent in the answer) when it is not (D416 §6).
func nextCursorOf(count int, limit int64, cursorOfLast func() string) string {
	if count == 0 || int64(count) < limit {
		return ""
	}
	return cursorOfLast()
}
