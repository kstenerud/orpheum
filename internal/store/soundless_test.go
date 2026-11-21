// SPDX-License-Identifier: AGPL-3.0-or-later

package store_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/store/rowstest"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// TestFilmsWithoutSound_ArePictureFilesWithNoAudioAndACoverIsNoPicture checks Needs attention's
// films without sound, listed and counted: a film whose file holds a picture and no audio track is
// one, whether its picture is its only video stream, follows a cover or precedes one; a film whose
// file's only video stream is its cover holds no picture, and one with sound is none.
func TestFilmsWithoutSound_ArePictureFilesWithNoAudioAndACoverIsNoPicture(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	// 1 has a picture alone; 2 a cover alone; 3 a cover at stream 0 and its picture at 1; 4 its
	// picture at 0 and a cover at 1; 5 a picture and sound.
	storetest.ExecRaw(t, database.Path(),
		rowstest.Node(rowstest.NodeSpec{ID: 1, Name: "local"}),
		rowstest.Library(rowstest.LibrarySpec{ID: 1, NodeID: 1}),
		soundlessFilm(1, "video"),
		soundlessFilm(2, "cover"),
		soundlessFilm(3, "cover", "video"),
		soundlessFilm(4, "video", "cover"),
		soundlessFilm(5, "video", "audio"))
	rows, err := database.Read().ListFilmsWithoutSound(t.Context(), 10)
	if err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for _, row := range rows {
		ids = append(ids, row.WorkID)
	}
	if !slices.Equal(ids, []int64{1, 3, 4}) {
		t.Errorf("films without sound %v; want 1, 3 and 4, whose pictures have no audio beside them, and not 2, "+
			"whose only video stream is a cover, nor 5, which has sound", ids)
	}
	count, err := database.Read().CountFilmsWithoutSound(t.Context())
	if err != nil || count != 3 {
		t.Errorf("films without sound counted %d (%v); want 3, the films listed", count, err)
	}
}

// soundlessFilm is film id in library 1, with one version and its one file, /m/<id>.mkv, of the
// streams named in order: a picture (video), FFmpeg's attached picture (cover) or a sound track
// (audio). The file's payload hash is its id's one byte.
func soundlessFilm(hashByte byte, streams ...string) string {
	id := int64(hashByte)
	built := []string{
		rowstest.Title(rowstest.ItemSpec{ID: id, LibraryID: 1}),
		rowstest.Version(rowstest.VersionSpec{ID: id, TitleID: id}),
		rowstest.File(rowstest.FileSpec{
			ID: id, SizeBytes: 10, PayloadHash: []byte{hashByte}, NodeID: 1, Path: fmt.Sprintf("/m/%d.mkv", id),
			LibraryKind: "movie", VersionID: id,
		}),
		rowstest.Probe(rowstest.ProbeSpec{FileID: id}),
	}
	for index, stream := range streams {
		spec := rowstest.StreamSpec{BytesFile: id, StreamIndex: int64(index), Kind: stream, Codec: "h264"}
		switch stream {
		case "cover":
			spec.Kind, spec.Codec, spec.DispositionFlags = "video", "mjpeg", attachedPic
		case "audio":
			spec.Codec = "aac"
		}
		built = append(built, rowstest.Stream(spec))
	}
	return rowstest.Join(built...)
}
