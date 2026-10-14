// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"context"
	"errors"
)

// What makes a database this project's. open.go, the opening itself, is shared with
// orpheum-connect as a twin (look-alike-watch-list.md); this file is each project's own.

// ApplicationID marks a SQLite file as ours: 'VPMS' (schema.md, Bookkeeping). A file without it
// is refused, never migrated.
const ApplicationID = 1447646258

// ErrNotOurs is returned for a database file that is not empty and lacks our application_id.
var ErrNotOurs = errors.New("not an Orpheum database: the file lacks Orpheum's application_id")

// Open opens the database at path, creating an empty file if there is none, and refuses to
// return it unless every pragma reads back as set and the file is ours or empty. An empty file
// becomes ours when Migrate runs its first step.
func Open(ctx context.Context, path string, options OpenOptions) (*Database, error) {
	return open(ctx, path, options, ApplicationID)
}
