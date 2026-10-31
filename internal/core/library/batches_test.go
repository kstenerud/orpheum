// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// A batch is one folder's, of at most so many videos, each with the files named for it; a file named
// for none of them counts as a video of its own (D240 §4, §8).
func TestBatchesOf_CutsEachFolderIntoBatchesOfVideosWithTheFilesNamedForThem(t *testing.T) {
	t.Parallel()
	file := func(kind nodeproto.ExamineKind, path string) nodeproto.ExamineFile {
		return nodeproto.ExamineFile{Path: nodeproto.Path{Text: path}, Kind: kind}
	}
	files := []nodeproto.ExamineFile{
		file(nodeproto.ExamineSubtitle, "Films/Arkon.en.srt"),
		file(nodeproto.ExamineVideo, "Films/Arkon.mkv"),
		file(nodeproto.ExamineNFO, "Films/Arkon.nfo"),
		file(nodeproto.ExamineVideo, "Films/Metropolis - 720p.mkv"),
		file(nodeproto.ExamineSubtitle, "Films/Metropolis - 720p.de.srt"),
		file(nodeproto.ExamineVideo, "Films/Metropolis.mkv"),
		file(nodeproto.ExamineNFO, "Films/movie.nfo"),
		file(nodeproto.ExamineVideo, "Films/Nosferatu.mkv"),
		file(nodeproto.ExamineNFO, "Shows/Dark/tvshow.nfo"),
		file(nodeproto.ExamineVideo, "Shows/Dark/S01E01.mkv"),
	}
	var got []string
	for _, batch := range batchesOf(files, 2) {
		var paths []string
		for _, each := range batch {
			paths = append(paths, each.Path.Text)
		}
		got = append(got, strings.Join(paths, ", "))
	}
	want := []string{
		"Films/Arkon.mkv, Films/Arkon.en.srt, Films/Arkon.nfo, Films/Metropolis - 720p.mkv, Films/Metropolis - 720p.de.srt",
		"Films/Metropolis.mkv, Films/Nosferatu.mkv",
		"Films/movie.nfo",
		"Shows/Dark/S01E01.mkv, Shows/Dark/tvshow.nfo",
	}
	if !slices.Equal(got, want) {
		t.Errorf("batches:\n  %s\nwant:\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}
