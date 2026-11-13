// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
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

// PageOrWholeInput asks for one page of a list a client may also read whole in one answer, as a
// list with an alphabet rail is (D475): its limit left out, the whole list, with no nextCursor; given,
// a page as PageInput asks for one.
type PageOrWholeInput struct {
	Cursor string `doc:"From the previous page's nextCursor; absent for the first page" query:"cursor"`
	Limit  int64  `doc:"At most this many, a page; absent, the whole list in one answer, with no nextCursor (D475)" maximum:"500" minimum:"1" query:"limit"` //nolint:lll // one struct tag
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

// nextCursorOf is a page's nextCursor: the cursor made from its last entry when the page is full,
// for there may be more, and "" (absent in the answer) when it is not (D416 §6).
// A list read whole (limit 0, D475) has none.
func nextCursorOf(count int, limit int64, cursorOfLast func() string) string {
	if count == 0 || limit == 0 || int64(count) < limit {
		return ""
	}
	return cursorOfLast()
}
