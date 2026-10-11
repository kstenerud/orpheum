// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"reflect"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/artwork"
	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/library"
)

// What identify found of items' images reaches the artwork service whole: the images each item's
// provider names, the rematched items' new works, and what a pass found beside the media.
func TestFoundImages_HandsTheArtworkServiceEveryImageAPassFound(t *testing.T) {
	t.Parallel()
	poster := identify.ImageRef{Kind: library.ArtPoster, Ref: "/p.jpg"}
	report := identify.Report{
		NamedImages: []identify.NamedImages{{ItemID: 1, Source: "tmdb", Refs: []identify.ImageRef{poster}}},
		Rematched:   []identify.NamedImages{{ItemID: 2, Source: "tmdb"}},
		BesideTheMedia: &identify.LocalImages{LibraryID: 3, Images: []identify.LocalImage{
			{ItemID: 1, Kind: library.ArtBackdrop, FileID: 4, SizeBytes: 5, ModifiedAtMs: 6},
		}},
	}
	want := artwork.Found{
		Named: []artwork.ItemImages{
			{ItemID: 1, Source: "tmdb", Refs: []artwork.Ref{{Kind: library.ArtPoster, SourceRef: "/p.jpg"}}},
		},
		Rematched: []artwork.ItemImages{{ItemID: 2, Source: "tmdb", Refs: []artwork.Ref{}}},
		BesideTheMedia: &artwork.LocalImages{LibraryID: 3, Images: []artwork.LocalImage{
			{ItemID: 1, Kind: library.ArtBackdrop, FileID: 4, SizeBytes: 5, ModifiedAtMs: 6},
		}},
	}
	if got := foundImages(report); !reflect.DeepEqual(got, want) {
		t.Errorf("handed %+v; want %+v", got, want)
	}
	if got := foundImages(identify.Report{}); got.BesideTheMedia != nil {
		t.Errorf("a match's report handed %+v beside the media; want nil, so nothing beside it is retired",
			got.BesideTheMedia)
	}
}
