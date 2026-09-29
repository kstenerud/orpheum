// SPDX-License-Identifier: GPL-3.0-or-later

package library

import (
	"context"
	"errors"
	"fmt"

	"github.com/kstenerud/orpheum/internal/core/textindex"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// ErrExists is returned when a live library already has the name.
var ErrExists = errors.New("a library with that name already exists")

// NewLibrary is a library to create: its name and kind, the three questions asked at creation
// and never defaulted (D184), and the roots of one node it covers, as that node reports them.
type NewLibrary struct {
	Name            string
	Kind            string // movies, shows or mixed
	MetadataSource  string // nfo_then_tmdb, tmdb or nfo
	ChangeDetection string // watch, interval or manual
	CheckEveryH     *int64 // interval only
	Writable        bool
	NodeID          int64
	Roots           []string
}

// Create adds a library and its roots. The schema's checks refuse a kind or an answer outside
// the closed sets.
func Create(ctx context.Context, database *store.Database, clockSource clock.Clock, library NewLibrary) (int64, error) {
	library.Name = textindex.Line(library.Name) // a person's text is shown to others (D211 §2)
	var id int64
	err := database.Write(ctx, func(queries *sqlcgen.Queries) error {
		_, err := store.One(queries.GetLiveLibraryByName(ctx, library.Name))
		if err == nil {
			return fmt.Errorf("%w: %q", ErrExists, library.Name)
		}
		if !errors.Is(err, store.ErrNotFound) {
			return err
		}
		writable := int64(0)
		if library.Writable {
			writable = 1
		}
		id, err = queries.InsertLibrary(ctx, sqlcgen.InsertLibraryParams{
			Name: library.Name, Kind: library.Kind, MetadataSource: library.MetadataSource,
			ChangeDetection: library.ChangeDetection, CheckEveryHours: library.CheckEveryH, IsWritable: writable,
			CreatedAt: clockSource.Now().UnixMilli(),
		})
		if err != nil {
			return fmt.Errorf("create library %q: %w", library.Name, err)
		}
		for _, root := range library.Roots {
			err = queries.InsertLibraryRoot(ctx, sqlcgen.InsertLibraryRootParams{
				LibraryID: id, NodeID: library.NodeID, Path: root,
			})
			if err != nil {
				return fmt.Errorf("add root %s to library %q: %w", root, library.Name, err)
			}
		}
		return nil
	})
	return id, err
}
