// SPDX-License-Identifier: AGPL-3.0-or-later

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

// ErrNameTaken is returned when a live library already has the name.
var ErrNameTaken = errors.New("a library with that name already exists")

// NewLibrary is a library to create: its name and kind, the three questions asked at creation
// and never defaulted (D184), and the roots of one node it covers, as that node reports them.
type NewLibrary struct {
	Name            string
	Kind            string // movies, shows, mixed or other
	MetadataSource  string // nfo_then_tmdb, tmdb or nfo
	ChangeDetection string // watch, interval or manual
	CheckEveryHours *int64 // interval only
	IsWritable      bool
	NodeID          int64
	Roots           []string
}

// Create adds a library and its roots. The schema's checks refuse a kind or an answer outside
// the closed sets.
func Create(
	ctx context.Context, database *store.Database, clockSource clock.Clock, newLibrary NewLibrary,
) (int64, error) {
	newLibrary.Name = textindex.Line(newLibrary.Name) // a person's text is shown to others (D211 §2)
	var id int64
	err := database.Write(ctx, func(queries *sqlcgen.Queries) error {
		_, err := store.OrNotFound(queries.GetLiveLibraryByName(ctx, newLibrary.Name))
		if err == nil {
			return fmt.Errorf("%w: %q", ErrNameTaken, newLibrary.Name)
		}
		if !errors.Is(err, store.ErrNotFound) {
			return err
		}
		writable := int64(0)
		if newLibrary.IsWritable {
			writable = 1
		}
		id, err = queries.InsertLibrary(ctx, sqlcgen.InsertLibraryParams{
			Name: newLibrary.Name, Kind: newLibrary.Kind, MetadataSource: newLibrary.MetadataSource,
			ChangeDetection: newLibrary.ChangeDetection, CheckEveryHours: newLibrary.CheckEveryHours,
			IsWritable: writable,
			CreatedAt:  clockSource.Now().UnixMilli(),
		})
		if err != nil {
			return fmt.Errorf("create library %q: %w", newLibrary.Name, err)
		}
		for _, root := range newLibrary.Roots {
			err = queries.InsertLibraryRoot(ctx, sqlcgen.InsertLibraryRootParams{
				LibraryID: id, NodeID: newLibrary.NodeID, Path: root,
			})
			if err != nil {
				return fmt.Errorf("add root %s to library %q: %w", root, newLibrary.Name, err)
			}
		}
		return nil
	})
	return id, err
}
