// SPDX-License-Identifier: GPL-3.0-or-later

package artwork

import (
	"encoding/json"
	"fmt"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/jobs/jobstest"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// insert adds rows to the bench item's artwork, as stored or refused images.
func (bench *bench) insert(t *testing.T, rows ...sqlcgen.InsertArtworkParams) {
	t.Helper()
	for _, row := range rows {
		row.ItemID = bench.itemID
		err := bench.database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
			_, err := queries.InsertArtwork(t.Context(), row)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

// liveImages are the bench item's live artwork, as "kind source", sorted.
func (bench *bench) liveImages(t *testing.T) []string {
	t.Helper()
	var live []string
	for _, row := range bench.rows(t) {
		live = append(live, row.Kind+" "+row.Source)
	}
	slices.Sort(live)
	return live
}

// queuedRefs are the refs each queued make_artwork_sizes job names, by its source.
func queuedRefs(t *testing.T, queue *jobstest.Queue) map[string][]Ref {
	t.Helper()
	refs := map[string][]Ref{}
	for _, job := range *queue {
		var args artworkJobArgs
		if err := json.Unmarshal([]byte(job.ArgsJSON), &args); err != nil {
			t.Fatal(err)
		}
		refs[args.ArtworkSource] = append(refs[args.ArtworkSource], args.Refs...)
	}
	return refs
}

func TestTake_QueuesOnlyTheImagesTheItemDoesNotHold(t *testing.T) {
	t.Parallel()
	bench := newBench(t, fakeFetcher{})
	queue := &jobstest.Queue{}
	bench.service.parts.Queue = queue
	poster := Ref{Kind: library.ArtPoster, SourceRef: "/heat-poster.jpg"}
	backdrop := Ref{Kind: library.ArtBackdrop, SourceRef: "/heat-back.jpg"}
	named := Found{Named: []ItemImages{{ItemID: bench.itemID, Source: "tmdb", Refs: []Ref{poster, backdrop}}}}
	bench.service.Take(t.Context(), named)
	if got := queuedRefs(t, queue)["tmdb"]; !slices.Equal(got, []Ref{poster, backdrop}) {
		t.Fatalf("queued %+v; want the film's poster and backdrop", got)
	}
	// The poster is stored; the backdrop was refused by the node, and is held with no hash.
	bench.insert(t,
		sqlcgen.InsertArtworkParams{
			Kind: library.ArtPoster, Source: "tmdb", SourceRef: &poster.SourceRef, SourceSha256: []byte{1},
		},
		sqlcgen.InsertArtworkParams{Kind: library.ArtBackdrop, Source: "tmdb", SourceRef: &backdrop.SourceRef},
	)
	*queue = nil
	bench.service.Take(t.Context(), named)
	if len(*queue) != 0 {
		t.Errorf("queued %+v once both are held; want nothing", *queue)
	}
	// The provider changed its poster: the new one is owed.
	changed := Ref{Kind: library.ArtPoster, SourceRef: "/heat-poster-2.jpg"}
	bench.service.Take(t.Context(),
		Found{Named: []ItemImages{{ItemID: bench.itemID, Source: "tmdb", Refs: []Ref{changed, backdrop}}}})
	if got := queuedRefs(t, queue)["tmdb"]; !slices.Equal(got, []Ref{changed}) {
		t.Errorf("queued %+v after the provider changed its poster; want only the new poster", got)
	}
}

func TestTake_ARematchRetiresTheEarlierWorksImagesTheNewOneLacks(t *testing.T) {
	t.Parallel()
	bench := newBench(t, fakeFetcher{})
	// Metropolis's poster and backdrop are stored, and a person chose a backdrop of their own.
	bench.insert(t,
		sqlcgen.InsertArtworkParams{
			Kind: library.ArtPoster, Source: "tmdb", SourceRef: new("/heat-poster.jpg"), SourceSha256: []byte{1},
		},
		sqlcgen.InsertArtworkParams{
			Kind: library.ArtBackdrop, Source: "tmdb", SourceRef: new("/heat-back.jpg"), SourceSha256: []byte{2},
		},
		sqlcgen.InsertArtworkParams{Kind: library.ArtBackdrop, Source: "user", SourceSha256: []byte{3}},
	)
	storetest.ExecRaw(t, bench.database.Path(), "UPDATE artwork SET is_chosen = 1 WHERE source = 'user'")
	leon := ItemImages{
		ItemID: bench.itemID, Source: "tmdb", Refs: []Ref{{Kind: library.ArtPoster, SourceRef: "/leon-poster.jpg"}},
	}
	bench.service.Take(t.Context(), Found{Rematched: []ItemImages{leon}, Named: []ItemImages{leon}})
	if live, want := bench.liveImages(t), []string{"backdrop user"}; !slices.Equal(live, want) {
		t.Errorf("after the rematch: %v live, want %v: none of Metropolis's images, the person's kept (D223 §9)", live, want)
	}
}

func TestTake_AnImageNoLongerBesideTheMediaIsRetired(t *testing.T) {
	t.Parallel()
	bench := newBench(t, fakeFetcher{})
	queue := &jobstest.Queue{}
	bench.service.parts.Queue = queue
	_, libraryID := librarytest.NodeWithLibrary(t, bench.database,
		librarytest.Library{Name: "L", Kind: "movies", MetadataSource: "nfo", Root: "/m"})
	storetest.ExecRaw(t, bench.database.Path(),
		fmt.Sprintf("UPDATE item SET library_id = %d WHERE id = %d", libraryID, bench.itemID))
	poster := LocalImage{ItemID: bench.itemID, Kind: library.ArtPoster, FileID: 40, SizeBytes: 100, ModifiedAtMs: 7}
	found := Found{BesideTheMedia: &LocalImages{LibraryID: libraryID, Images: []LocalImage{poster}}}
	bench.service.Take(t.Context(), found)
	want := []Ref{{Kind: library.ArtPoster, SourceRef: "file:40:100:7"}}
	if got := queuedRefs(t, queue)[SourceLocal]; !slices.Equal(got, want) {
		t.Fatalf("queued %+v; want the poster beside the film, named by its file, size and time", got)
	}
	// The poster was stored, as the make_artwork_sizes job would have, and a person chose a backdrop.
	bench.insert(t,
		sqlcgen.InsertArtworkParams{
			Kind: library.ArtPoster, Source: SourceLocal, SourceRef: &want[0].SourceRef, SourceSha256: []byte{1},
		},
		sqlcgen.InsertArtworkParams{
			Kind: library.ArtBackdrop, Source: SourceLocal, SourceRef: new("file:41:1:1"), SourceSha256: []byte{2},
		},
	)
	storetest.ExecRaw(t, bench.database.Path(), "UPDATE artwork SET is_chosen = 1 WHERE kind = 'backdrop'")
	bench.service.Take(t.Context(), found)
	if live, want := bench.liveImages(t), []string{"backdrop local", "poster local"}; !slices.Equal(live, want) {
		t.Errorf("with the poster still beside the film: %v live, want %v", live, want)
	}
	bench.service.Take(t.Context(), Found{BesideTheMedia: &LocalImages{LibraryID: libraryID}}) // poster.jpg deleted
	if live, want := bench.liveImages(t), []string{"backdrop local"}; !slices.Equal(live, want) {
		t.Errorf("with poster.jpg gone: %v live, want %v: the person's choice, the poster retired", live, want)
	}
}
